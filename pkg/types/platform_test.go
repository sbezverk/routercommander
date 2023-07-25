package types

import (
	"reflect"
	"testing"
)

func TestPopulatePlatformInfo(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		platform *platform
		fail     bool
	}{
		{
			name: "asr9k",
			input: []byte(`Tue Jul 25 09:31:57.141 GMT
Node            Type                      State            Config State
-----------------------------------------------------------------------------
0/RSP0/CPU0     A9K-RSP880-TR(Active)     IOS XR RUN       PWR,NSHUT,MON
0/RSP1/CPU0     A9K-RSP880-TR(Standby)    IOS XR RUN       PWR,NSHUT,MON
0/0/CPU0        A9K-8X100GE-TR            IOS XR RUN       PWR,NSHUT,MON
0/2/CPU0        A9K-8X100GE-TR            IOS XR RUN       PWR,NSHUT,MON
`),
			platform: &platform{
				rps: &rps{
					rps: map[string]*rp{
						"0/RSP0/CPU0": {
							isActive: true,
							location: "0/RSP0/CPU0",
						},
						"0/RSP1/CPU0": {
							isActive: false,
							location: "0/RSP1/CPU0",
						},
					},
				},
				lcs: &lcs{
					lcs: map[string]*lc{
						"0/0/CPU0": {
							location: "0/0/CPU0",
						},
						"0/2/CPU0": {
							location: "0/2/CPU0",
						},
					},
				},
			},
		},
		{
			name: "fretta",
			input: []byte(`Tue Jul 25 09:32:20.342 GMT
Node              Type                       State             Config state
--------------------------------------------------------------------------------
0/0/CPU0          NC55-36X100G-A-SE          IOS XR RUN        NSHUT
0/0/NPU0          Slice                      UP                
0/0/NPU1          Slice                      UP                
0/0/NPU2          Slice                      UP                
0/0/NPU3          Slice                      UP                
0/1/CPU0          NC55-36X100G-A-SE          IOS XR RUN        NSHUT
0/1/NPU0          Slice                      UP                
0/1/NPU1          Slice                      UP                
0/1/NPU2          Slice                      UP                
0/1/NPU3          Slice                      UP                
0/2/CPU0          NC57-24DD                  IOS XR RUN        NSHUT
0/2/NPU0          Slice                      UP                
0/2/NPU1          Slice                      UP                
0/3/CPU0          NC57-24DD                  IOS XR RUN        NSHUT
0/3/NPU0          Slice                      UP                
0/3/NPU1          Slice                      UP                
0/4/CPU0          NC57-24DD                  IOS XR RUN        NSHUT
0/4/NPU0          Slice                      UP                
0/4/NPU1          Slice                      UP                
0/RP0/CPU0        NC55-RP-E(Active)          IOS XR RUN        NSHUT
0/RP1/CPU0        NC55-RP-E(Standby)         IOS XR RUN        NSHUT
0/FC0             NC55-5508-FC2              OPERATIONAL       NSHUT
0/FC1             NC55-5508-FC2              OPERATIONAL       NSHUT
0/FC2             NC55-5508-FC2              OPERATIONAL       NSHUT
0/FC3             NC55-5508-FC2              OPERATIONAL       NSHUT
0/FC4             NC55-5508-FC2              OPERATIONAL       NSHUT
0/FC5             NC55-5508-FC2              OPERATIONAL       NSHUT
0/FT0             NC55-5508-FAN2             OPERATIONAL       NSHUT
0/FT1             NC55-5508-FAN2             OPERATIONAL       NSHUT
0/FT2             NC55-5508-FAN2             OPERATIONAL       NSHUT
0/PM0             NC55-PWR-3KW-DC            OPERATIONAL       NSHUT
0/PM1             NC55-PWR-3KW-DC            OPERATIONAL       NSHUT
0/PM2             NC55-PWR-3KW-DC            OPERATIONAL       NSHUT
0/PM3             NC55-PWR-3KW-DC            OPERATIONAL       NSHUT
0/PM4             NC55-PWR-3KW-DC            OPERATIONAL       NSHUT
0/PM5             NC55-PWR-3KW-DC            OPERATIONAL       NSHUT
0/PM6             NC55-PWR-3KW-DC            OPERATIONAL       NSHUT
0/PM7             NC55-PWR-3KW-DC            OPERATIONAL       NSHUT
0/SC0             NC55-SC                    OPERATIONAL       NSHUT
0/SC1             NC55-SC                    OPERATIONAL       NSHUT
`),
			platform: &platform{
				rps: &rps{
					rps: map[string]*rp{
						"0/RP0/CPU0": {
							isActive: true,
							location: "0/RP0/CPU0",
						},
						"0/RP1/CPU0": {
							isActive: false,
							location: "0/RP1/CPU0",
						},
					},
				},
				lcs: &lcs{
					lcs: map[string]*lc{
						"0/0/CPU0": {
							location: "0/0/CPU0",
						},
						"0/1/CPU0": {
							location: "0/1/CPU0",
						},
						"0/2/CPU0": {
							location: "0/2/CPU0",
						},
						"0/3/CPU0": {
							location: "0/3/CPU0",
						},
						"0/4/CPU0": {
							location: "0/4/CPU0",
						},
					},
				},
			},
		},
		{
			name: "spitfire",
			input: []byte(`Node              Type                     State                    Config state
--------------------------------------------------------------------------------
0/RP0/CPU0        8201-32FH(Active)        IOS XR RUN               NSHUT
0/PM0             PSU2KW-DCPI              OPERATIONAL              NSHUT
0/PM1             PSU2KW-DCPI              OPERATIONAL              NSHUT
0/FT0             FAN-1RU-PI               OPERATIONAL              NSHUT
0/FT1             FAN-1RU-PI               OPERATIONAL              NSHUT
0/FT2             FAN-1RU-PI               OPERATIONAL              NSHUT
0/FT3             FAN-1RU-PI               OPERATIONAL              NSHUT
0/FT4             FAN-1RU-PI               OPERATIONAL              NSHUT
0/FT5             FAN-1RU-PI               OPERATIONAL              NSHUT
`),
			platform: &platform{
				rps: &rps{
					rps: map[string]*rp{
						"0/RP0/CPU0": {
							isActive: true,
							location: "0/RP0/CPU0",
						},
					},
				},
				lcs: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := populatePlatformInfo(tt.input)
			if err != nil && !tt.fail {
				t.Fatalf("test suppoed to succeed but failed with error: %+v", err)
			}
			if err == nil && tt.fail {
				t.Fatal("test supposed to fail but succeeded")
			}
			if err != nil {
				return
			}
			if !reflect.DeepEqual(p, tt.platform) {
				t.Fatalf("computed and expected platforms do not match")
			}
		})
	}
}
