package pipeline

import (
	"errors"
	"strings"
	"testing"

	"github.com/sbezverk/routercommander/pkg/log"
	"github.com/sbezverk/routercommander/pkg/types"
)

type testRouter struct {
	commands int
}

func (r *testRouter) IsExistingLocation(string) bool { return false }
func (r *testRouter) GetAllLCs() []string            { return nil }
func (r *testRouter) GetAllRPs() []string            { return nil }
func (r *testRouter) GetActiveRP() string            { return "" }
func (r *testRouter) GetAllLocations() []string      { return nil }
func (r *testRouter) GetName() string                { return "router-1" }
func (r *testRouter) GetData(string, bool, int) ([]byte, error) {
	return nil, nil
}
func (r *testRouter) ProcessCommand(cmd *types.Command, collectResult bool) ([]*types.CmdResult, error) {
	r.commands++
	if !collectResult {
		return nil, nil
	}
	return []*types.CmdResult{{Cmd: cmd.Cmd, Result: []byte("output")}}, nil
}
func (r *testRouter) Close()                {}
func (r *testRouter) GetLogger() log.Logger { return nil }

func TestBuildRenderDataIncludesRouterAndScopeValues(t *testing.T) {
	ctx := &types.RunContext{
		RouterName: "router-1",
		Variables:  map[string]string{"vrf": "NMNET"},
		Current:    types.Record{"prefix": "10.0.0.0/24"},
	}

	data := buildRenderData(ctx)
	if data["RouterName"] != "router-1" {
		t.Fatalf("RouterName = %v, want router-1", data["RouterName"])
	}
	if data["vrf"] != "NMNET" || data["prefix"] != "10.0.0.0/24" {
		t.Fatalf("unexpected render data: %+v", data)
	}
}

func TestExecuteStepAppendsRepeatedStepResults(t *testing.T) {
	commandsRun := 0
	ctx := &types.RunContext{
		RouterName:  "router-1",
		Variables:   map[string]string{},
		Collections: map[string][]types.Record{},
		StepResults: map[string][]types.StepResult{},
		MaxDepth:    8,
		CommandsRun: &commandsRun,
		MaxCommands: 10,
	}
	router := &testRouter{}
	step := &types.PipelineStep{
		ID:  "discover",
		Run: &types.Command{Cmd: "show platform"},
	}

	if err := executeStep(router, step, ctx); err != nil {
		t.Fatalf("first execution failed: %v", err)
	}
	if err := executeStep(router, step, ctx); err != nil {
		t.Fatalf("second execution failed: %v", err)
	}

	if router.commands != 2 {
		t.Fatalf("router received %d commands, want 2", router.commands)
	}
	if got := len(ctx.StepResults[step.ID]); got != 2 {
		t.Fatalf("stored %d step results, want 2", got)
	}
	if commandsRun != 2 {
		t.Fatalf("CommandsRun = %d, want 2", commandsRun)
	}
}

func TestStepErrorIncludesExecutionContext(t *testing.T) {
	ctx := &types.RunContext{
		RouterName: "router-1",
		StepPath:   []string{"iterate", "inspect"},
		Current:    types.Record{"prefix": "10.0.0.0/24"},
	}
	step := &types.PipelineStep{
		ID:  "inspect",
		Run: &types.Command{Cmd: "show cef {{.prefix}}"},
	}

	err := stepError(ctx, step, errors.New("render failed"))
	message := err.Error()
	for _, expected := range []string{"router-1", "inspect", "iterate -> inspect", "show cef {{.prefix}}", "10.0.0.0/24", "render failed"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("error %q does not contain %q", message, expected)
		}
	}
}
