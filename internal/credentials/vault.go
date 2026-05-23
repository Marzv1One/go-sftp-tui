package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sftp-tui/internal/sftpclient"

	"filippo.io/age"
)

type Store interface {
	Get(passphrase, name string) (Entry, error)
	Save(passphrase, name string, entry Entry) error
	List(passphrase string) ([]string, error)
}

type Entry struct {
	Host       string    `json:"host"`
	Port       string    `json:"port"`
	Username   string    `json:"username"`
	Password   string    `json:"password,omitempty"`
	PrivateKey string    `json:"privateKey,omitempty"`
	Passphrase string    `json:"passphrase,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type AgeVault struct {
	Path string
}

func DefaultVaultPath() string {
	configDir, err := os.UserConfigDir()
	if err == nil && configDir != "" {
		return filepath.Join(configDir, "sftp-tui", "credentials.age")
	}
	homeDir, err := os.UserHomeDir()
	if err == nil && homeDir != "" {
		return filepath.Join(homeDir, ".sftp-tui", "credentials.age")
	}
	return "credentials.age"
}

func (v AgeVault) Get(passphrase, name string) (Entry, error) {
	entries, err := v.load(passphrase)
	if err != nil {
		return Entry{}, err
	}
	entry, ok := entries[normalizeName(name)]
	if !ok {
		return Entry{}, fmt.Errorf("credential %q not found", name)
	}
	return entry, nil
}

func (v AgeVault) Save(passphrase, name string, entry Entry) error {
	name = normalizeName(name)
	if name == "" {
		return errors.New("credential name is required")
	}
	entries, err := v.load(passphrase)
	if err != nil {
		return err
	}
	entry.UpdatedAt = time.Now().UTC()
	entries[name] = entry
	return v.save(passphrase, entries)
}

func (v AgeVault) List(passphrase string) ([]string, error) {
	entries, err := v.load(passphrase)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (e Entry) Config() sftpclient.Config {
	return sftpclient.Config{
		Host:       e.Host,
		Port:       e.Port,
		Username:   e.Username,
		Password:   e.Password,
		PrivateKey: e.PrivateKey,
		Passphrase: e.Passphrase,
	}
}

func EntryFromConfig(cfg sftpclient.Config) Entry {
	return Entry{
		Host:       cfg.Host,
		Port:       cfg.Port,
		Username:   cfg.Username,
		Password:   cfg.Password,
		PrivateKey: cfg.PrivateKey,
		Passphrase: cfg.Passphrase,
	}
}

func MergeConfig(stored, typed sftpclient.Config) sftpclient.Config {
	if typed.Host != "" {
		stored.Host = typed.Host
	}
	if typed.Port != "" {
		stored.Port = typed.Port
	}
	if typed.Username != "" {
		stored.Username = typed.Username
	}
	if typed.Password != "" {
		stored.Password = typed.Password
	}
	if typed.PrivateKey != "" {
		stored.PrivateKey = typed.PrivateKey
	}
	if typed.Passphrase != "" {
		stored.Passphrase = typed.Passphrase
	}
	if stored.Port == "" {
		stored.Port = "22"
	}
	if typed.Timeout > 0 {
		stored.Timeout = typed.Timeout
	}
	return stored
}

func (v AgeVault) load(passphrase string) (map[string]Entry, error) {
	if passphrase == "" {
		return nil, errors.New("vault passphrase is required")
	}
	vaultPath := v.path()
	file, err := os.Open(vaultPath)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, err
	}
	reader, err := age.Decrypt(file, identity)
	if err != nil {
		return nil, err
	}
	plaintext, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(plaintext))) == 0 {
		return map[string]Entry{}, nil
	}
	entries := map[string]Entry{}
	if err := json.Unmarshal(plaintext, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (v AgeVault) save(passphrase string, entries map[string]Entry) error {
	if passphrase == "" {
		return errors.New("vault passphrase is required")
	}
	vaultPath := v.path()
	if err := os.MkdirAll(filepath.Dir(vaultPath), 0o700); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(vaultPath), ".credentials-*.age")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		_ = tempFile.Close()
		return err
	}
	writer, err := age.Encrypt(tempFile, recipient)
	if err != nil {
		_ = tempFile.Close()
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(entries); err != nil {
		_ = writer.Close()
		_ = tempFile.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, vaultPath)
}

func (v AgeVault) path() string {
	if v.Path != "" {
		return v.Path
	}
	return DefaultVaultPath()
}

func normalizeName(name string) string {
	return strings.TrimSpace(name)
}
