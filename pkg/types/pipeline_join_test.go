package types

import (
	"strings"
	"testing"
)

func TestPipelineJoinValidateRejectsInvalidKeys(t *testing.T) {
	tests := []struct {
		name string
		on   []string
		want string
	}{
		{name: "empty key", on: []string{" "}, want: "cannot be empty"},
		{name: "duplicate key", on: []string{"npu", " npu "}, want: "duplicate"},
		{name: "reserved key", on: []string{"Location"}, want: "reserved"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &PipelineJoin{
				Left: "left", Right: "right", On: tt.on, Output: "joined",
			}
			err := spec.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func TestPipelineJoinValidationRegistersCompleteOutputSchema(t *testing.T) {
	steps := []*PipelineStep{
		pipelineTestRun("discover_left"),
		pipelineTestExtract("extract_left", "discover_left", "left", `(?P<key>\S+)\s+(?P<left_value>\S+)`, "", nil),
		pipelineTestRun("discover_right"),
		pipelineTestExtract("extract_right", "discover_right", "right", `(?P<key>\S+)\s+(?P<right_value>\S+)`, "", nil),
		{
			ID: "join",
			Join: &PipelineJoin{
				Left: "left", Right: "right", On: []string{"key"}, Output: "joined",
			},
		},
	}

	err, ctx := validateTestPipeline(steps)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	for _, field := range []string{"key", "left_value", "right_value"} {
		if _, ok := ctx.pipelineSymbols.collectionFields["joined"][field]; !ok {
			t.Fatalf("joined collection is missing field %q", field)
		}
	}
	if _, ok := ctx.collectionNames["joined"]; !ok {
		t.Fatal("joined collection name was not registered")
	}
}
