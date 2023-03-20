package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/log"
	"github.com/sbezverk/routercommander/pkg/messenger"
	"github.com/sbezverk/routercommander/pkg/messenger/email"
	"github.com/sbezverk/routercommander/pkg/types"
	"golang.org/x/crypto/ssh"
)

var (
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
	flag.StringVar(&rtrFile, "routers-file", "", "File with routers' names")
	flag.StringVar(&cmdFile, "commands-file", "", "YAML formated file with commands to collect")
	flag.StringVar(&rtrName, "router-name", "", "name of the router")
	flag.StringVar(&login, "username", "admin", "username to use to ssh to a router")
	flag.StringVar(&pass, "password", "", "Password to use for ssh session")
	flag.IntVar(&port, "port", 22, "Port to use for SSH sessions, default 22")
	flag.BoolVar(&notify, "notify", false, "If set to true, email notification will be send.")
	flag.StringVar(&smtpServer, "smtp-server", "", "ip address or dns name with tcp port of smtp server, example: smtp.gmain.com:587")
	flag.StringVar(&smtpUser, "smtp-user", "", "a user name to use to authenticate to the smtp server")
	flag.StringVar(&smtpPass, "smtp-pass", "", "a password to use to authenticate to the smtp server")
	flag.StringVar(&smtpFrom, "smtp-from", "", "email address to use for sending the report from")
	flag.StringVar(&smtpTo, "smtp-to", "", "comma separated list of emails for sending the report to")
}

func remoteHostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	if glog.V(5) {
		glog.Infof("Callback is called with hostname: %s remote address: %s", strings.Split(hostname, ":")[0], remote.String())
	}

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
    | routercommander                  v0.0.4           |
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
	var n messenger.Notifier
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
		go collect(r, commands, n)
	}
	wg.Wait()
}

func repro(r types.Router, commands *types.Commander, n messenger.Notifier) {

}
func collect(r types.Router, commands *types.Commander, n messenger.Notifier) {
	defer wg.Done()
	mode := "collect"
	if commands.Repro != nil {
		mode = "repro"
	}
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
	}
	switch mode {
	case "repro":
		glog.Infof("router %s: mode \"repro\", the command set will be executed %d time(s) with the interval of %d seconds", r.GetName(), iterations, interval)
	case "collect":
		glog.Infof("router %s: mode \"collect\"", r.GetName())
	}
	triggered := false

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

out:
	for it := 0; it < iterations; it++ {
		glog.Infof("router %s: executing iteration - %d/%d:", r.GetName(), it+1, iterations)
		for _, c := range commands.List {
			collectResult := true
			if mode == "repro" {
				collectResult = c.ProcessResult
			}
			results, err := r.ProcessCommand(c, collectResult)
			if err != nil {
				glog.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
				return
			}
			if hc || c.ProcessResult {
				for _, re := range results {
					for _, p := range c.Patterns {
						if i := p.RegExp.FindIndex(re.Result); i != nil {
							// There are to possibilities to react, matching against a pattern and get out if the match is found,
							// OR if capture struct exists, to capture requested field and compare with the previous value, if values are not equal, then get out
							// otherwise continue
							if p.Capture == nil {
								// First case, when only matching is required
								triggered = true
								glog.Errorf("router %s: found matching line: %q, command: %q", r.GetName(), strings.Trim(string(re.Result[i[0]:i[1]]), "\n\r\t"), re.Cmd)
								break out
							}
							if it == 0 {
								// If it is first iteration just storing  first captured value
								v, err := getValue(re.Result, i, p.Capture)
								if err != nil {
									glog.Errorf("failed to extract value of field %d, separator: %q from data: %q with error: %+v", p.Capture.FieldNumber, p.Capture.Separator, string(re.Result), err)
									break out
								}
								p.Capture.Value = v
								continue
							}
							v, err := getValue(re.Result, i, p.Capture)
							if err != nil {
								glog.Errorf("failed to extract value of field %d, separator: %q from data: %q with error: %+v", p.Capture.FieldNumber, p.Capture.Separator, string(re.Result), err)
								break out
							}
							if p.Capture.Value != v {
								triggered = true
								glog.Infof("router %s: detected change of value, previous value %+v current value %+v", r.GetName(), p.Capture.Value, v)
								break out
							}
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
				glog.Errorf("router %s: failed to process command %q with error %+v", r.GetName(), c.Cmd, err)
				return
			}
		}
		return
	}
	if !triggered && mode == "repro" {
		glog.Infof("router %s: repro process has not succeeded triggering the failure condition", r.GetName())
		return
	}
	if triggered {
		glog.Errorf("router %s: health check validation failed, check collected log", r.GetName())
	} else {
		glog.Errorf("router %s: collection completed successfully.", r.GetName())
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

func getValue(b []byte, index []int, capture *types.Capture) (interface{}, error) {
	previousEndLine, err := regexp.Compile(`(?m)$`)
	if err != nil {
		return nil, err
	}
	// First, find the start of the line with matching pattern
	sIndex := previousEndLine.FindAllIndex(b[:index[0]], -1)
	if sIndex == nil {
		return nil, fmt.Errorf("failed to find the start of line in data: %s", string(b[:index[0]]))
	}
	// Second, find  the end of the string with matching pattern
	eIndex := previousEndLine.FindIndex(b[sIndex[len(sIndex)-1][0]:])
	if eIndex == nil {
		return nil, fmt.Errorf("failed to find the end of line in data: %s", string(b[sIndex[len(sIndex)-1][0]:]))
	}
	s := string(b[sIndex[len(sIndex)-1][0] : sIndex[len(sIndex)-1][0]+eIndex[0]])
	// Splitting the resulting string using provided separator
	parts := strings.Split(s, capture.Separator)
	if len(parts) < capture.FieldNumber-1 {
		return nil, fmt.Errorf("failed to split string %s with separator %q to have field number %d", s, capture.Separator, capture.FieldNumber)
	}

	return strings.Trim(parts[capture.FieldNumber-1], " \n\t,"), nil
}
