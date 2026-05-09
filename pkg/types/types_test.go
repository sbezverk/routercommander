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
			name: "capture case 1",
			input: []byte(`commands:
- command: "run netstat -aup | grep tcp"
  times: 3600
  interval: 1
  process_result: true
  patterns:
  - pattern_string:  SndbufErrors:\s*[0-9+]
    captured_values:
    - field_number: 2
      operation: "compare_with_previous"
    capture:
      field_number: [2]
      separator: ":"
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
