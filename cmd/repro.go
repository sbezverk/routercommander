package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/messenger"
	"github.com/sbezverk/routercommander/pkg/types"
)

func repro(r types.Router, commands *types.Commander, n messenger.Notifier) {
	defer wg.Done()

	if commands.Repro == nil {
		glog.Errorf("repro section is nil")
		return
	}
	iterations := 1
	interval := 0
	if commands.Repro.Times > 0 {
		iterations = commands.Repro.Times
	}
	if commands.Repro.Interval > 0 {
		interval = commands.Repro.Interval
	}

	glog.Infof("router %s: mode \"repro\", the command set will be executed %d time(s) with the interval of %d seconds", r.GetName(), iterations, interval)

	defer func() {
		if n != nil {
			glog.Infof("notification requested, attempting to send out the log for router %s", r.GetName())
			li := r.GetLogger()
			if li == nil {
				glog.Error("logger interface is nil")
				return
			}
			if err := n.Notify(li.GetLogFileName(), li.GetLog()); err != nil {
				glog.Errorf("failed to Notify with error: %+v", err)
				return
			}
			glog.Infof("routercommander sent log for router: %s", r.GetName())
		}
	}()

	triggered := false
	var err error
	for it := 0; it < iterations; it++ {
		glog.Infof("router %s: executing iteration - %d/%d:", r.GetName(), it+1, iterations)

		if triggered, err = processReproGroupOfCommands(r, commands.MainCommandGroup, it, commands.Repro); err != nil {
			glog.Errorf("router %s: reported repro failure with error: %+v", r.GetName(), err)
		}
		if triggered {
			break
		}
		glog.Infof("router %s: iteration - %d/%d completed,", r.GetName(), it+1, iterations)
		types.Delay(interval)
	}
	// If the issue was triggered, collecting commands needed to troubleshooting
	if triggered {
		glog.Infof("repro process on router %s succeeded triggering the failure condition, collecting post-mortem commands...", r.GetName())
		for _, c := range commands.Repro.PostMortemCommandGroup {
			_, err := r.ProcessCommand(c, true)
			if err != nil {
				glog.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
				return
			}
		}
	} else {
		glog.Infof("router %s: repro process has not succeeded triggering the failure condition", r.GetName())
	}
}

func processReproGroupOfCommands(r types.Router, commands []*types.Command, iteration int, repro *types.Repro) (bool, error) {
	for _, c := range commands {
		results, err := r.ProcessCommand(c, true)
		if err != nil {
			glog.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
			return false, fmt.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
		}
		for _, re := range results {
			for _, p := range c.Patterns {
				if i := p.RegExp.FindAllIndex(re.Result, -1); i != nil {
					// There are two possibilities to react, matching against a pattern and get out if the match is found,
					// OR if capture struct exists, to capture requested fields and follow Captured Values processing logic.
					if p.Capture == nil {
						// First case, when only matching is required
						glog.Errorf("router %s: found matching line: %q, command: %q", r.GetName(), strings.Trim(string(re.Result[i[0][0]:i[0][1]]), "\n\r\t"), re.Cmd)
						return true, nil
					}
					// Capture is mot nil, continue processing
					// In case there are multiple occurrences of the same string in the output and a specific occurrence is of interest
					indx := i[0]
					if p.Capture.Occurrence > 0 && p.Capture.Occurrence < len(i) {
						indx = i[p.Capture.Occurrence-1]
					}
					vm, err := getValue(re.Result, indx, p.Capture)
					if err != nil {
						return false, fmt.Errorf("failed to extract values defined by Capture tag for pattern: %s with error: %+v", p.PatternString, err)
					}
					// Storing extracted fields in pattern's Values per iterations map.
					p.ValuesStore[iteration] = vm

					if len(repro.CapturedValuesProcessing) == 0 {
						glog.Infof("Captured Values Processing is empty")
						continue
					}
					pc, ok := repro.CapturedValuesProcessing[c.Cmd]
					if !ok {
						glog.Infof("Command: %s is not found in CapturedValuesProcessing", c.Cmd)
						continue
					}
					pp, ok := pc[p.PatternString]
					if !ok {
						glog.Infof("pattern: %s for command: %s is not found in CapturedValuesProcessing", p.PatternString, c.Cmd)
						continue
					}
					triggered := false
				out:
					for f, v := range p.ValuesStore[iteration] {
						glog.Infof("Current iteration: %d value: %s for field: %d", iteration+1, v, f)
						fp, ok := pp[f]
						if !ok {
							glog.Infof("field: %d pattern: %s for command: %s is not found in CapturedValuesProcessing", f, p.PatternString, c.Cmd)
							continue
						}
						//						glog.Infof("><SB> Captured values %s operation: %s", v, fp.Operation)
						switch fp.Operation {
						case "compare_with_previous_neq":
							if iteration == 0 {
								continue
							}
							//							glog.Infof("><SB> Previous value: %s current value: %s", p.ValuesStore[iteration-1][f], v)
							if v != p.ValuesStore[iteration-1][f] {
								triggered = true
								break out
							}
						case "compare_with_previous_eq":
							if iteration == 0 {
								continue
							}
							//							glog.Infof("><SB> Previous value: %s current value: %s", p.ValuesStore[iteration-1][f], v)
							if v == p.ValuesStore[iteration-1][f] {
								triggered = true
								break out
							}
						case "compare_with_value_neq":
							//							glog.Infof("><SB> value: %s current value: %s", p.Capture.Values[f], v)
							if v != p.Capture.Values[f] {
								triggered = true
								break out
							}
						case "compare_with_value_eq":
							//							glog.Infof("><SB> value: %s current value: %s", p.ValuesStore[iteration-1][f], v)
							if v == p.Capture.Values[f] {
								triggered = true
								break out
							}
						default:
							return false, fmt.Errorf("unknown operation: %s for field: %d for pattern: %s", fp.Operation, fp.FieldNumber, p.PatternString)
						}
					}
					if triggered {
						if cmds, ok := repro.PerCmdPerPatternCommands[c.Cmd][p.PatternString]; ok {
							glog.Infof("Executing pattern specific commands...")
							if _, err := processReproGroupOfCommands(r, cmds, 0, nil); err != nil {
								return false, fmt.Errorf("failed to process pattern: %s commands with error: %+v", p.PatternString, err)
							}
						}
						return true, nil
					}
				}
			}
		}
	}

	return false, nil
}

func getValue(b []byte, index []int, capture *types.Capture) (map[int]interface{}, error) {
	previousEndLine, err := regexp.Compile(`(?m)$`)
	if err != nil {
		return nil, err
	}
	// First, find the start of the line with matching pattern
	sIndex := previousEndLine.FindAllIndex(b[:index[0]], -1)
	if sIndex == nil {
		return nil, fmt.Errorf("failed to find the start of line in data: %s", string(b[:index[0]]))
	}
	// Second, find  the end of the string with matching pattern
	eIndex := previousEndLine.FindIndex(b[sIndex[len(sIndex)-1][0]:])
	if eIndex == nil {
		return nil, fmt.Errorf("failed to find the end of line in data: %s", string(b[sIndex[len(sIndex)-1][0]:]))
	}
	s := string(b[sIndex[len(sIndex)-1][0] : sIndex[len(sIndex)-1][0]+eIndex[0]])
	// Splitting the resulting string using provided separator
	parts := strings.Split(s, capture.Separator)
	m := make(map[int]interface{})
	for _, f := range capture.FieldNumber {
		if len(parts) < f-1 {
			return nil, fmt.Errorf("failed to split string %s with separator %q to have field number %d", s, capture.Separator, f)
		}
		m[f] = strings.Trim(parts[f-1], " \n\t,")
	}

	return m, nil
}
