package types

import (
	"fmt"
	"io"
	"os"
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
					if isReservedPipelineRenderField(field) {
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
				if isReservedPipelineRenderField(field) {
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
			found := false
			if len(extractSteps) > 0 {
				for y := 0; y < len(extractSteps); y++ {
					if step.ForEach.In == pSteps[extractSteps[y]].Extract.RecordSpec.Name {
						found = true
						break
					}
				}
			}
			if !found {
				return fmt.Errorf("ForEach step 'in' field references a non-existent record collection: %s", step.ForEach.In), nil
			}
			child := childContext(ctx)
			for field := range ctx.pipelineSymbols.collectionFields[step.ForEach.In] {
				child.pipelineSymbols.contextFields[field] = struct{}{}
			}
			if err, s := doPipelineStepValidation(step.ForEach.Steps, child); err != nil {
				return fmt.Errorf("ForEach step validation failed: %v", err), nil
			} else {
				step.ForEach.Steps = s
			}
		}
		if step.Branch != nil {
			if len(step.Branch.Cases) == 0 && len(step.Branch.Default) == 0 {
				return fmt.Errorf("Branch step must have at least one case or a default branch"), nil
			}
			for _, c := range step.Branch.Cases {
				if len(c.Steps) == 0 {
					continue
				}
				child := childContext(ctx)
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
	}
	return nil, pSteps
}

func childContext(parent *pipelineValidationContext) *pipelineValidationContext {
	child := *parent

	child.pipelineSymbols.contextFields = make(map[string]struct{})
	for field := range parent.pipelineSymbols.contextFields {
		child.pipelineSymbols.contextFields[field] = struct{}{}
	}

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

func isReservedPipelineRenderField(field string) bool {
	switch field {
	case "RouterName", "Location":
		return true
	default:
		return false
	}
}
