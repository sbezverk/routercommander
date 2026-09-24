package types

// CloneForRun returns an independent command tree with fresh per-run result state.
func (c *Commander) CloneForRun() *Commander {
	if c == nil {
		return nil
	}

	out := &Commander{
		MainCommandGroup: cloneCommandsForRun(c.MainCommandGroup),
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
