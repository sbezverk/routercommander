package pipeline

import (
	"fmt"

	"github.com/sbezverk/routercommander/pkg/types"
)

func executeExport(ctx *types.RunContext, spec *types.PipelineExport) error {
	if ctx == nil {
		return fmt.Errorf("run context is nil")
	}
	if ctx.InRecordScope {
		return fmt.Errorf("cannot execute export in record scope")
	}
	if spec == nil {
		return fmt.Errorf("export spec is nil")
	}
	coll, err := checkForCollection(ctx, spec.From)
	if err != nil {
		return fmt.Errorf("export, collection %q error: %w", spec.From, err)
	}
	if types.IsReservedPipelineRenderField(spec.Field) {
		return fmt.Errorf("export, field %q cannot reference reserved field", spec.Field)
	}
	if types.IsReservedPipelineRenderField(spec.As) {
		return fmt.Errorf("export, variable %q cannot reference reserved field", spec.As)
	}
	value, ok := getFieldValue(coll, spec.Field)
	if !ok {
		return fmt.Errorf("export, field %q not found or empty in collection %q", spec.Field, spec.From)
	}
	_, exists := ctx.Variables[spec.As]
	if exists && !spec.Overwrite {
		return fmt.Errorf("export, variable %q already exists and overwrite is not allowed", spec.As)
	}
	ctx.Variables[spec.As] = value

	return nil
}

func getFieldValue(coll []types.Record, field string) (string, bool) {
	count := 0
	returnValue := ""
	for _, record := range coll {
		value, ok := record[field]
		if !ok {
			continue
		}
		count++
		if count == 1 {
			returnValue = value
		}
	}

	if count == 1 {
		return returnValue, true
	}

	return "", false
}
