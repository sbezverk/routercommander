package types

import (
	"os/exec"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/log"
)

var _ Router = &localRouter{}

type localRouter struct {
	name   string
	logger log.Logger
}

func (l *localRouter) GetName() string {
	return l.name
}

func (l *localRouter) GetLogger() log.Logger {
	return l.logger
}

func (l *localRouter) ProcessCommand(cmd *Command, collectResult bool) ([]*CmdResult, error) {
	c := cmd.Cmd
	results := make([]*CmdResult, 0)

	// TODO (sbezverk) Add some sanity check for this timer

	if cmd.WaitBefore != 0 {
		Delay(cmd.WaitBefore)
	}
	commandTimeout := DefaultCommandTimeout
	if cmd.CmdTimeout != 0 {
		commandTimeout = cmd.CmdTimeout
	}
	var err error
	rs, err := l.sendCommand(c, cmd.Times, cmd.Interval, cmd.Debug, commandTimeout)
	if err != nil {
		return nil, err
	}
	if collectResult {
		results = append(results, rs...)
	}
	if cmd.WaitAfter != 0 {
		Delay(cmd.WaitAfter)
	}

	return results, nil
}

func (l *localRouter) sendCommand(cmd string, times, interval int, debug bool, commandTimeout int) ([]*CmdResult, error) {
	if glog.V(5) {
		if interval == 0 || times == 0 {
			glog.Infof("Sending command: %q to router: %q", cmd, l.GetName())
		} else {
			glog.Infof("Sending command: %q, %d times with interval of %d seconds to router: %q", cmd, times, interval, l.GetName())
		}
	}
	if interval == 0 || times == 0 {
		b, err := l.GetData(cmd, debug, commandTimeout)
		if err != nil {
			return nil, err
		}
		if l.logger != nil {
			l.logger.Log([]byte("=========> " + cmd + "\n"))
			l.logger.Log(b)
			l.logger.Log([]byte("\n\n"))
		}
		return []*CmdResult{
			{
				Cmd:    cmd,
				Result: b,
			},
		}, err
	}
	results := make([]*CmdResult, 0)
	ticker := time.NewTicker(time.Second * time.Duration(interval))
	defer ticker.Stop()
	for t := 0; t < times; t++ {
		b, err := l.GetData(cmd, debug, commandTimeout)
		if err != nil {
			return nil, err
		}
		if l.logger != nil {
			l.logger.Log([]byte("=========> " + cmd + "\n"))
			l.logger.Log(b)
			l.logger.Log([]byte("\n\n"))
		}
		results = append(results, &CmdResult{
			Cmd:    cmd,
			Result: b,
		})
		<-ticker.C
	}

	return results, nil
}

func (l *localRouter) Close() {
}

func (l *localRouter) GetData(cmd string, debug bool, commandTimeout int) ([]byte, error) {
	parts := strings.Split(cmd, " ")
	var c *exec.Cmd
	if len(parts) > 1 {
		// commands has parameters
		if parts[0] == "bash" && len(parts) > 2 {
			// Special case of executing bash internal command
			glog.Infof("><SB> special case for bash")
			c = exec.Command(parts[0], parts[1], strings.Join(parts[2:], " "))
		} else {
			c = exec.Command(parts[0], parts[1:]...)
		}
	} else {
		c = exec.Command(parts[0])
	}

	glog.Infof("><SB> command: %+v", c.String())

	return c.Output()
}

func NewLocalRouter(router string, li log.Logger) Router {
	return &localRouter{
		name:   router,
		logger: li,
	}
}
