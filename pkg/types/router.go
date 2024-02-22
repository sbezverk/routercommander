package types

import (
	"bytes"
	"fmt"
	"html/template"
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
	IsExistingLocation(string) bool
	GetAllLCs() []string
	GetAllRPs() []string
	GetActiveRP() string
	GetAllLocations() []string
	GetName() string
	GetData(string, bool, int) ([]byte, error)
	ProcessCommand(*Command, bool) ([]*CmdResult, error)
	Close()
	GetLogger() log.Logger
}

func (r *router) IsExistingLocation(l string) bool {
	if _, found := r.platform.rps.rps[l]; found {
		return true
	}
	if _, found := r.platform.lcs.lcs[l]; found {
		return true
	}

	return false
}

func (r *router) GetAllLCs() []string {
	if r.platform.lcs == nil {
		return r.GetAllRPs()
	}
	if len(r.platform.lcs.lcs) == 0 {
		return r.GetAllRPs()
	}
	lcs := make([]string, len(r.platform.lcs.lcs))
	i := 0
	for lc := range r.platform.lcs.lcs {
		lcs[i] = lc
		i++
	}

	return lcs
}

func (r *router) GetAllRPs() []string {
	if r.platform.rps == nil {
		return nil
	}
	if len(r.platform.rps.rps) == 0 {
		return nil
	}
	rps := make([]string, len(r.platform.rps.rps))
	i := 0
	for rp := range r.platform.rps.rps {
		rps[i] = rp
		i++
	}

	return rps
}

func (r *router) GetAllLocations() []string {
	locations := make([]string, 0)
	rps := r.GetAllRPs()
	if rps != nil {
		locations = append(locations, rps...)
	}
	lcs := r.GetAllLCs()
	if lcs != nil {
		locations = append(locations, lcs...)
	}

	return locations
}

func (r *router) GetActiveRP() string {
	if r.platform.rps == nil {
		return ""
	}
	if len(r.platform.rps.rps) == 0 {
		return ""
	}
	for loc, rp := range r.platform.rps.rps {
		if rp.isActive {
			return loc
		}
	}
	return ""
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
		rs, err := r.sendCommand(c+pipeModifier, cmd.Times, cmd.Interval, cmd.Debug, commandTimeout)
		if err != nil {
			return nil, err
		}
		if collectResult {
			results = append(results, rs...)
		}
	} else {
		locs, err := prepareLocations(r, cmd)
		if err != nil {
			return nil, err
		}
		rs, err := r.sendCommandWithLocations(cmd, locs, pipeModifier, commandTimeout)
		if err != nil {
			return nil, err
		}
		if collectResult {
			results = append(results, rs...)
		}
	}
	if cmd.WaitAfter != 0 {
		Delay(cmd.WaitAfter)
	}

	return results, nil
}

func transforLocation(tmpl *template.Template, loc string) (string, error) {
	l := strings.Split(loc, "/")
	if len(l) < 3 {
		return "", fmt.Errorf("location %s is in unknown format", loc)
	}
	slot, err := strconv.ParseInt(l[1], 10, 0)
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, struct {
		Slot int
	}{
		Slot: int(slot),
	}); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func prepareLocations(r *router, cmd *Command) ([]string, error) {
	locs := make([]string, 0)
	for _, l := range cmd.Location {
		switch l {
		case "all":
			locs = append(locs, r.GetAllLocations()...)
		case "all-rp":
			locs = append(locs, r.GetAllRPs()...)
		case "all-lc":
			locs = append(locs, r.GetAllLCs()...)
		default:
			locs = append(locs, l)
		}
	}
	if cmd.LocationFmtTmpl == "" {
		// No location customization format
		return locs, nil
	}
	tmpl, err := template.New("Slot").Parse(cmd.LocationFmtTmpl)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(locs); i++ {
		locs[i], err = transforLocation(tmpl, locs[i])
		if err != nil {
			return nil, err
		}
	}

	return locs, nil
}

func (r *router) sendCommandWithLocations(cmd *Command, locations []string, pipeModifier string, commandTimeout int) ([]*CmdResult, error) {
	results := make([]*CmdResult, 0)
	locs := make([]string, 0)
	var tmpl *template.Template
	var err error
	if cmd.LocationCustomized {
		tmpl, err = template.New("Command").Parse(cmd.Cmd)
		if err != nil {
			return nil, err
		}
	}
	for _, l := range locations {
		switch l {
		case "all":
			locs = append(locs, r.GetAllLocations()...)
			rs, err := r.sendCommandWithLocations(cmd, locs, pipeModifier, commandTimeout)
			if err != nil {
				return nil, err
			}
			results = append(results, rs...)
		case "all-rp":
			locs = append(locs, r.GetAllRPs()...)
			rs, err := r.sendCommandWithLocations(cmd, locs, pipeModifier, commandTimeout)
			if err != nil {
				return nil, err
			}
			results = append(results, rs...)
		case "all-lc":
			locs = append(locs, r.GetAllLCs()...)
			rs, err := r.sendCommandWithLocations(cmd, locs, pipeModifier, commandTimeout)
			if err != nil {
				return nil, err
			}
			results = append(results, rs...)
		default:
			var fc string
			if !cmd.LocationCustomized {
				fc = cmd.Cmd + " " + "location " + l + " " + pipeModifier

			} else {
				buf := new(bytes.Buffer)
				if err := tmpl.Execute(buf, struct {
					Location string
				}{
					Location: l,
				}); err != nil {
					return nil, err
				}
				fc = buf.String() + " " + pipeModifier
			}
			rs, err := r.sendCommand(fc, cmd.Times, cmd.Interval, cmd.Debug, commandTimeout)
			if err != nil {
				return nil, err
			}
			results = append(results, rs...)
		}
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
	platform  *platform
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
	// Getting platform information
	b, err := r.GetData("show platform", false, DefaultCommandTimeout)
	if err != nil {
		return nil, err
	}
	p, err := populatePlatformInfo(b)
	if err != nil {
		return nil, err
	}
	r.platform = p
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
	// TODO (sbezverk)  consider buffer resizing logic
	fullInput := make([]byte, 20480*10240)
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
				if patterns.Prompt.FindIndex(fullInput[:index]) != nil ||
					patterns.SysadminPrompt.FindIndex(fullInput[:index]) != nil ||
					patterns.RunShellPrompt.FindIndex(fullInput[:index]) != nil {
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
			end = patterns.SysadminPrompt.FindIndex(buffer)
			if end == nil {
				end = patterns.RunShellPrompt.FindIndex(buffer)
				if end == nil {
					return nil, fmt.Errorf("failed to find end of command %q in output, buffer: %s", cmd, string(buffer))
				}
			}
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
