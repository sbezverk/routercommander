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
	var test *types.CommandTest
	triggered := false
	for _, c := range commands {
		results, err := r.ProcessCommand(c, true)
		if err != nil {
			glog.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
			return false, fmt.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
		}
		if len(results) == 0 {
			return false, fmt.Errorf("command %q prodiced no results", c.Cmd)
		}
		re := results[0]
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
		// glog.Warningf("><SB> Tests found for Command: %q", c.Cmd)
		test, ok = tests[c.TestID]
		if !ok {
			// The command specifies a non existing Test ID, continue
			//			glog.Warningf("><SB> No test ID %d is found for Command: %q", c.TestID, c.Cmd)
			continue
		}
		glog.Infof("Executing Test ID %d for Command: %q", c.TestID, c.Cmd)
		// Test found, executing it against all instances of Result, the command can return
		// several instances of Result, when `times` keyword is more than 1
		if test.Pattern == nil {
			// Pattern is nil, nothing can be done, continue
			glog.Warningf("Pattern for command %q test id %d is nil", c.Cmd, test.ID)
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
		matches := p.FindAllIndex(re.Result, -1)
		if matches == nil {
			glog.Warningf("Pattern %s is not found in result output: %s", p.String(), string(re.Result))
			continue
		}
		// glog.Infof("><SB> Pattern %s is found in result", p.String())
		// Test the number of hits of the patter, if does not match, considered the issue triggered
		if test.NumberOfOccurences != nil {
			// glog.Infof("><SB> Number of instances test for command: %q result: %s", c.Cmd, string(re.Result))
			if *test.NumberOfOccurences != len(matches) {
				glog.V(5).Infof("Number of expected occurrences %d does not match with the actual number of occurrence(s) %d", *test.NumberOfOccurences, len(matches))
				triggered = true
				// since triggered breaking out of the loop
				// and execute per command post-mortem commands
				break
			} else {
				glog.V(5).Infof("Number of expected occurrences %d matches with the actual number of occurrence(s) %d", *test.NumberOfOccurences, len(matches))
			}
			continue
		}
		// glog.Warningf("><SB> Pattern for command %q test id %d was found %d time(s)", c.Cmd, test.ID, len(i))
		if len(matches) <= test.Occurrence-1 {
			// Test requesting to check specific occurrence number, but the number of found occurrences is less
			glog.Infof("router %s: found matching line: %q, command: %q but the requested occurrence %d is more than the number of found occurrences %d",
				r.GetName(), strings.Trim(string(re.Result[matches[0][0]:matches[0][1]]), "\n\r\t"), re.Cmd, test.Occurrence, len(matches))
			triggered = true
			break
		}
		// glog.Warningf("><SB> Pattern for command %q test id %d has %d number of field(s)", c.Cmd, test.ID, len(test.Fields))
		if len(test.Fields) == 0 {
			// No fields related tests, but the match was found
			glog.Infof("router %s: found matching line: %q, command: %q", r.GetName(), strings.Trim(string(re.Result[matches[0][0]:matches[0][1]]), "\n\r\t"), re.Cmd)
			triggered = true
			break
		}
		// When multiple instances of match exists and not specific occurence number is requested, then
		// all instances must be checked for triggerring conditions. If check_all_results is true then all checks must
		// return "triggered"
		nm := len(matches)
		indx := 0
		if test.Occurrence != 0 {
			nm = test.Occurrence
			indx = test.Occurrence - 1
		}
		perMatchTrigger := 0
		for ; indx < nm; indx++ {
			// When test has one or more fields and all fields' checks should produce a tru condition, check_all_results is set to True
			// number variable is used to calculate a number of "true" condirtions
			perFieldTrigger := 0
			for _, field := range test.Fields {
				triggered = false
				vm, err := getValue(re.Result, matches[indx], field, test.Separator)
				if err != nil {
					return false, fmt.Errorf("failed to extract value field id %d for command %q test id %d with error: %+v", field.FieldNumber, c.Cmd, test.ID, err)
				}
				// Storing extracted fields in pattern's Values per iterations map.
				if _, ok := test.ValuesStore[iteration]; !ok {
					test.ValuesStore[iteration] = make(map[int]interface{})
				}
				test.ValuesStore[iteration] = map[int]interface{}{field.FieldNumber: vm}
				trgrd, err := check(field.Operation, iteration, field, test.ValuesStore)
				if err != nil {
					return false, err
				}
				if trgrd {
					perFieldTrigger++
				}
			}
			if test.CheckAllResults {
				// Need to check all fields, only then the match[indx] considered as a trigger
				if perFieldTrigger == len(test.Fields) {
					perMatchTrigger++
				}
			} else {
				perMatchTrigger++
			}
		}
		if test.CheckAllResults {
			// Need to check all matches, only then the test considered as the trigger
			if perMatchTrigger == len(matches) {
				triggered = true
				break
			}
		} else {
			triggered = true
			break
		}
	}
	if triggered {
		if test != nil {
			if len(test.IfTriggeredCommands) != 0 {
				glog.Infof("Executing test id %d specific commands...", test.ID)
				if _, err := processReproGroupOfCommands(r, test.IfTriggeredCommands, 0, nil); err != nil {
					return false, fmt.Errorf("failed to process pattern: %s commands with error: %+v", test.Pattern.PatternString, err)
				}
			}
		}
		return true, nil
	}

	return false, nil
}

func check(op string, iteration int, field *types.Field, store map[int]map[int]interface{}) (bool, error) {
	switch op {
	case "compare_with_previous_neq":
		if iteration == 0 {
			return false, nil
		}
		glog.Infof("Previous value: %s current value: %s", store[iteration-1][field.FieldNumber], store[iteration][field.FieldNumber])
		if store[iteration][field.FieldNumber] != store[iteration-1][field.FieldNumber] {
			return true, nil
		}
	case "compare_with_previous_eq":
		if iteration == 0 {
			return false, nil
		}
		glog.Infof("Previous value: %s current value: %s", store[iteration-1][field.FieldNumber], store[iteration][field.FieldNumber])
		if store[iteration][field.FieldNumber] != store[iteration-1][field.FieldNumber] {
			return true, nil
		}
	case "compare_with_value_neq":
		glog.Infof("Expected value: %s current value: %s", field.Value, store[iteration][field.FieldNumber])
		if store[iteration][field.FieldNumber] != field.Value {
			return true, nil
		}
	case "compare_with_value_eq":
		glog.Infof("Expected value: %s current value: %s", field.Value, store[iteration][field.FieldNumber])
		if store[iteration][field.FieldNumber] == field.Value {
			return true, nil
		}
	case "contain_substring":
		glog.Infof("substring value: %s current value: %s", field.Value, store[iteration][field.FieldNumber])
		if !strings.Contains(store[iteration][field.FieldNumber].(string), field.Value) {
			return true, nil
		}
	case "not_contain_substring":
		glog.Infof("substring value: %s current value: %s", field.Value, store[iteration][field.FieldNumber])
		if strings.Contains(store[iteration][field.FieldNumber].(string), field.Value) {
			return true, nil
		}
	default:
		return false, fmt.Errorf("unknown operation: %s for field number: %d",
			field.Operation, field.FieldNumber)
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
