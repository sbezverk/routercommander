package types

import (
	"strings"
	"testing"
)

func TestPipelineExportValidate(t *testing.T) {
	valid := PipelineExport{
		From:    "targets",
		Field:   "npu",
		As:      "hosting_npu",
		Require: "exactly_one",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid export rejected: %v", err)
	}

	tests := []struct {
		name string
		spec PipelineExport
		want string
	}{
		{
			name: "missing source collection",
			spec: PipelineExport{Field: "npu", As: "hosting_npu", Require: "exactly_one"},
			want: "from",
		},
		{
			name: "missing source field",
			spec: PipelineExport{From: "targets", As: "hosting_npu", Require: "exactly_one"},
			want: "field",
		},
		{
			name: "missing destination",
			spec: PipelineExport{From: "targets", Field: "npu", Require: "exactly_one"},
			want: "as",
		},
		{
			name: "invalid requirement",
			spec: PipelineExport{From: "targets", Field: "npu", As: "hosting_npu", Require: "first"},
			want: "exactly_one",
		},
		{
			name: "reserved source field",
			spec: PipelineExport{From: "targets", Field: "Location", As: "hosting_npu", Require: "exactly_one"},
			want: "reserved",
		},
		{
			name: "reserved destination",
			spec: PipelineExport{From: "targets", Field: "npu", As: "RouterName", Require: "exactly_one"},
			want: "reserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func TestPipelineExportSemanticValidation(t *testing.T) {
	t.Run("records field and exported variable are registered", func(t *testing.T) {
		steps := []*PipelineStep{
			pipelineTestRun("discover"),
			pipelineTestExtract("extract", "discover", "targets", `(?P<npu>\S+)`, "", nil),
			{
				ID: "export",
				Export: &PipelineExport{
					From:    "targets",
					Field:   "npu",
					As:      "hosting_npu",
					Require: "exactly_one",
				},
			},
		}

		err, ctx := validateTestPipeline(steps)
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if _, ok := ctx.pipelineSymbols.variableFields["hosting_npu"]; !ok {
			t.Fatal("exported variable was not registered")
		}
	})

	t.Run("unknown collection is rejected", func(t *testing.T) {
		steps := []*PipelineStep{{
			ID: "export",
			Export: &PipelineExport{
				From:    "missing",
				Field:   "npu",
				As:      "hosting_npu",
				Require: "exactly_one",
			},
		}}
		if err, _ := validateTestPipeline(steps); err == nil {
			t.Fatal("expected unknown collection validation error")
		}
	})

	t.Run("unknown field is rejected", func(t *testing.T) {
		steps := []*PipelineStep{
			pipelineTestRun("discover"),
			pipelineTestExtract("extract", "discover", "targets", `(?P<npu>\S+)`, "", nil),
			{
				ID: "export",
				Export: &PipelineExport{
					From:    "targets",
					Field:   "db_id",
					As:      "hosting_db",
					Require: "exactly_one",
				},
			},
		}
		if err, _ := validateTestPipeline(steps); err == nil {
			t.Fatal("expected unknown field validation error")
		}
	})

	t.Run("export inside for each is rejected", func(t *testing.T) {
		steps := []*PipelineStep{
			pipelineTestRun("discover"),
			pipelineTestExtract("extract", "discover", "targets", `(?P<npu>\S+)`, "", nil),
			{
				ID: "iterate",
				ForEach: &PipelineForEach{
					In: "targets",
					Steps: []*PipelineStep{{
						ID: "export",
						Export: &PipelineExport{
							From:    "targets",
							Field:   "npu",
							As:      "hosting_npu",
							Require: "exactly_one",
						},
					}},
				},
			},
		}
		if err, _ := validateTestPipeline(steps); err == nil {
			t.Fatal("expected export-in-for-each validation error")
		}
	})

	t.Run("child collection is not visible to parent export", func(t *testing.T) {
		steps := []*PipelineStep{
			pipelineTestRun("discover"),
			pipelineTestExtract("extract", "discover", "targets", `(?P<npu>\S+)`, "", nil),
			{
				ID: "iterate",
				ForEach: &PipelineForEach{
					In: "targets",
					Steps: []*PipelineStep{
						pipelineTestRun("child_discover"),
						pipelineTestExtract("child_extract", "child_discover", "child_targets", `(?P<npu>\S+)`, "", nil),
					},
				},
			},
			{
				ID: "export",
				Export: &PipelineExport{
					From:    "child_targets",
					Field:   "npu",
					As:      "hosting_npu",
					Require: "exactly_one",
				},
			},
		}
		if err, _ := validateTestPipeline(steps); err == nil {
			t.Fatal("expected child collection visibility validation error")
		}
	})

	t.Run("mixed context and exported predicate fields are accepted", func(t *testing.T) {
		value := "ready"
		steps := []*PipelineStep{
			pipelineTestRun("discover"),
			pipelineTestExtract("extract", "discover", "targets", `(?P<npu>\S+)`, `state=(?P<state>\S+)`, nil),
			{
				ID: "export",
				Export: &PipelineExport{
					From:    "targets",
					Field:   "npu",
					As:      "hosting_npu",
					Require: "exactly_one",
				},
			},
			{
				ID: "classify",
				Branch: &PipelineBranch{
					Cases: []*PipelineCase{
						{
							When: &Predicate{All: []*Predicate{
								{Field: "state", Equals: &value},
								{Field: "hosting_npu", Exists: boolPointer(true)},
							}},
							Steps: []*PipelineStep{pipelineTestRun("matched")},
						},
					},
				},
			},
		}
		if err, _ := validateTestPipeline(steps); err != nil {
			t.Fatalf("unexpected mixed-symbol validation error: %v", err)
		}
	})
}
