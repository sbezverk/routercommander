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
			input: []byte(`main_command_group:
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
								Capture: &Capture{

									FieldNumber: []int{2},
									Separator:   ":",
									Values:      make(map[int]interface{}),
								},
								RegExp: getRegExp(`SndbufErrors:\s*[0-9+]`),
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
			if !reflect.DeepEqual(commands, tt.expect) {
				t.Logf("Diffs: %+v", deep.Equal(commands, tt.expect))
				t.Fatal("computed members do not match with expected members")
			}
		})
	}
}
