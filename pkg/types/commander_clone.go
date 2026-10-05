package types

// CloneForRun returns an independent command tree with fresh per-run result state.
func (c *Commander) CloneForRun() *Commander {
	if c == nil {
		return nil
	}

	out := &Commander{
		MainCommandGroup: cloneCommandsForRun(c.MainCommandGroup),
		Pipeline:         clonePipelineStepsForRun(c.Pipeline),
	}
	if c.PipelineLimits != nil {
		limits := *c.PipelineLimits
		out.PipelineLimits = &limits
	}
	if c.Repro != nil {
		repro := *c.Repro
		repro.PostMortemCommandGroup = cloneCommandsForRun(c.Repro.PostMortemCommandGroup)
		out.Repro = &repro
	}
	if c.Collect != nil {
		collect := *c.Collect
		out.Collect = &collect
	}
	if len(c.Tests) != 0 {
		out.Tests = make([]*Tests, len(c.Tests))
		out.CommandsWithTests = make(map[string]*Tests, len(c.Tests))
		for i, tests := range c.Tests {
			out.Tests[i] = cloneTestsForRun(tests)
			if out.Tests[i] != nil {
				out.CommandsWithTests[out.Tests[i].Cmd] = out.Tests[i]
			}
		}
	}

	return out
}

func cloneCommandsForRun(commands []*Command) []*Command {
	if len(commands) == 0 {
		return nil
	}
	out := make([]*Command, len(commands))
	for i, cmd := range commands {
		out[i] = cloneCommandForRun(cmd)
	}
	return out
}

func clonePipelineStepsForRun(steps []*PipelineStep) []*PipelineStep {
	if len(steps) == 0 {
		return nil
	}
	out := make([]*PipelineStep, len(steps))
	for i, step := range steps {
		out[i] = clonePipelineStepForRun(step)
	}
	return out
}

func clonePipelineStepForRun(step *PipelineStep) *PipelineStep {
	if step == nil {
		return nil
	}
	out := *step
	out.Run = cloneCommandForRun(step.Run)
	out.Extract = clonePipelineExtractForRun(step.Extract)
	out.ForEach = clonePipelineForEachForRun(step.ForEach)
	out.Branch = clonePipelineBranchForRun(step.Branch)
	return &out
}

func clonePipelineExtractForRun(extract *PipelineExtract) *PipelineExtract {
	if extract == nil {
		return nil
	}
	out := *extract
	out.RecordSpec = cloneRecordSpecForRun(extract.RecordSpec)
	if len(extract.Context) != 0 {
		out.Context = make([]*Context, len(extract.Context))
		for i, context := range extract.Context {
			if context == nil {
				continue
			}
			contextCopy := *context
			contextCopy.Pattern = clonePatternForRun(context.Pattern)
			out.Context[i] = &contextCopy
		}
	}
	return &out
}

func cloneRecordSpecForRun(spec *RecordSpec) *RecordSpec {
	if spec == nil {
		return nil
	}
	out := *spec
	out.Pattern = clonePatternForRun(spec.Pattern)
	out.Inherit = append([]string(nil), spec.Inherit...)
	out.DeduplicateBy = append([]string(nil), spec.DeduplicateBy...)
	out.Where = clonePredicatesForRun(spec.Where)
	return &out
}

func clonePredicatesForRun(predicates []*Predicate) []*Predicate {
	if len(predicates) == 0 {
		return nil
	}
	out := make([]*Predicate, len(predicates))
	for i, predicate := range predicates {
		out[i] = clonePredicateForRun(predicate)
	}
	return out
}

func clonePredicateForRun(predicate *Predicate) *Predicate {
	if predicate == nil {
		return nil
	}
	out := *predicate
	if predicate.Equals != nil {
		value := *predicate.Equals
		out.Equals = &value
	}
	if predicate.NotEquals != nil {
		value := *predicate.NotEquals
		out.NotEquals = &value
	}
	if predicate.Contains != nil {
		value := *predicate.Contains
		out.Contains = &value
	}
	if predicate.Matches != nil {
		value := *predicate.Matches
		out.Matches = &value
	}
	if predicate.Exists != nil {
		value := *predicate.Exists
		out.Exists = &value
	}
	out.In = append([]string(nil), predicate.In...)
	out.All = clonePredicatesForRun(predicate.All)
	out.Any = clonePredicatesForRun(predicate.Any)
	return &out
}

func clonePipelineForEachForRun(forEach *PipelineForEach) *PipelineForEach {
	if forEach == nil {
		return nil
	}
	out := *forEach
	out.Steps = clonePipelineStepsForRun(forEach.Steps)
	return &out
}

func clonePipelineBranchForRun(branch *PipelineBranch) *PipelineBranch {
	if branch == nil {
		return nil
	}
	out := *branch
	if len(branch.Cases) != 0 {
		out.Cases = make([]*PipelineCase, len(branch.Cases))
		for i, branchCase := range branch.Cases {
			if branchCase == nil {
				continue
			}
			caseCopy := *branchCase
			caseCopy.When = clonePredicateForRun(branchCase.When)
			caseCopy.Steps = clonePipelineStepsForRun(branchCase.Steps)
			out.Cases[i] = &caseCopy
		}
	}
	out.Default = clonePipelineStepsForRun(branch.Default)
	return &out
}

func clonePatternForRun(pattern *Pattern) *Pattern {
	if pattern == nil {
		return nil
	}
	patternCopy := *pattern
	return &patternCopy
}

func cloneCommandForRun(cmd *Command) *Command {
	if cmd == nil {
		return nil
	}
	out := *cmd
	out.Location = append([]string(nil), cmd.Location...)
	out.TestIDs = append([]int(nil), cmd.TestIDs...)
	out.Patterns = clonePatternsForRun(cmd.Patterns)
	out.CommandResult = &CommandResult{
		PatternMatch:  make([]string, 0),
		TriggeredTest: make([]int, 0),
	}
	return &out
}

func clonePatternsForRun(patterns []*Pattern) []*Pattern {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]*Pattern, len(patterns))
	for i, pattern := range patterns {
		if pattern == nil {
			continue
		}
		patternCopy := *pattern
		out[i] = &patternCopy
	}
	return out
}

func cloneTestsForRun(tests *Tests) *Tests {
	if tests == nil {
		return nil
	}
	out := *tests
	out.Source = make([]*Test, len(tests.Source))
	out.Tests = make(map[int]*Test, len(tests.Source))
	for i, test := range tests.Source {
		out.Source[i] = cloneTestForRun(test)
		if out.Source[i] != nil {
			out.Tests[out.Source[i].ID] = out.Source[i]
		}
	}
	return &out
}

func cloneTestForRun(test *Test) *Test {
	if test == nil {
		return nil
	}
	out := *test
	if test.NumberOfOccurences != nil {
		n := *test.NumberOfOccurences
		out.NumberOfOccurences = &n
	}
	if test.Pattern != nil {
		pattern := *test.Pattern
		out.Pattern = &pattern
	}
	if len(test.Fields) != 0 {
		out.Fields = make([]*Field, len(test.Fields))
		for i, field := range test.Fields {
			if field == nil {
				continue
			}
			fieldCopy := *field
			out.Fields[i] = &fieldCopy
		}
	}
	out.IfTriggeredCommands = cloneCommandsForRun(test.IfTriggeredCommands)
	out.ValuesStore = make(map[int]map[int]interface{})
	return &out
}
