package patterns

import (
	"regexp"
	"testing"
)

func TestShowControllersNPUStatsTrapsAll(t *testing.T) {
	regExp := regexp.MustCompile(`^\s*(\w+\s*(\(([a-zA-Z_\-0-9]+\s*)+\))?\s+)(\w+\s+){5}[1-9.]+\s*$`)
	tests := []struct {
		name  string
		line  []byte
		match bool
	}{
		{
			name:  "case 1",
			line:  []byte("RxTrapStpStateBlock_EXT_HDR                   0    52   0x34        32048   0                    0"),
			match: false,
		},
		{
			name:  "case 1_1",
			line:  []byte("RxTrapStpStateBlock_EXT_HDR                   0    52   0x34        32048   0                    1"),
			match: true,
		},
		{
			name:  "case 2",
			line:  []byte("RxTrapAdjacentCheckFail (Intf-Down)           0    57   0x39        32043   0                    0"),
			match: false,
		},
		{
			name:  "case 2_1",
			line:  []byte("RxTrapAdjacentCheckFail (Intf-Down)           0    57   0x39        32043   0                    1"),
			match: true,
		},
		{
			name:  "case 3",
			line:  []byte("RxTrapTrillUnknownUc (flooding UC disable)    0    83   0x53        32044   0                    0"),
			match: false,
		},
		{
			name:  "case 3_1",
			line:  []byte("RxTrapTrillUnknownUc (flooding UC disable)    0    83   0x53        32044   0                    1"),
			match: true,
		},
		{
			name:  "case 4",
			line:  []byte("RxTrapOamEthUpAccelerated(OAM_BDL_UP_NON_CCM) 0    164  0xa4        32034   0                    0"),
			match: false,
		},
		{
			name:  "case 4_1",
			line:  []byte("RxTrapOamEthUpAccelerated(OAM_BDL_UP_NON_CCM) 0    164  0xa4        32034   0                    1"),
			match: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := regExp.Match(tt.line)
			if m && !tt.match {
				t.Fatal("Supposed to not match, but match was found..")
			}
			if !m && tt.match {
				t.Fatal("Supposed to match, but no match was found..")
			}
		})
	}
}
