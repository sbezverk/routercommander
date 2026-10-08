package pipeline

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/sbezverk/routercommander/pkg/types"
)

func forEachTestContext(source []types.Record, output string, maxRecords int) *types.RunContext {
	commandsRun := 0
	return &types.RunContext{
		RouterName: "router-1",
		Variables:  map[string]string{},
		Collections: map[string][]types.Record{
			"source": source,
		},
		StepResults: map[string][]types.StepResult{
			"discover": {{Output: []byte(output)}},
		},
		MaxDepth:    8,
		CommandsRun: &commandsRun,
		MaxCommands: 50,
		MaxRecords:  maxRecords,
	}
}

func forEachExtractStep() *types.PipelineStep {
	return forEachExtractStepNamed("child_records")
}

func forEachExtractStepNamed(collection string) *types.PipelineStep {
	return &types.PipelineStep{
		ID: "extract",
		Extract: &types.PipelineExtract{
			FromStepID: "discover",
			RecordSpec: &types.RecordSpec{
				Name: collection,
				Pattern: &types.Pattern{
					PatternString: `^(?P<value>\S+)$`,
					RegExp:        regexp.MustCompile(`^(?P<value>\S+)$`),
				},
			},
		},
	}
}

func forEachTestStep(outputs ...*types.Output) *types.PipelineStep {
	return &types.PipelineStep{
		ID: "iterate",
		ForEach: &types.PipelineForEach{
			In:      "source",
			Steps:   []*types.PipelineStep{forEachExtractStep()},
			Outputs: outputs,
		},
	}
}

func TestExecuteForEachAggregatesInSourceOrder(t *testing.T) {
	ctx := forEachTestContext(
		[]types.Record{{"id": "one"}, {"id": "two"}},
		"a\nb\n",
		20,
	)

	err := executeStep(&testRouter{}, forEachTestStep(&types.Output{
		From: "child_records",
		Into: "aggregate",
	}), ctx)
	if err != nil {
		t.Fatalf("executeStep() error = %v", err)
	}

	want := []types.Record{
		{"value": "a"},
		{"value": "b"},
		{"value": "a"},
		{"value": "b"},
	}
	if got := ctx.Collections["aggregate"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("aggregate = %#v, want %#v", got, want)
	}
}

func TestExecuteForEachInitializesEmptyAggregate(t *testing.T) {
	ctx := forEachTestContext(nil, "a\n", 20)

	if err := executeStep(&testRouter{}, forEachTestStep(&types.Output{
		From: "child_records",
		Into: "aggregate",
	}), ctx); err != nil {
		t.Fatalf("executeStep() error = %v", err)
	}

	aggregate, ok := ctx.Collections["aggregate"]
	if !ok || aggregate == nil || len(aggregate) != 0 {
		t.Fatalf("aggregate = %#v, present = %t, want non-nil empty collection", aggregate, ok)
	}
}

func TestExecuteForEachPreservesNonNilAggregateWhenChildProducesNoRecords(t *testing.T) {
	ctx := forEachTestContext([]types.Record{{"id": "one"}}, "no matching records\n", 20)

	if err := executeStep(&testRouter{}, forEachTestStep(&types.Output{
		From: "child_records",
		Into: "aggregate",
	}), ctx); err != nil {
		t.Fatalf("executeStep() error = %v", err)
	}

	aggregate, ok := ctx.Collections["aggregate"]
	if !ok || aggregate == nil || len(aggregate) != 0 {
		t.Fatalf("aggregate = %#v, present = %t, want non-nil empty collection", aggregate, ok)
	}
}

func TestExecuteForEachDeduplicatesAcrossIterations(t *testing.T) {
	ctx := forEachTestContext(
		[]types.Record{{"id": "one"}, {"id": "two"}},
		"a\na\nb\n",
		20,
	)

	if err := executeStep(&testRouter{}, forEachTestStep(&types.Output{
		From:          "child_records",
		Into:          "aggregate",
		DeduplicateBy: []string{"value"},
	}), ctx); err != nil {
		t.Fatalf("executeStep() error = %v", err)
	}

	want := []types.Record{{"value": "a"}, {"value": "b"}}
	if got := ctx.Collections["aggregate"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("aggregate = %#v, want %#v", got, want)
	}
}

func TestExecuteForEachEnforcesAggregateLimitPerSuccessfulIteration(t *testing.T) {
	ctx := forEachTestContext(
		[]types.Record{{"id": "one"}, {"id": "two"}},
		"a\nb\n",
		3,
	)

	err := executeStep(&testRouter{}, forEachTestStep(&types.Output{
		From: "child_records",
		Into: "aggregate",
	}), ctx)
	if err == nil || !strings.Contains(err.Error(), "aggregate collection") {
		t.Fatalf("error = %v, want aggregate-limit error", err)
	}
	if got := ctx.Collections["aggregate"]; !reflect.DeepEqual(got, []types.Record{{"value": "a"}, {"value": "b"}}) {
		t.Fatalf("aggregate after limit error = %#v, want first iteration only", got)
	}
}

func TestExecuteForEachDoesNotPublishPartialIteration(t *testing.T) {
	ctx := forEachTestContext([]types.Record{{"id": "one"}}, "a\n", 20)
	step := forEachTestStep(
		&types.Output{From: "child_records", Into: "aggregate"},
		&types.Output{From: "missing", Into: "other"},
	)

	err := executeStep(&testRouter{}, step, ctx)
	if err == nil || !strings.Contains(err.Error(), `collection "missing" does not exist`) {
		t.Fatalf("error = %v, want missing child collection error", err)
	}
	if got := ctx.Collections["aggregate"]; got == nil || len(got) != 0 {
		t.Fatalf("aggregate = %#v, want empty after failed iteration", got)
	}
	if got := ctx.Collections["other"]; got == nil || len(got) != 0 {
		t.Fatalf("other = %#v, want empty after failed iteration", got)
	}
}

func TestExecuteForEachRetainsEarlierSuccessfulIterationOnNoData(t *testing.T) {
	ctx := forEachTestContext([]types.Record{{"id": "one"}, {"id": "two"}}, "a\n", 20)
	missing := &types.PipelineStep{
		ID:  "no_data",
		Run: &types.Command{Cmd: "no-data"},
	}
	step := forEachTestStep(&types.Output{From: "child_records", Into: "aggregate"})
	step.ForEach.Steps = []*types.PipelineStep{
		{
			ID: "choose",
			Branch: &types.PipelineBranch{
				Cases: []*types.PipelineCase{{
					When:  &types.Predicate{Field: "id", Equals: stringPointer("two")},
					Steps: []*types.PipelineStep{missing},
				}},
				Default: []*types.PipelineStep{forEachExtractStep()},
			},
		},
	}
	router := &testRouter{results: map[string][]*types.CmdResult{"no-data": {}}}

	err := executeStep(router, step, ctx)
	if !errors.Is(err, types.ErrPipelineNoData) {
		t.Fatalf("error = %v, want ErrPipelineNoData", err)
	}
	if got := ctx.Collections["aggregate"]; !reflect.DeepEqual(got, []types.Record{{"value": "a"}}) {
		t.Fatalf("aggregate = %#v, want earlier successful iteration", got)
	}
}

func TestExecuteNestedForEachPublishesOnlyToImmediateParent(t *testing.T) {
	ctx := forEachTestContext([]types.Record{{"id": "outer"}}, "a\n", 20)
	ctx.Collections["inner_source"] = []types.Record{{"id": "one"}, {"id": "two"}}

	inner := &types.PipelineStep{
		ID: "inner",
		ForEach: &types.PipelineForEach{
			In:    "inner_source",
			Steps: []*types.PipelineStep{forEachExtractStepNamed("inner_records")},
			Outputs: []*types.Output{{
				From: "inner_records", Into: "inner_aggregate",
			}},
		},
	}
	outer := &types.PipelineStep{
		ID: "outer",
		ForEach: &types.PipelineForEach{
			In:    "source",
			Steps: []*types.PipelineStep{inner},
			Outputs: []*types.Output{{
				From: "inner_aggregate", Into: "outer_aggregate",
			}},
		},
	}

	if err := executeStep(&testRouter{}, outer, ctx); err != nil {
		t.Fatalf("executeStep() error = %v", err)
	}
	if got := ctx.Collections["outer_aggregate"]; !reflect.DeepEqual(got, []types.Record{{"value": "a"}, {"value": "a"}}) {
		t.Fatalf("outer aggregate = %#v, want records published by inner parent", got)
	}
	if _, exists := ctx.Collections["inner_aggregate"]; exists {
		t.Fatal("inner aggregate leaked into the outer root context")
	}
}

func TestExecuteForEachRejectsMissingRuntimeDeduplicationField(t *testing.T) {
	ctx := forEachTestContext([]types.Record{{"id": "one"}}, "a\n", 20)
	step := forEachTestStep(&types.Output{
		From:          "child_records",
		Into:          "aggregate",
		DeduplicateBy: []string{"missing"},
	})

	err := executeStep(&testRouter{}, step, ctx)
	if err == nil || !strings.Contains(err.Error(), "deduplication field") {
		t.Fatalf("error = %v, want missing deduplication field error", err)
	}
	if got := ctx.Collections["aggregate"]; got == nil || len(got) != 0 {
		t.Fatalf("aggregate = %#v, want empty after runtime validation error", got)
	}
}

func TestExecuteForEachContinueRecordSkipsDeltaFailure(t *testing.T) {
	ctx := forEachTestContext([]types.Record{{"id": "one"}, {"id": "two"}}, "a\n", 20)
	step := forEachTestStep(&types.Output{
		From: "child_records",
		Into: "aggregate",
	})
	step.OnError = types.OnErrorTypeContinueRecord
	step.ForEach.Steps = []*types.PipelineStep{
		{
			ID: "choose",
			Branch: &types.PipelineBranch{
				Cases: []*types.PipelineCase{{
					When: &types.Predicate{Field: "id", Equals: stringPointer("two")},
					Steps: []*types.PipelineStep{{
						ID:  "fail",
						Run: &types.Command{Cmd: "fail"},
					}},
				}},
				Default: []*types.PipelineStep{forEachExtractStep()},
			},
		},
	}
	router := &testRouter{commandErrors: map[string]error{"fail": errors.New("child failed")}}

	if err := executeStep(router, step, ctx); err != nil {
		t.Fatalf("executeStep() error = %v, want skipped iteration", err)
	}
	if got := ctx.Collections["aggregate"]; !reflect.DeepEqual(got, []types.Record{{"value": "a"}}) {
		t.Fatalf("aggregate = %#v, want only successful iteration", got)
	}
}

func stringPointer(value string) *string {
	return &value
}
