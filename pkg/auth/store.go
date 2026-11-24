package auth

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
