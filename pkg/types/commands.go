package types

import (
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

func readCommandFile(fn string) ([]byte, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, fmt.Errorf("fail to open file %s with error: %+v", fn, err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("fail to read file %s with error: %+v", fn, err)
	}
	return b, nil
}

func parseCommandFile(b []byte) (*Commander, error) {
	c := &Commander{}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("fail to unmarshal commands yaml with error: %+v", err)
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	return c, nil

}

func GetCommands(fn string) (*Commander, error) {
	b, err := readCommandFile(fn)
	if err != nil {
		return nil, err
	}

	return parseCommandFile(b)
}

func validatePipeline(steps []*PipelineStep, ctx *pipelineValidationContext) (error, []*PipelineStep) {
	if len(steps) == 0 {
		return fmt.Errorf("pipeline is empty"), nil
	}
	if ctx == nil {
		return fmt.Errorf("pipeline validation context is nil"), nil
	}
	var err error
	pSteps := make([]*PipelineStep, 0, len(steps))
	if err, pSteps = doPipelineStepValidation(steps, ctx); err != nil {
		return err, nil
	}

	return nil, pSteps
}

func doPipelineStepValidation(steps []*PipelineStep, ctx *pipelineValidationContext) (error, []*PipelineStep) {
	if len(steps) == 0 {
		return fmt.Errorf("pipeline is empty"), nil
	}
	if ctx.levels+1 > ctx.maxPipelineDepth {
		return fmt.Errorf("pipeline exceeds maximum allowed depth of %d", ctx.maxPipelineDepth), nil
	}
	ctx.levels++
	defer func() {
		ctx.levels--
	}()
	pSteps := make([]*PipelineStep, 0, len(steps))
	// Syntactical validation
	for _, step := range steps {
		if step == nil {
			return fmt.Errorf("pipeline step is nil"), nil
		}
		if err := step.Validate(); err != nil {
			return fmt.Errorf("pipeline step validation failed: %v", err), nil
		}
		if _, ok := ctx.stepIDs[step.ID]; ok {
			return fmt.Errorf("duplicate pipeline step ID: %s", step.ID), nil
		}
		ctx.stepIDs[step.ID] = struct{}{}
		pSteps = append(pSteps, step)
	}
	// Semantical validation
	runSteps := make([]int, 0, len(pSteps))
	extractSteps := make([]int, 0, len(pSteps))
	for i := 0; i < len(pSteps); i++ {
		step := pSteps[i]
		if step.Run != nil {
			runSteps = append(runSteps, i)
			continue
		}
		if step.Extract != nil {
			step.Extract.FromStepID = strings.TrimSpace(step.Extract.FromStepID)
			if step.Extract.FromStepID == "" {
				return fmt.Errorf("Extract step 'from_step_id' cannot be empty"), nil
			}
			found := false
			if len(runSteps) > 0 {
				for y := 0; y < len(runSteps); y++ {
					if step.Extract.FromStepID == pSteps[runSteps[y]].ID {
						found = true
						break
					}
				}
			}
			if !found {
				return fmt.Errorf("Extract step 'from_step_id' references a non-existent run step: %s", step.Extract.FromStepID), nil
			}
			// No need to check if RecordSpec for nil, as previous syntactical validation ensures it is not nil.
			for contextIndex, context := range step.Extract.Context {
				fields, err := namedCaptureFields(context.Pattern, fmt.Sprintf("extract step %q context %d", step.ID, contextIndex))
				if err != nil {
					return err, nil
				}
				for field := range fields {
					if IsReservedPipelineRenderField(field) {
						return fmt.Errorf("extract step %q context field %q conflicts with reserved render field", step.ID, field), nil
					}
					ctx.pipelineSymbols.contextFields[field] = struct{}{}
				}
			}
			if _, ok := ctx.collectionNames[step.Extract.RecordSpec.Name]; ok {
				return fmt.Errorf("duplicate record collection name: %s", step.Extract.RecordSpec.Name), nil
			} else {
				ctx.collectionNames[step.Extract.RecordSpec.Name] = struct{}{}
			}
			collectionFields := ctx.pipelineSymbols.collectionFields[step.Extract.RecordSpec.Name]
			if collectionFields == nil {
				collectionFields = make(map[string]struct{})
				ctx.pipelineSymbols.collectionFields[step.Extract.RecordSpec.Name] = collectionFields
			}
			recordFields, err := namedCaptureFields(step.Extract.RecordSpec.Pattern, fmt.Sprintf("extract step %q record pattern", step.ID))
			if err != nil {
				return err, nil
			}
			for field := range recordFields {
				if IsReservedPipelineRenderField(field) {
					return fmt.Errorf("extract step %q record field %q conflicts with reserved render field", step.ID, field), nil
				}
				if _, ok := ctx.pipelineSymbols.contextFields[field]; ok {
					return fmt.Errorf("extract step %q record field %q conflicts with a context field", step.ID, field), nil
				}
				collectionFields[field] = struct{}{}
			}
			for _, inherit := range step.Extract.RecordSpec.Inherit {
				if _, ok := ctx.pipelineSymbols.contextFields[inherit]; !ok {
					return fmt.Errorf("Inherited context field '%s' is not defined in the pipeline context", inherit), nil
				}
				if _, ok := recordFields[inherit]; ok {
					return fmt.Errorf("extract step %q record field %q conflicts with an inherited context field", step.ID, inherit), nil
				}
				collectionFields[inherit] = struct{}{}
			}
			extractSteps = append(extractSteps, i)
		}
		if step.ForEach != nil {
			step.ForEach.In = strings.TrimSpace(step.ForEach.In)
			if step.ForEach.In == "" {
				return fmt.Errorf("ForEach step 'in' field cannot be empty"), nil
			}
			// A ForEach source may be created by Extract, Join, or a
			// ForEach output published by an inner scope. All are registered
			// in collectionNames as soon as they become visible in this scope.
			if _, found := ctx.collectionNames[step.ForEach.In]; !found {
				return fmt.Errorf("ForEach step 'in' field references a non-existent record collection: %s", step.ForEach.In), nil
			}
			child := childContext(ctx)
			for field := range ctx.pipelineSymbols.collectionFields[step.ForEach.In] {
				child.pipelineSymbols.contextFields[field] = struct{}{}
			}
			child.inForEachStep = true
			if err, s := doPipelineStepValidation(step.ForEach.Steps, child); err != nil {
				return fmt.Errorf("ForEach step validation failed: %v", err), nil
			} else {
				step.ForEach.Steps = s
			}
			if step.ForEach.Outputs != nil {
				uniqueInto := make(map[string]struct{})
				for i, output := range step.ForEach.Outputs {
					if _, ok := uniqueInto[output.Into]; ok {
						return fmt.Errorf("ForEach step 'output.into' at index %d references a duplicate collection: %s", i, output.Into), nil
					}
					if _, ok := ctx.collectionNames[output.Into]; ok {
						return fmt.Errorf("ForEach step 'output.into' at index %d references an existing collection: %s", i, output.Into), nil
					}
					if IsReservedPipelineRenderField(output.Into) {
						return fmt.Errorf("ForEach step 'output.into' at index %d references a reserved field: %s", i, output.Into), nil
					}
					if _, ok := ctx.collectionNames[output.From]; ok {
						return fmt.Errorf("ForEach step 'output.from' at index %d must reference a child-local collection: %s", i, output.From), nil
					}
					if _, ok := child.collectionNames[output.From]; !ok {
						return fmt.Errorf("ForEach step 'output.from' at index %d references a non-existent collection: %s", i, output.From), nil
					}
					from := child.pipelineSymbols.collectionFields[output.From]
					if from == nil {
						return fmt.Errorf("ForEach step 'output.from' at index %d has no known schema: %s", i, output.From), nil
					}
					for _, dedup := range output.DeduplicateBy {
						if _, ok := from[dedup]; !ok {
							return fmt.Errorf("ForEach step 'output.deduplicate_by' at index %d contains a non-existent field: %s", i, dedup), nil
						}
					}
					uniqueInto[output.Into] = struct{}{}
				}
				// Publish the aggregate symbols only after every output declaration
				// has passed validation.
				for _, output := range step.ForEach.Outputs {
					ctx.collectionNames[output.Into] = struct{}{}
					ctx.pipelineSymbols.collectionFields[output.Into] = maps.Clone(child.pipelineSymbols.collectionFields[output.From])
				}
			}
		}
		if step.Branch != nil {
			if len(step.Branch.Cases) == 0 && len(step.Branch.Default) == 0 {
				return fmt.Errorf("Branch step must have at least one case or a default branch"), nil
			}
			for caseIndex, c := range step.Branch.Cases {
				if len(c.Steps) == 0 {
					continue
				}
				if c.When == nil {
					return fmt.Errorf("Branch case must have a 'when' condition"), nil
				}
				child := childContext(ctx)
				fieldsStore := mergeFieldsStores(ctx.pipelineSymbols.contextFields, child.pipelineSymbols.variableFields)
				if err := validatePredicateFields(fieldsStore, c.When, ""); err != nil {
					return fmt.Errorf("branch step %q case %d predicate validation failed: %w", step.ID, caseIndex, err), nil
				}
				if err, s := doPipelineStepValidation(c.Steps, child); err != nil {
					return fmt.Errorf("Branch case validation failed: %v", err), nil
				} else {
					c.Steps = s
				}
			}
			child := childContext(ctx)
			if len(step.Branch.Default) > 0 {
				if err, s := doPipelineStepValidation(step.Branch.Default, child); err != nil {
					return fmt.Errorf("Branch default validation failed: %v", err), nil
				} else {
					step.Branch.Default = s
				}
			}
		}
		if step.Export != nil {
			if ctx.inForEachStep {
				return fmt.Errorf("Export step cannot be used inside a ForEach step"), nil
			}
			coll, ok := ctx.pipelineSymbols.collectionFields[step.Export.From]
			if !ok {
				return fmt.Errorf("Export step 'from' field references a non-existent collection: %s", step.Export.From), nil
			}
			if _, ok := coll[step.Export.Field]; !ok {
				return fmt.Errorf("Export step 'field' references a non-existent field in collection: %s", step.Export.Field), nil
			}
			if IsReservedPipelineRenderField(step.Export.As) {
				return fmt.Errorf("Export step 'as' field cannot reference reserved field: %s", step.Export.As), nil
			}
			if _, ok := ctx.pipelineSymbols.variableFields[step.Export.As]; ok {
				if !step.Export.Overwrite {
					return fmt.Errorf("Export step 'as' field references an existing variable: %s and 'overwrite' is not set to true", step.Export.As), nil
				}
			} else {
				ctx.pipelineSymbols.variableFields[step.Export.As] = struct{}{}
			}
		}
		if step.Join != nil {
			if ctx.inForEachStep {
				return fmt.Errorf("Join step cannot be used inside a ForEach step"), nil
			}
			if _, ok := ctx.pipelineSymbols.collectionFields[step.Join.Output]; ok {
				return fmt.Errorf("Join step 'output' field references an existing collection: %s", step.Join.Output), nil
			}
			rightColl, ok := ctx.pipelineSymbols.collectionFields[step.Join.Right]
			if !ok {
				return fmt.Errorf("Join step 'right' field references a non-existent collection: %s", step.Join.Right), nil
			}
			leftColl, ok := ctx.pipelineSymbols.collectionFields[step.Join.Left]
			if !ok {
				return fmt.Errorf("Join step 'left' field references a non-existent collection: %s", step.Join.Left), nil
			}
			if _, ok := ctx.collectionNames[step.Join.Output]; ok {
				return fmt.Errorf("Join step 'output' field references an existing collection name: %s", step.Join.Output), nil
			}
			for _, on := range step.Join.On {
				if _, ok := leftColl[on]; !ok {
					return fmt.Errorf("Join step 'on' field %q not found in left collection: %s", on, step.Join.Left), nil
				}
				if _, ok := rightColl[on]; !ok {
					return fmt.Errorf("Join step 'on' field %q not found in right collection: %s", on, step.Join.Right), nil
				}
			}
			for k := range rightColl {
				if slices.Contains(step.Join.On, k) {
					continue
				}
				if _, ok := leftColl[k]; ok {
					return fmt.Errorf("Join step 'on' field collision: non-key field %q exists in both left and right collections", k), nil
				}
			}
			for k := range leftColl {
				if slices.Contains(step.Join.On, k) {
					continue
				}
				if _, ok := rightColl[k]; ok {
					return fmt.Errorf("Join step 'on' field collision: non-key field %q exists in both left and right collections", k), nil
				}
			}
			ctx.pipelineSymbols.collectionFields[step.Join.Output] = make(map[string]struct{})
			for _, on := range step.Join.On {
				if IsReservedPipelineRenderField(on) {
					return fmt.Errorf("Join step 'on' field cannot reference reserved field: %s", on), nil
				}
				ctx.pipelineSymbols.collectionFields[step.Join.Output][on] = struct{}{}
			}
			mergeRecords(leftColl, rightColl, step.Join.On, ctx.pipelineSymbols.collectionFields[step.Join.Output])
			ctx.collectionNames[step.Join.Output] = struct{}{}
		}
	}
	return nil, pSteps
}

func mergeRecords(left, right map[string]struct{}, skip []string, result map[string]struct{}) {
	if result == nil {
		result = make(map[string]struct{})
	}
	skipSet := make(map[string]struct{}, len(skip))
	for _, field := range skip {
		skipSet[field] = struct{}{}
	}
	for k := range left {
		if _, shouldSkip := skipSet[k]; shouldSkip {
			continue
		}
		result[k] = struct{}{}
	}
	for k := range right {
		if _, shouldSkip := skipSet[k]; shouldSkip {
			continue
		}
		result[k] = struct{}{}
	}
}

func childContext(parent *pipelineValidationContext) *pipelineValidationContext {
	child := *parent
	child.pipelineSymbols.variableFields = maps.Clone(parent.pipelineSymbols.variableFields)
	child.pipelineSymbols.contextFields = maps.Clone(parent.pipelineSymbols.contextFields)
	child.pipelineSymbols.collectionFields = maps.Clone(parent.pipelineSymbols.collectionFields)
	child.collectionNames = maps.Clone(parent.collectionNames)
	return &child
}

func namedCaptureFields(pattern *Pattern, description string) (map[string]struct{}, error) {
	if pattern == nil || pattern.RegExp == nil {
		return nil, fmt.Errorf("%s has no compiled regular expression", description)
	}

	fields := make(map[string]struct{})
	for _, field := range pattern.RegExp.SubexpNames() {
		if field == "" {
			continue
		}
		if _, exists := fields[field]; exists {
			return nil, fmt.Errorf("%s contains duplicate named capture %q", description, field)
		}
		fields[field] = struct{}{}
	}

	return fields, nil
}

func IsReservedPipelineRenderField(field string) bool {
	switch field {
	case "RouterName", "Location":
		return true
	default:
		return false
	}
}

func validatePredicateFields(symbols map[string]struct{}, predicate *Predicate, path string) error {
	if predicate == nil {
		return fmt.Errorf("predicate is nil")
	}
	if predicate.All != nil {
		if len(predicate.All) == 0 {
			return fmt.Errorf("all predicate has no children")
		}
		for i, child := range predicate.All {
			childPath := predicateChildPath(path, "all", i)
			if err := validatePredicateFields(symbols, child, childPath); err != nil {
				return err
			}
		}
		return nil
	}
	if predicate.Any != nil {
		if len(predicate.Any) == 0 {
			return fmt.Errorf("any predicate has no children")
		}
		for i, child := range predicate.Any {
			childPath := predicateChildPath(path, "any", i)
			if err := validatePredicateFields(symbols, child, childPath); err != nil {
				return err
			}
		}
		return nil
	}

	if _, ok := symbols[predicate.Field]; !ok {
		if path == "" {
			path = "root"
		}
		return fmt.Errorf("%w: predicate %q references unavailable field %q", ErrPipelineFieldNotFound, path, predicate.Field)
	}
	return nil
}

func predicateChildPath(parent, operator string, index int) string {
	child := fmt.Sprintf("%s[%d]", operator, index)
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func mergeFieldsStores(stores ...map[string]struct{}) map[string]struct{} {
	merged := make(map[string]struct{})
	for _, store := range stores {
		for k := range store {
			merged[k] = struct{}{}
		}
	}
	return merged
}
