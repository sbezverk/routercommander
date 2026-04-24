package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/golang/glog"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Verifier interface {
	GetSSHConfig(user, pass string) *ssh.ClientConfig
}

var _ Verifier = &verifier{}

type verifier struct {
	mx         sync.Mutex
	f          *os.File
	knownHosts map[string]ssh.PublicKey
	insecure   bool
	sshClient  *ssh.ClientConfig
}

func (v *verifier) GetSSHConfig(user, pass string) *ssh.ClientConfig {
	c := ssh.ClientConfig{}
	if v.sshClient != nil {
		c = *v.sshClient
	}
	c.User = user
	c.Auth = []ssh.AuthMethod{
		ssh.Password(pass),
	}

	return &c
}

func (v *verifier) remoteHostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	if glog.V(5) {
		glog.Infof("Callback is called with hostname: %s remote address: %s", strings.Split(hostname, ":")[0], remote.String())
	}
	if !v.insecure {
		v.mx.Lock()
		defer v.mx.Unlock()
		normalizedHost := normalizeKnownHost(hostname)
		if k, ok := v.knownHosts[normalizedHost]; ok {
			if bytes.Equal(k.Marshal(), key.Marshal()) {
				return nil
			}
			return fmt.Errorf("host key mismatch for host %s, expected: %s, got: %s", normalizedHost, ssh.FingerprintSHA256(k), ssh.FingerprintSHA256(key))
		} else {
			// Need to update known hosts file with the new host key
			line := knownhosts.Line([]string{normalizedHost}, key)
			if _, err := v.f.WriteString(line + "\n"); err != nil {
				return fmt.Errorf("failed to update known hosts file with new host key for host %s: %+v", normalizedHost, err)
			}
			v.knownHosts[normalizedHost] = key
		}

	}

	return nil
}

func normalizeKnownHost(hostname string) string {
	host := strings.TrimSpace(hostname)

	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	host = strings.ToLower(host)

	return host
}

func NewVerifier(knownHostsFile string, isInsecure bool) (Verifier, error) {
	v := &verifier{
		sshClient: nil,
	}
	v.insecure = isInsecure
	var err error
	v.f, err = os.OpenFile(knownHostsFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open known hosts file %s with error: %+v", knownHostsFile, err)
	}
	v.knownHosts = make(map[string]ssh.PublicKey)
	scanner := bufio.NewScanner(v.f)
	for scanner.Scan() {
		line := scanner.Bytes()
		_, hosts, key, _, _, err := ssh.ParseKnownHosts(line)
		if err != nil {
			return nil, fmt.Errorf("failed to parse known hosts file %s with error: %+v", knownHostsFile, err)
		}
		for _, host := range hosts {
			v.knownHosts[normalizeKnownHost(host)] = key
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read known hosts file %s with error: %+v", knownHostsFile, err)
	}

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

	v.sshClient = &ssh.ClientConfig{
		//		User: login,
		//		Auth: []ssh.AuthMethod{
		//			ssh.Password(pass),
		//		},
		Config:          c,
		HostKeyCallback: v.remoteHostKeyCallback,
	}

	return v, nil
}
