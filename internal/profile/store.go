package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

const keyringService = "zbxcli"

type Profile struct {
	URL           string     `json:"url"`
	Username      string     `json:"username,omitempty"`
	APIVersion    string     `json:"api_version"`
	TokenID       string     `json:"token_id"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	TLSThumbprint string     `json:"tls_thumbprint,omitempty"`
}

type Config struct {
	Profiles map[string]Profile `json:"profiles"`
}

type SecretStore interface {
	Get(profile string) (string, error)
	Set(profile, token string) error
	Delete(profile string) error
}

type KeyringStore struct{}

func (KeyringStore) Get(profile string) (string, error) { return keyring.Get(keyringService, profile) }
func (KeyringStore) Set(profile, token string) error {
	return keyring.Set(keyringService, profile, token)
}
func (KeyringStore) Delete(profile string) error { return keyring.Delete(keyringService, profile) }

type Store struct {
	Path    string
	Secrets SecretStore
}

func Default() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate user config directory: %w", err)
	}
	return &Store{Path: filepath.Join(dir, "zbxcli", "profiles.json"), Secrets: KeyringStore{}}, nil
}

func (s *Store) Load() (Config, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Profiles: make(map[string]Profile)}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read profiles: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse profiles: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]Profile)
	}
	return cfg, nil
}

func (s *Store) Save(cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(s.Path), 0700); err != nil {
		return fmt.Errorf("secure config directory: %w", err)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profiles: %w", err)
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".profiles-*")
	if err != nil {
		return fmt.Errorf("create profile temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure profile temp file: %w", err)
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("write profiles: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close profiles: %w", err)
	}
	if err := os.Rename(tmpName, s.Path); err != nil {
		return fmt.Errorf("replace profiles: %w", err)
	}
	return os.Chmod(s.Path, 0600)
}

func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "/\\\x00") {
		return errors.New("profile name must not be empty or contain path separators")
	}
	return nil
}

func Names(cfg Config) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
