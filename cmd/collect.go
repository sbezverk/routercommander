package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/messenger"
	"github.com/sbezverk/routercommander/pkg/types"
)

func collect(r types.Router, commands *types.Commander, n messenger.Notifier) {
	defer wg.Done()
	processResult := false
	if commands.Collect != nil {
		processResult = commands.Collect.HealthCheck
	}
	glog.Infof("router %s: mode \"collect\" process results: %t", r.GetName(), processResult)
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

	for _, c := range commands.MainCommandGroup {
		glog.Infof("router %s: command: %q number of patterns: %d", r.GetName(), c.Cmd, len(c.Patterns))
		var results []*types.CmdResult
		var err error
		if c.ProcessResult {
			results, err = r.ProcessCommand(c, c.ProcessResult)
		} else {
			results, err = r.ProcessCommand(c, processResult)
		}
		if err != nil {
			glog.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
			return
		}
		if processResult || c.ProcessResult {
			matches, err := checkCommandOutput(results, c.Patterns)
			if err != nil {
				glog.Errorf("router %s: %+v", r.GetName(), err)
			} else {
				for _, m := range matches {
					glog.Infof("\t%s", m)
				}
			}
		}
	}
	glog.Errorf("router %s: collection completed successfully.", r.GetName())
}

func checkCommandOutput(results []*types.CmdResult, patterns []*types.Pattern) ([]string, error) {
	matches := make([]string, 0)
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
			for _, p := range patterns {
				if i := p.RegExp.FindIndex(b); i != nil {
					matches = append(matches, strings.Trim(string(b), "\n\r\t"))
				}
			}
		}
	}

	return matches, nil
}
