package main

import (
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
					RegExp: regexp.MustCompile(`.+?(DN)`),
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
			matches, err := checkCommandOutput(tt.input, tt.patterns)
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
