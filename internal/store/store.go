// Package store persists saved connections and app settings.
// Non-secret fields live in a JSON file under the user config dir; passwords
// and passphrases go to the OS keyring, falling back to a 0600 secrets file
// when no keyring service is available.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

const keyringService = "db-copier"

type SSHConfig struct {
	Enabled    bool   `json:"enabled"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	AuthMethod string `json:"authMethod"` // password | key | agent
	KeyPath    string `json:"keyPath"`
	Password   string `json:"password,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

type Connection struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Color    string    `json:"color"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	Database string    `json:"database"`
	User     string    `json:"user"`
	Password string    `json:"password,omitempty"`
	SSLMode  string    `json:"sslMode"`
	SSH      SSHConfig `json:"ssh"`
}

type Settings struct {
	PgDumpPath    string `json:"pgDumpPath"`
	PgRestorePath string `json:"pgRestorePath"`
}

type fileData struct {
	Connections []Connection `json:"connections"`
	Settings    Settings     `json:"settings"`
}

type Store struct {
	mu   sync.Mutex
	dir  string
	data fileData
}

func New() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return NewAt(filepath.Join(base, "db-copier"))
}

func NewAt(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir}
	b, err := os.ReadFile(s.dataPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, fmt.Errorf("parse %s: %w", s.dataPath(), err)
		}
	}
	return s, nil
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) dataPath() string    { return filepath.Join(s.dir, "connections.json") }
func (s *Store) secretsPath() string { return filepath.Join(s.dir, "secrets.json") }

// List returns all connections with secrets filled in, sorted by name.
func (s *Store) List() ([]Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Connection, 0, len(s.data.Connections))
	for _, c := range s.data.Connections {
		s.loadSecrets(&c)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (s *Store) Get(id string) (Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.data.Connections {
		if c.ID == id {
			s.loadSecrets(&c)
			return c, nil
		}
	}
	return Connection{}, fmt.Errorf("connection %q not found", id)
}

// Save inserts or updates a connection and returns the stored version.
func (s *Store) Save(c Connection) (Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if err := s.storeSecrets(c); err != nil {
		return Connection{}, err
	}
	stripped := c
	stripped.Password = ""
	stripped.SSH.Password = ""
	stripped.SSH.Passphrase = ""

	replaced := false
	for i := range s.data.Connections {
		if s.data.Connections[i].ID == c.ID {
			s.data.Connections[i] = stripped
			replaced = true
			break
		}
	}
	if !replaced {
		s.data.Connections = append(s.data.Connections, stripped)
	}
	return c, s.flush()
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.data.Connections[:0]
	for _, c := range s.data.Connections {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	s.data.Connections = kept
	s.storeSecrets(Connection{ID: id})
	return s.flush()
}

func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Settings
}

func (s *Store) SaveSettings(st Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Settings = st
	return s.flush()
}

func (s *Store) flush() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.dataPath(), b, 0o600)
}

func secretKeys(id string) map[string]string {
	return map[string]string{
		"password":       id + ":password",
		"ssh_password":   id + ":ssh_password",
		"ssh_passphrase": id + ":ssh_passphrase",
	}
}

func (s *Store) storeSecrets(c Connection) error {
	values := map[string]string{
		"password":       c.Password,
		"ssh_password":   c.SSH.Password,
		"ssh_passphrase": c.SSH.Passphrase,
	}
	keys := secretKeys(c.ID)
	if keyringAvailable() {
		for k, v := range values {
			if v == "" {
				_ = keyring.Delete(keyringService, keys[k])
				continue
			}
			if err := keyring.Set(keyringService, keys[k], v); err != nil {
				return fmt.Errorf("keyring: %w", err)
			}
		}
		return nil
	}
	file := s.readSecretsFile()
	for k, v := range values {
		if v == "" {
			delete(file, keys[k])
		} else {
			file[keys[k]] = v
		}
	}
	b, _ := json.MarshalIndent(file, "", "  ")
	return writeFileAtomic(s.secretsPath(), b, 0o600)
}

func (s *Store) loadSecrets(c *Connection) {
	keys := secretKeys(c.ID)
	get := func(k string) string {
		if keyringAvailable() {
			v, _ := keyring.Get(keyringService, keys[k])
			return v
		}
		return s.readSecretsFile()[keys[k]]
	}
	c.Password = get("password")
	c.SSH.Password = get("ssh_password")
	c.SSH.Passphrase = get("ssh_passphrase")
}

func (s *Store) readSecretsFile() map[string]string {
	m := map[string]string{}
	if b, err := os.ReadFile(s.secretsPath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

var (
	keyringOnce sync.Once
	keyringOK   bool
)

// keyringAvailable probes the OS keyring once. Tests can disable it via
// DB_COPIER_NO_KEYRING=1.
func keyringAvailable() bool {
	keyringOnce.Do(func() {
		if os.Getenv("DB_COPIER_NO_KEYRING") == "1" {
			return
		}
		const probe = "__probe__"
		if err := keyring.Set(keyringService, probe, "1"); err != nil {
			return
		}
		_ = keyring.Delete(keyringService, probe)
		keyringOK = true
	})
	return keyringOK
}

func writeFileAtomic(path string, b []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
