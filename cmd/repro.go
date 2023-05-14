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
		glog.Infof("router %s: executing iteration - %d/%d", r.GetName(), it+1, iterations)

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
		triggered := false
		results, err := r.ProcessCommand(c, true)
		if err != nil {
			glog.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
			return false, fmt.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
		}
		//		glog.Infof("><SB> Command: %q", c.Cmd)
		if repro == nil {
			continue
		}
		if repro.CommandTests == nil {
			continue
		}
		tests, ok := repro.CommandTests[c.Cmd]
		if !ok {
			// There is no test for the command, continue
			//			glog.Warningf("><SB> No tests found for Command: %q", c.Cmd)
			continue
		}
		glog.Warningf("><SB> Tests found for Command: %q", c.Cmd)
		test, ok := tests[c.TestID]
		if !ok {
			// The command specifies a non existing Test ID, continue
			//			glog.Warningf("><SB> No test ID %d is found for Command: %q", c.TestID, c.Cmd)
			continue
		}
		glog.Warningf("><SB> Test ID %d found for Command: %q", c.TestID, c.Cmd)
		// Test found, executing it against all instances of Result, the command can return
		// several instances of Result, when `times` keyword is more than 1
		if test.Pattern == nil {
			// Pattern is nil, nothing can be done, continue
			//			glog.Warningf("><SB> Pattern for command %q test id %d is nil", c.Cmd, test.ID)
			continue
		}
		if test.Pattern.RegExp == nil {
			// By some reason regular expression has not been initialized, attempting to compile it
			p, err := regexp.Compile(test.Pattern.PatternString)
			if err != nil {
				glog.Warningf("Fail to compile regular experssion for command %d test id %d with error: %+v", c.Cmd, c.TestID, err)
				continue
			}
			test.Pattern.RegExp = p
		}
		p := test.Pattern.RegExp
		// When test has one or more fields and all fields' checks should produce a tru condition, check_all_results is set to True
		// number variable is used to calculate a number of "true" condirtions
		number := 0
		for _, re := range results {
			if i := p.FindAllIndex(re.Result, -1); i != nil {
				glog.Infof("><SB> Pattern %s is found in result", p.String())
				// Test the number of hits of the patter, if does not match, considered the issue triggered
				if test.NumberOfOccurences != nil {
					glog.Infof("><SB> Number of instances test for command: %q result: %s", c.Cmd, string(re.Result))
					if *test.NumberOfOccurences != len(i) {
						glog.Infof("><SB> Number of expected occurrences %d does not match with the actual number of occurrence(s) %d", *test.NumberOfOccurences, len(i))
						triggered = true
						// since triggered breaking out of the loop
						// and execute per command post-mortem commands
						break
					} else {
						glog.Infof("><SB> Number of expected occurrences %d matches with the actual number of occurrence(s) %d", *test.NumberOfOccurences, len(i))
					}
					continue
				}
				glog.Warningf("><SB> Pattern for command %q test id %d has %d number of field(s)", c.Cmd, test.ID, len(test.Fields))
				if len(test.Fields) == 0 {
					// No fields related tests, but the match was found
					glog.Errorf("router %s: found matching line: %q, command: %q", r.GetName(), strings.Trim(string(re.Result[i[0][0]:i[0][1]]), "\n\r\t"), re.Cmd)
					triggered = true
					break
				}
				for _, field := range test.Fields {
					glog.Infof("\t><SB> processing field %d", field.FieldNumber)
					indx := i[0]
					if test.Occurrence > 0 && test.Occurrence < len(i) {
						indx = i[test.Occurrence-1]
					}
					vm, err := getValue(re.Result, indx, field, test.Separator)
					if err != nil {
						return false, fmt.Errorf("failed to extract value field id %d for command %q test id %d with error: %+v", field.FieldNumber, c.Cmd, test.ID, err)
					}
					// Storing extracted fields in pattern's Values per iterations map.
					if _, ok := test.ValuesStore[iteration]; !ok {
						test.ValuesStore[iteration] = make(map[int]interface{})
					}
					test.ValuesStore[iteration] = map[int]interface{}{field.FieldNumber: vm}
					glog.Infof("\t><SB> Extracted value: %s field operation: %s", vm, field.Operation)
					switch field.Operation {
					case "compare_with_previous_neq":
						if iteration == 0 {
							continue
						}
						//							glog.Infof("><SB> Previous value: %s current value: %s", p.ValuesStore[iteration-1][f], v)
						if vm != test.ValuesStore[iteration-1][field.FieldNumber] {
							triggered = true
							number++
						}
					case "compare_with_previous_eq":
						if iteration == 0 {
							continue
						}
						//							glog.Infof("><SB> Previous value: %s current value: %s", p.ValuesStore[iteration-1][f], v)
						if vm != test.ValuesStore[iteration-1][field.FieldNumber] {
							triggered = true
							number++
						}
					case "compare_with_value_neq":
						glog.Infof("><SB> value: %s current value: %s", field.Value, vm)
						if vm != field.Value {
							triggered = true
							number++
						}
					case "compare_with_value_eq":
						glog.Infof("><SB> value: %s current value: %s", field.Value, vm)
						if vm == field.Value {
							triggered = true
							number++
						}
					default:
						return false, fmt.Errorf("unknown operation: %s for field number: %d for command: %q test id: %d",
							field.Operation, field.FieldNumber, c.Cmd, test.ID)
					}
				}
			} else {
				glog.Infof("><SB> Pattern %s is not found in result output: %s", p.String(), string(re.Result))
			}
			if triggered {
				if !test.CheckAllResults {
					return true, nil
				}
				if len(test.Fields) == number {
					// All checkes returned "triggered" condition
					if len(test.IfTriggeredCommands) != 0 {
						glog.Infof("Executing test id %d specific commands...", test.ID)
						if _, err := processReproGroupOfCommands(r, test.IfTriggeredCommands, 0, nil); err != nil {
							return false, fmt.Errorf("failed to process pattern: %s commands with error: %+v", test.Pattern.PatternString, err)
						}
						return true, nil
					}
				} else {
					// Since not all checkes returned "triggered" condition, returning triggered == false
					return false, nil
				}
			}
		}
	}

	return false, nil
}

func getValue(b []byte, index []int, field *types.Field, separator string) (string, error) {
	if separator == "" {
		separator = " "
	}
	endLine, err := regexp.Compile(`(?m)$`)
	if err != nil {
		return "", err
	}
	startLine, err := regexp.Compile(`(?m)^`)
	if err != nil {
		return "", err
	}
	// First, find the end of the line with matching pattern
	eIndex := endLine.FindIndex(b[index[0]:])
	if eIndex == nil {
		return "", fmt.Errorf("failed to find the end of line in data: %s", string(b[index[0]:]))
	}
	en := index[0] + eIndex[0]
	// Second, find the start of the string with matching pattern
	sIndex := startLine.FindAllIndex(b[:en-1], -1)
	if sIndex == nil {
		return "", fmt.Errorf("failed to find the start of line in data: %s", string(b[:index[0]]))
	}
	st := sIndex[len(sIndex)-1][0]
	s := string(b[st:en])
	// Splitting the resulting string using provided separator
	sepreg, err := regexp.Compile("[" + separator + "]+")
	if err != nil {
		return "", err
	}
	parts := sepreg.Split(s, -1)
	if len(parts) < field.FieldNumber-1 {
		return "", fmt.Errorf("failed to split string %s with separator %q to have field number %d", s, separator, field.FieldNumber)
	}

	return strings.Trim(parts[field.FieldNumber-1], " \n\t,"), nil
}
