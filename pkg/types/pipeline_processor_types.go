package types

import (
	"fmt"
	"maps"
	"regexp"
	"strings"
)

const (
	DefaultMaxPipelineDepth    = 8
	MaximumPipelineDepth       = 10
	DefaultMaxPipelineCommands = 5000
	MaximumPipelineCommands    = 60000
	DefaultMaxRecords          = 500
	MaximumRecords             = 1000
)

const (
	OnErrorTypeContinueRecord = "continue_record"
	OnErrorTypeStopRouter     = "stop_router"
)

// Record is one extracted item, represented as field names and string values.
type Record map[string]string

type StepResult struct {
	Output   []byte
	Command  string
	Location string
}

// RunContext contains the per-router state shared while a pipeline executes.
type RunContext struct {
	Variables     map[string]string       // Scalar values available to templates.
	Collections   map[string][]Record     // Extracted records keyed by collection name.
	StepResults   map[string][]StepResult // Results of executed steps.
	Current       Record                  // Record currently processed by a for-each step.
	InRecordScope bool                    // True while executing steps for one for-each record.
	RouterName    string                  // Router owning this pipeline invocation.
	Depth         int                     // Current recursive step depth.
	StepPath      []string                // IDs of nested steps currently executing.
	MaxDepth      int                     // Maximum permitted recursive depth.
	CommandsRun   *int                    // Number of generated commands executed so far.
	MaxCommands   int                     // Maximum permitted generated commands.
	MaxRecords    int                     // Maximum permitted records in a collection.
}

func (ctx *RunContext) ChildForRecord(record Record) *RunContext {
	child := *ctx

	child.Variables = maps.Clone(ctx.Variables)
	child.Current = maps.Clone(record)
	child.InRecordScope = true
	child.Collections = maps.Clone(ctx.Collections)
	child.StepPath = append([]string(nil), ctx.StepPath...)
	child.StepResults = maps.Clone(ctx.StepResults)

	return &child
}

// Context describes a pattern that updates current context values while
// command output is scanned, such as the current VRF.
type Context struct {
	Pattern *Pattern `yaml:"pattern"` // Named captures become context variables.
}

func (c *Context) Validate() error {
	if c == nil {
		return fmt.Errorf("Context is nil")
	}
	if c.Pattern == nil {
		return fmt.Errorf("Context 'pattern' field cannot be nil")
	}
	if err := c.Pattern.Compile(); err != nil {
		return fmt.Errorf("Context 'pattern' field validation failed: %v", err)
	}

	return nil
}

// RecordSpec is the YAML `records` block describing how to create a named
// collection of records. It is an extraction specification, not the resulting
// record data; resulting data is stored in RunContext.Collections.
type RecordSpec struct {
	Name          string       `yaml:"name"`                  // Key used in RunContext.Collections.
	Pattern       *Pattern     `yaml:"pattern"`               // Named-capture expression for one record.
	Inherit       []string     `yaml:"inherit"`               // Context variable names copied into each record.
	DeduplicateBy []string     `yaml:"deduplicate_by"`        // Fields that define record uniqueness.
	Where         []*Predicate `yaml:"where,omitempty"`       // Predicates applied after extraction.
	MaxRecords    int          `yaml:"max_records,omitempty"` // Maximum records retained.
}

func (r *RecordSpec) Validate() error {
	if r == nil {
		return fmt.Errorf("RecordSpec is nil")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("Records 'name' field cannot be empty")
	}
	if r.Pattern == nil {
		return fmt.Errorf("Records 'pattern' field cannot be nil")
	}
	if err := r.Pattern.Compile(); err != nil {
		return fmt.Errorf("Records 'pattern' field validation failed: %v", err)
	}
	for i, inherit := range r.Inherit {
		if strings.TrimSpace(inherit) == "" {
			return fmt.Errorf("Records 'inherit' field contains an empty string at index %d", i)
		}
		r.Inherit[i] = strings.TrimSpace(inherit)
	}
	for i, dedup := range r.DeduplicateBy {
		if strings.TrimSpace(dedup) == "" {
			return fmt.Errorf("Records 'deduplicate_by' field contains an empty string at index %d", i)
		}
		r.DeduplicateBy[i] = strings.TrimSpace(dedup)
	}
	for i, pred := range r.Where {
		if pred == nil {
			return fmt.Errorf("Records 'where' field contains a nil predicate at index %d", i)
		}
		if err := pred.Validate(); err != nil {
			return fmt.Errorf("Records 'where' field validation failed at index %d: %v", i, err)
		}
	}
	if r.MaxRecords < 0 {
		return fmt.Errorf("Records 'max_records' field cannot be negative")
	}
	if r.MaxRecords > MaximumRecords {
		return fmt.Errorf("Records 'max_records' field cannot exceed %d", MaximumRecords)
	}

	return nil
}

type Predicate struct {
	Field     string       `yaml:"field,omitempty"`      // Record field or context variable to inspect.
	Equals    *string      `yaml:"equals,omitempty"`     // Requires an exact value match.
	NotEquals *string      `yaml:"not_equals,omitempty"` // Requires a different value.
	Contains  *string      `yaml:"contains,omitempty"`   // Requires the value to contain this text.
	Matches   *string      `yaml:"matches,omitempty"`    // Requires the value to match a regular expression.
	Exists    *bool        `yaml:"exists,omitempty"`     // Tests whether the field is present.
	In        []string     `yaml:"in,omitempty"`         // Requires the value to be one of these values.
	All       []*Predicate `yaml:"all,omitempty"`        // Nested predicates that must all match.
	Any       []*Predicate `yaml:"any,omitempty"`        // Nested predicates where at least one must match.
}

func (p *Predicate) Validate() error {
	if p == nil {
		return fmt.Errorf("Predicate is nil")
	}

	operators := make([]string, 0, 6)
	if p.Equals != nil {
		operators = append(operators, "equals")
	}
	if p.NotEquals != nil {
		operators = append(operators, "not_equals")
	}
	if p.Contains != nil {
		operators = append(operators, "contains")
	}
	if p.Matches != nil {
		operators = append(operators, "matches")
	}
	if p.Exists != nil {
		operators = append(operators, "exists")
	}
	for i, in := range p.In {
		if strings.TrimSpace(in) == "" {
			return fmt.Errorf("Predicate 'in' field contains an empty string at index %d", i)
		}
		p.In[i] = strings.TrimSpace(in)
	}
	if p.In != nil {
		operators = append(operators, "in")
		if len(p.In) == 0 {
			return fmt.Errorf("Predicate 'in' must contain at least one value")
		}
	}

	hasAll := p.All != nil
	hasAny := p.Any != nil
	if hasAll && hasAny {
		return fmt.Errorf("Predicate cannot define both 'all' and 'any'")
	}
	if hasAll || hasAny {
		if strings.TrimSpace(p.Field) != "" {
			return fmt.Errorf("compound Predicate cannot define 'field'")
		}
		if len(operators) != 0 {
			return fmt.Errorf("compound Predicate cannot define leaf operators: %s", strings.Join(operators, ", "))
		}

		name := "all"
		children := p.All
		if hasAny {
			name = "any"
			children = p.Any
		}
		if len(children) == 0 {
			return fmt.Errorf("Predicate '%s' must contain at least one child predicate", name)
		}
		for i, child := range children {
			if child == nil {
				return fmt.Errorf("Predicate '%s' contains a nil child at index %d", name, i)
			}
			if err := child.Validate(); err != nil {
				return fmt.Errorf("Predicate '%s' child at index %d validation failed: %v", name, i, err)
			}
		}
		return nil
	}

	if strings.TrimSpace(p.Field) == "" {
		return fmt.Errorf("Predicate 'field' cannot be empty")
	}
	if len(operators) == 0 {
		return fmt.Errorf("Predicate must define exactly one operator")
	}
	if len(operators) > 1 {
		return fmt.Errorf("Predicate cannot define conflicting operators: %s", strings.Join(operators, ", "))
	}
	if p.Matches != nil {
		if _, err := regexp.Compile(*p.Matches); err != nil {
			return fmt.Errorf("Predicate 'matches' regular expression is invalid: %v", err)
		}
	}

	return nil
}

type Default struct {
	Run *PipelineRun `yaml:"run,omitempty"` // Optional default command wrapper retained for compatibility.
}

// PipelineCase associates one predicate with the child steps to execute when
// that predicate matches.
type PipelineCase struct {
	When  *Predicate      `yaml:"when"`  // Condition evaluated against the current context or record.
	Steps []*PipelineStep `yaml:"steps"` // Steps executed when When matches.
}

func (pc *PipelineCase) Validate() error {
	if pc.When == nil {
		return fmt.Errorf("PipelineCase 'when' field cannot be nil")
	}
	if err := pc.When.Validate(); err != nil {
		return fmt.Errorf("PipelineCase 'when' field validation failed: %v", err)
	}
	if len(pc.Steps) == 0 {
		return fmt.Errorf("PipelineCase 'steps' field cannot be empty, at least one step is required")
	}
	for _, step := range pc.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("PipelineCase step validation failed: %v", err)
		}
	}

	return nil
}

type PipelineBranch struct {
	Cases   []*PipelineCase `yaml:"cases"`   // Ordered cases; the first matching case is selected.
	Default []*PipelineStep `yaml:"default"` // Steps used when no case matches.
}

func (pb *PipelineBranch) Validate() error {
	if len(pb.Cases) == 0 && len(pb.Default) == 0 {
		return fmt.Errorf("PipelineBranch must have at least one case or a default branch")
	}
	for _, c := range pb.Cases {
		if c == nil {
			return fmt.Errorf("PipelineBranch contains a nil case")
		}
		if err := c.Validate(); err != nil {
			return fmt.Errorf("PipelineBranch case validation failed: %v", err)
		}
	}
	for _, step := range pb.Default {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("PipelineBranch default step validation failed: %v", err)
		}
	}

	return nil
}

type PipelineExtract struct {
	FromStepID string      `yaml:"from_step_id"` // ID of the run step whose raw output is parsed.
	RecordSpec *RecordSpec `yaml:"record_spec"`  // Rules for creating one named record collection.
	Context    []*Context  `yaml:"context"`      // Patterns that update context while scanning output.
}

func (pe *PipelineExtract) Validate() error {
	if strings.TrimSpace(pe.FromStepID) == "" {
		return fmt.Errorf("PipelineExtract 'from_step_id' field cannot be empty")
	}
	if pe.RecordSpec == nil {
		return fmt.Errorf("PipelineExtract 'record_spec' field cannot be nil")
	}
	if err := pe.RecordSpec.Validate(); err != nil {
		return fmt.Errorf("PipelineExtract 'record_spec' field validation failed: %v", err)
	}
	for _, ctx := range pe.Context {
		if ctx == nil || ctx.Pattern == nil {
			return fmt.Errorf("PipelineExtract 'context' contains a nil entry")
		}
		if err := ctx.Validate(); err != nil {
			return fmt.Errorf("PipelineExtract 'context' field validation failed: %v", err)
		}
	}

	return nil
}

type PipelineForEach struct {
	In    string          `yaml:"in"`    // Existing RunContext.Collections key to iterate over.
	Steps []*PipelineStep `yaml:"steps"` // Child steps run once for each current record.
}

func (fe *PipelineForEach) Validate() error {
	if strings.TrimSpace(fe.In) == "" {
		return fmt.Errorf("PipelineForEach 'in' field cannot be empty")
	}
	if len(fe.Steps) == 0 {
		return fmt.Errorf("PipelineForEach 'steps' field cannot be empty")
	}
	for _, step := range fe.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("PipelineForEach step validation failed: %v", err)
		}
	}
	return nil
}

// PipelineRun is the command wrapper used by the original pipeline model.
// PipelineStep currently stores Command directly in its Run field.
type PipelineRun struct {
	Command *Command `yaml:"command"` // Command configuration to execute.
}

// PipelineStep is one executable or control-flow unit. Exactly one primary
// operation should be set: Run, Extract, ForEach, Branch, Export, Join.
type PipelineStep struct {
	ID      string           `yaml:"id"`                 // Identifier used for references, logs, and step paths.
	Run     *Command         `yaml:"run,omitempty"`      // Execute one command and retain its raw output.
	Extract *PipelineExtract `yaml:"extract,omitempty"`  // Parse output from another run step into records.
	ForEach *PipelineForEach `yaml:"for_each,omitempty"` // Iterate child steps over a named collection.
	Branch  *PipelineBranch  `yaml:"branch,omitempty"`   // Select child steps using predicates.
	Export  *PipelineExport  `yaml:"export,omitempty"`
	Join    *PipelineJoin    `yaml:"join,omitempty"`
	OnError string           `yaml:"on_error,omitempty"` // Failure policy applied to this step.
}

func (ps *PipelineStep) Validate() error {
	if ps == nil {
		return fmt.Errorf("PipelineStep object is nil")
	}
	if strings.TrimSpace(ps.ID) == "" {
		return fmt.Errorf("PipelineStep 'id' field cannot be empty")
	}
	if !isOnlyOneFunction(ps) {
		return fmt.Errorf("PipelineStep must have exactly one of 'run', 'extract', 'for_each', 'branch', 'export', 'join' defined")
	}
	if ps.Run != nil {
		if err := ps.Run.Validate(); err != nil {
			return fmt.Errorf("PipelineStep 'run' validation failed: %v", err)
		}
	}
	if ps.Extract != nil {
		if err := ps.Extract.Validate(); err != nil {
			return fmt.Errorf("PipelineStep 'extract' validation failed: %v", err)
		}
	}
	if ps.ForEach != nil {
		if err := ps.ForEach.Validate(); err != nil {
			return fmt.Errorf("PipelineStep 'for_each' validation failed: %v", err)
		}
	}
	if ps.Branch != nil {
		if err := ps.Branch.Validate(); err != nil {
			return fmt.Errorf("PipelineStep 'branch' validation failed: %v", err)
		}
	}
	if ps.Export != nil {
		if err := ps.Export.Validate(); err != nil {
			return fmt.Errorf("PipelineStep 'export' validation failed: %v", err)
		}
	}
	if ps.Join != nil {
		if err := ps.Join.Validate(); err != nil {
			return fmt.Errorf("PipelineStep 'join' validation failed: %v", err)
		}
	}
	ps.OnError = strings.ToLower(strings.TrimSpace(ps.OnError))
	if ps.OnError == "" {
		ps.OnError = "stop_router"
	} else if ps.OnError != "continue_record" && ps.OnError != "stop_router" {
		return fmt.Errorf("PipelineStep 'on_error' field must be either 'continue_record' or 'stop_router'")
	}

	return nil
}

func isOnlyOneFunction(step *PipelineStep) bool {
	count := 0
	if step.Run != nil {
		count++
	}
	if step.Branch != nil {
		count++
	}
	if step.Extract != nil {
		count++
	}
	if step.ForEach != nil {
		count++
	}
	if step.Export != nil {
		count++
	}
	if step.Join != nil {
		count++
	}
	return count == 1
}

type PipelineExport struct {
	From      string `yaml:"from"`
	Field     string `yaml:"field"`
	As        string `yaml:"as"`
	Require   string `yaml:"require"` // one of: "exactly_one".
	Overwrite bool   `yaml:"overwrite,omitempty"`
}

func (pe *PipelineExport) Validate() error {
	if pe == nil {
		return fmt.Errorf("PipelineExport object is nil")
	}
	pe.From = strings.TrimSpace(pe.From)
	pe.Field = strings.TrimSpace(pe.Field)
	pe.As = strings.TrimSpace(pe.As)
	pe.Require = strings.TrimSpace(pe.Require)
	if pe.From == "" {
		return fmt.Errorf("PipelineExport 'from' field cannot be empty")
	}
	if pe.Field == "" {
		return fmt.Errorf("PipelineExport 'field' field cannot be empty")
	}
	if IsReservedPipelineRenderField(pe.Field) {
		return fmt.Errorf("PipelineExport 'field' field cannot reference reserved field: %s", pe.Field)
	}
	if pe.As == "" {
		return fmt.Errorf("PipelineExport 'as' field cannot be empty")
	}
	if IsReservedPipelineRenderField(pe.As) {
		return fmt.Errorf("PipelineExport 'as' field cannot reference reserved field: %s", pe.As)
	}
	if pe.Require == "" {
		return fmt.Errorf("PipelineExport 'require' field cannot be empty")
	}
	switch pe.Require {
	case "exactly_one":
		// valid
	default:
		return fmt.Errorf("PipelineExport 'require' field must be one of: 'exactly_one'")
	}

	return nil
}

type PipelineJoin struct {
	Left      string   `yaml:"left"`
	Right     string   `yaml:"right"`
	On        []string `yaml:"on"`
	Output    string   `yaml:"output"`
	Unmatched string   `yaml:"unmatched,omitempty"`
}

func (pj *PipelineJoin) Validate() error {
	if pj == nil {
		return fmt.Errorf("PipelineJoin object is nil")
	}
	pj.Left = strings.TrimSpace(pj.Left)
	pj.Right = strings.TrimSpace(pj.Right)
	pj.Output = strings.TrimSpace(pj.Output)
	pj.Unmatched = strings.TrimSpace(pj.Unmatched)
	if pj.Left == "" {
		return fmt.Errorf("PipelineJoin 'left' field cannot be empty")
	}
	if pj.Right == "" {
		return fmt.Errorf("PipelineJoin 'right' field cannot be empty")
	}
	if len(pj.On) == 0 {
		return fmt.Errorf("PipelineJoin 'on' field cannot be empty")
	}
	if pj.Output == "" {
		return fmt.Errorf("PipelineJoin 'output' field cannot be empty")
	}
	uniqueOn := make(map[string]struct{})
	for i, on := range pj.On {
		on = strings.TrimSpace(on)
		if on == "" {
			return fmt.Errorf("PipelineJoin 'on' field at index %d cannot be empty", i)
		}
		if IsReservedPipelineRenderField(on) {
			return fmt.Errorf("PipelineJoin 'on' field cannot reference reserved field: %s", on)
		}
		if _, exists := uniqueOn[on]; exists {
			return fmt.Errorf("PipelineJoin 'on' field contains duplicate: %s", on)
		}
		uniqueOn[on] = struct{}{}
		pj.On[i] = on
	}
	switch pj.Unmatched {
	case "ignore":
	case "":
		pj.Unmatched = "ignore"
	case "error":
	default:
		return fmt.Errorf("PipelineJoin 'unmatched' field must be one of: 'ignore', 'error'")
	}

	return nil
}
