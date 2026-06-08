package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/log"
	"github.com/sbezverk/routercommander/pkg/messenger"
	"github.com/sbezverk/routercommander/pkg/messenger/email"
	"github.com/sbezverk/routercommander/pkg/types"
	"gopkg.in/yaml.v3"
)

var (
	local                   bool
	rtrFile                 string
	rtrName                 string
	cmdFile                 string
	login                   string
	pass                    string
	port                    int
	notify                  bool
	smtpServer              string
	smtpUser                string
	smtpPass                string
	smtpFrom                string
	smtpTo                  string
	logLoc                  string
	knownHostsFile          string
	insecureSSH             bool
	passwordStdin           bool
	maxConcurrentSessions   int
	sessionsStartIntervalMS int
)

func init() {
	flag.BoolVar(&local, "local", false, "when set to true, routercommander is running on the local router")
	// Breaking change
	flag.StringVar(&rtrFile, "routers-file", "", "routers' inventory yaml file")
	flag.StringVar(&cmdFile, "commands-file", "", "YAML formated file with commands to collect")
	flag.StringVar(&rtrName, "router-name", "", "name of the router")
	flag.StringVar(&login, "username", "", "username to use to ssh to a router")
	flag.StringVar(&pass, "password", "", "Password to use for ssh session")
	flag.IntVar(&port, "port", 22, "Port to use for SSH sessions, default 22")
	flag.BoolVar(&notify, "notify", false, "If set to true, email notification will be send.")
	flag.StringVar(&smtpServer, "smtp-server", "", "ip address or dns name with tcp port of smtp server, example: smtp.gmain.com:587")
	flag.StringVar(&smtpUser, "smtp-user", "", "a user name to use to authenticate to the smtp server")
	flag.StringVar(&smtpPass, "smtp-pass", "", "a password to use to authenticate to the smtp server")
	flag.StringVar(&smtpFrom, "smtp-from", "", "email address to use for sending the report from")
	flag.StringVar(&smtpTo, "smtp-to", "", "comma separated list of emails for sending the report to")
	flag.StringVar(&logLoc, "log", "", "path for the log file.")
	flag.StringVar(&knownHostsFile, "known-hosts-file", defaultKnownHostsFile(), "path to the known hosts file for SSH")
	flag.BoolVar(&insecureSSH, "insecure-ssh", false, "when set to true, SSH host key verification will be disabled and new host keys will not be added to the known hosts file")
	flag.BoolVar(&passwordStdin, "password-stdin", false, "read the password from stdin")
	flag.IntVar(&maxConcurrentSessions, "max-concurrent-sessions", 10, "maximum number of concurrent SSH sessions to routers, default 10")
	flag.IntVar(&sessionsStartIntervalMS, "sessions-start-interval-ms", 500, "time interval in milliseconds between starting SSH sessions to routers, default 500ms")
}

type RouterInventory struct {
	Routers map[string]*RouterTarget `yaml:"routers"`
}

type RouterTarget struct {
	Address  string `yaml:"address"`
	Port     int    `yaml:"port"`
	Platform string `yaml:"platform"`
	Username string `yaml:"username"`
}

type ResolvedTarget struct {
	Name     string
	Address  string
	Port     int
	Platform string
	Username string
}

func normalizeRouterName(name string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(name)), "\n\t,")
}

func defaultKnownHostsFile() string {
	return filepath.Join(os.TempDir(), "routercommander_known_hosts")
}

func resolveRouterTarget(name string, inventory *RouterInventory, defaultPort int, defaultUser string) (*ResolvedTarget, error) {
	normalized := normalizeRouterName(name)
	if inventory == nil {
		// Not failing if inventory is not provided, will be using specified name as actual address to connect to
		glog.Warningf("routers inventory is not provided, using specified router name %s as an address to connect to", normalized)
		return nil, nil
	}
	target, ok := inventory.Routers[normalized]
	if !ok {
		// Not failing if router is not found in the inventory, will be using specified name as actual address to connect to
		glog.Warningf("router %s is not found in the inventory, using specified router name as an address to connect to", normalized)
		return nil, nil
	}
	if target == nil {
		target = &RouterTarget{}
	}
	address := strings.TrimSpace(target.Address)
	if address == "" {
		address = normalized
	}
	port := target.Port
	if port == 0 {
		port = defaultPort
	}
	username := strings.TrimSpace(target.Username)
	if username == "" {
		username = defaultUser
	}
	return &ResolvedTarget{
		Name:     normalized,
		Address:  address,
		Port:     port,
		Platform: target.Platform,
		Username: username,
	}, nil
}

func getRoutersInventory(fileName string) (*RouterInventory, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to open router inventory file %s with error: %+v", fileName, err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read router inventory file %s with error: %+v", fileName, err)
	}
	inventory := &RouterInventory{}
	if err := yaml.Unmarshal(b, inventory); err != nil {
		return nil, fmt.Errorf("failed to unmarshal router inventory file %s with error: %+v", fileName, err)
	}
	normalized := &RouterInventory{
		Routers: make(map[string]*RouterTarget),
	}
	for name, target := range inventory.Routers {
		normName := normalizeRouterName(name)
		if normName == "" {
			glog.Warningf("router with empty name is found in the inventory file %s, skipping...", fileName)
			continue
		}
		if target == nil {
			target = &RouterTarget{}
		}
		target.Address = strings.TrimSpace(target.Address)
		if target.Address == "" {
			target.Address = normName
		}
		target.Username = strings.TrimSpace(target.Username)
		if target.Port == 0 {
			target.Port = 22
		}
		normalized.Routers[normName] = target
	}
	if glog.V(3) {
		glog.Infof("loaded %d router(s) from inventory file %s", len(normalized.Routers), fileName)
	}

	return normalized, nil
}

func main() {
	logo := `
    +---------------------------------------------------+
    | routercommander                  v0.5.1           |
    | Developed and maintained by Serguei Bezverkhi     |
    | sbezverk@cisco.com                                |
    +---------------------------------------------------+
`

	flag.Parse()
	_ = flag.Set("logtostderr", "true")

	glog.Infof("\n%s\n", logo)

	if cmdFile == "" {
		glog.Infof("no commands file is specified, nothing to do, exiting...")
		os.Exit(1)
	}
	if passwordStdin && pass != "" {
		glog.Error("both --password and --password-stdin parameters cannot be provided simultaneously, exiting...")
		os.Exit(1)
	}
	var n messenger.Notifier
	routers := make([]string, 0)
	var inventory *RouterInventory
	var err error
	var fatalErr error
	var wg sync.WaitGroup

	if sessionsStartIntervalMS <= 0 {
		glog.Errorf("invalid value for --sessions-start-interval-ms parameter: %d, it cannot be negative or zero, exiting...", sessionsStartIntervalMS)
		os.Exit(1)
	}
	if maxConcurrentSessions <= 0 {
		glog.Errorf("invalid value for --max-concurrent-sessions parameter: %d, it cannot be negative or zero, exiting...", maxConcurrentSessions)
		os.Exit(1)
	}
	singleRouterCase := rtrName != ""
	if !local {
		switch {
		case rtrName != "" && rtrFile == "":
			// Case when only router's name if provided without inventory file
			// this case requires both username and password to be provided
			if login == "" || (pass == "" && !passwordStdin) {
				glog.Error("--username and --password or --password-stdin are mandatory parameters, when no inventory file is provided, exiting...")
				os.Exit(1)
			}
			routers = append(routers, rtrName)
		case rtrName != "" && rtrFile != "":
			// Case when both router's name and inventory file are provided, inventory will be used to get more details abot a router
			if pass == "" && !passwordStdin {
				glog.Error("--password or --password-stdin is a mandatory parameter, when routers' inventory file is provided, exiting...")
				os.Exit(1)
			}
			inventory, err = getRoutersInventory(rtrFile)
			if err != nil {
				glog.Errorf("failed to get routers inventory from file: %s with error: %+v, exiting...", rtrFile, err)
				os.Exit(1)
			}
			routers = append(routers, rtrName)
		case rtrName == "" && rtrFile != "":
			// Case when only inventory file is provided, all routers from the inventory will be processed
			if pass == "" && !passwordStdin {
				glog.Error("--password or --password-stdin is a mandatory parameter, when routers' inventory file is provided, exiting...")
				os.Exit(1)
			}
			inventory, err = getRoutersInventory(rtrFile)
			if err != nil {
				glog.Errorf("failed to get routers inventory from file: %s with error: %+v, exiting...", rtrFile, err)
				os.Exit(1)
			}
			for name := range inventory.Routers {
				routers = append(routers, normalizeRouterName(name))
			}
		default:
			glog.Error("either --router-name or --routers-file parameter should be provided, exiting...")
			os.Exit(1)
		}

		if notify {
			failCheck := false
			switch {
			case smtpServer == "":
				glog.Errorf("\"--smtp-server\" parameter cannot be empty")
				failCheck = true
			case smtpUser == "":
				glog.Errorf("\"--smtp-user\" parameter cannot be empty")
				failCheck = true
			case smtpPass == "":
				glog.Errorf("\"--smtp-pass\" parameter cannot be empty")
				failCheck = true
			case smtpFrom == "":
				glog.Errorf("\"--smtp-from\" parameter cannot be empty")
				failCheck = true
			case smtpTo == "":
				glog.Errorf("\"--smtp-to\" parameter cannot be empty")
				failCheck = true
			}
			if failCheck {
				glog.Errorf("validation of notification parameters failed")
				os.Exit(1)
			}
			n, err = email.NewEmailNotifier(smtpServer, smtpUser, smtpPass, smtpFrom, smtpTo)
			if err != nil {
				glog.Errorf("failed to initialize email notifier with error: %+v, exiting...", err)
				os.Exit(1)
			}
		}
	}
	if local {
		b, err := exec.Command("hostname").Output()
		if err != nil {
			glog.Errorf("failed to get hostname of a local router with error: %+v, exiting...", err)
			os.Exit(1)
		}
		routers = append(routers, strings.Trim(string(b), " \n\t,"))
	}
	commands, err := types.GetCommands(cmdFile)
	if err != nil {
		glog.Errorf("failed to get list of commands from file: %s with error: %+v, exiting...", cmdFile, err)
		os.Exit(1)
	}
	stopOnError := true
	if commands != nil {
		if commands.Collect != nil {
			stopOnError = commands.Collect.StopOnError
		}
	}
	errCh := make(chan error, (len(routers)))
	runProcessing := func(r types.Router, commander *types.Commander) {
		errCh <- process(r, commander, n)
	}

	if passwordStdin {
		var pw string
		pw, err = readPasswordFromStdin()
		if err != nil {
			glog.Errorf("failed to read password from stdin with error: %+v, exiting...", err)
			os.Exit(1)
		}
		pass = pw
	}
	if glog.V(3) {
		glog.Infof("number of routers selected for processing: %d", len(routers))
	}
	processesStarted := 0
	sessionStartTicker := time.NewTicker(time.Duration(sessionsStartIntervalMS) * time.Millisecond)
	defer sessionStartTicker.Stop()
	breakCh := make(chan os.Signal, 1)
	signal.Notify(breakCh, os.Interrupt)
	broken := false
	var availableWorkers atomic.Int32
	availableWorkers.Store(int32(maxConcurrentSessions))
	for i, router := range routers {
		sessionStartTicker.Reset(time.Duration(sessionsStartIntervalMS) * time.Millisecond)
		if broken {
			glog.Infof("skipping starting session for router %s as interrupt signal is received", router)
			break
		}
		if i != 0 {
			select {
			case <-breakCh:
				glog.Infof("interrupt signal received, stopping the process...")
				broken = true
				continue
			case <-sessionStartTicker.C:
				sessionStartTicker.Stop()
			}
		}
		actRouter := router
		actPort := port
		actLogin := login
		actPlatform := ""
		if inventory != nil {
			var target *ResolvedTarget
			target, err = resolveRouterTarget(router, inventory, port, login)
			if err != nil {
				glog.Errorf("failed to resolve router target for router: %s with error: %+v", router, err)
				if !stopOnError && !singleRouterCase {
					continue
				}
				fatalErr = err
				break
			}
			if target != nil {
				actRouter = target.Address
				actPort = target.Port
				actPlatform = target.Platform
				actLogin = target.Username
			}
		}
		// Loop to wait for available worker before starting the process, this is needed to control the number of concurrent SSH sessions to routers, which can cause resource exhaustion on the machine running routercommander or on the routers themselves if too many sessions are started at the same time
		for !broken {
			select {
			case <-breakCh:
				broken = true
				continue
			default:
			}
			if availableWorkers.Add(-1) < 0 {
				availableWorkers.Add(1)
				time.Sleep(time.Duration(sessionsStartIntervalMS/2) * time.Millisecond)
			} else {
				break
			}
			continue
		}
		if !broken {
			var li log.Logger
			li, err = log.NewLogger(router, logLoc)
			if err != nil {
				glog.Errorf("failed to instantiate logger interface with error: %+v", err)
				os.Exit(1)
			}
			var r types.Router
			if local {
				r = types.NewLocalRouter(actRouter, li)
			} else {
				var sshVerifier Verifier
				sshVerifier, err = NewVerifier(knownHostsFile, insecureSSH)
				if err != nil {
					glog.Errorf("failed to get SSH configuration with error: %+v, exiting...", err)
					if li != nil {
						li.Close()
					}
					if !stopOnError && !singleRouterCase {
						availableWorkers.Add(1)
						continue
					}
					fatalErr = err
					break
				}
				r, err = types.NewRouter(actRouter, actPort, actPlatform, sshVerifier.GetSSHConfig(actLogin, pass), li)
				if err != nil {
					glog.Errorf("failed to instantiate router object for router: %s:%d with error: %+v", actRouter, actPort, err)
					if li != nil {
						li.Close()
					}
					if !stopOnError && !singleRouterCase {
						availableWorkers.Add(1)
						continue
					}
					fatalErr = err
					break
				}
			}
			routerCommands := commands.CloneForRun()
			wg.Add(1)
			go func(r types.Router, commander *types.Commander) {
				defer wg.Done()
				runProcessing(r, commander)
				availableWorkers.Add(1)
			}(r, routerCommands)
			processesStarted++
		}
	}
	pass = ""
	for i := 0; i < processesStarted; i++ {
		err := <-errCh
		if err != nil {
			glog.Errorf("processing finished with error: %+v", err)
			fatalErr = err
		}
	}
	if processesStarted != 0 {
		wg.Wait()
	}
	close(errCh)
	glog.Infof("all processes have finished, exiting...")
	if fatalErr == nil {
		os.Exit(0)
	}
	os.Exit(1)
}

func readPasswordFromStdin() (string, error) {
	pw := ""
	s := ""
	var err error
	if term.IsTerminal(uintptr(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stdout, "Session password: ")
		var bytePassword []byte
		bytePassword, err = term.ReadPassword(uintptr(os.Stdin.Fd()))
		fmt.Fprintln(os.Stdout)
		if err != nil {
			return "", fmt.Errorf("failed to read password from terminal with error: %+v", err)
		}
		s = string(bytePassword)
	} else {
		if err = os.Stdin.SetReadDeadline(time.Now().Add(5 * time.Second)); err == nil {
			defer os.Stdin.SetReadDeadline(time.Time{})
		}
		s, err = bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !(errors.Is(err, io.EOF) && s != "") {
			return "", fmt.Errorf("failed to read password from stdin: %+v", err)
		}
	}

	pw = strings.TrimRight(s, "\r\n")
	if pw == "" {
		return "", fmt.Errorf("no password received on stdin")
	}

	return pw, nil
}
