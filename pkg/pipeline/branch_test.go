package pipeline

import (
	"errors"
	"strings"
	"testing"

	"github.com/sbezverk/routercommander/pkg/types"
)

func branchTestContext() *types.RunContext {
	commandsRun := 0
	return &types.RunContext{
		RouterName:  "router-1",
		Variables:   map[string]string{},
		Collections: map[string][]types.Record{},
		StepResults: map[string][]types.StepResult{},
		MaxDepth:    8,
		CommandsRun: &commandsRun,
		MaxCommands: 20,
	}
}

func branchString(value string) *string {
	return &value
}

func TestExecuteBranchSelectsFirstMatchingCase(t *testing.T) {
	ctx := branchTestContext()
	ctx.Current = types.Record{"state": "ready"}
	router := &testRouter{}
	step := &types.PipelineStep{
		ID: "classify",
		Branch: &types.PipelineBranch{
			Cases: []*types.PipelineCase{
				{
					When: &types.Predicate{Field: "state", Equals: branchString("ready")},
					Steps: []*types.PipelineStep{{
						ID:  "first",
						Run: &types.Command{Cmd: "first"},
					}},
				},
				{
					When: &types.Predicate{Field: "state", Equals: branchString("ready")},
					Steps: []*types.PipelineStep{{
						ID:  "second",
						Run: &types.Command{Cmd: "second"},
					}},
				},
			},
		},
	}

	if err := executeStep(router, step, ctx); err != nil {
		t.Fatalf("executeStep failed: %v", err)
	}
	if got := router.commandIDs; len(got) != 1 || got[0] != "first" {
		t.Fatalf("executed commands = %v, want [first]", got)
	}
}

func TestExecuteBranchRunsDefault(t *testing.T) {
	ctx := branchTestContext()
	ctx.Current = types.Record{"state": "other"}
	router := &testRouter{}
	step := &types.PipelineStep{
		ID: "classify",
		Branch: &types.PipelineBranch{
			Cases: []*types.PipelineCase{{
				When:  &types.Predicate{Field: "state", Equals: branchString("ready")},
				Steps: []*types.PipelineStep{{ID: "matched", Run: &types.Command{Cmd: "matched"}}},
			}},
			Default: []*types.PipelineStep{{ID: "default", Run: &types.Command{Cmd: "default"}}},
		},
	}

	if err := executeStep(router, step, ctx); err != nil {
		t.Fatalf("executeStep failed: %v", err)
	}
	if got := router.commandIDs; len(got) != 1 || got[0] != "default" {
		t.Fatalf("executed commands = %v, want [default]", got)
	}
}

func TestExecuteBranchNoMatchSkipsOnlyCurrentRecord(t *testing.T) {
	ctx := branchTestContext()
	ctx.Collections["targets"] = []types.Record{{"state": "skip"}, {"state": "ready"}}
	router := &testRouter{}
	step := &types.PipelineStep{
		ID: "iterate",
		ForEach: &types.PipelineForEach{
			In: "targets",
			Steps: []*types.PipelineStep{
				{
					ID: "classify",
					Branch: &types.PipelineBranch{
						Cases: []*types.PipelineCase{{
							When:  &types.Predicate{Field: "state", Equals: branchString("ready")},
							Steps: []*types.PipelineStep{{ID: "selected", Run: &types.Command{Cmd: "selected"}}},
						}},
					},
				},
				{ID: "after", Run: &types.Command{Cmd: "after"}},
			},
		},
	}

	if err := executeStep(router, step, ctx); err != nil {
		t.Fatalf("executeStep failed: %v", err)
	}
	if got := router.commandIDs; len(got) != 2 || got[0] != "selected" || got[1] != "after" {
		t.Fatalf("executed commands = %v, want [selected after]", got)
	}
}

func TestExecuteBranchDoesNotRewriteChildStopRouterError(t *testing.T) {
	childErr := errors.New("child command failed")
	ctx := branchTestContext()
	ctx.Current = types.Record{"state": "ready"}
	router := &testRouter{commandErrors: map[string]error{"child": childErr}}
	step := &types.PipelineStep{
		ID:      "classify",
		OnError: types.OnErrorTypeContinueRecord,
		Branch: &types.PipelineBranch{
			Cases: []*types.PipelineCase{{
				When:  &types.Predicate{Field: "state", Equals: branchString("ready")},
				Steps: []*types.PipelineStep{{ID: "child", Run: &types.Command{Cmd: "child"}}},
			}},
		},
	}

	err := executeStep(router, step, ctx)
	if err == nil || !strings.Contains(err.Error(), childErr.Error()) {
		t.Fatalf("error = %v, want child error", err)
	}
	if errors.Is(err, types.ErrPipelineSkipRecord) {
		t.Fatalf("child stop_router error was converted to skip-record: %v", err)
	}
}

func TestExecuteBranchPropagatesChildNoData(t *testing.T) {
	ctx := branchTestContext()
	ctx.Current = types.Record{"state": "ready"}
	router := &testRouter{results: map[string][]*types.CmdResult{"child": {}}}
	step := &types.PipelineStep{
		ID: "classify",
		Branch: &types.PipelineBranch{
			Cases: []*types.PipelineCase{{
				When:  &types.Predicate{Field: "state", Equals: branchString("ready")},
				Steps: []*types.PipelineStep{{ID: "child", Run: &types.Command{Cmd: "child"}}},
			}},
		},
	}

	if err := executeStep(router, step, ctx); !errors.Is(err, types.ErrPipelineNoData) {
		t.Fatalf("error = %v, want ErrPipelineNoData", err)
	}
}

func TestExecuteNestedBranch(t *testing.T) {
	ctx := branchTestContext()
	ctx.Current = types.Record{"state": "ready", "mode": "detail"}
	router := &testRouter{}
	step := &types.PipelineStep{
		ID: "outer",
		Branch: &types.PipelineBranch{
			Cases: []*types.PipelineCase{{
				When: &types.Predicate{Field: "state", Equals: branchString("ready")},
				Steps: []*types.PipelineStep{{
					ID: "inner",
					Branch: &types.PipelineBranch{
						Cases: []*types.PipelineCase{{
							When:  &types.Predicate{Field: "mode", Equals: branchString("detail")},
							Steps: []*types.PipelineStep{{ID: "detail", Run: &types.Command{Cmd: "detail"}}},
						}},
					},
				}},
			}},
		},
	}

	if err := executeStep(router, step, ctx); err != nil {
		t.Fatalf("executeStep failed: %v", err)
	}
	if got := router.commandIDs; len(got) != 1 || got[0] != "detail" {
		t.Fatalf("executed commands = %v, want [detail]", got)
	}
}

func TestExecuteBranchChildCanExtractAndIterate(t *testing.T) {
	ctx := branchTestContext()
	ctx.Current = types.Record{"state": "ready"}
	router := &testRouter{results: map[string][]*types.CmdResult{
		"discover": {{Cmd: "discover", Result: []byte("target\n")}},
	}}
	step := &types.PipelineStep{
		ID: "classify",
		Branch: &types.PipelineBranch{
			Cases: []*types.PipelineCase{{
				When: &types.Predicate{Field: "state", Equals: branchString("ready")},
				Steps: []*types.PipelineStep{
					{ID: "discover", Run: &types.Command{Cmd: "discover"}},
					{
						ID: "extract",
						Extract: &types.PipelineExtract{
							FromStepID: "discover",
							RecordSpec: &types.RecordSpec{
								Name:    "targets",
								Pattern: compiledPipelinePattern(t, `^(?P<value>\S+)$`),
							},
						},
					},
					{
						ID: "iterate",
						ForEach: &types.PipelineForEach{
							In:    "targets",
							Steps: []*types.PipelineStep{{ID: "inspect", Run: &types.Command{Cmd: "inspect {{.value}}"}}},
						},
					},
				},
			}},
		},
	}

	if err := executeStep(router, step, ctx); err != nil {
		t.Fatalf("executeStep failed: %v", err)
	}
	if got := router.commandIDs; len(got) != 2 || got[0] != "discover" || got[1] != "inspect {{.value}}" {
		t.Fatalf("executed commands = %v, want [discover inspect {{.value}}]", got)
	}
}
