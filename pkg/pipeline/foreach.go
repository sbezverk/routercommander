package pipeline

import (
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/sbezverk/routercommander/pkg/types"
)

func executeForEach(r types.Router, step *types.PipelineStep, ctx *types.RunContext) error {
	if ctx.Collections == nil {
		ctx.Collections = make(map[string][]types.Record)
	}

	records, ok := ctx.Collections[step.ForEach.In]
	if !ok {
		return forEachError(ctx, step, fmt.Errorf("collection %q does not exist", step.ForEach.In))
	}

	if err := initializeForEachOutputs(ctx, step.ForEach.Outputs); err != nil {
		return forEachError(ctx, step, err)
	}

	for _, record := range records {
		childCtx := ctx.ChildForRecord(record)
		// Aggregates belong to the parent scope and must not be visible while
		// this child iteration is running.
		for _, output := range step.ForEach.Outputs {
			if output != nil {
				delete(childCtx.Collections, output.Into)
			}
		}

		if err := executePipeline(r, step.ForEach.Steps, childCtx); err != nil {
			if errors.Is(err, types.ErrPipelineNoData) {
				return err
			}
			if errors.Is(err, types.ErrPipelineSkipRecord) || step.OnError == types.OnErrorTypeContinueRecord {
				continue
			}
			return err
		}

		deltas, err := buildCollectionDeltas(childCtx, step.ForEach.Outputs)
		if err != nil {
			if step.OnError == types.OnErrorTypeContinueRecord {
				continue
			}
			return forEachError(childCtx, step, err)
		}
		if err := mergeCollectionDeltas(ctx, step.ForEach.Outputs, deltas); err != nil {
			if step.OnError == types.OnErrorTypeContinueRecord {
				continue
			}
			return forEachError(childCtx, step, err)
		}
	}

	return nil
}

func forEachError(ctx *types.RunContext, step *types.PipelineStep, cause error) error {
	if step.OnError == types.OnErrorTypeContinueRecord {
		return skipRecordError(ctx, step, cause)
	}
	return stepError(ctx, step, cause)
}

func initializeForEachOutputs(ctx *types.RunContext, outputs []*types.Output) error {
	seen := make(map[string]struct{}, len(outputs))
	for _, output := range outputs {
		if output == nil {
			return fmt.Errorf("for_each output is nil")
		}
		if err := output.Validate(); err != nil {
			return err
		}
		if _, exists := seen[output.Into]; exists {
			return fmt.Errorf("for_each output target %q is declared more than once", output.Into)
		}
		seen[output.Into] = struct{}{}
		if _, exists := ctx.Collections[output.Into]; exists {
			return fmt.Errorf("collection %q already exists", output.Into)
		}
	}
	for _, output := range outputs {
		ctx.Collections[output.Into] = make([]types.Record, 0)
	}
	return nil
}

func buildCollectionDeltas(ctx *types.RunContext, outputs []*types.Output) ([]types.CollectionDelta, error) {
	deltas := make([]types.CollectionDelta, 0, len(outputs))
	for _, output := range outputs {
		from, ok := ctx.Collections[output.From]
		if !ok {
			return nil, fmt.Errorf("collection %q does not exist in child context", output.From)
		}

		delta := types.CollectionDelta{
			Target:  output.Into,
			Records: make([]types.Record, 0, len(from)),
		}
		for _, record := range from {
			clone := maps.Clone(record)
			if clone == nil {
				clone = make(types.Record)
			}
			for _, field := range output.DeduplicateBy {
				if _, exists := clone[field]; !exists {
					return nil, fmt.Errorf("collection %q record is missing deduplication field %q", output.From, field)
				}
			}
			delta.Records = append(delta.Records, clone)
		}
		deltas = append(deltas, delta)
	}
	return deltas, nil
}

func mergeCollectionDeltas(ctx *types.RunContext, outputs []*types.Output, deltas []types.CollectionDelta) error {
	if len(outputs) != len(deltas) {
		return fmt.Errorf("for_each output and delta counts differ: %d vs %d", len(outputs), len(deltas))
	}

	updated := make(map[string][]types.Record, len(deltas))
	for i, delta := range deltas {
		current, ok := ctx.Collections[delta.Target]
		if !ok {
			return fmt.Errorf("aggregate collection %q does not exist", delta.Target)
		}

		candidate := make([]types.Record, len(current))
		copy(candidate, current)
		seen := make(map[string]struct{})
		if len(outputs[i].DeduplicateBy) > 0 {
			for _, record := range candidate {
				key, err := collectionRecordKey(record, outputs[i].DeduplicateBy)
				if err != nil {
					return fmt.Errorf("aggregate collection %q: %w", delta.Target, err)
				}
				seen[key] = struct{}{}
			}
		}

		for _, record := range delta.Records {
			if len(outputs[i].DeduplicateBy) > 0 {
				key, err := collectionRecordKey(record, outputs[i].DeduplicateBy)
				if err != nil {
					return fmt.Errorf("aggregate collection %q: %w", delta.Target, err)
				}
				if _, duplicate := seen[key]; duplicate {
					continue
				}
				seen[key] = struct{}{}
			}
			if len(candidate) >= effectiveMaxRecords(ctx) {
				return fmt.Errorf("aggregate collection %q exceeds maximum allowed records: %d", delta.Target, effectiveMaxRecords(ctx))
			}
			candidate = append(candidate, record)
		}
		updated[delta.Target] = candidate
	}

	// Publish only after every output delta has been validated and prepared.
	for _, delta := range deltas {
		ctx.Collections[delta.Target] = updated[delta.Target]
	}
	return nil
}

func collectionRecordKey(record types.Record, fields []string) (string, error) {
	var key strings.Builder
	for _, field := range fields {
		value, ok := record[field]
		if !ok {
			return "", fmt.Errorf("record is missing deduplication field %q", field)
		}
		key.WriteString(strconv.Itoa(len(value)))
		key.WriteByte(':')
		key.WriteString(value)
		key.WriteByte(';')
	}
	return key.String(), nil
}

func effectiveMaxRecords(ctx *types.RunContext) int {
	if ctx.MaxRecords > 0 {
		return ctx.MaxRecords
	}
	return types.DefaultMaxRecords
}
