package pipeline

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sbezverk/routercommander/pkg/types"
)

func executeJoin(ctx *types.RunContext, spec *types.PipelineJoin) error {
	if ctx == nil {
		return fmt.Errorf("run context is nil")
	}
	if ctx.InRecordScope {
		return fmt.Errorf("cannot execute join in record scope")
	}
	if spec == nil {
		return fmt.Errorf("join spec is nil")
	}
	leftColl, err := checkForCollection(ctx, spec.Left)
	if err != nil {
		return fmt.Errorf("join, left collection %q error: %w", spec.Left, err)
	}
	for _, on := range spec.On {
		for _, rec := range leftColl {
			if _, ok := rec[on]; !ok {
				return fmt.Errorf("join, field %q not found in left collection record", on)
			}
		}
	}
	rightColl, err := checkForCollection(ctx, spec.Right)
	if err != nil {
		return fmt.Errorf("join, right collection %q error: %w", spec.Right, err)
	}
	output := make([]types.Record, 0)
	rightIndex := make(map[string][]types.Record)

	for _, rightRecord := range rightColl {
		key, err := buildJoinKey(rightRecord, spec.On)
		if err != nil {
			return err
		}
		rightIndex[key] = append(rightIndex[key], rightRecord)
	}
	for _, leftRecord := range leftColl {
		key, err := buildJoinKey(leftRecord, spec.On)
		if err != nil {
			return err
		}

		matches := rightIndex[key]
		if len(matches) == 0 && spec.Unmatched == "error" {
			return fmt.Errorf("no matching records found for left record with key %q", key)
		}

		for _, rightRecord := range matches {
			// Validate collisions and create a fresh merged record.
			merged, err := mergeJoinRecords(leftRecord, rightRecord, spec.On)
			if err != nil {
				return err
			}

			if len(output)+1 > ctx.MaxRecords {
				return fmt.Errorf("join output exceeds maximum allowed records: %d", ctx.MaxRecords)
			}
			output = append(output, merged)

		}
	}
	ctx.Collections[spec.Output] = output

	return nil
}

func mergeJoinRecords(
	leftRec, rightRec types.Record,
	onFields []string,
) (types.Record, error) {
	result := make(types.Record)
	joinFields := make(map[string]struct{}, len(onFields))

	// Join fields must exist on both sides and have equal values.
	for _, field := range onFields {
		joinFields[field] = struct{}{}

		leftValue, leftOK := leftRec[field]
		rightValue, rightOK := rightRec[field]
		if !leftOK || !rightOK {
			return nil, fmt.Errorf(
				"join field %q is missing in one of the records", field)
		}
		if leftValue != rightValue {
			return nil, fmt.Errorf(
				"join collision on field %q: %q vs %q",
				field, leftValue, rightValue)
		}

		result[field] = leftValue
	}

	// Add non-key fields from the left record.
	for field, value := range leftRec {
		if _, isJoinField := joinFields[field]; isJoinField {
			continue
		}
		result[field] = value
	}

	// Add non-key fields from the right record.
	for field, value := range rightRec {
		if _, isJoinField := joinFields[field]; isJoinField {
			continue
		}
		if _, exists := result[field]; exists {
			return nil, fmt.Errorf(
				"non-key field collision on %q", field)
		}
		result[field] = value
	}

	return result, nil
}

func buildJoinKey(record types.Record, fields []string) (string, error) {
	var key strings.Builder

	for _, field := range fields {
		value, ok := record[field]
		if !ok {
			return "", fmt.Errorf("join field %q is missing", field)
		}

		key.WriteString(strconv.Itoa(len(value)))
		key.WriteByte(':')
		key.WriteString(value)
		key.WriteByte(';')
	}

	return key.String(), nil
}
