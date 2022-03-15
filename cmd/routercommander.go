package main

import (
	"bufio"
	"bytes"
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
	"golang.org/x/crypto/ssh"
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
	flag.StringVar(&cmdFile, "commands-file", "", "File commands to collect")
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
	commands, err := getInfoFromFile(cmdFile)
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
		go worker(router, commands, sshConfig)
	}
	wg.Wait()
}

// Router interface is a collection of methods
type Router interface {
	closeClient()
	//	getInventory() error
	getName() string
	getType(t string) []*element
	collectOutput(cmds []string) ([]byte, error)
}

type router struct {
	name      string
	client    *ssh.Client
	sshConfig *ssh.ClientConfig
	inventory []*element
}

type element struct {
	Slot    string
	Type    string
	SubType string
	State   string
	Pwr     string
	Shut    string
	Mon     string
}

func newRouter(routerName string, sshConfig *ssh.ClientConfig) (Router, error) {
	sshClient, err := ssh.Dial("tcp", routerName, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial router: %s with error: %+v", routerName, err)
	}
	glog.Infof("Successfully dialed router: %s", routerName)
	return &router{
		name:      routerName,
		client:    sshClient,
		sshConfig: sshConfig,
		inventory: []*element{},
	}, nil
}

func (r *router) closeClient() {
	r.client.Close()
}

func (r *router) getName() string {
	return r.name
}

func (r *router) getType(t string) []*element {
	// TODO RP is a special case needs to treat it separaterly
	elements := []*element{}
	for _, e := range r.inventory {
		if strings.ToUpper(t) == strings.ToUpper(e.Type) {
			elements = append(elements, e)
		}
	}
	return elements
}

// func (r *router) getInventory() error {
// 	reply, err := runCmd(*inventoryCommand, r.client)
// 	if err != nil {
// 		return fmt.Errorf("Failed to run inventory command: %s with error: %+v", *inventoryCommand, err)
// 	}
// 	log.Debugf("getInventory(): Received reply from %+v of %d bytes, Reply: %s ", r.client.Conn.RemoteAddr(), len(reply), string(reply))

// 	result, err := parseReply(reply)
// 	if err != nil {
// 		return fmt.Errorf("Failed to parse reply for router: %s with error: %+v", r.name, err)
// 	}
// 	log.Debugf("getInventory(): parseReply result of %d bytes, Result: %v ", len(result), result)

// 	r.inventory, err = parseInventory(result)
// 	if err != nil {
// 		return fmt.Errorf("Failed to parse inventory for router: %s with error: %+v", r.name, err)
// 	}
// 	log.Debugf("getInventory(): parseInventory router: %s inventory: %+v", r.name, r.inventory)

// 	return nil
// }

func (r *router) collectOutput(cmds []string) ([]byte, error) {
	// Create sesssion
	session, err := r.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to establish a session with error: %+v", err)
	}

	defer session.Close()
	var buffInfo bytes.Buffer
	var buffErr bytes.Buffer
	// Enable system stdout
	// Comment these if you uncomment to store in variable
	session.Stdout = &buffInfo
	session.Stderr = &buffErr
	//	modes := ssh.TerminalModes{
	//		ssh.ECHO:          1,     // disable echoing
	//		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
	//		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	//	}

	if err := session.RequestPty("vt100", 80, 40, ssh.TerminalModes{}); err != nil {
		return nil, fmt.Errorf("failed to pty with error: %+v", err)
	}

	// StdinPipe for commands
	stdin, err := session.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to establish stdin pipe with error: %+v", err)
	}

	// Start remote shell
	err = session.Shell()
	if err != nil {
		return nil, fmt.Errorf("failed to establish a session shell with error: %+v", err)
	}

	// send the commands
	for _, cmd := range cmds {
		if _, err := fmt.Fprintf(stdin, "%s\n", cmd); err != nil {
			return nil, fmt.Errorf("failed to send command %s  with error: %+v", cmd, err)
		}
	}
	// Closing session
	if _, err := fmt.Fprintf(stdin, "%s\n", "exit"); err != nil {
		return buffErr.Bytes(), fmt.Errorf("failed to send command %s  with error: %+v", "exit", err)
	}
	// Wait for sess to finish

	if err := session.Wait(); err != nil {
		return buffErr.Bytes(), fmt.Errorf("failed to send command %s  with error: %+v", "exit", err)
	}

	return buffInfo.Bytes(), nil
}

func fpx(cmd, slot string) string {
	s := strings.ToLower(slot)
	s = "node" + s
	s = strings.Replace(s, "/", "_", -1)
	c := fmt.Sprintf(cmd, s)
	return c
}

func worker(rn string, commands []string, sshConfig *ssh.ClientConfig) {
	defer wg.Done()
	glog.Infof("router name: %s", rn)
	routerName := string(rn) + ":22"

	router, err := newRouter(routerName, sshConfig)
	if err != nil {
		glog.Errorf("Failed to instantiate router object with error: %+v", err)
		return
	}
	defer router.closeClient()

	// // Bulding inventory struct of the router
	// if err := router.getInventory(); err != nil {
	// 	glog.Errorf("Failed to collect router: %s  inventory with error: %+v", router.getName(), err)
	// 	return
	// }

	result, err := router.collectOutput(commands)
	if err != nil {
		glog.Errorf("failed to collect output on router: %s wirh error: %+v", router.getName(), err)
		if result != nil {
			// In case of an error result carries stderr
			glog.Errorf("stderr: %s", string(result))
		}
		return
	}

	glog.Infof("router name: %s \n ----------------------- \n results: %s", router.getName(), string(result))
}

func parseReply(reply []byte) ([]string, error) {
	result := []string{}
	sr := bufio.NewReader(strings.NewReader(string(reply)))
	for {
		l := ""
		b, _, err := sr.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		for _, e := range strings.Split(string(b), " ") {
			if e != "" {
				if l != "" {
					l += ","
				}
				l += e
			}
		}
		result = append(result, string(l))
	}
	return result, nil
}

func parseInventory(data []string) ([]*element, error) {
	var validSlot = regexp.MustCompile(`[0-9]+/[a-zA-Z0-9]+/[a-zA-Z0-9]+`)
	elements := []*element{}
	for i, l := range data {
		p := strings.Split(l, ",")
		if !validSlot.MatchString(p[0]) {
			glog.Infof("parseInventory(): %s is not a valid slot", p[0])
			continue
		}
		e := element{
			Slot:    p[0],
			Type:    p[1],
			SubType: p[2],
		}
		if len(p) >= 7 {
			e.Pwr = p[len(p)-3]
			e.Shut = p[len(p)-2]
			e.Mon = p[len(p)-1]
			for i := 0; i < len(p)-6; i++ {
				e.State += p[3+i] + " "
			}
		}
		e.State = strings.Trim(e.State, " ")
		elements = append(elements, &e)
		glog.Infof("parseInventory(): parsed inventory elemenet number: %d value: %v", i, e)
	}
	if len(elements) == 0 {
		return nil, fmt.Errorf("no inventory data found")
	}
	return elements, nil
}
