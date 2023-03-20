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

		if triggered, err = processReproGroupOfCommands(r, commands.MainCommandGroup); err != nil {
			glog.Errorf("router %s: reported repro failure with error: %+v", r.GetName(), err)
		}

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
		return
	}
	if !triggered {
		glog.Infof("router %s: repro process has not succeeded triggering the failure condition", r.GetName())
		return
	}
}

func processReproGroupOfCommands(r types.Router, commands []*types.Command) (bool, error) {
	for _, c := range commands {
		results, err := r.ProcessCommand(c, true)
		if err != nil {
			glog.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
			return false, fmt.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
		}
		for _, re := range results {
			for _, p := range c.Patterns {
				if i := p.RegExp.FindIndex(re.Result); i != nil {
					// There are to possibilities to react, matching against a pattern and get out if the match is found,
					// OR if capture struct exists, to capture requested field and compare with the previous value, if values are not equal, then get out
					// otherwise continue
					if p.Capture == nil {
						// First case, when only matching is required
						glog.Errorf("router %s: found matching line: %q, command: %q", r.GetName(), strings.Trim(string(re.Result[i[0]:i[1]]), "\n\r\t"), re.Cmd)
						return true, nil
					}
					// 			if it == 0 {
					// 				// If it is first iteration just storing  first captured value
					// 				v, err := getValue(re.Result, i, p.Capture)
					// 				if err != nil {
					// 					glog.Errorf("failed to extract value of field %d, separator: %q from data: %q with error: %+v", p.Capture.FieldNumber, p.Capture.Separator, string(re.Result), err)
					// 					break out
					// 				}
					// 				p.Capture.Value = v
					// 				continue
					// 			}
					// 			v, err := getValue(re.Result, i, p.Capture)
					// 			if err != nil {
					// 				glog.Errorf("failed to extract value of field %d, separator: %q from data: %q with error: %+v", p.Capture.FieldNumber, p.Capture.Separator, string(re.Result), err)
					// 				break out
					// 			}
					// 			if p.Capture.Value != v {
					// 				triggered = true
					// 				glog.Infof("router %s: detected change of value, previous value %+v current value %+v", r.GetName(), p.Capture.Value, v)
					// 				break out
					// 			}
				}

				// TODO (sbezverk) Further action depends of the logic coded above

				if p.PatternCommands != nil {
					if _, err := processReproGroupOfCommands(r, p.PatternCommands); err != nil {
						return false, fmt.Errorf("failed to process pattern: %s commands with error: %+v", p.PatternString, err)
					}
				}
				return true, nil
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