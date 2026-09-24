package types

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/go-test/deep"
)

func getRegExp(s string) *regexp.Regexp {
	r, _ := regexp.Compile(s)
	return r
}

func normalizeCommanderForTest(c *Commander) *Commander {
	if c == nil {
		return nil
	}

	out := &Commander{
		Repro:             c.Repro,
		Collect:           c.Collect,
		CommandsWithTests: nil,
	}

	if len(c.MainCommandGroup) != 0 {
		out.MainCommandGroup = make([]*Command, len(c.MainCommandGroup))
		for i, cmd := range c.MainCommandGroup {
			if cmd == nil {
				continue
			}
			cmdCopy := *cmd
			cmdCopy.CommandResult = nil
			if len(cmd.Patterns) != 0 {
				cmdCopy.Patterns = make([]*Pattern, len(cmd.Patterns))
				for j, p := range cmd.Patterns {
					if p == nil {
						continue
					}
					pCopy := *p
					pCopy.RegExp = nil
					cmdCopy.Patterns[j] = &pCopy
				}
			}
			out.MainCommandGroup[i] = &cmdCopy
		}
	}

	if len(c.Tests) != 0 {
		out.Tests = make([]*Tests, len(c.Tests))
		for i, tests := range c.Tests {
			if tests == nil {
				continue
			}
			testsCopy := *tests
			testsCopy.Tests = nil
			if len(tests.Source) != 0 {
				testsCopy.Source = make([]*Test, len(tests.Source))
				for j, test := range tests.Source {
					if test == nil {
						continue
					}
					testCopy := *test
					testCopy.ValuesStore = nil
					if test.Pattern != nil {
						pCopy := *test.Pattern
						pCopy.RegExp = nil
						testCopy.Pattern = &pCopy
					}
					testsCopy.Source[j] = &testCopy
				}
			}
			out.Tests[i] = &testsCopy
		}
	}

	return out
}

func TestParseCommandFile(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		expect *Commander
		fail   bool
	}{
		{
			name:   "empty input",
			input:  []byte(``),
			expect: &Commander{},
			fail:   false,
		},
		{
			name: "command patterns",
			input: []byte(`commands:
- command: "run netstat -aup | grep tcp"
  times: 3600
  interval: 1
  process_result: true
  patterns:
  - pattern_string:  SndbufErrors:\s*[0-9+]
  debug: false`),
			expect: &Commander{
				MainCommandGroup: []*Command{
					{
						Cmd:           "run netstat -aup | grep tcp",
						Times:         3600,
						Interval:      1,
						ProcessResult: true,
						Patterns: []*Pattern{
							{
								PatternString: `SndbufErrors:\s*[0-9+]`,
								RegExp:        getRegExp(`SndbufErrors:\s*[0-9+]`),
							},
						},
					},
				},
			},
			fail: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands, err := parseCommandFile(tt.input)
			if err != nil && !tt.fail {
				t.Fatalf("test supposed to succeed but failed with error: %+v", err)
			}
			if err == nil && tt.fail {
				t.Fatalf("test supposed to fail but succeeded")
			}
			if err != nil {
				return
			}
			got := normalizeCommanderForTest(commands)
			want := normalizeCommanderForTest(tt.expect)
			if !reflect.DeepEqual(got, want) {
				t.Logf("Diffs: %+v", deep.Equal(got, want))
				t.Fatal("computed members do not match with expected members")
			}
		})
	}
}

func TestParseCommandFileBindsCurrentTestModel(t *testing.T) {
	input := []byte(`collect:
  process_result: true
repro:
  times: 2
  interval: 1
  if_triggered_commands:
  - command: "show tech-support"
tests:
- command: "show health"
  command_tests:
  - id: 1
    pattern:
      pattern_string: "FAULT"
    separator: "|"
    fields:
    - field_number: 2
      operation: "compare_with_value_neq"
      value: "UP"
    if_triggered_commands:
    - command: "show alarms"
    check_all_results: true
  - id: 2
    pattern:
      pattern_string: "WARN"
commands:
- command: "show health"
  process_result: true
  command_test_ids: [2]
  patterns:
  - pattern_string: "FAULT"
`)

	commands, err := parseCommandFile(input)
	if err != nil {
		t.Fatalf("failed to parse current command model: %+v", err)
	}
	if commands.Collect == nil || !commands.Collect.ProcessResult {
		t.Fatalf("collect.process_result did not bind")
	}
	if commands.Repro == nil {
		t.Fatalf("repro did not bind")
	}
	if commands.Repro.Times != 2 || commands.Repro.Interval != 1 {
		t.Fatalf("repro timing did not bind: %+v", commands.Repro)
	}
	if len(commands.Repro.PostMortemCommandGroup) != 1 || commands.Repro.PostMortemCommandGroup[0].Cmd != "show tech-support" {
		t.Fatalf("repro.if_triggered_commands did not bind: %+v", commands.Repro.PostMortemCommandGroup)
	}
	if len(commands.MainCommandGroup) != 1 {
		t.Fatalf("expected one command, got %d", len(commands.MainCommandGroup))
	}
	cmd := commands.MainCommandGroup[0]
	if !reflect.DeepEqual(cmd.TestIDs, []int{2}) {
		t.Fatalf("command_test_ids did not bind: %+v", cmd.TestIDs)
	}
	if len(cmd.Patterns) != 1 || cmd.Patterns[0].RegExp == nil {
		t.Fatalf("command patterns were not compiled: %+v", cmd.Patterns)
	}
	tests := commands.CommandsWithTests["show health"]
	if tests == nil {
		t.Fatalf("tests were not indexed by command")
	}
	if len(tests.Tests) != 2 {
		t.Fatalf("expected two tests, got %d", len(tests.Tests))
	}
	test := tests.Tests[1]
	if test == nil {
		t.Fatalf("test id 1 was not indexed")
	}
	if test.Pattern == nil || test.Pattern.RegExp == nil {
		t.Fatalf("test pattern was not compiled: %+v", test.Pattern)
	}
	if test.Separator != "|" {
		t.Fatalf("separator did not bind: %q", test.Separator)
	}
	if len(test.Fields) != 1 {
		t.Fatalf("expected one field, got %d", len(test.Fields))
	}
	field := test.Fields[0]
	if field.FieldNumber != 2 || field.Operation != "compare_with_value_neq" || field.Value != "UP" {
		t.Fatalf("field did not bind: %+v", field)
	}
	if len(test.IfTriggeredCommands) != 1 || test.IfTriggeredCommands[0].Cmd != "show alarms" {
		t.Fatalf("test if_triggered_commands did not bind: %+v", test.IfTriggeredCommands)
	}
	if !test.CheckAllResults {
		t.Fatalf("check_all_results did not bind")
	}
}

func TestCommanderCloneForRunIsolatesMutableState(t *testing.T) {
	occurrences := 2
	commands := &Commander{
		Collect: &Collect{
			ProcessResult: true,
		},
		MainCommandGroup: []*Command{
			{
				Cmd:      "show version",
				Location: []string{"0/0/cpu0"},
				TestIDs:  []int{10},
				Patterns: []*Pattern{
					{
						PatternString: "Version",
						RegExp:        getRegExp("Version"),
					},
				},
				CommandResult: &CommandResult{
					PatternMatch:  []string{"original"},
					TriggeredTest: []int{1},
				},
			},
		},
		Tests: []*Tests{
			{
				Cmd: "show version",
				Source: []*Test{
					{
						ID:                 10,
						NumberOfOccurences: &occurrences,
						Pattern: &Pattern{
							PatternString: "Version",
							RegExp:        getRegExp("Version"),
						},
						Fields: []*Field{
							{
								FieldNumber: 1,
								Operation:   "compare_with_previous_eq",
							},
						},
						IfTriggeredCommands: []*Command{
							{
								Cmd: "show logging",
							},
						},
						ValuesStore: map[int]map[int]interface{}{
							0: {
								1: "original",
							},
						},
					},
				},
			},
		},
	}
	commands.CommandsWithTests = map[string]*Tests{
		"show version": commands.Tests[0],
	}
	commands.Tests[0].Tests = map[int]*Test{
		10: commands.Tests[0].Source[0],
	}

	clone := commands.CloneForRun()
	clone.MainCommandGroup[0].Location[0] = "0/1/cpu0"
	clone.MainCommandGroup[0].TestIDs[0] = 20
	clone.MainCommandGroup[0].CommandResult.PatternMatch = append(clone.MainCommandGroup[0].CommandResult.PatternMatch, "clone")
	clone.Tests[0].Source[0].Fields[0].FieldNumber = 2
	clone.Tests[0].Source[0].IfTriggeredCommands[0].Cmd = "show tech"
	clone.Tests[0].Source[0].ValuesStore[0] = map[int]interface{}{
		2: "clone",
	}
	*clone.Tests[0].Source[0].NumberOfOccurences = 3

	if commands.MainCommandGroup[0].Location[0] != "0/0/cpu0" {
		t.Fatalf("source command location was mutated")
	}
	if commands.MainCommandGroup[0].TestIDs[0] != 10 {
		t.Fatalf("source command test IDs were mutated")
	}
	if len(commands.MainCommandGroup[0].CommandResult.PatternMatch) != 1 {
		t.Fatalf("source command result was shared")
	}
	if commands.Tests[0].Source[0].Fields[0].FieldNumber != 1 {
		t.Fatalf("source test field was mutated")
	}
	if commands.Tests[0].Source[0].IfTriggeredCommands[0].Cmd != "show logging" {
		t.Fatalf("source triggered command was mutated")
	}
	if commands.Tests[0].Source[0].ValuesStore[0][1] != "original" {
		t.Fatalf("source values store was shared")
	}
	if *commands.Tests[0].Source[0].NumberOfOccurences != 2 {
		t.Fatalf("source occurrence pointer was shared")
	}
	if clone.CommandsWithTests["show version"] != clone.Tests[0] {
		t.Fatalf("cloned command test index does not point to the cloned tests")
	}
	if clone.CommandsWithTests["show version"] == commands.CommandsWithTests["show version"] {
		t.Fatalf("cloned command test index points to source tests")
	}
}
