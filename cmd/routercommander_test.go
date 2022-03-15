package main

import (
	"fmt"
	"testing"
)

var (
	showPlatform = `
Wed Jan 16 20:42:10.834 EST
Node          Type              PLIM               State           Config State
------------- ----------------- ------------------ --------------- ---------------
0/0/CPU0      MSC               Jacket Card        IOS XR RUN      PWR,NSHUT,MON
0/0/0         MSC(SPA)          OC192RPR-XFP       OK              PWR,NSHUT,MON
0/0/1         MSC(SPA)          OC192RPR-XFP       OK              PWR,NSHUT,MON
0/0/2         MSC(SPA)          OC192RPR-XFP       OK              PWR,NSHUT,MON
0/0/4         MSC(SPA)          1x10GE             OK              PWR,NSHUT,MON
0/1/CPU0      MSC-B             Jacket Card        IOS XR RUN      PWR,NSHUT,MON
0/1/0         MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/1/1         MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/1/2         MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/1/4         MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/1/5         MSC-B(SPA)        5X1GE              OK              PWR,NSHUT,MON
0/2/CPU0      MSC               1OC768-POS         IOS XR RUN      PWR,NSHUT,MON
0/3/CPU0      MSC               1OC768-POS         IOS XR RUN      PWR,NSHUT,MON
0/4/CPU0      MSC               Jacket Card        IOS XR RUN      PWR,NSHUT,MON
0/4/0         MSC(SPA)          OC192RPR-XFP       OK              PWR,NSHUT,MON
0/4/1         MSC(SPA)          OC192RPR-XFP       OK              PWR,NSHUT,MON
0/4/2         MSC(SPA)          1x10GE             OK              PWR,NSHUT,MON
0/4/4         MSC(SPA)          OC192RPR-XFP       OK              PWR,NSHUT,MON
0/5/CPU0      MSC-B             Jacket Card        IOS XR RUN      PWR,NSHUT,MON
0/5/0         MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/5/1         MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/5/2         MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/6/CPU0      MSC               1OC768-POS         IOS XR RUN      PWR,NSHUT,MON
0/8/CPU0      MSC               Jacket Card        IOS XR RUN      PWR,NSHUT,MON
0/8/0         MSC(SPA)          OC192RPR-XFP       OK              PWR,NSHUT,MON
0/8/1         MSC(SPA)          4XOC48-POS         OK              PWR,NSHUT,MON
0/8/2         MSC(SPA)          1x10GE             OK              PWR,NSHUT,MON
0/8/4         MSC(SPA)          4XOC3-POS          OK              PWR,NSHUT,MON
0/8/5         MSC(SPA)          4XOC3-POS          OK              PWR,NSHUT,MON
0/9/CPU0      MSC-B             Jacket Card        IOS XR RUN      PWR,NSHUT,MON
0/9/0         MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/9/1         MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/9/4         MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/9/5         MSC-B(SPA)        5X1GE              OK              PWR,NSHUT,MON
0/10/CPU0     MSC-B             Jacket Card        IOS XR RUN      PWR,NSHUT,MON
0/10/0        MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/10/1        MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/10/2        MSC-B(SPA)        1x10GE             OK              PWR,NSHUT,MON
0/11/CPU0     MSC-B             Jacket Card        IOS XR RUN      PWR,NSHUT,MON
0/11/0        MSC-B(SPA)        4XOC48-POS         OK              PWR,NSHUT,MON
0/11/1        MSC-B(SPA)        2XOC48-POS         OK              PWR,NSHUT,MON
0/12/CPU0     MSC               Jacket Card        IOS XR RUN      PWR,NSHUT,MON
0/12/0        MSC(SPA)          OC192RPR-XFP       OK              PWR,NSHUT,MON
0/12/1        MSC(SPA)          OC192RPR-XFP       OK              PWR,NSHUT,MON
0/12/2        MSC(SPA)          OC192RPR-XFP       OK              PWR,NSHUT,MON
0/12/4        MSC(SPA)          4XOC48-POS         OK              PWR,NSHUT,MON
0/13/CPU0     MSC-B             1OC768-POS         IOS XR RUN      PWR,NSHUT,MON
0/14/CPU0     MSC-B             1OC768-POS         IOS XR RUN      PWR,NSHUT,MON
0/15/CPU0     MSC               1OC768-POS         IOS XR RUN      PWR,NSHUT,MON
0/RP0/CPU0    RP(Standby)       N/A                IOS XR RUN      PWR,NSHUT,MON
0/RP1/CPU0    RP(Active)        N/A                IOS XR RUN      PWR,NSHUT,MON
1/0/CPU0      FP-140G           14-10GbE           IOS XR RUN      PWR,NSHUT,MON
1/1/CPU0      FP-140G           1-100GbE           IOS XR RUN      PWR,NSHUT,MON
1/2/CPU0      FP-140G           1-100GbE           IOS XR RUN      PWR,NSHUT,MON
1/3/CPU0      FP-X              40-10GbE           IOS XR RUN      PWR,NSHUT,MON
1/5/CPU0      FP-140G           1-100GbE           IOS XR RUN      PWR,NSHUT,MON
1/6/CPU0      FP-140G           14-10GbE           IOS XR RUN      PWR,NSHUT,MON
1/7/CPU0      FP-140G           1-100GbE           IOS XR RUN      PWR,NSHUT,MON
1/8/CPU0      FP-140G           1-100GbE           IOS XR RUN      PWR,NSHUT,MON
1/10/CPU0     FP-140G           14-10GbE           IOS XR RUN      PWR,NSHUT,MON
1/11/CPU0     MSC               1OC768-DWDM        IOS XR RUN      PWR,NSHUT,MON
1/12/CPU0     FP-140G           1-100GbE           IOS XR RUN      PWR,NSHUT,MON
1/14/CPU0     FP-140G           1-100GbE           IOS XR RUN      PWR,NSHUT,MON
1/15/CPU0     FP-140G           14-10GbE           IOS XR RUN      PWR,NSHUT,MON
1/RP0/CPU0    RP(Active)        N/A                IOS XR RUN      PWR,NSHUT,MON
1/RP1/CPU0    RP(Standby)       N/A                IOS XR RUN      PWR,NSHUT,MON
2/0/CPU0      FP-X              40-10GbE           IOS XR RUN      PWR,NSHUT,MON
2/1/CPU0      FP-X              4-100GbE           IOS XR RUN      PWR,NSHUT,MON
2/3/CPU0      FP-X              4-100GbE           IOS XR RUN      PWR,NSHUT,MON
2/4/CPU0      FP-X              4-100GbE           IOS XR RUN      PWR,NSHUT,MON
2/5/CPU0      FP-X              4-100GbE           IOS XR RUN      PWR,NSHUT,MON
2/8/CPU0      FP-X              4-100GbE           IOS XR RUN      PWR,NSHUT,MON
2/9/CPU0      FP-X              4-100GbE           IOS XR RUN      PWR,NSHUT,MON
2/11/CPU0     FP-X              4-100GbE           IOS XR RUN      PWR,NSHUT,MON
2/13/CPU0     FP-X              4-100GbE           IOS XR RUN      PWR,NSHUT,MON
2/RP0/CPU0    RP(Standby)       N/A                IOS XR RUN      PWR,NSHUT,MON
2/RP1/CPU0    RP(Active)        N/A                IOS XR RUN      PWR,NSHUT,MON 
`
)

func TestParsing(t *testing.T) {

	result, err := parseReply([]byte(showPlatform))
	if err != nil {
		t.Fatalf("Parsing failed with error: %v", err)
	}
	if result == nil {
		t.Fatalf("Test failed as result is not supposed to be nil")
	}
	fmt.Printf("fp-x cards:%v", result)
}

func TestInventoryParsing(t *testing.T) {
	result, _ := parseReply([]byte(showPlatform))
	_, err := parseInventory(result)
	if err != nil {
		t.Errorf("Failed to parse inventory with error: %+v", err)
	}
}

func TestFPX(t *testing.T) {
	slot := "0/2/CPU0"
	want := "run on -f node0_2_cpu0 pcie_cfrw -w 0 0 0 2 2 1"
	cmd := "run on -f %s pcie_cfrw -w 0 0 0 2 2 1"
	got := fpx(cmd, slot)
	if want != got {
		t.Errorf("Test FPX failed wanted: %s but got: %s", want, got)
	}
}
