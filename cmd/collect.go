package main

import (
	"strings"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/messenger"
	"github.com/sbezverk/routercommander/pkg/types"
)

func collect(r types.Router, commands *types.Commander, n messenger.Notifier) {
	defer wg.Done()
	glog.Infof("router %s: mode \"collect\"", r.GetName())
	processResult := commands.Collect.HealthCheck

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
			for _, re := range results {
				for _, p := range c.Patterns {
					if i := p.RegExp.FindIndex(re.Result); i != nil {
						glog.Warningf("router %s: found matching line: %q, command: %q", r.GetName(), strings.Trim(string(re.Result[i[0]:i[1]]), "\n\r\t"), re.Cmd)
					}
				}
			}
		}
		glog.Errorf("router %s: collection completed successfully.", r.GetName())
	}
}
