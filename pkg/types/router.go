package types

import (
	"bytes"
	"fmt"
	"io"
	"strconv"

	"regexp"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/log"
	"github.com/sbezverk/routercommander/pkg/patterns"
	"golang.org/x/crypto/ssh"
)

// Router interface is a collection of methods

const (
	DefaultCommandTimeout = 120
)

type Router interface {
	GetName() string
	GetData(string, bool, int) ([]byte, error)
	ProcessCommand(*Command, bool) ([]*CmdResult, error)
	Close()
	GetLogger() log.Logger
}

func (r *router) GetName() string {
	return r.name
}

func (r *router) GetLogger() log.Logger {
	return r.logger
}

type CmdResult struct {
	Cmd    string
	Result []byte
}

func Delay(d int) {
	t := time.NewTimer(time.Duration(d) * time.Second)
	defer t.Stop()
	<-t.C
}

func (r *router) ProcessCommand(cmd *Command, collectResult bool) ([]*CmdResult, error) {
	c := cmd.Cmd
	results := make([]*CmdResult, 0)

	// TODO (sbezverk) Add some sanity check for this timer

	if cmd.WaitBefore != 0 {
		Delay(cmd.WaitBefore)
	}
	commandTimeout := DefaultCommandTimeout
	if cmd.CmdTimeout != 0 && cmd.CmdTimeout > DefaultCommandTimeout {
		commandTimeout = cmd.CmdTimeout
	}
	pipeModifier := ""
	if cmd.PipeModifier != "" {
		pipeModifier += " | " + cmd.PipeModifier
	}
	if len(cmd.Location) == 0 {
		var err error
		rs, err := r.sendCommand(c+" "+pipeModifier, cmd.Times, cmd.Interval, cmd.Debug, commandTimeout)
		if err != nil {
			return nil, err
		}
		if collectResult {
			results = append(results, rs...)
		}
	} else {
		for _, l := range cmd.Location {
			fc := c + " " + "location " + l + " " + pipeModifier
			rs, err := r.sendCommand(fc, cmd.Times, cmd.Interval, cmd.Debug, commandTimeout)
			if err != nil {
				return nil, err
			}
			if collectResult {
				results = append(results, rs...)
			}
		}
	}
	if cmd.WaitAfter != 0 {
		Delay(cmd.WaitAfter)
	}

	return results, nil
}

func (r *router) sendCommand(cmd string, times, interval int, debug bool, commandTimeout int) ([]*CmdResult, error) {
	if glog.V(5) {
		if interval == 0 || times == 0 {
			glog.Infof("Sending command: %q to router: %q, command timeout: %d seconds", cmd, r.GetName(), commandTimeout)
		} else {
			glog.Infof("Sending command: %q, %d times with interval of %d seconds to router: %q, command timeout: %d seconds", cmd, times, interval, r.GetName(), commandTimeout)
		}
	}
	if interval == 0 || times == 0 {
		b, err := r.GetData(cmd, debug, commandTimeout)
		if err != nil {
			return nil, err
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
		b, err := r.GetData(cmd, debug, commandTimeout)
		if err != nil {
			return nil, err
		}
		results = append(results, &CmdResult{
			Cmd:    cmd,
			Result: b,
		})
		<-ticker.C
	}

	return results, nil
}

var _ Router = &router{}

type router struct {
	name      string
	port      int
	sshConfig *ssh.ClientConfig
	stdin     io.WriteCloser
	stdout    io.Reader
	session   *ssh.Session
	sshClient *ssh.Client
	logger    log.Logger
}

func (r *router) Close() {
	r.session.Close()
	r.sshClient.Close()
}

func (r *router) GetData(cmd string, debug bool, commandTimeout int) ([]byte, error) {
	buffer, err := sendCommand(r.stdin, r.stdout, cmd, debug, r.logger, commandTimeout)
	if err != nil {
		return nil, err
	}

	return buffer, nil
}

func NewRouter(rn string, port int, sshConfig *ssh.ClientConfig, l log.Logger) (Router, error) {
	r := &router{
		name:      rn,
		port:      port,
		sshConfig: sshConfig,
		logger:    l,
	}
	// Dial and if successful, create ssh session
	var err error
	r.sshClient, err = ssh.Dial("tcp", r.name+":"+strconv.Itoa(r.port), r.sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial router: %s with error: %+v", r.name, err)
	}
	r.session, err = r.sshClient.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to establish a session with error: %+v", err)
	}
	glog.Infof("Successfully dialed router: %s", rn)
	if err := r.session.RequestPty("vt100", 256, 40, ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		return nil, fmt.Errorf("failed to pty with error: %+v", err)
	}

	// StdinPipe for commands
	r.stdin, err = r.session.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to establish stdin pipe with error: %+v", err)
	}

	r.stdout, err = r.session.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to establish stdin pipe with error: %+v", err)
	}

	// Start remote shell
	if err := r.session.Shell(); err != nil {
		return nil, fmt.Errorf("failed to establish a session shell with error: %+v", err)
	}
	// Prepare session with correct parameters
	if _, err := r.GetData("terminal w 256", false, DefaultCommandTimeout); err != nil {
		return nil, err
	}
	if _, err := r.GetData("terminal l 0", false, DefaultCommandTimeout); err != nil {
		return nil, err
	}

	return r, nil
}

func sendCommand(stdin io.WriteCloser, stdout io.Reader, cmd string, debug bool, l log.Logger, commandTimeout int) ([]byte, error) {
	sanitizedcmd := strings.Replace(cmd, "|", "\\|", -1)
	// Some h/w specific commands send `\` escape, adding another escape to escape the original
	s1 := string(bytes.Replace([]byte(sanitizedcmd), []byte(`\`), []byte(`\\`), -1))
	commandParts := strings.Split(s1, " ")
	startPartial := commandParts[0]
	startPattern := regexp.MustCompile(startPartial)
	errCh := make(chan error)
	doneCh := make(chan []byte)
	if commandTimeout == 0 {
		commandTimeout = 120
	}
	timeout := time.NewTimer(time.Second * time.Duration(commandTimeout))
	defer func() {
		close(errCh)
		close(doneCh)
		timeout.Stop()
	}()
	fullInput := make([]byte, 10240*10240)
	index := 0
	startFound := false
	endFound := false
	go func(done chan []byte, eCh chan error) {
		lb := make([]byte, 1024)
		cmdFound := false
		for {
			if n, err := stdout.Read(lb); err == nil {
				copy(fullInput[index:index+n], lb)
				index += n
				if !cmdFound {
					ns := startPattern.FindIndex(fullInput[:index])
					if ns != nil {
						// glog.Infof("Command: %s found in buffer: %s", cmd, fullInput)
						cmdFound = true
						startFound = true
						copy(fullInput, fullInput[ns[0]:index])
						index -= ns[0]
						// glog.Infof("Buffer after trimming: %s", fullInput)
					}
				}
				if !cmdFound {
					continue
				}
				if patterns.Prompt.FindIndex(fullInput[:index]) != nil {
					if debug {
						glog.Infof("completed router's reply with prompt: %s\n", string(fullInput[:index]))
					}
					endFound = true
					done <- fullInput[:index]
					return
				}
			} else {
				eCh <- err
				return
			}
		}
	}(doneCh, errCh)

	// If logging is enabled, sending the command to the logger process
	if l != nil {
		l.Log([]byte("=========> " + cmd + "\n"))
	}
	if debug {
		glog.Infof("Sending \"%s\"", cmd)
	}
	if _, err := fmt.Fprintf(stdin, "%s\n", cmd); err != nil {
		return nil, fmt.Errorf("failed to send command %s  with error: %+v", cmd, err)
	}
	select {
	case err := <-errCh:
		return nil, err
	case buff := <-doneCh:
		// Attempt to catch extra 2 bytes
		buffer := bytes.Replace(buff, []byte{0x0d}, []byte{}, -1)
		// Removing the actual command from the buffer

		start := startPattern.FindIndex(buffer)
		if start == nil {
			return nil, fmt.Errorf("failed to find start of command %q failing pattern %q in output, buffer: %s", cmd, startPattern.String(), string(buffer))
		}
		eol := regexp.MustCompile(`\n`).FindIndex(buffer[start[0]:])
		if eol != nil {
			start[1] = start[0] + eol[0]
		}
		end := patterns.Prompt.FindIndex(buffer)
		if end == nil {
			return nil, fmt.Errorf("failed to find end of command %q in output, buffer: %s", cmd, string(buffer))
		}
		b := make([]byte, len(buffer[start[1]:end[0]]))
		copy(b, buffer[start[1]:end[0]])
		if debug {
			glog.Infof("Data buffer passed for processing: %s\n", string(b))
		}
		// If logging is enabled, sending received buffer to the logger process
		if l != nil {
			l.Log(b)
			l.Log([]byte("\n\n"))
		}
		return b, nil
	case <-timeout.C:
		glog.Errorf("router's reply buffer full buffer: %s", string(fullInput))
		return nil, fmt.Errorf("time out waiting for the result of %q, start found %t, end found %t", cmd, startFound, endFound)
	}
}
