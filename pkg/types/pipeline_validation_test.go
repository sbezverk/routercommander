package types

import (
	"bytes"
	"testing"
)

func pipelineTestPattern(pattern string) *Pattern {
	return &Pattern{PatternString: pattern}
}

func pipelineTestRun(id string) *PipelineStep {
	return &PipelineStep{
		ID:  id,
		Run: &Command{Cmd: "show platform"},
	}
}

func pipelineTestExtract(id, from, collection, recordPattern, contextPattern string, inherit []string) *PipelineStep {
	step := &PipelineStep{
		ID: id,
		Extract: &PipelineExtract{
			FromStepID: from,
			RecordSpec: &RecordSpec{
				Name:    collection,
				Pattern: pipelineTestPattern(recordPattern),
				Inherit: inherit,
			},
		},
	}
	if contextPattern != "" {
		step.Extract.Context = []*Context{{Pattern: pipelineTestPattern(contextPattern)}}
	}
	return step
}

func validateTestPipeline(steps []*PipelineStep) (error, *pipelineValidationContext) {
	ctx := &pipelineValidationContext{
		inForEachStep:   false,
		stepIDs:         make(map[string]struct{}),
		collectionNames: make(map[string]struct{}),
		pipelineSymbols: pipelineSymbols{
			variableFields:   make(map[string]struct{}),
			contextFields:    make(map[string]struct{}),
			collectionFields: make(map[string]map[string]struct{}),
		},
		maxPipelineDepth: DefaultMaxPipelineDepth,
	}
	err, _ := validatePipeline(steps, ctx)
	return err, ctx
}

func TestPipelineValidationRejectsInvalidReferences(t *testing.T) {
	tests := []struct {
		name  string
		steps []*PipelineStep
	}{
		{
			name: "forward extract reference",
			steps: []*PipelineStep{
				pipelineTestExtract("extract", "discover", "routes", `(?P<prefix>\S+)`, "", nil),
				pipelineTestRun("discover"),
			},
		},
		{
			name: "extract references non-run step",
			steps: []*PipelineStep{
				pipelineTestRun("discover"),
				pipelineTestExtract("first", "discover", "first", `(?P<value>\S+)`, "", nil),
				pipelineTestExtract("second", "first", "second", `(?P<value>\S+)`, "", nil),
			},
		},
		{
			name: "for-each references unknown collection",
			steps: []*PipelineStep{{
				ID: "iterate",
				ForEach: &PipelineForEach{
					In:    "missing",
					Steps: []*PipelineStep{pipelineTestRun("child")},
				},
			}},
		},
		{
			name: "inherit references unknown context field",
			steps: []*PipelineStep{
				pipelineTestRun("discover"),
				pipelineTestExtract("extract", "discover", "routes", `(?P<prefix>\S+)`, "", []string{"missing"}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err, _ := validateTestPipeline(tt.steps); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestPipelineValidationRejectsDuplicateIDsAndCollections(t *testing.T) {
	duplicateIDs := []*PipelineStep{
		pipelineTestRun("same"),
		pipelineTestRun("same"),
	}
	if err, _ := validateTestPipeline(duplicateIDs); err == nil {
		t.Fatal("expected duplicate step ID error")
	}

	duplicateCollections := []*PipelineStep{
		pipelineTestRun("discover"),
		pipelineTestExtract("routes", "discover", "routes", `(?P<prefix>\S+)`, "", nil),
		{
			ID: "iterate",
			ForEach: &PipelineForEach{
				In: "routes",
				Steps: []*PipelineStep{
					pipelineTestRun("nested-discover"),
					pipelineTestExtract("nested-routes", "nested-discover", "routes", `(?P<prefix>\S+)`, "", nil),
				},
			},
		},
	}
	if err, _ := validateTestPipeline(duplicateCollections); err == nil {
		t.Fatal("expected duplicate collection-name error")
	}
}

func TestPipelineSymbolsCollectAndIsolateScopes(t *testing.T) {
	steps := []*PipelineStep{
		pipelineTestRun("discover"),
		pipelineTestExtract("routes-extract", "discover", "routes", `(?P<prefix>\S+)`, `vrf=(?P<vrf>\S+)`, []string{"vrf"}),
		{
			ID: "iterate",
			ForEach: &PipelineForEach{
				In: "routes",
				Steps: []*PipelineStep{
					pipelineTestRun("nested-discover"),
					pipelineTestExtract("details-extract", "nested-discover", "details", `(?P<next_hop>\S+)`, `detail=(?P<child_context>\S+)`, []string{"child_context"}),
				},
			},
		},
	}

	err, ctx := validateTestPipeline(steps)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if _, ok := ctx.pipelineSymbols.contextFields["vrf"]; !ok {
		t.Fatal("expected root context field vrf")
	}
	if _, ok := ctx.pipelineSymbols.contextFields["child_context"]; ok {
		t.Fatal("child context field leaked into parent scope")
	}
	for field := range map[string]bool{"prefix": true, "vrf": true} {
		if _, ok := ctx.pipelineSymbols.collectionFields["routes"][field]; !ok {
			t.Fatalf("expected routes collection field %q", field)
		}
	}
	if _, ok := ctx.pipelineSymbols.collectionFields["details"]; ok {
		t.Fatal("child collection leaked into parent scope")
	}
}

func TestPipelineSymbolsAllowInheritedFieldsWithoutRecordCaptures(t *testing.T) {
	steps := []*PipelineStep{
		pipelineTestRun("discover"),
		pipelineTestExtract("extract", "discover", "markers", `^marker$`, `vrf=(?P<vrf>\S+)`, []string{"vrf"}),
	}

	err, ctx := validateTestPipeline(steps)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if _, ok := ctx.pipelineSymbols.collectionFields["markers"]["vrf"]; !ok {
		t.Fatal("expected inherited field in markers collection")
	}
}

func TestPipelineValidationRejectsRenderFieldCollisions(t *testing.T) {
	tests := []struct {
		name  string
		steps []*PipelineStep
	}{
		{
			name: "record field conflicts with context field",
			steps: []*PipelineStep{
				pipelineTestRun("discover"),
				pipelineTestExtract("extract", "discover", "routes", `(?P<vrf>\S+)`, `vrf=(?P<vrf>\S+)`, nil),
			},
		},
		{
			name: "record field conflicts with inherited context",
			steps: []*PipelineStep{
				pipelineTestRun("discover"),
				pipelineTestExtract("extract", "discover", "routes", `(?P<vrf>\S+)`, `vrf=(?P<vrf>\S+)`, []string{"vrf"}),
			},
		},
		{
			name: "reserved record field",
			steps: []*PipelineStep{
				pipelineTestRun("discover"),
				pipelineTestExtract("extract", "discover", "routes", `(?P<Location>\S+)`, "", nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err, _ := validateTestPipeline(tt.steps); err == nil {
				t.Fatal("expected render-field collision validation error")
			}
		})
	}
}

func TestBranchPredicateFieldValidation(t *testing.T) {
	stringValue := func(value string) *string { return &value }

	branch := func(predicate *Predicate) *PipelineStep {
		return &PipelineStep{
			ID: "classify",
			Branch: &PipelineBranch{
				Cases: []*PipelineCase{{
					When:  predicate,
					Steps: []*PipelineStep{pipelineTestRun("matched")},
				}},
			},
		}
	}

	base := func(predicate *Predicate) []*PipelineStep {
		return []*PipelineStep{
			pipelineTestRun("discover"),
			pipelineTestExtract(
				"extract",
				"discover",
				"records",
				`^(?P<value>\S+)$`,
				`^(?P<field_1>\S+)\s+(?P<field_2>\S+)$`,
				nil,
			),
			branch(predicate),
		}
	}

	tests := []struct {
		name      string
		predicate *Predicate
		wantErr   bool
	}{
		{
			name:      "known simple field",
			predicate: &Predicate{Field: "field_1", Equals: stringValue("value")},
		},
		{
			name:      "unknown simple field",
			predicate: &Predicate{Field: "missing", Equals: stringValue("value")},
			wantErr:   true,
		},
		{
			name: "known all fields",
			predicate: &Predicate{All: []*Predicate{
				{Field: "field_1", Exists: boolPointer(true)},
				{Field: "field_2", Exists: boolPointer(true)},
			}},
		},
		{
			name: "unknown field nested in all",
			predicate: &Predicate{All: []*Predicate{
				{Field: "field_1", Exists: boolPointer(true)},
				{Field: "missing", Exists: boolPointer(true)},
			}},
			wantErr: true,
		},
		{
			name: "any validates every child",
			predicate: &Predicate{Any: []*Predicate{
				{Field: "field_1", Equals: stringValue("value")},
				{Field: "missing", Equals: stringValue("value")},
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err, _ := validateTestPipeline(base(tt.predicate))
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateTestPipeline() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBranchCanBeFirstChildOfForEach(t *testing.T) {
	steps := []*PipelineStep{
		pipelineTestRun("discover"),
		pipelineTestExtract("extract", "discover", "records", `(?P<field_1>\S+)`, "", nil),
		{
			ID: "iterate",
			ForEach: &PipelineForEach{
				In: "records",
				Steps: []*PipelineStep{{
					ID: "classify",
					Branch: &PipelineBranch{
						Cases: []*PipelineCase{{
							When:  &Predicate{Field: "field_1", Exists: boolPointer(true)},
							Steps: []*PipelineStep{pipelineTestRun("matched")},
						}},
					},
				}},
			},
		},
	}

	if err, _ := validateTestPipeline(steps); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestForEachOutputRegistersAggregateSchema(t *testing.T) {
	steps := []*PipelineStep{
		pipelineTestRun("discover"),
		pipelineTestExtract("extract", "discover", "routes", `(?P<id>\S+)`, "", nil),
		{
			ID: "iterate",
			ForEach: &PipelineForEach{
				In: "routes",
				Steps: []*PipelineStep{
					pipelineTestRun("child_discover"),
					pipelineTestExtract("child_extract", "child_discover", "child_interfaces", `(?P<interface>\S+)`, "", nil),
				},
				Outputs: []*Output{{
					From:          "child_interfaces",
					Into:          "all_interfaces",
					DeduplicateBy: []string{"interface"},
				}},
			},
		},
		{
			ID: "export",
			Export: &PipelineExport{
				From:    "all_interfaces",
				Field:   "interface",
				As:      "selected_interface",
				Require: "exactly_one",
			},
		},
	}

	err, ctx := validateTestPipeline(steps)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if _, ok := ctx.collectionNames["all_interfaces"]; !ok {
		t.Fatal("aggregate collection name was not registered")
	}
	if _, ok := ctx.pipelineSymbols.collectionFields["all_interfaces"]["interface"]; !ok {
		t.Fatal("aggregate collection schema was not registered")
	}
}

func TestForEachCanReferenceJoinOutput(t *testing.T) {
	steps := []*PipelineStep{
		pipelineTestRun("discover_left"),
		pipelineTestExtract("extract_left", "discover_left", "hosting_npus", `(?P<npu>\S+)`, "", nil),
		pipelineTestRun("discover_right"),
		pipelineTestExtract("extract_right", "discover_right", "acl_databases", `(?P<npu>\S+)\s+(?P<db_id>\S+)`, "", nil),
		{
			ID: "correlate",
			Join: &PipelineJoin{
				Left:   "hosting_npus",
				Right:  "acl_databases",
				On:     []string{"npu"},
				Output: "fia_targets",
			},
		},
		{
			ID: "inspect",
			ForEach: &PipelineForEach{
				In:    "fia_targets",
				Steps: []*PipelineStep{pipelineTestRun("inspect_target")},
			},
		},
	}

	if err, _ := validateTestPipeline(steps); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestForEachOutputValidationRejectsInvalidReferences(t *testing.T) {
	base := func(output []*Output) []*PipelineStep {
		return []*PipelineStep{
			pipelineTestRun("discover"),
			pipelineTestExtract("extract", "discover", "routes", `(?P<id>\S+)`, "", nil),
			{
				ID: "iterate",
				ForEach: &PipelineForEach{
					In: "routes",
					Steps: []*PipelineStep{
						pipelineTestRun("child_discover"),
						pipelineTestExtract("child_extract", "child_discover", "child_interfaces", `(?P<interface>\S+)`, "", nil),
					},
					Outputs: output,
				},
			},
		}
	}

	tests := []struct {
		name  string
		steps []*PipelineStep
	}{
		{
			name: "parent collection used as source",
			steps: base([]*Output{{
				From: "routes", Into: "all_interfaces",
			}}),
		},
		{
			name: "unknown deduplication field",
			steps: base([]*Output{{
				From: "child_interfaces", Into: "all_interfaces", DeduplicateBy: []string{"missing"},
			}}),
		},
		{
			name: "duplicate target",
			steps: base([]*Output{
				{From: "child_interfaces", Into: "all_interfaces"},
				{From: "child_interfaces", Into: "all_interfaces"},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err, _ := validateTestPipeline(tt.steps); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDefaultOnlyBranchDoesNotRequirePrecedingExtraction(t *testing.T) {
	steps := []*PipelineStep{{
		ID: "default_branch",
		Branch: &PipelineBranch{
			Default: []*PipelineStep{pipelineTestRun("default")},
		},
	}}

	if err, _ := validateTestPipeline(steps); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func TestPipelineValidationPoliciesLimitsAndTemplates(t *testing.T) {
	ambiguousProfile := &Commander{
		MainCommandGroup: []*Command{{Cmd: "show platform"}},
		Pipeline:         []*PipelineStep{pipelineTestRun("run")},
	}
	if err := ambiguousProfile.Validate(); err == nil {
		t.Fatal("expected commands and pipeline ambiguity error")
	}

	defaultPolicy := &Commander{Pipeline: []*PipelineStep{pipelineTestRun("run")}}
	if err := defaultPolicy.Validate(); err != nil {
		t.Fatalf("unexpected default-policy validation error: %v", err)
	}
	if got := defaultPolicy.Pipeline[0].OnError; got != "stop_router" {
		t.Fatalf("unexpected default on_error policy: got %q, want %q", got, "stop_router")
	}

	invalidPolicy := &Commander{Pipeline: []*PipelineStep{{
		ID:      "run",
		Run:     &Command{Cmd: "show platform"},
		OnError: "unknown",
	}}}
	if err := invalidPolicy.Validate(); err == nil {
		t.Fatal("expected invalid on_error policy")
	}

	invalidLimits := &Commander{
		Pipeline:       []*PipelineStep{pipelineTestRun("run")},
		PipelineLimits: &PipelineLimits{MaxDepth: 0, MaxCommands: 1},
	}
	if err := invalidLimits.Validate(); err == nil {
		t.Fatal("expected invalid pipeline limits")
	}

	validTemplate := &Command{
		Cmd:             "show cef {{.prefix}}",
		LocationFmtTmpl: "node{{.Slot}}",
	}
	if err := validTemplate.Validate(); err != nil {
		t.Fatalf("unexpected valid-template error: %v", err)
	}

	invalidCommandTemplate := &Command{Cmd: "show {{.prefix"}
	if err := invalidCommandTemplate.Validate(); err == nil {
		t.Fatal("expected invalid command-template error")
	}

	invalidLocationTemplate := &Command{
		Cmd:             "show platform",
		LocationFmtTmpl: "node{{.Slot",
	}
	if err := invalidLocationTemplate.Validate(); err == nil {
		t.Fatal("expected invalid location-template error")
	}
}

func TestCommandTemplatesRejectMissingValuesAtExecution(t *testing.T) {
	command := &Command{
		Cmd:             "show cef {{.prefix}}",
		LocationFmtTmpl: "node{{.Slot}}",
	}
	if err := command.Validate(); err != nil {
		t.Fatalf("unexpected template validation error: %v", err)
	}

	var commandOutput bytes.Buffer
	if err := command.templatedCmd.Execute(&commandOutput, map[string]string{}); err == nil {
		t.Fatal("expected missing command template value to fail")
	}

	locationCommand := &Command{
		Cmd:             "show platform",
		LocationFmtTmpl: "node{{.Missing}}",
	}
	if err := locationCommand.Validate(); err != nil {
		t.Fatalf("unexpected location template validation error: %v", err)
	}

	var locationOutput bytes.Buffer
	if err := locationCommand.templatedLocation.Execute(&locationOutput, struct{ Slot int }{Slot: 1}); err == nil {
		t.Fatal("expected missing location template value to fail")
	}
}

func TestCommandRenderUsesOnePathForStaticAndDynamicCommands(t *testing.T) {
	staticCommand := &Command{Cmd: "show platform"}
	if err := staticCommand.Validate(); err != nil {
		t.Fatalf("unexpected static command validation error: %v", err)
	}
	if got, err := staticCommand.Render(nil); err != nil || got != "show platform" {
		t.Fatalf("static command render = %q, %v; want %q", got, err, "show platform")
	}

	dynamicCommand := &Command{Cmd: "show cef {{.prefix}}"}
	if err := dynamicCommand.Validate(); err != nil {
		t.Fatalf("unexpected dynamic command validation error: %v", err)
	}
	got, err := dynamicCommand.Render(map[string]string{"prefix": "10.0.0.0/24"})
	if err != nil {
		t.Fatalf("dynamic command render failed: %v", err)
	}
	if got != "show cef 10.0.0.0/24" {
		t.Fatalf("dynamic command render = %q; want %q", got, "show cef 10.0.0.0/24")
	}
}

func TestValidateRenderedCommand(t *testing.T) {
	command := &Command{Cmd: "show platform"}
	tests := []struct {
		name     string
		rendered string
		wantErr  bool
	}{
		{name: "valid", rendered: "show platform"},
		{name: "empty", rendered: "   ", wantErr: true},
		{name: "newline", rendered: "show platform\nshow version", wantErr: true},
		{name: "nul", rendered: "show platform\x00", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := command.ValidateRendered(tt.rendered); (err != nil) != tt.wantErr {
				t.Fatalf("ValidateRendered(%q) error = %v, wantErr %v", tt.rendered, err, tt.wantErr)
			}
		})
	}
}
