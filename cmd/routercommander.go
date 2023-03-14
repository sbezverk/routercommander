package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/log"
	"github.com/sbezverk/routercommander/pkg/types"
	"golang.org/x/crypto/ssh"
)

var (
	rtrFile string
	rtrName string
	cmdFile string
	login   string
	pass    string
	//	hc      bool
	port int
)

var wg sync.WaitGroup

func init() {
	runtime.GOMAXPROCS(1)
	flag.StringVar(&rtrFile, "routers-file", "", "File with routers' names")
	flag.StringVar(&cmdFile, "commands-file", "", "YAML formated file with commands to collect")
	flag.StringVar(&rtrName, "router-name", "", "name of the router")
	flag.StringVar(&login, "username", "admin", "username to use to ssh to a router")
	flag.StringVar(&pass, "password", "", "Password to use for ssh session")
	//	flag.BoolVar(&hc, "health-check", false, "when health-check is true, patterns specified for each command will be checked for matches")
	flag.IntVar(&port, "port", 22, "Port to use for SSH sessions, default 22")
}

func remoteHostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	glog.Infof("Callback is called with hostname: %s remote address: %s", strings.Split(hostname, ":")[0], remote.String())
	return nil
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
    | routercommander                  v0.0.2           |
    | Developed and maintained by Serguei Bezverkhi     |
    | sbezverk@cisco.com                                |
    +---------------------------------------------------+
`

	flag.Parse()
	_ = flag.Set("logtostderr", "true")

	glog.Infof("\n%s\n", logo)

	if login == "" || pass == "" {
		glog.Error("--username and --password are mandatory parameters, exiting...")
		os.Exit(1)
	}
	if rtrFile != "" && rtrName != "" {
		glog.Error("--file and --list are mutually exclusive, exiting...")
		os.Exit(1)
	}
	if cmdFile == "" {
		glog.Infof("no commands file is specified, nothing to do, exiting...")
		os.Exit(1)
	}
	routers := make([]string, 0)
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
		r, err := types.NewRouter(router, port, sshConfig(), li)
		if err != nil {
			glog.Errorf("failed to instantiate router object for router: %s with error: %+v", rtrName, err)
			os.Exit(1)
		}
		wg.Add(1)
		go collect(r, commands)
	}
	wg.Wait()
}

func collect(r types.Router, commands *types.Commander) {
	defer wg.Done()
	mode := "collect"
	if commands.Repro != nil {
		mode = "repro"
	}
	glog.Infof("router: %s mode: %s", r.GetName(), mode)
	hc := false
	if commands.Collect != nil {
		hc = commands.Collect.HealthCheck
	}
	iterations := 1
	interval := 0
	if commands.Repro != nil {
		// In order to detect error condition, health check must be enabled in repro mode
		hc = true
		if commands.Repro.Times > 0 {
			iterations = commands.Repro.Times
		}
		if commands.Repro.Interval > 0 {
			interval = commands.Repro.Interval
		}
		glog.Infof("router %s in repro mode, the command set will be executed %d time(s) with the interval of %d seconds", r.GetName(), iterations, interval)
	}
	triggered := false
out:
	for i := 0; i < iterations; i++ {
		glog.Infof("router %s, executing iteration number %d out of total %d...", r.GetName(), i+1, iterations)
		for _, c := range commands.List {
			collectResult := true
			if mode == "repro" {
				collectResult = c.CollectResult
			}
			results, err := r.ProcessCommand(c, collectResult)
			if err != nil {
				glog.Errorf("router %s failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
				return
			}
			if hc {
				for _, re := range results {
					for _, p := range c.RegExp {
						if i := p.FindIndex(re.Result); i != nil {
							triggered = true
							glog.Errorf("router %s found matching line: %q, command: %q", r.GetName(), strings.Trim(string(re.Result[i[0]:i[1]]), "\n\r\t"), re.Cmd)
							break out
						}
					}
				}
			}
		}
		types.Delay(interval)
	}
	// If the issue was triggered, collecting commands needed to troubleshooting
	if triggered && mode == "repro" {
		glog.Infof("repro process on router %s succeeded triggering the failure condition, collecting post-mortem commands...", r.GetName())
		for _, c := range commands.Repro.PostMortemList {
			_, err := r.ProcessCommand(c, true)
			if err != nil {
				glog.Errorf("router %s failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
				return
			}
		}
		return
	}
	if !triggered && mode == "repro" {
		glog.Infof("repro process on router %s did not succeed triggering the failure condition", r.GetName())
		return
	}
	if triggered {
		glog.Errorf("health check validation failed on router %s, check collected log", r.GetName())
	} else {
		glog.Errorf("collection completed successfully on router %s", r.GetName())
	}
}

func sshConfig() *ssh.ClientConfig {
	c := ssh.Config{}
	c.SetDefaults()
	c.KeyExchanges = append(
		c.KeyExchanges,
		"diffie-hellman-group-exchange-sha256",
		"diffie-hellman-group-exchange-sha1",
		"diffie-hellman-group1-sha1",
	)
	c.Ciphers = append(
		c.Ciphers,
		"aes128-cbc",
		"aes192-cbc",
		"aes256-cbc",
		"3des-cbc")

	return &ssh.ClientConfig{
		User: login,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		Config:          c,
		HostKeyCallback: remoteHostKeyCallback,
	}
}
