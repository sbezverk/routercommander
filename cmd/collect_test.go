package main

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/go-test/deep"
	"github.com/sbezverk/routercommander/pkg/types"
	"github.com/sbezverk/tools/sort"
)

func TestCheckCommandOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    []*types.CmdResult
		patterns []*types.Pattern
		matches  []string
	}{
		{
			name: "admin show controller fabric plane all",
			input: []*types.CmdResult{
				{
					Cmd: "admin show controller fabric plane all",
					Result: []byte(`Sun Jun 25 07:21:25.682 GMT

Plane Admin Plane    up->dn  up->mcast
Id    State State    counter   counter
--------------------------------------
0     UP    UP             4         7
1     DN    UP             4        11
2     UP    DN             5        14
3     UP    UP            46         6
4     UP    UP             4         7
5     DN    DN             4         4
`),
				},
			},
			patterns: []*types.Pattern{
				{
					PatternString: "`.+?(DN)",
					RegExp:        regexp.MustCompile(`.+?(DN)`),
				},
			},
			matches: []string{
				`1     DN    UP             4        11`,
				`2     UP    DN             5        14`,
				`5     DN    DN             4         4`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := matchPatterns(tt.input, tt.patterns)
			if err != nil {
				t.Fatalf("failed with error: %+v", err)
			}
			if len(matches) != len(tt.matches) {
				t.Fatalf("number of expected matches: %d does not match the computed: %d", len(tt.matches), len(matches))
			}

			s1 := sort.SortMergeComparableSlice(matches)
			s2 := sort.SortMergeComparableSlice(tt.matches)
			for i := 0; i < len(tt.matches); i++ {
				if strings.Trim(s1[i], " \n\t") != strings.Trim(s2[i], " \n\t") {
					t.Logf("element %d diffs for  %+v", i, deep.Equal(s1[i], s2[i]))
					t.Fatal("expected result does not match to computed result")
				}
			}
		})
	}
}

func TestRunTest(t *testing.T) {
	tests := []struct {
		name      string
		input     []*types.CmdResult
		test      *types.Test
		iteration int
		triggered bool
	}{
		{
			name: "admin show controller sfe driver location all all clean",
			input: []*types.CmdResult{
				{
					Cmd: "admin show controller sfe driver location all",
					Result: []byte(`Tue Jul  4 09:54:27.413 GMT
Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC0/0   | UP | 1|s123| UP/UP| 0/A | DONE| NRML       | 0| WB    |  1|  0|
| 0/FC0/1   | UP | 1|s123| UP/UP| 0/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+

Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC1/0   | UP | 1|s123| UP/UP| 1/A | DONE| NRML       | 0| WB    |  1|  0|
| 0/FC1/1   | UP | 1|s123| UP/UP| 1/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+

Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC2/0   | UP | 1|s123| UP/UP| 2/A | DONE| NRML       | 0| WB    |  1|  0|
| 0/FC2/1   | UP | 1|s123| UP/UP| 2/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+

Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC3/0   | UP | 1|s123| UP/UP| 3/A | DONE| NRML       | 0| WB    |  1|  0|
| 0/FC3/1   | UP | 1|s123| UP/UP| 3/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+

Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC4/0   | UP | 1|s123| UP/UP| 4/A | DONE| NRML       | 0| WB    |  1|  0|
| 0/FC4/1   | UP | 1|s123| UP/UP| 4/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+

Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC5/0   | UP | 1|s123| UP/UP| 5/A | DONE| NRML       | 0| WB    |  1|  0|
| 0/FC5/1   | UP | 1|s123| UP/UP| 5/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+`),
				},
			},
			triggered: false,
			iteration: 0,
			test: &types.Test{
				ValuesStore: make(map[int]map[int]interface{}),
				Pattern: &types.Pattern{
					PatternString: "0/FC[0-5]/[0-4]",
				},
				Separator: "|",
				Fields: []*types.Field{
					{
						FieldNumber: 2,
						Operation:   "compare_with_value_neq",
						Value:       "UP",
					},
					{
						FieldNumber: 5,
						Operation:   "compare_with_value_neq",
						Value:       "UP/UP",
					},
					{
						FieldNumber: 8,
						Operation:   "compare_with_value_neq",
						Value:       "NRML",
					},
				},
			},
		},
		{
			name: "admin show controller sfe driver location all all DN",
			input: []*types.CmdResult{
				{
					Cmd: "admin show controller sfe driver location all",
					Result: []byte(`Tue Jul  4 09:54:27.413 GMT

Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC4/0   | UP | 1|s123| UP/UP| 4/A | DONE| NRML       | 0| WB    |  1|  0|
| 0/FC4/1   | UP | 1|s123| UP/UP| 4/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+

Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC5/0   | DN | 1|s123| UP/UP| 5/A | DONE| NRML       | 0| WB    |  1|  0|
| 0/FC5/1   | UP | 1|s123| UP/UP| 5/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+`),
				},
			},
			triggered: true,
			iteration: 0,
			test: &types.Test{
				ValuesStore: make(map[int]map[int]interface{}),
				Pattern: &types.Pattern{
					PatternString: "0/FC[0-5]/[0-4]",
				},
				Separator: "|",
				Fields: []*types.Field{
					{
						FieldNumber: 2,
						Operation:   "compare_with_value_neq",
						Value:       "UP",
					},
					{
						FieldNumber: 5,
						Operation:   "compare_with_value_neq",
						Value:       "UP/UP",
					},
					{
						FieldNumber: 8,
						Operation:   "compare_with_value_neq",
						Value:       "NRML",
					},
				},
			},
		},
		{
			name: "admin show controller sfe driver location all all UP/DN",
			input: []*types.CmdResult{
				{
					Cmd: "admin show controller sfe driver location all",
					Result: []byte(`Tue Jul  4 09:54:27.413 GMT

Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC4/0   | UP | 1|s123| UP/DN| 4/A | DONE| NRML       | 0| WB    |  1|  0|
| 0/FC4/1   | UP | 1|s123| UP/UP| 4/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+

Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC5/0   | UP | 1|s123| UP/UP| 5/A | DONE| NRML       | 0| WB    |  1|  0|
| 0/FC5/1   | UP | 1|s123| UP/UP| 5/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+`),
				},
			},
			triggered: true,
			iteration: 0,
			test: &types.Test{
				ValuesStore: make(map[int]map[int]interface{}),
				Pattern: &types.Pattern{
					PatternString: "0/FC[0-5]/[0-4]",
				},
				Separator: "|",
				Fields: []*types.Field{
					{
						FieldNumber: 2,
						Operation:   "compare_with_value_neq",
						Value:       "UP",
					},
					{
						FieldNumber: 5,
						Operation:   "compare_with_value_neq",
						Value:       "UP/UP",
					},
					{
						FieldNumber: 8,
						Operation:   "compare_with_value_neq",
						Value:       "NRML",
					},
				},
			},
		},
		{
			name: "admin show controller sfe driver location all all not NRML",
			input: []*types.CmdResult{
				{
					Cmd: "admin show controller sfe driver location all",
					Result: []byte(`Tue Jul  4 09:54:27.413 GMT

Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC4/0   | UP | 1|s123| UP/UP| 4/A | DONE| NRML       | 0| WB    |  1|  0|
| 0/FC4/1   | UP | 1|s123| UP/UP| 4/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+

Asics :
HP - HotPlug event,  PON - Power ON reset,     WB - Warm Boot,  A - All
HR - Hard Reset,     DC  - Disconnect signal,  DL - DownLoad
+---------------------------------------------------------------------------+
| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC5/0   | UP | 1|s123| UP/UP| 5/A | DONE| NOT        | 0| WB    |  1|  0|
| 0/FC5/1   | UP | 1|s123| UP/UP| 5/A | DONE| NRML       | 0| WB    |  1|  0|
+---------------------------------------------------------------------------+`),
				},
			},
			triggered: true,
			iteration: 0,
			test: &types.Test{
				ValuesStore: make(map[int]map[int]interface{}),
				Pattern: &types.Pattern{
					PatternString: "0/FC[0-5]/[0-4]",
				},
				Separator: "|",
				Fields: []*types.Field{
					{
						FieldNumber: 2,
						Operation:   "compare_with_value_neq",
						Value:       "UP",
					},
					{
						FieldNumber: 5,
						Operation:   "compare_with_value_neq",
						Value:       "UP/UP",
					},
					{
						FieldNumber: 8,
						Operation:   "compare_with_value_neq",
						Value:       "NRML",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triggered, err := runTest(tt.input, tt.test, tt.iteration)
			if err != nil {
				t.Fatalf("failed with error: %+v", err)
			}
			if tt.triggered && !triggered {
				t.Fatalf("expect triggered to be %t but got %t", tt.triggered, triggered)
			}
		})
	}
}

func TestFrettaHealthCheckHealthySamplesDoNotTrigger(t *testing.T) {
	tests := []struct {
		name  string
		input []*types.CmdResult
		test  *types.Test
	}{
		{
			name: "fabric plane all up",
			input: []*types.CmdResult{
				{
					Cmd: "admin show controller fabric plane all",
					Result: []byte(`Plane Admin Plane    up->dn  up->mcast
Id    State State    counter   counter
--------------------------------------
0     UP    UP             0         2
1     UP    UP             0         2
2     UP    UP             0         2
3     UP    UP             0         2
4     UP    UP             0         2
5     UP    UP             0         2
`),
				},
			},
			test: &types.Test{
				ValuesStore: make(map[int]map[int]interface{}),
				Pattern: &types.Pattern{
					PatternString: `(?m)^[0-5]\s+\w`,
				},
				Separator: " ",
				Fields: []*types.Field{
					{
						FieldNumber: 1,
						Operation:   "compare_with_value_neq",
						Value:       "UP",
					},
					{
						FieldNumber: 2,
						Operation:   "compare_with_value_neq",
						Value:       "UP",
					},
				},
			},
		},
		{
			name: "sfe driver all healthy",
			input: []*types.CmdResult{
				{
					Cmd: "admin show controller sfe driver location all",
					Result: []byte(`| Asic inst.|card|HP|Asic| Admin|plane| Fgid| Asic State |DC| Last  |PON|HR |
|  (R/S/A)  |pwrd|  |type| /Oper|/grp | DL  |            |  | init  |(#)|(#)|
+---------------------------------------------------------------------------+
| 0/FC0/0   | UP | 1|s123| UP/UP| 0/A | DONE| NRML       | 0| PON   |  1|  0|
| 0/FC0/1   | UP | 1|s123| UP/UP| 0/A | DONE| NRML       | 0| PON   |  1|  0|
`),
				},
			},
			test: &types.Test{
				ValuesStore: make(map[int]map[int]interface{}),
				Pattern: &types.Pattern{
					PatternString: "0/FC[0-9]/[0-9]",
				},
				Separator: "|",
				Fields: []*types.Field{
					{
						FieldNumber: 2,
						Operation:   "compare_with_value_neq",
						Value:       "UP",
					},
					{
						FieldNumber: 5,
						Operation:   "compare_with_value_neq",
						Value:       "UP/UP",
					},
					{
						FieldNumber: 8,
						Operation:   "compare_with_value_neq",
						Value:       "NRML",
					},
				},
			},
		},
		{
			name: "watchdog memory normal",
			input: []*types.CmdResult{
				{
					Cmd: "show watchdog memory-state location all",
					Result: []byte(`---- node0_4_CPU0 ----
Memory information:
    Physical Memory	: 29183.0   MB
    Free Memory		: 24460.871 MB
    Memory State	:   Normal
`),
				},
			},
			test: &types.Test{
				ValuesStore: make(map[int]map[int]interface{}),
				Pattern: &types.Pattern{
					PatternString: `Memory\s+State\s+:`,
				},
				Separator: ":",
				Fields: []*types.Field{
					{
						FieldNumber: 1,
						Operation:   "compare_with_value_neq",
						Value:       "Normal",
					},
				},
			},
		},
		{
			name: "fabric health ok",
			input: []*types.CmdResult{
				{
					Cmd: "admin show controller fabric health",
					Result: []byte(`|SFE status  |  Ok |  Ok |  Ok |  Ok |  Ok |  Ok |
`),
				},
			},
			test: &types.Test{
				ValuesStore: make(map[int]map[int]interface{}),
				Pattern: &types.Pattern{
					PatternString: "SFE status",
				},
				Separator: "|",
				Fields: []*types.Field{
					{
						FieldNumber: 2,
						Operation:   "compare_with_value_neq",
						Value:       "Ok",
					},
					{
						FieldNumber: 3,
						Operation:   "compare_with_value_neq",
						Value:       "Ok",
					},
					{
						FieldNumber: 4,
						Operation:   "compare_with_value_neq",
						Value:       "Ok",
					},
					{
						FieldNumber: 5,
						Operation:   "compare_with_value_neq",
						Value:       "Ok",
					},
					{
						FieldNumber: 6,
						Operation:   "compare_with_value_neq",
						Value:       "Ok",
					},
					{
						FieldNumber: 7,
						Operation:   "compare_with_value_neq",
						Value:       "Ok",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triggered, err := runTest(tt.input, tt.test, 0)
			if err != nil {
				t.Fatalf("failed with error: %+v", err)
			}
			if triggered {
				t.Fatalf("healthy sample triggered test")
			}
		})
	}
}

func TestRunTestsWithoutExplicitIDsRunsAllCommandTests(t *testing.T) {
	results := []*types.CmdResult{
		{
			Cmd:    "show health",
			Result: []byte("router is healthy\nFAULT detected\n"),
		},
	}
	tests := &types.Tests{
		Cmd: "show health",
		Tests: map[int]*types.Test{
			1: {
				ID: 1,
				Pattern: &types.Pattern{
					PatternString: "MISSING",
				},
			},
			2: {
				ID: 2,
				Pattern: &types.Pattern{
					PatternString: "FAULT",
				},
			},
		},
	}

	triggers, err := runTests(nil, results, nil, tests, 0, false)
	if err != nil {
		t.Fatalf("failed to run tests: %+v", err)
	}
	if !reflect.DeepEqual(triggers, []int{2}) {
		t.Fatalf("expected omitted command_test_ids to run all tests and trigger [2], got %+v", triggers)
	}
}
