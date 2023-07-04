package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/messenger"
	"github.com/sbezverk/routercommander/pkg/types"
)

func process(r types.Router, commander *types.Commander, n messenger.Notifier) {
	iterations := 1
	interval := 0
	stopWhenTriggered := true
	if commander.Repro != nil {
		if commander.Repro.Times > 0 {
			iterations = commander.Repro.Times
		}
		if commander.Repro.Interval > 0 {
			interval = commander.Repro.Interval
		}
		stopWhenTriggered = commander.Repro.StopWhenTriggered
	}
	glog.Infof("router %s: command set will be executed %d time(s) with the interval of %d seconds", r.GetName(), iterations, interval)
	// Setting up the notification to be sent at the end of execution
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
	// Setting up to inform main function that the processing is completed
	defer wg.Done()

	triggered := false
	var err error
	for it := 0; it < iterations; it++ {
		if iterations > 1 {
			glog.Infof("router %s: executing iteration - %d/%d", r.GetName(), it+1, iterations)
		}
		if triggered, err = processMainGroupOfCommands(r, commander, it); err != nil {
			glog.Errorf("router %s: reported repro failure with error: %+v", r.GetName(), err)
		}
		if triggered {
			// If the issue was triggered, collecting common Repro.PostMortemCommandGroup commands needed to troubleshooting
			glog.Infof("repro process on router %s succeeded triggering the failure condition, collecting post-mortem commands...", r.GetName())
			for _, c := range commander.Repro.PostMortemCommandGroup {
				_, err := r.ProcessCommand(c, true)
				if err != nil {
					glog.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
					return
				}
			}
			if stopWhenTriggered {
				break
			}
		}
		glog.Infof("router %s: iteration - %d/%d completed,", r.GetName(), it+1, iterations)
		types.Delay(interval)
	}
	if triggered {
		glog.Infof("repro process on router %s succeeded triggering the failure condition, collecting post-mortem commands...", r.GetName())
	} else {
		glog.Infof("router %s: repro process has not succeeded triggering the failure condition", r.GetName())
	}
}

func processMainGroupOfCommands(r types.Router, commander *types.Commander, iteration int) (bool, error) {
	pr := false
	stopWhenTriggered := false
	if commander.Collect != nil {
		pr = commander.Collect.ProcessResult
	}
	if commander.Repro != nil {
		pr = true
		stopWhenTriggered = commander.Repro.StopWhenTriggered
	}
	triggered := false
	for _, c := range commander.MainCommandGroup {
		var results []*types.CmdResult
		var err error
		if c.ProcessResult {
			results, err = r.ProcessCommand(c, c.ProcessResult)
		} else {
			results, err = r.ProcessCommand(c, pr)
		}
		if err != nil {
			return false, fmt.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
		}
		if pr || c.ProcessResult {
			matches, err := matchPatterns(results, c.Patterns)
			if err != nil {
				glog.Errorf("router %s: %+v", r.GetName(), err)
			} else {
				c.CommandResult.PatternMatch = matches
			}
		}
		if len(c.CommandResult.PatternMatch) != 0 {
			for p, ms := range c.CommandResult.PatternMatch {
				glog.Infof("router %s: command %q pattern: %s", r.GetName(), c.Cmd, p)
				for _, m := range ms {
					glog.Infof("\t%s", m)
				}
			}
		}
		// If no tests to do, just continue pattern matching
		if commander.Tests == nil {
			continue
		}
		// Check if there are tests for the current command
		tests, ok := commander.CommandsWithTests[c.Cmd]
		if !ok {
			continue
		}
		triggers, err := runTests(r, results, c.TestIDs, tests, iteration, stopWhenTriggered)
		if err != nil {
			return false, fmt.Errorf("router %s: failed to execute tests for command %q with error %+v", r.GetName(), c.Cmd, err)
		}
		c.CommandResult.TriggeredTest = triggers
		if len(triggers) > 0 {
			triggered = true
		}
		if len(c.CommandResult.TriggeredTest) != 0 {
			glog.Infof("router %s: command %q triggered test ids: %v", r.GetName(), c.Cmd, c.CommandResult.TriggeredTest)
		}
		if stopWhenTriggered && triggered {
			return true, nil
		}
	}

	return triggered, nil
}

func runTests(r types.Router, results []*types.CmdResult, toRun []int, tests *types.Tests, iteration int, stopWhenTriggered bool) ([]int, error) {
	triggers := make([]int, 0)

out:
	for _, tr := range toRun {
		t, ok := tests.Tests[tr]
		if !ok {
			continue
		}
		triggered, err := runTest(results, t, iteration)
		if err != nil {
			return nil, err
		}
		if triggered {
			// Since test id is trigger, executing the list of commands for the test ID
			if len(t.IfTriggeredCommands) != 0 {
				if err := processCommandsIfTriggered(r, t.IfTriggeredCommands); err != nil {
					return nil, err
				}
			}
			triggers = append(triggers, tr)
			if stopWhenTriggered {
				break out
			}
			triggered = false
		}
	}

	return triggers, nil
}

func runTest(results []*types.CmdResult, t *types.Test, iteration int) (bool, error) {
	triggered := false
	if len(results) == 0 {
		return false, nil
	}
	for _, re := range results {
		glog.Infof("Executing Test ID %d for Command: %q", t.ID, re.Cmd)
		// Test found, executing it against all instances of Result, the command can return
		// several instances of Result, when `times` keyword is more than 1
		if t.Pattern == nil {
			// Pattern is nil, nothing can be done, continue
			glog.Warningf("Pattern for command %q test id %d is nil", re.Cmd, t.ID)
			continue
		}
		if t.Pattern.RegExp == nil {
			// By some reason regular expression has not been initialized, attempting to compile it
			p, err := regexp.Compile(t.Pattern.PatternString)
			if err != nil {
				glog.Warningf("Fail to compile regular experssion for command %s test id %d with error: %+v", re.Cmd, t.ID, err)
				continue
			}
			t.Pattern.RegExp = p
		}
		p := t.Pattern.RegExp
		matches := p.FindAllIndex(re.Result, -1)
		if matches == nil {
			glog.Warningf("Test ID: %d Command: %q pattern %q is not found", t.ID, re.Cmd, p.String())
			continue
		}
		// Test the number of hits of the patter, if does not match, considered the issue triggered
		if t.NumberOfOccurences != nil {
			if *t.NumberOfOccurences != len(matches) {
				return true, nil
			}
		}
		if len(matches) <= t.Occurrence-1 {
			// Since requested occurence number does not exists, returning false
			glog.Warningf("Test ID: %d Command: %q requested occurence %d is more than number of matches %d", t.ID, re.Cmd, t.Occurrence, len(matches))
			return false, nil
		}
		if len(t.Fields) == 0 {
			// No fields related tests, but the match was found
			return true, nil
		}
		// When multiple instances of match exists and not specific occurence number is requested, then
		// all instances must be checked for triggerring conditions. If check_all_results is true then all checks must
		// return "triggered"
		nm := len(matches)
		indx := 0
		if t.Occurrence != 0 {
			nm = t.Occurrence
			indx = t.Occurrence - 1
		}
		perMatchTrigger := 0
		for ; indx < nm; indx++ {
			// When test has one or more fields and all fields' checks should produce a tru condition, check_all_results is set to True
			// number variable is used to calculate a number of "true" condirtions
			perFieldTrigger := 0
			for _, field := range t.Fields {
				triggered = false
				vm, err := getValue(re.Result, matches[indx], field, t.Separator)
				if err != nil {
					return false, fmt.Errorf("failed to extract value field id %d for command %q test id %d with error: %+v", field.FieldNumber, re.Cmd, t.ID, err)
				}
				// Storing extracted fields in pattern's Values per iterations map.
				if _, ok := t.ValuesStore[iteration]; !ok {
					t.ValuesStore[iteration] = make(map[int]interface{})
				}
				t.ValuesStore[iteration] = map[int]interface{}{field.FieldNumber: vm}
				trgrd, err := check(field.Operation, iteration, field, t.ValuesStore)
				if err != nil {
					return false, err
				}
				if trgrd {
					perFieldTrigger++
				}
			}
			if t.CheckAllResults {
				// Need to check all fields, only then the match[indx] considered as a trigger
				if perFieldTrigger == len(t.Fields) {
					return true, nil
				}
			} else {
				perMatchTrigger++
			}
		}
		if t.CheckAllResults {
			// Need to check all matches, only then the test considered as the trigger
			if perMatchTrigger == len(matches) {
				return true, nil
			}
		} else {
			perMatchTrigger++
		}
	}

	return triggered, nil
}

func processCommandsIfTriggered(r types.Router, commands []*types.Command) error {
	for _, c := range commands {
		_, err := r.ProcessCommand(c, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func matchPatterns(results []*types.CmdResult, patterns []*types.Pattern) (map[string][]string, error) {
	matches := make(map[string][]string)
	for _, re := range results {
		reader := bufio.NewReader(bytes.NewReader(re.Result))
		done := false
		for !done {
			b, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					return nil, fmt.Errorf("failed to read command: %q result buffer with error: %v", re.Cmd, err)
				}
				done = true
			}
			var m []string
			var ok bool
			for _, p := range patterns {
				if i := p.RegExp.FindIndex(b); i != nil {
					m, ok = matches[p.PatternString]
					if !ok {
						m = make([]string, 0)
					}
					m = append(m, strings.Trim(string(b), "\n\r\t"))
					matches[p.PatternString] = m
				}
			}
		}
	}

	return matches, nil
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
