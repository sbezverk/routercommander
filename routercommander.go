package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

const (
	inventoryCommand = "show module"
)

var (
	list     = flag.String("routerlist", "", "List with routers' names")
	login    = flag.String("username", "admin", "username to use to ssh to a router")
	password = flag.String("password", "", "Password to use for ssh session")
	logging  = flag.Int("v", 4, "Logging verbosity level, 1 - Panic, 2 - Fatal, 3 - Error, 4 - Warining, 5 - Info, 6 - Debug")
	wg       sync.WaitGroup
)

var log *logrus.Logger

func init() {
	flag.Parse()
	log = logrus.New()
	log.Formatter = new(logrus.JSONFormatter)
	log.Formatter = new(logrus.TextFormatter)                  //default
	log.Formatter.(*logrus.TextFormatter).DisableColors = true // remove colors
	switch *logging {
	case 1:
		log.Level = logrus.PanicLevel
	case 2:
		log.Level = logrus.FatalLevel
	case 3:
		log.Level = logrus.ErrorLevel
	case 4:
		log.Level = logrus.WarnLevel
	case 5:
		log.Level = logrus.InfoLevel
	case 6:
		log.Level = logrus.DebugLevel
	default:
		log.Fatalf("Inavlid value %d for logging verbosity level", *logging)
	}
}

func remoteHostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	log.Infof("Callback is called with hostname: %s remote address: %s", hostname, remote.String())
	return nil
}

func main() {

	if *login == "" || *password == "" {
		log.Fatalf("--username and --password are mandatory parameters, exiting...")
		flag.Usage()
	}
	commands := []string{"run on -f %s pcie_cfrw -w 0 0 0 2 2 1"}

	f, err := os.OpenFile(*list, os.O_RDONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open routers' list file with error: %+v", err)
	}
	defer f.Close()

	sshConfig := &ssh.ClientConfig{
		User: *login,
		Auth: []ssh.AuthMethod{
			ssh.Password(*password),
		},
		HostKeyCallback: remoteHostKeyCallback,
	}

	fr := bufio.NewReader(f)
	for {
		router, _, err := fr.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("Failed to read routers' list file with error: %+v", err)
		}

		wg.Add(1)
		go worker(router, commands, "FP-X", sshConfig)
	}
	wg.Wait()
}

// Router interface is a collection of methods
type Router interface {
	closeClient()
	getInventory() error
	getName() string
	getType(t string) []*element
	collectOutput(elements []*element, cmds []string, conversion func(string, string) string) []string
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
		return nil, fmt.Errorf("Failed to dial router: %s with error: %+v", routerName, err)
	}
	log.Debugf("Successfully dialed router: %s", routerName)
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

func runCmd(cmd string, client *ssh.Client) ([]byte, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to establish a session with error: %+v", err)
	}
	defer session.Close()
	reply, err := session.Output(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to run command %s with error: %+v", cmd, err)
	}
	return reply, nil
}

func (r *router) getInventory() error {
	reply, err := runCmd(inventoryCommand, r.client)
	if err != nil {
		return fmt.Errorf("Failed to run inventory command: %s with error: %+v", inventoryCommand, err)
	}
	log.Debugf("getInventory(): Received reply from %+v of %d bytes, Reply: %s ", r.client.Conn.RemoteAddr(), len(reply), string(reply))

	result, err := parseReply(reply)
	if err != nil {
		return fmt.Errorf("Failed to parse reply for router: %s with error: %+v", r.name, err)
	}
	log.Debugf("getInventory(): parseReply result of %d bytes, Result: %v ", len(result), result)

	r.inventory, err = parseInventory(result)
	if err != nil {
		return fmt.Errorf("Failed to parse inventory for router: %s with error: %+v", r.name, err)
	}
	log.Debugf("getInventory(): parseInventory router: %s inventory: %+v", r.name, r.inventory)

	return nil
}

func (r *router) collectOutput(elements []*element, cmds []string, conversion func(string, string) string) []string {
	result := []string{}
	for _, element := range elements {
		for _, cmd := range cmds {
			c := conversion(cmd, element.Slot)
			reply, err := runCmd(c, r.client)
			if err != nil {
				log.Warnf("collectOutput(): Failed to run command %s against router: %+v with error: %+v", c, r.client.Conn.RemoteAddr(), err)
				continue
			}
			log.Debugf("collectOutput(): Received reply from %+v of %d bytes for command: %s, Reply: %s ",
				r.client.Conn.RemoteAddr(), len(reply), c, string(reply))
			result = append(result, string(reply))
		}
	}
	return result
}

func fpx(cmd, slot string) string {
	s := strings.ToLower(slot)
	s = "node" + s
	s = strings.Replace(s, "/", "_", -1)
	c := fmt.Sprintf(cmd, s)
	return c
}

func worker(rn []byte, commands []string, elementType string, sshConfig *ssh.ClientConfig) {
	defer wg.Done()
	log.Infof("router name: %s", string(rn))
	routerName := string(rn) + ":22"

	router, err := newRouter(routerName, sshConfig)
	if err != nil {
		log.Errorf("Failed to instantiate router object with error: %+v", err)
		return
	}
	defer router.closeClient()

	// Bulding inventory struct of the router
	if err := router.getInventory(); err != nil {
		log.Errorf("Failed to collect router: %s  inventory with error: %+v", router.getName(), err)
		return
	}

	e := router.getType(elementType)
	if len(e) == 0 {
		// Nothing to do
		log.Errorf("No elements of type: %s was found in router: %s  inventory", elementType, router.getName())
		return
	}

	result := router.collectOutput(e, commands, fpx)
	if len(result) == 0 {
		log.Errorf("failed to collect output on router: %s", router.getName())
		return
	}
	log.Printf("router name: %s \n ----------------------- \n results: %+v", router.getName(), result)
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
			log.Infof("parseInventory(): %s is not a valid slot", p[0])
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
		log.Debugf("parseInventory(): parsed inventory elemenet number: %d value: %v", i, e)
	}
	if len(elements) == 0 {
		return nil, fmt.Errorf("no inventory data found")
	}
	return elements, nil
}
