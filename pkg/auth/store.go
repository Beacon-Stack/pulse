package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileStore keeps the login settings in one JSON file next to the app's
// database, readable only by the app's user. Deleting the file puts the
// app back to no login, which is how a forgotten password is recovered
// by someone with access to the files.
type FileStore struct {
	Path string
}

// Load reads the saved settings. found is false when nothing has been
// saved yet.
func (f FileStore) Load() (s Settings, found bool, err error) {
	b, err := os.ReadFile(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return Settings{}, false, nil
	}
	if err != nil {
		return Settings{}, false, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}, false, err
	}
	return s, true, nil
}

// Save writes the settings, replacing the file in one step so a crash
// can't leave half of it.
func (f FileStore) Save(s Settings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), ".auth-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.Path)
}

// LoadOrCreateKey returns the API key kept in path, making a new random
// one the first time. Apps that aren't given a key in their config use
// it so the key, and with it every login session, survives a restart.
func LoadOrCreateKey(path string) (key string, created bool, err error) {
	b, err := os.ReadFile(path)
	if err == nil {
		if k := strings.TrimSpace(string(b)); k != "" {
			return k, false, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", false, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", false, err
	}
	key = hex.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return "", false, err
	}
	return key, true, nil
}
