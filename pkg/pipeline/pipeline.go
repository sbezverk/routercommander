package pipeline

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sbezverk/routercommander/pkg/messenger"
	"github.com/sbezverk/routercommander/pkg/types"
)

func ExecutePipeline(r types.Router, commander *types.Commander, n messenger.Notifier) error {
	runCtx := NewRunContext(r, commander)

	if err := executePipeline(r, commander.Pipeline, runCtx); err != nil {
		return err
	}

	return nil
}

func executePipeline(r types.Router, step []*types.PipelineStep, runCtx *types.RunContext) error {
	for _, s := range step {
		if err := executeStep(r, s, runCtx); err != nil {
			return err
		}
	}

	return nil
}

func NewRunContext(r types.Router, commander *types.Commander) *types.RunContext {
	maxDepth := types.DefaultMaxPipelineDepth
	if commander.PipelineLimits != nil && commander.PipelineLimits.MaxDepth != 0 {
		maxDepth = commander.PipelineLimits.MaxDepth
	}
	maxCommands := types.DefaultMaxPipelineCommands
	if commander.PipelineLimits != nil && commander.PipelineLimits.MaxCommands != 0 {
		maxCommands = commander.PipelineLimits.MaxCommands
	}
	commandsRun := 0
	return &types.RunContext{
		Variables:   map[string]string{},
		Collections: map[string][]types.Record{},
		RouterName:  r.GetName(),
		StepResults: map[string][]types.StepResult{},
		Depth:       0,
		MaxDepth:    maxDepth,
		CommandsRun: &commandsRun,
		MaxCommands: maxCommands,
	}
}

func executeStep(
	r types.Router,
	step *types.PipelineStep,
	ctx *types.RunContext,
) error {
	if step == nil {
		return fmt.Errorf("router %q: pipeline step is nil", ctx.RouterName)
	}
	if ctx.Depth+1 > ctx.MaxDepth {
		if step.OnError == types.OnErrorTypeContinueRecord {
			return skipRecordError(ctx, step, fmt.Errorf("maximum pipeline depth exceeded"))
		}
		return stepError(ctx, step, fmt.Errorf("maximum pipeline depth exceeded"))
	}
	ctx.Depth++
	previousPath := ctx.StepPath
	ctx.StepPath = append(append([]string(nil), ctx.StepPath...), step.ID)
	defer func() {
		ctx.Depth--
		ctx.StepPath = previousPath
	}()
	if step.ForEach != nil {
		records, ok := ctx.Collections[step.ForEach.In]
		if !ok {
			if step.OnError == types.OnErrorTypeContinueRecord {
				return skipRecordError(ctx, step, fmt.Errorf("collection %q does not exist", step.ForEach.In))
			}
			return stepError(ctx, step, fmt.Errorf("collection %q does not exist", step.ForEach.In))
		}
		for _, record := range records {
			childCtx := ctx.ChildForRecord(record)
			if err := executePipeline(r, step.ForEach.Steps, childCtx); err != nil {
				if errors.Is(err, types.ErrPipelineSkipRecord) {
					continue
				}
				if step.OnError == types.OnErrorTypeContinueRecord {
					continue
				}
				return err
			}
		}

		return nil
	}

	if step.Branch != nil {
		return fmt.Errorf("branch execution not implemented")
	}
	if step.Run != nil {
		renderData := buildRenderData(ctx)

		// Do not modify the YAML command shared by the pipeline.
		cmd := *step.Run
		cmd.SetRuntimeRenderData(renderData)

		results, err := r.ProcessCommand(&cmd, true)
		if err != nil {
			if step.OnError == types.OnErrorTypeContinueRecord {
				return skipRecordError(ctx, step, err)
			}
			return stepError(ctx, step, err)
		}
		if (*ctx.CommandsRun)+len(results) > ctx.MaxCommands {
			if step.OnError == types.OnErrorTypeContinueRecord {
				return skipRecordError(ctx, step, fmt.Errorf("maximum pipeline commands exceeded"))
			}
			return stepError(ctx, step, fmt.Errorf("maximum pipeline commands exceeded"))
		}
		(*ctx.CommandsRun) += len(results)
		stepResults, ok := ctx.StepResults[step.ID]
		if !ok {
			stepResults = make([]types.StepResult, 0)
		}
		stepResults = append(stepResults, convertResults(results)...)
		ctx.StepResults[step.ID] = stepResults
	}

	if step.Extract != nil {
		records, err := extractRecords(ctx, step.Extract)
		if err != nil {
			if step.OnError == types.OnErrorTypeContinueRecord {
				return skipRecordError(ctx, step, err)
			}
			return stepError(ctx, step, err)
		}
		_ = records

	}

	return nil
}

func stepError(ctx *types.RunContext, step *types.PipelineStep, cause error) error {
	parts := []string{
		fmt.Sprintf("router %q", ctx.RouterName),
		fmt.Sprintf("pipeline step %q", step.ID),
	}
	if path := strings.Join(ctx.StepPath, " -> "); path != "" {
		parts = append(parts, fmt.Sprintf("path %q", path))
	}
	if step.Run != nil {
		parts = append(parts, fmt.Sprintf("command %q", step.Run.Cmd))
	}
	if len(ctx.Current) != 0 {
		parts = append(parts, fmt.Sprintf("record %v", ctx.Current))
	}

	return fmt.Errorf("%s: %w", strings.Join(parts, ", "), cause)
}

func skipRecordError(ctx *types.RunContext, step *types.PipelineStep, cause error) error {
	diagnostic := stepError(ctx, step, cause)
	return fmt.Errorf("%w: %v", types.ErrPipelineSkipRecord, diagnostic)
}

func convertResults(results []*types.CmdResult) []types.StepResult {
	if results == nil {
		return nil
	}
	stepResults := make([]types.StepResult, len(results))
	for i, r := range results {
		stepResults[i] = types.StepResult{
			Output:   r.Result,
			Command:  r.Cmd,
			Location: r.Location,
		}
	}
	return stepResults
}

func buildRenderData(runCtx *types.RunContext) map[string]any {
	data := map[string]any{
		"RouterName": runCtx.RouterName,
	}
	// Location is added by the router execution layer for each concrete
	// location after this scope data has been prepared.

	// Populating with Inherited variables
	for name, value := range runCtx.Variables {
		data[name] = value
	}
	// Populating with current step context
	for name, value := range runCtx.Current {
		data[name] = value
	}

	return data
}
