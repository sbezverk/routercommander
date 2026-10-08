package pipeline

import (
	"reflect"
	"testing"

	"github.com/sbezverk/routercommander/pkg/types"
)

func compiledPipelinePattern(t *testing.T, expression string) *types.Pattern {
	t.Helper()
	pattern := &types.Pattern{PatternString: expression}
	if err := pattern.Compile(); err != nil {
		t.Fatalf("failed to compile pattern %q: %v", expression, err)
	}
	return pattern
}

func stringValue(value string) *string {
	return &value
}

func boolValue(value bool) *bool {
	return &value
}

func TestExtractRecordsPreservesContextAcrossLinesAndFinalLine(t *testing.T) {
	ctx := &types.RunContext{
		MaxRecords: types.DefaultMaxRecords,
		StepResults: map[string][]types.StepResult{
			"discover": {{
				Location: "0/RP0/CPU0",
				Output:   []byte("timestamp\nVRF: NMNET\n10.0.0.0/24 10.0.0.1\n10.0.1.0/24 10.0.1.1"),
			}},
		},
		Collections: map[string][]types.Record{},
	}
	extraction := &types.PipelineExtract{
		FromStepID: "discover",
		Context: []*types.Context{{
			Pattern: compiledPipelinePattern(t, `^VRF:\s+(?P<vrf>\S+)$`),
		}},
		RecordSpec: &types.RecordSpec{
			Name:    "routes",
			Pattern: compiledPipelinePattern(t, `^(?P<prefix>[0-9.]+/\d+)\s+(?P<next_hop>\S+)$`),
			Inherit: []string{"vrf"},
		},
	}

	records, err := extractRecords(ctx, extraction)
	if err != nil {
		t.Fatalf("extractRecords failed: %v", err)
	}
	want := []types.Record{
		{"vrf": "NMNET", "prefix": "10.0.0.0/24", "next_hop": "10.0.0.1"},
		{"vrf": "NMNET", "prefix": "10.0.1.0/24", "next_hop": "10.0.1.1"},
	}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %#v, want %#v", records, want)
	}
}

func TestExtractRecordsResetsContextBetweenResults(t *testing.T) {
	ctx := &types.RunContext{
		MaxRecords: types.DefaultMaxRecords,
		StepResults: map[string][]types.StepResult{
			"discover": {
				{Output: []byte("VRF: NMNET\n10.0.0.0/24")},
				{Location: "0/1/CPU0", Output: []byte("10.0.1.0/24")},
			},
		},
		Collections: map[string][]types.Record{},
	}
	extraction := &types.PipelineExtract{
		FromStepID: "discover",
		Context: []*types.Context{{
			Pattern: compiledPipelinePattern(t, `^VRF:\s+(?P<vrf>\S+)$`),
		}},
		RecordSpec: &types.RecordSpec{
			Name:    "routes",
			Pattern: compiledPipelinePattern(t, `^(?P<prefix>\S+)$`),
			Inherit: []string{"vrf"},
		},
	}

	if _, err := extractRecords(ctx, extraction); err == nil {
		t.Fatal("expected missing context error for the second result")
	}
}

func TestExtractRecordsFiltersDeduplicatesAndLimits(t *testing.T) {
	ctx := &types.RunContext{
		MaxRecords: types.DefaultMaxRecords,
		StepResults: map[string][]types.StepResult{
			"discover": {{Output: []byte(
				"VRF: NMNET\n" +
					"10.0.0.0/24 nh1\n" +
					"10.0.0.0/24 nh1\n" +
					"10.0.1.0/24 nh2\n" +
					"10.0.2.0/24 nh3\n")}},
		},
		Collections: map[string][]types.Record{},
	}
	extraction := &types.PipelineExtract{
		FromStepID: "discover",
		Context: []*types.Context{{
			Pattern: compiledPipelinePattern(t, `^VRF:\s+(?P<vrf>\S+)$`),
		}},
		RecordSpec: &types.RecordSpec{
			Name:          "routes",
			Pattern:       compiledPipelinePattern(t, `^(?P<prefix>\S+)\s+(?P<next_hop>\S+)$`),
			Inherit:       []string{"vrf"},
			Where:         []*types.Predicate{{Field: "prefix", Contains: stringValue("10.0.")}},
			DeduplicateBy: []string{"vrf", "prefix", "next_hop"},
			MaxRecords:    2,
		},
	}

	records, err := extractRecords(ctx, extraction)
	if err != nil {
		t.Fatalf("extractRecords failed: %v", err)
	}
	want := []types.Record{
		{"vrf": "NMNET", "prefix": "10.0.0.0/24", "next_hop": "nh1"},
		{"vrf": "NMNET", "prefix": "10.0.1.0/24", "next_hop": "nh2"},
	}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %#v, want %#v", records, want)
	}
	if got := ctx.Collections["routes"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored collection = %#v, want %#v", got, want)
	}
}

func TestExtractRecordsUsesGlobalLimitAndClampsLocalLimit(t *testing.T) {
	tests := []struct {
		name        string
		globalLimit int
		localLimit  int
		want        int
	}{
		{name: "zero local inherits global", globalLimit: 3, localLimit: 0, want: 3},
		{name: "local limit is capped by global", globalLimit: 2, localLimit: 5, want: 2},
		{name: "local limit is tighter", globalLimit: 5, localLimit: 2, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &types.RunContext{
				MaxRecords: tt.globalLimit,
				StepResults: map[string][]types.StepResult{
					"discover": {{Output: []byte("one\ntwo\nthree\nfour\n")}},
				},
				Collections: map[string][]types.Record{},
			}
			extraction := &types.PipelineExtract{
				FromStepID: "discover",
				RecordSpec: &types.RecordSpec{
					Name:       "values",
					Pattern:    compiledPipelinePattern(t, `^(?P<value>\S+)$`),
					MaxRecords: tt.localLimit,
				},
			}

			records, err := extractRecords(ctx, extraction)
			if err != nil {
				t.Fatalf("extractRecords() error = %v", err)
			}
			if got := len(records); got != tt.want {
				t.Fatalf("record count = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExtractRecordsStoresEmptyCollection(t *testing.T) {
	ctx := &types.RunContext{
		StepResults: map[string][]types.StepResult{
			"discover": {{Output: []byte("no matching records")}},
		},
	}
	extraction := &types.PipelineExtract{
		FromStepID: "discover",
		RecordSpec: &types.RecordSpec{
			Name:    "routes",
			Pattern: compiledPipelinePattern(t, `^(?P<prefix>\d+)$`),
		},
	}

	records, err := extractRecords(ctx, extraction)
	if err != nil {
		t.Fatalf("extractRecords failed: %v", err)
	}
	if records == nil || len(records) != 0 {
		t.Fatalf("records = %#v, want an empty non-nil slice", records)
	}
	if stored, ok := ctx.Collections["routes"]; !ok || len(stored) != 0 {
		t.Fatalf("empty collection was not stored: %#v, present=%t", stored, ok)
	}
}

func TestEvaluatePredicateOperators(t *testing.T) {
	record := types.Record{
		"vrf":    "NMNET",
		"prefix": "10.0.0.0/24",
		"state":  "unresolved",
		"empty":  "",
	}
	tests := []struct {
		name      string
		predicate *types.Predicate
		want      bool
	}{
		{name: "equals", predicate: &types.Predicate{Field: "vrf", Equals: stringValue("NMNET")}, want: true},
		{name: "not equals", predicate: &types.Predicate{Field: "vrf", NotEquals: stringValue("GI")}, want: true},
		{name: "contains", predicate: &types.Predicate{Field: "prefix", Contains: stringValue("10.")}, want: true},
		{name: "matches", predicate: &types.Predicate{Field: "prefix", Matches: stringValue(`^10\.`)}, want: true},
		{name: "in", predicate: &types.Predicate{Field: "vrf", In: []string{"GI", "NMNET"}}, want: true},
		{name: "exists true", predicate: &types.Predicate{Field: "empty", Exists: boolValue(true)}, want: true},
		{name: "exists false", predicate: &types.Predicate{Field: "missing", Exists: boolValue(false)}, want: true},
		{name: "missing equals", predicate: &types.Predicate{Field: "missing", Equals: stringValue("anything")}, want: false},
		{name: "missing not equals", predicate: &types.Predicate{Field: "missing", NotEquals: stringValue("anything")}, want: false},
		{
			name: "all",
			predicate: &types.Predicate{All: []*types.Predicate{
				{Field: "vrf", Equals: stringValue("NMNET")},
				{Field: "state", Equals: stringValue("unresolved")},
			}},
			want: true,
		},
		{
			name: "any",
			predicate: &types.Predicate{Any: []*types.Predicate{
				{Field: "vrf", Equals: stringValue("GI")},
				{Field: "state", Equals: stringValue("unresolved")},
			}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluatePredicate(record, tt.predicate)
			if err != nil {
				t.Fatalf("evaluatePredicate failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("evaluatePredicate = %t, want %t", got, tt.want)
			}
		})
	}
}
