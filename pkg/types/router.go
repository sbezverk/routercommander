package types

import (
	"bytes"
	"fmt"
	"io"

	"regexp"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/log"
	"github.com/sbezverk/routercommander/pkg/patterns"
	"golang.org/x/crypto/ssh"
)

// Router interface is a collection of methods

type Router interface {
	GetName() string
	GetData(string, bool) ([]byte, error)
	Close()
}

func (r *router) GetName() string {
	return r.name
}

// func (r *router) sendCommands(stdin io.WriteCloser, list []*ShowCommand) error {
// 	// send the commands
// 	for _, cmd := range list {
// 		c := cmd.Cmd
// 		if len(cmd.Location) == 0 {
// 			if err := sendShowCommand(stdin, c, cmd.Times, cmd.Interval); err != nil {
// 				return err
// 			}
// 			continue
// 		}
// 		for _, l := range cmd.Location {
// 			fc := c + " " + "location " + l
// 			if err := sendShowCommand(stdin, fc, cmd.Times, cmd.Interval); err != nil {
// 				return err
// 			}
// 		}
// 	}

// 	return nil
// }

// func sendShowCommand(stdin io.WriteCloser, cmd string, times, interval int) error {
// 	switch {
// 	case interval == 0 || times == 0:
// 		glog.Infof("sending \"%s\"", cmd)
// 		if _, err := fmt.Fprintf(stdin, "%s\n", cmd); err != nil {
// 			return fmt.Errorf("failed to send command %s  with error: %+v", cmd, err)
// 		}
// 		return nil
// 	default:
// 		glog.Infof("sending \"%s\" %d times with interval %d seconds", cmd, times, interval)
// 		ticker := time.NewTicker(time.Second * time.Duration(interval))
// 		defer ticker.Stop()
// 		for t := 0; t < times; t++ {
// 			if _, err := fmt.Fprintf(stdin, "%s\n", cmd); err != nil {
// 				return fmt.Errorf("failed to send command %s  with error: %+v", cmd, err)
// 			}
// 			<-ticker.C
// 		}
// 	}

// 	return nil
// }

type router struct {
	name      string
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

func (r *router) GetData(cmd string, debug bool) ([]byte, error) {
	buffer, err := sendCommand(r.stdin, r.stdout, cmd, debug, r.logger)
	if err != nil {
		return nil, err
	}

	return buffer, nil
}

func NewRouter(rn string, sshConfig *ssh.ClientConfig, l log.Logger) (Router, error) {
	routerName := string(rn) + ":22"
	glog.Infof("Successfully dialed router: %s", routerName)
	r := &router{
		name:      routerName,
		sshConfig: sshConfig,
		logger:    l,
	}
	// Create sesssion
	var err error
	r.sshClient, err = ssh.Dial("tcp", r.name, r.sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial router: %s with error: %+v", r.name, err)
	}
	r.session, err = r.sshClient.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to establish a session with error: %+v", err)
	}

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
	if _, err := r.GetData("terminal w 256", false); err != nil {
		return nil, err
	}
	if _, err := r.GetData("terminal l 0", false); err != nil {
		return nil, err
	}

	return r, nil
}

func sendCommand(stdin io.WriteCloser, stdout io.Reader, cmd string, debug bool, l log.Logger) ([]byte, error) {
	sanitizedcmd := strings.Replace(cmd, "|", "\\|", -1)
	// Some h/w specific commands send `\` escape, adding another escape to escape the original
	s1 := string(bytes.Replace([]byte(sanitizedcmd), []byte(`\`), []byte(`\\`), -1))
	commandParts := strings.Split(s1, " ")
	startPattern := regexp.MustCompile(commandParts[0] + `\s+` + commandParts[1] + `\s+`)
	errCh := make(chan error)
	doneCh := make(chan []byte)
	timeout := time.NewTimer(time.Second * 120)
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
		l := make([]byte, 1024)
		cmdFound := false
		for {
			if n, err := stdout.Read(l); err == nil {
				copy(fullInput[index:index+n], l)
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
		if !strings.HasPrefix(cmd, "term") {
			glog.Infof("Sending \"%s\"", cmd)
		}
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
