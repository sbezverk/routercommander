package main

import (
	"testing"

	"github.com/sbezverk/routercommander/pkg/types"
)

func TestGetValue(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		capture    *types.Capture
		index      []int
		fieldValue string
		found      bool
	}{
		{
			name: "test_1",
			input: []byte(`Thu May 11 04:13:41.018 UTC
Active Internet connections (only servers)
Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name
tcp        0      0 0.0.0.0:57800           0.0.0.0:*               LISTEN      34082/emsd
tcp        0      0 0.0.0.0:9449            0.0.0.0:*               LISTEN      34082/emsd
tcp        0      0 0.0.0.0:22              0.0.0.0:*               LISTEN      11510/sshd
tcp6       0      0 :::22                   :::*                    LISTEN      11510/sshd
udp        0      0 0.0.0.0:68              0.0.0.0:*                           11446/xr_dhcpcd
udp        0      0 0.0.0.0:33433           0.0.0.0:*                           6597/igmp
udp6       0      0 :::33433                :::*                                6586/mld
		`),
			capture: &types.Capture{
				FieldNumber: []int{4},
				Separator:   " ",
				Occurrence:  1,
			},
			index: []int{292, 297},
		},
		{
			name: "test_2",
			input: []byte(`tcp        0      0 0.0.0.0:57800           0.0.0.0:*               LISTEN      34082/emsd
tcp        0      0 0.0.0.0:9449            0.0.0.0:*               LISTEN      34082/emsd
tcp        0      0 0.0.0.0:22              0.0.0.0:*               LISTEN      11510/sshd
tcp6       0      0 :::22                   :::*                    LISTEN      11510/sshd
udp        0      0 0.0.0.0:68              0.0.0.0:*                           11446/xr_dhcpcd
udp        0      0 0.0.0.0:33433           0.0.0.0:*                           6597/igmp
udp6       0      0 :::33433                :::*                                6586/mld
`),
			capture: &types.Capture{
				FieldNumber: []int{4},
				Separator:   " ",
				Occurrence:  1,
			},
			index: []int{119, 124},
		},
		{
			name: "test_3",
			input: []byte(`tcp        0      0 0.0.0.0:9449            0.0.0.0:*               LISTEN      34082/emsd      
tcp        0      0 0.0.0.0:22              0.0.0.0:*               LISTEN      11510/sshd      
tcp6       0      0 :::22                   :::*                    LISTEN      11510/sshd      
udp        0      0 0.0.0.0:68              0.0.0.0:*                           11446/xr_dhcpcd 
udp        0      0 0.0.0.0:33433           0.0.0.0:*                           6597/igmp       
udp6       0      0 :::33433                :::*                                6586/mld        
`),
			capture: &types.Capture{
				FieldNumber: []int{4},
				Separator:   " ",
				Occurrence:  1,
			},
			index: []int{28, 33},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := getValue(tt.input, tt.index, tt.capture)
			if err != nil {
				t.Fatalf("failed with error: %+v", err)
			}
			t.Logf("Result: %+v", r)
		})
	}
}
