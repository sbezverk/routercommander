package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/log"
	"github.com/sbezverk/routercommander/pkg/messenger"
	"github.com/sbezverk/routercommander/pkg/messenger/email"
	"github.com/sbezverk/routercommander/pkg/types"
	"gopkg.in/yaml.v3"
)

var (
	local          bool
	rtrFile        string
	rtrName        string
	cmdFile        string
	login          string
	pass           string
	port           int
	notify         bool
	smtpServer     string
	smtpUser       string
	smtpPass       string
	smtpFrom       string
	smtpTo         string
	logLoc         string
	knownHostsFile string
	insecureSSH    bool
)

var wg sync.WaitGroup

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
	flag.StringVar(&knownHostsFile, "known-hosts-file", "/tmp/routercommander_known_hosts", "path to the known hosts file for SSH")
	flag.BoolVar(&insecureSSH, "insecure-ssh", false, "when set to true, SSH host key verification will be disabled and new host keys will not be added to the known hosts file")
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
	if target.Address == "" {
		return nil, fmt.Errorf("address for router %s is not specified in the inventory", name)
	}
	if target.Port == 0 {
		target.Port = defaultPort
	}
	if target.Username == "" {
		target.Username = defaultUser
	}
	return &ResolvedTarget{
		Name:     normalized,
		Address:  target.Address,
		Port:     target.Port,
		Platform: target.Platform,
		Username: target.Username,
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
		if target.Address == "" {
			glog.Warningf("router %s has empty address in the inventory file %s, skipping...", name, fileName)
			continue
		}
		if target.Port == 0 {
			target.Port = 22
		}
		normalized.Routers[normName] = target
	}

	return normalized, nil
}

func main() {
	logo := `
    +---------------------------------------------------+
    | routercommander                  v0.5.0           |
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
	var n messenger.Notifier
	routers := make([]string, 0)
	var inventory *RouterInventory
	var err error
	var fatalErr error
	singleRouterCase := rtrName != ""
	if !local {
		switch {
		case rtrName != "" && rtrFile == "":
			// Case when only router's name if provided without inventory file
			// this case requires both username and password to be provided
			if login == "" || pass == "" {
				glog.Error("--username and --password are mandatory parameters, when no inventory file is provided, exiting...")
				os.Exit(1)
			}
			routers = append(routers, rtrName)
		case rtrName != "" && rtrFile != "":
			// Case when both router's name and inventory file are provided, inventory will be used to get more details abot a router
			if pass == "" {
				glog.Error("--password is a mandatory parameter, when routers' inventory file is provided, exiting...")
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
			if pass == "" {
				glog.Error("--password is a mandatory parameter, when routers' inventory file is provided, exiting...")
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
	runProcessing := func(r types.Router) {
		errCh <- process(r, commands, n)
	}

	processesStarted := 0
	for _, router := range routers {
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
				os.Exit(1)
			}
			r, err = types.NewRouter(actRouter, actPort, actPlatform, sshVerifier.GetSSHConfig(actLogin, pass), li)
			if err != nil {
				glog.Errorf("failed to instantiate router object for router: %s:%d with error: %+v", actRouter, actPort, err)
				if !stopOnError && !singleRouterCase {
					continue
				}
				fatalErr = err
				break
			}
		}
		if runtime.GOOS != "windows" {
			wg.Add(1)
			go func(r types.Router) {
				defer wg.Done()
				runProcessing(r)
			}(r)
		} else {
			runProcessing(r)
		}
		processesStarted++
	}
	if fatalErr == nil {
		wg.Wait()
		for i := 0; i < processesStarted; i++ {
			err := <-errCh
			if err != nil {
				glog.Errorf("processing finished with error: %+v", err)
				os.Exit(1)
			}
		}
	}
	close(errCh)
	glog.Infof("all processes have finished, exiting...")
	if fatalErr == nil {
		os.Exit(0)
	}
	os.Exit(1)
}
