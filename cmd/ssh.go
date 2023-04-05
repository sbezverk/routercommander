package main

import (
	"net"
	"strings"

	"github.com/golang/glog"
	"golang.org/x/crypto/ssh"
)

func remoteHostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	if glog.V(5) {
		glog.Infof("Callback is called with hostname: %s remote address: %s", strings.Split(hostname, ":")[0], remote.String())
	}

	return nil
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
