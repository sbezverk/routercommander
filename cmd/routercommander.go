package main

import (
	"bufio"
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
)

var (
	local      bool
	rtrFile    string
	rtrName    string
	cmdFile    string
	login      string
	pass       string
	port       int
	notify     bool
	smtpServer string
	smtpUser   string
	smtpPass   string
	smtpFrom   string
	smtpTo     string
)

var wg sync.WaitGroup

func init() {
	runtime.GOMAXPROCS(1)
	flag.BoolVar(&local, "local", false, "when set to true, routercommander is running on the local router")
	flag.StringVar(&rtrFile, "routers-file", "", "File with routers' names")
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
}

func getInfoFromFile(fn string) ([]string, error) {
	list := make([]string, 0)
	f, err := os.Open(fn)
	if err != nil {
		return nil, fmt.Errorf("fail to open file %s with error: %+v", fn, err)
	}
	defer f.Close()

	fr := bufio.NewReader(f)
	for {
		item, err := fr.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		list = append(list, strings.Trim(item, "\n"))
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("file %s is empty", fn)
	}

	return list, nil
}

func main() {
	logo := `
    +---------------------------------------------------+
    | routercommander                  v0.3.0           |
    | Developed and maintained by Serguei Bezverkhi     |
    | sbezverk@cisco.com                                |
    +---------------------------------------------------+
`

	flag.Parse()
	_ = flag.Set("logtostderr", "true")

	glog.Infof("\n%s\n", logo)

	var n messenger.Notifier
	routers := make([]string, 0)

	if cmdFile == "" {
		glog.Infof("no commands file is specified, nothing to do, exiting...")
		os.Exit(1)
	}

	if !local {
		if login == "" || pass == "" {
			glog.Error("--username and --password are mandatory parameters, exiting...")
			os.Exit(1)
		}
		if rtrFile != "" && rtrName != "" {
			glog.Error("--file and --list are mutually exclusive, exiting...")
			os.Exit(1)
		}
		var err error
		switch {
		case rtrFile != "":
			routers, err = getInfoFromFile(rtrFile)
			if err != nil {
				glog.Errorf("failed to get routers list from file: %s with error: %+v, exiting...", rtrFile, err)
				os.Exit(1)
			}
		case rtrName != "":
			routers = append(routers, rtrName)
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
	for _, router := range routers {
		li, err := log.NewLogger(router)
		if err != nil {
			glog.Errorf("failed to instantiate logger interface with error: %+v", err)
			os.Exit(1)
		}
		var r types.Router
		if local {
			r = types.NewLocalRouter(router, li)
		} else {
			r, err = types.NewRouter(router, port, sshConfig(), li)
			if err != nil {
				glog.Errorf("failed to instantiate router object for router: %s with error: %+v", rtrName, err)
				os.Exit(1)
			}
		}
		ci := &types.Commander{}
		*ci = *commands
		wg.Add(1)
		if ci.Repro != nil {
			go repro(r, ci, n)
		} else {
			go collect(r, ci, n)
		}
	}
	wg.Wait()
}
