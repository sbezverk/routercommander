package main

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func newTestPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	privateKey, err := rsaGenerateKey()
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	return signer.PublicKey()
}

func rsaGenerateKey() (any, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

type dummyAddr string

func (d dummyAddr) Network() string { return "tcp" }
func (d dummyAddr) String() string  { return string(d) }

func newTestVerifier(t *testing.T, knownHostsFile string, isInsecure bool) *verifier {
	t.Helper()

	vf, err := NewVerifier(knownHostsFile, isInsecure)
	if err != nil {
		t.Fatalf("failed to initialize verifier: %v", err)
	}
	v, ok := vf.(*verifier)
	if !ok {
		t.Fatalf("expected concrete verifier implementation")
	}
	t.Cleanup(func() {
		if v.f != nil {
			_ = v.f.Close()
		}
	})

	return v
}

func TestNormalizeKnownHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "hostname with default port", in: "Router1.example.com:22", want: "router1.example.com"},
		{name: "hostname only", in: "Router1.example.com", want: "router1.example.com"},
		{name: "ipv4 with default port", in: "192.0.2.10:22", want: "192.0.2.10"},
		{name: "ipv6 bracketed with default port", in: "[2001:db8::1]:22", want: "2001:db8::1"},
		{name: "ipv6 bracketed without port", in: "[2001:db8::1]", want: "2001:db8::1"},
		{name: "hostname with non-default port", in: "Router1.example.com:2222", want: "router1.example.com:2222"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeKnownHost(tt.in); got != tt.want {
				t.Fatalf("normalizeKnownHost(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRemoteHostKeyCallbackTOFUAndMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	khFile := filepath.Join(tmpDir, "known_hosts")
	v := newTestVerifier(t, khFile, false)

	host := "Router1.example.com:22"
	normalizedHost := "router1.example.com"
	addr := dummyAddr("192.0.2.10:22")
	key1 := newTestPublicKey(t)
	key2 := newTestPublicKey(t)

	if err := v.remoteHostKeyCallback(host, addr, key1); err != nil {
		t.Fatalf("expected first-seen host key to be accepted via TOFU, got error: %v", err)
	}
	if got, ok := v.knownHosts[normalizedHost]; !ok {
		t.Fatalf("expected learned host %q to be stored in knownHosts", normalizedHost)
	} else if !strings.EqualFold(ssh.FingerprintSHA256(got), ssh.FingerprintSHA256(key1)) {
		t.Fatalf("stored key fingerprint %s does not match learned key fingerprint %s", ssh.FingerprintSHA256(got), ssh.FingerprintSHA256(key1))
	}

	contents, err := os.ReadFile(khFile)
	if err != nil {
		t.Fatalf("failed to read known hosts file: %v", err)
	}
	foundNormalized := false
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		_, hosts, _, _, _, err := ssh.ParseKnownHosts(scanner.Bytes())
		if err != nil {
			t.Fatalf("failed to parse persisted known_hosts line: %v", err)
		}
		for _, hostEntry := range hosts {
			if normalizeKnownHost(hostEntry) == normalizedHost {
				foundNormalized = true
				break
			}
		}
		if foundNormalized {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("failed reading known_hosts content: %v", err)
	}
	if !foundNormalized {
		t.Fatalf("known hosts file does not contain normalized host %q: %s", normalizedHost, string(contents))
	}

	if err := v.remoteHostKeyCallback(host, addr, key1); err != nil {
		t.Fatalf("expected previously learned host key to be accepted, got error: %v", err)
	}

	if err := v.remoteHostKeyCallback(host, addr, key2); err == nil {
		t.Fatalf("expected mismatched host key to be rejected")
	}
}

func TestNewVerifierProvidesSSHConfig(t *testing.T) {
	tmpDir := t.TempDir()
	khFile := filepath.Join(tmpDir, "known_hosts")
	v := newTestVerifier(t, khFile, false)

	cfg := v.GetSSHConfig("cisco", "cisco123")
	if cfg == nil {
		t.Fatalf("expected non-nil SSH config")
	}
	if cfg.User != "cisco" {
		t.Fatalf("expected ssh user %q, got %q", "cisco", cfg.User)
	}
	if len(cfg.Auth) != 1 {
		t.Fatalf("expected a single auth method, got %d", len(cfg.Auth))
	}
	if cfg.HostKeyCallback == nil {
		t.Fatalf("expected host key callback to be configured")
	}
}
