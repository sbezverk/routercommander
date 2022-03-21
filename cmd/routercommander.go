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
	"github.com/sbezverk/routercommander/pkg/types"
	"golang.org/x/crypto/ssh"
	yaml "gopkg.in/yaml.v2"
)

var (
	rtrFile string
	rtrName string
	cmdFile string
	login   string
	pass    string
)

var wg sync.WaitGroup

func init() {
	runtime.GOMAXPROCS(1)
	flag.StringVar(&rtrFile, "routers-file", "", "File with routers' names")
	flag.StringVar(&cmdFile, "commands-file", "", "YAML formated file with commands to collect")
	flag.StringVar(&rtrName, "router-name", "", "name of the router")
	flag.StringVar(&login, "username", "admin", "username to use to ssh to a router")
	flag.StringVar(&pass, "password", "", "Password to use for ssh session")
}

func remoteHostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	glog.Infof("Callback is called with hostname: %s remote address: %s", hostname, remote.String())
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
	flag.Parse()
	_ = flag.Set("logtostderr", "true")

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

	commands, err := getCommands(cmdFile)
	if err != nil {
		glog.Errorf("failed to get list of commands from file: %s with error: %+v, exiting...", cmdFile, err)
		os.Exit(1)
	}
	sshConfig := &ssh.ClientConfig{
		User: login,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		HostKeyCallback: remoteHostKeyCallback,
	}

	for _, router := range routers {
		wg.Add(1)
		go collect(router, commands, sshConfig)
	}
	wg.Wait()
}

func collect(rn string, commands *types.Commands, sshConfig *ssh.ClientConfig) {
	defer wg.Done()
	glog.Infof("router name: %s", rn)
	routerName := string(rn) + ":22"

	router, err := types.NewRouter(routerName, sshConfig)
	if err != nil {
		glog.Errorf("Failed to instantiate router object with error: %+v", err)
		return
	}
	defer router.Close()

	result, err := router.CollectOutput(commands)
	if err != nil {
		glog.Errorf("failed to collect output on router: %s wirh error: %+v", rn, err)
		if result != nil {
			// In case of an error result carries stderr
			glog.Errorf("stderr: %s", string(result))
		}
		return
	}
	// Saving result in the file
	r, err := os.Create("./" + rn + ".log")
	if err != nil {
		glog.Errorf("failed to create log file for router %s with error: %+v", rn, err)
		return
	}
	defer r.Close()
	if _, err := r.Write(result); err != nil {
		glog.Errorf("failed to write to log file for router %s with error: %+v", rn, err)
	}
}

func getCommands(fn string) (*types.Commands, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, fmt.Errorf("fail to open file %s with error: %+v", fn, err)
	}
	defer f.Close()
	l, err := os.Stat(fn)
	if err != nil {
		return nil, fmt.Errorf("fail to get stats for file %s with error: %+v", fn, err)
	}
	b := make([]byte, l.Size())
	if _, err := f.Read(b); err != nil {
		return nil, fmt.Errorf("fail to read file %s with error: %+v", fn, err)
	}
	c := &types.Commands{
		List: make([]*types.ShowCommand, 0),
	}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("fail tp unmarshal commands yaml file %s with error: %+v", fn, err)
	}

	return c, nil
}
