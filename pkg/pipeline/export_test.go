package pipeline

import (
	"strings"
	"testing"

	"github.com/sbezverk/routercommander/pkg/types"
)

func exportTestContext() *types.RunContext {
	return &types.RunContext{
		Variables:   map[string]string{},
		Collections: map[string][]types.Record{},
	}
}

func TestExecuteExportExactlyOneRequestedField(t *testing.T) {
	tests := []struct {
		name      string
		records   []types.Record
		wantValue string
		wantErr   bool
	}{
		{
			name: "one matching field among records with other fields",
			records: []types.Record{
				{"npu": "0", "interface": "Te0"},
				{"db_id": "42"},
			},
			wantValue: "0",
		},
		{
			name: "no matching field",
			records: []types.Record{
				{"db_id": "42"},
			},
			wantErr: true,
		},
		{
			name: "multiple matching fields with different values",
			records: []types.Record{
				{"npu": "0"},
				{"npu": "1"},
			},
			wantErr: true,
		},
		{
			name: "multiple matching fields with identical values",
			records: []types.Record{
				{"npu": "0"},
				{"npu": "0"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := exportTestContext()
			ctx.Collections["targets"] = tt.records

			err := executeExport(ctx, &types.PipelineExport{
				From:    "targets",
				Field:   "npu",
				As:      "hosting_npu",
				Require: "exactly_one",
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("executeExport() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got := ctx.Variables["hosting_npu"]; got != tt.wantValue {
				t.Fatalf("exported value = %q, want %q", got, tt.wantValue)
			}
		})
	}
}

func TestExecuteExportRejectsMissingOrEmptyCollections(t *testing.T) {
	tests := []struct {
		name       string
		collection map[string][]types.Record
		wantText   string
	}{
		{
			name:     "missing collection",
			wantText: "not found",
		},
		{
			name: "nil collection",
			collection: map[string][]types.Record{
				"targets": nil,
			},
			wantText: "is nil",
		},
		{
			name: "empty collection",
			collection: map[string][]types.Record{
				"targets": {},
			},
			wantText: "is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := exportTestContext()
			ctx.Collections = tt.collection
			err := executeExport(ctx, &types.PipelineExport{
				From:    "targets",
				Field:   "npu",
				As:      "hosting_npu",
				Require: "exactly_one",
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("executeExport() error = %v, want text %q", err, tt.wantText)
			}
		})
	}
}

func TestExecuteExportEnforcesScopeReservedNamesAndOverwrite(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*types.RunContext, *types.PipelineExport)
		wantText string
	}{
		{
			name: "record scope",
			mutate: func(ctx *types.RunContext, _ *types.PipelineExport) {
				ctx.InRecordScope = true
			},
			wantText: "record scope",
		},
		{
			name: "reserved source field",
			mutate: func(_ *types.RunContext, spec *types.PipelineExport) {
				spec.Field = "Location"
			},
			wantText: "reserved field",
		},
		{
			name: "reserved destination",
			mutate: func(_ *types.RunContext, spec *types.PipelineExport) {
				spec.As = "RouterName"
			},
			wantText: "reserved field",
		},
		{
			name: "existing destination without overwrite",
			mutate: func(ctx *types.RunContext, _ *types.PipelineExport) {
				ctx.Variables["hosting_npu"] = "old"
			},
			wantText: "overwrite is not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := exportTestContext()
			ctx.Collections["targets"] = []types.Record{{"npu": "0"}}
			spec := &types.PipelineExport{
				From:    "targets",
				Field:   "npu",
				As:      "hosting_npu",
				Require: "exactly_one",
			}
			tt.mutate(ctx, spec)

			err := executeExport(ctx, spec)
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("executeExport() error = %v, want text %q", err, tt.wantText)
			}
		})
	}
}

func TestExecuteExportAllowsExplicitOverwrite(t *testing.T) {
	ctx := exportTestContext()
	ctx.Collections["targets"] = []types.Record{{"npu": "0"}}
	ctx.Variables["hosting_npu"] = "old"

	err := executeExport(ctx, &types.PipelineExport{
		From:      "targets",
		Field:     "npu",
		As:        "hosting_npu",
		Require:   "exactly_one",
		Overwrite: true,
	})
	if err != nil {
		t.Fatalf("executeExport() unexpected error: %v", err)
	}
	if got := ctx.Variables["hosting_npu"]; got != "0" {
		t.Fatalf("overwritten value = %q, want %q", got, "0")
	}
}
