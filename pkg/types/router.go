package types

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/golang/glog"
	"golang.org/x/crypto/ssh"
)

// Router interface is a collection of methods
type Router interface {
	Close()
	GetName() string
	CollectOutput(cmds *Commands) ([]byte, error)
}

type router struct {
	name   string
	client *ssh.Client
	// sshConfig *ssh.ClientConfig
}

func NewRouter(routerName string, sshConfig *ssh.ClientConfig) (Router, error) {
	sshClient, err := ssh.Dial("tcp", routerName, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial router: %s with error: %+v", routerName, err)
	}
	glog.Infof("Successfully dialed router: %s", routerName)
	return &router{
		name:   routerName,
		client: sshClient,
		// sshConfig: sshConfig,
	}, nil
}

func (r *router) GetName() string {
	return r.name
}

func (r *router) Close() {
	r.client.Close()
}

func (r *router) CollectOutput(cmds *Commands) ([]byte, error) {
	// Create sesssion
	session, err := r.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to establish a session with error: %+v", err)
	}
	defer session.Close()

	if err := session.RequestPty("vt100", 80, 40, ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		return nil, fmt.Errorf("failed to pty with error: %+v", err)
	}

	// StdinPipe for commands
	stdin, err := session.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to establish stdin pipe with error: %+v", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to establish stdin pipe with error: %+v", err)
	}

	buffer := make([]byte, 0)
	go func() {
		l := make([]byte, 1024)
		for {
			if n, err := stdout.Read(l); err == nil {
				// fmt.Printf("%s", l[:n])
				nl := bytes.Replace(l[:n], []byte{0x0d}, []byte{}, -1)
				buffer = append(buffer, nl...)
			} else {
				return
			}
		}
	}()
	// Start remote shell
	if err := session.Shell(); err != nil {
		return nil, fmt.Errorf("failed to establish a session shell with error: %+v", err)
	}
	// Making sure the output is not paged and at the same time attempting to find router's prompt
	glog.Infof("sending \"term len 0\"")
	if _, err := fmt.Fprintf(stdin, "%s\n", "term len 0"); err != nil {
		return nil, fmt.Errorf("failed to send command %s  with error: %+v", "term len 0", err)
	}

	glog.Infof("sending \"term width 256\"")
	if _, err := fmt.Fprintf(stdin, "%s\n", "term width 256"); err != nil {
		return nil, fmt.Errorf("failed to send command %s  with error: %+v", "term len 0", err)
	}

	glog.Infof("sending \"show version\"")
	if _, err := fmt.Fprintf(stdin, "%s\n", "show version"); err != nil {
		return nil, fmt.Errorf("failed to send command %s  with error: %+v", "term width 256", err)
	}

	if err := r.sendCommands(stdin, cmds.List); err != nil {
		return nil, err
	}

	glog.Infof("sending \"exit\"")
	if _, err := fmt.Fprintf(stdin, "%s\n", "exit"); err != nil {
		return nil, fmt.Errorf("failed to send command %s  with error: %+v", "exit", err)
	}

	// Waiting for the session to close
	glog.Infof("waiting for the session with %s to exit", r.name)
	if err := session.Wait(); err != nil {
		if _, ok := err.(*ssh.ExitMissingError); !ok {
			return nil, fmt.Errorf("failed to wait for the session with error: %+v", err)
		}
	}

	return buffer, nil
}

func (r *router) sendCommands(stdin io.WriteCloser, list []*ShowCommand) error {
	// send the commands
	for _, cmd := range list {
		c := cmd.Cmd
		if len(cmd.Location) == 0 {
			if err := sendShowCommand(stdin, c, cmd.Times, cmd.Interval); err != nil {
				return err
			}
			continue
		}
		for _, l := range cmd.Location {
			fc := c + " " + "location " + l
			if err := sendShowCommand(stdin, fc, cmd.Times, cmd.Interval); err != nil {
				return err
			}
		}
	}

	return nil
}

func sendShowCommand(stdin io.WriteCloser, cmd string, times, interval int) error {
	switch {
	case interval == 0 || times == 0:
		glog.Infof("sending \"%s\"", cmd)
		if _, err := fmt.Fprintf(stdin, "%s\n", cmd); err != nil {
			return fmt.Errorf("failed to send command %s  with error: %+v", cmd, err)
		}
		return nil
	default:
		glog.Infof("sending \"%s\" %d times with interval %d seconds", cmd, times, interval)
		ticker := time.NewTicker(time.Second * time.Duration(interval))
		defer ticker.Stop()
		for t := 0; t < times; t++ {
			if _, err := fmt.Fprintf(stdin, "%s\n", cmd); err != nil {
				return fmt.Errorf("failed to send command %s  with error: %+v", cmd, err)
			}
			<-ticker.C
		}
	}

	return nil
}
