// Package tunnel opens an SSH local port-forward to a remote Postgres host.
package tunnel

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"db-copier/internal/store"
)

type Tunnel struct {
	client   *ssh.Client
	listener net.Listener
	wg       sync.WaitGroup
	once     sync.Once
}

// Open connects to the SSH server and forwards a random local port to
// remoteHost:remotePort as seen from the SSH server. knownHostsPath stores
// host keys trust-on-first-use; a changed key is rejected.
func Open(cfg store.SSHConfig, remoteHost string, remotePort int, knownHostsPath string) (*Tunnel, error) {
	auth, err := authMethods(cfg)
	if err != nil {
		return nil, err
	}
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: tofuCallback(knownHostsPath),
		Timeout:         15 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("ssh %s: %w", addr, err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		client.Close()
		return nil, err
	}
	t := &Tunnel{client: client, listener: ln}
	target := net.JoinHostPort(remoteHost, strconv.Itoa(remotePort))
	t.wg.Add(1)
	go t.serve(target)
	return t, nil
}

// LocalPort is the port on 127.0.0.1 that forwards to the remote database.
func (t *Tunnel) LocalPort() int { return t.listener.Addr().(*net.TCPAddr).Port }

func (t *Tunnel) Close() error {
	t.once.Do(func() {
		t.listener.Close()
		t.client.Close()
	})
	t.wg.Wait()
	return nil
}

func (t *Tunnel) serve(target string) {
	defer t.wg.Done()
	for {
		local, err := t.listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer local.Close()
			remote, err := t.client.Dial("tcp", target)
			if err != nil {
				return
			}
			defer remote.Close()
			done := make(chan struct{}, 2)
			go func() { io.Copy(remote, local); done <- struct{}{} }()
			go func() { io.Copy(local, remote); done <- struct{}{} }()
			<-done
		}()
	}
}

func authMethods(cfg store.SSHConfig) ([]ssh.AuthMethod, error) {
	switch cfg.AuthMethod {
	case "password":
		return []ssh.AuthMethod{
			ssh.Password(cfg.Password),
			ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
				ans := make([]string, len(qs))
				for i := range ans {
					ans[i] = cfg.Password
				}
				return ans, nil
			}),
		}, nil
	case "key":
		path := expandHome(cfg.KeyPath)
		pem, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read key %s: %w", path, err)
		}
		var signer ssh.Signer
		if cfg.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(pem, []byte(cfg.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(pem)
		}
		if err != nil {
			var missing *ssh.PassphraseMissingError
			if errors.As(err, &missing) {
				return nil, errors.New("private key is encrypted: passphrase required")
			}
			return nil, fmt.Errorf("parse key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	case "agent", "":
		sock := os.Getenv("SSH_AUTH_SOCK")
		if sock == "" {
			return nil, errors.New("SSH_AUTH_SOCK not set: no ssh-agent available")
		}
		conn, err := net.Dial("unix", sock)
		if err != nil {
			return nil, fmt.Errorf("ssh-agent: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeysCallback(agent.NewClient(conn).Signers)}, nil
	default:
		return nil, fmt.Errorf("unknown ssh auth method %q", cfg.AuthMethod)
	}
}

var knownHostsMu sync.Mutex

func tofuCallback(path string) ssh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		knownHostsMu.Lock()
		defer knownHostsMu.Unlock()
		want := key.Type() + " " + base64.StdEncoding.EncodeToString(key.Marshal())
		if f, err := os.Open(path); err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				host, stored, ok := strings.Cut(sc.Text(), " ")
				if !ok || host != hostname {
					continue
				}
				f.Close()
				if stored != want {
					return fmt.Errorf("host key for %s changed (fingerprint now %s); remove its line from %s if this is expected",
						hostname, ssh.FingerprintSHA256(key), path)
				}
				return nil
			}
			f.Close()
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = fmt.Fprintf(f, "%s %s\n", hostname, want)
		return err
	}
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
