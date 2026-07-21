package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type testSecrets struct{ values map[string]string }

func (s *testSecrets) Get(name string) (string, error) {
	value, ok := s.values[name]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}
func (s *testSecrets) Set(name, value string) error { s.values[name] = value; return nil }
func (s *testSecrets) Delete(name string) error     { delete(s.values, name); return nil }

func TestSaveCreatesPrivateAtomicConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "profiles.json")
	store := &Store{Path: path, Secrets: &testSecrets{values: map[string]string{}}}
	if err := store.Save(Config{Profiles: map[string]Profile{"default": {URL: "https://zabbix.example/api_jsonrpc.php", TokenID: "1"}}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("config permissions = %o, want 0600", info.Mode().Perm())
	}
	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profiles["default"].TokenID != "1" {
		t.Fatalf("profile = %#v", cfg.Profiles["default"])
	}
}
