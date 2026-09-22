package credentials

import (
	"path/filepath"
	"testing"

	"sftp-tui/internal/sftpclient"
)

func TestAgeVaultSaveGetList(t *testing.T) {
	vault := AgeVault{Path: filepath.Join(t.TempDir(), "credentials.age")}
	entry := Entry{
		Host:       "example.com",
		Port:       "2222",
		Username:   "user",
		Password:   "secret",
		PrivateKey: "",
		Passphrase: "",
	}
	if err := vault.Save("vault-passphrase", "prod", entry); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	got, err := vault.Get("vault-passphrase", "prod")
	if err != nil {
		t.Fatalf("get credential: %v", err)
	}
	if got.Host != entry.Host || got.Port != entry.Port || got.Username != entry.Username || got.Password != entry.Password {
		t.Fatalf("stored credential mismatch: %#v", got)
	}
	names, err := vault.List("vault-passphrase")
	if err != nil {
		t.Fatalf("list credentials: %v", err)
	}
	if len(names) != 1 || names[0] != "prod" {
		t.Fatalf("names = %#v", names)
	}
}

func TestAgeVaultDelete(t *testing.T) {
	vault := AgeVault{Path: filepath.Join(t.TempDir(), "credentials.age")}
	entry := Entry{Host: "example.com", Port: "22", Username: "user", Password: "secret"}
	if err := vault.Save("pass", "prod", entry); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := vault.Save("pass", "staging", entry); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := vault.Delete("pass", "prod"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	names, err := vault.List("pass")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 1 || names[0] != "staging" {
		t.Fatalf("names after delete = %#v", names)
	}
}

func TestAgeVaultDeleteNotFound(t *testing.T) {
	vault := AgeVault{Path: filepath.Join(t.TempDir(), "credentials.age")}
	err := vault.Delete("pass", "nonexistent")
	if err == nil {
		t.Fatal("expected error deleting nonexistent credential")
	}
}

func TestMergeConfigPrefersTypedValues(t *testing.T) {
	stored := sftpclient.Config{Host: "stored", Port: "22", Username: "stored-user", Password: "stored-pass"}
	typed := sftpclient.Config{Host: "typed", Port: "2222", Username: "typed-user"}
	got := MergeConfig(stored, typed)
	if got.Host != "typed" || got.Username != "typed-user" || got.Password != "stored-pass" || got.Port != "2222" {
		t.Fatalf("merged config = %#v", got)
	}
}

func TestMergeConfigKeepsStoredPortWhenTypedPortEmpty(t *testing.T) {
	stored := sftpclient.Config{Host: "stored", Port: "2222", Username: "stored-user", Password: "stored-pass"}
	typed := sftpclient.Config{Host: "typed", Username: "typed-user"}
	got := MergeConfig(stored, typed)
	if got.Port != "2222" {
		t.Fatalf("merged port = %q want %q", got.Port, "2222")
	}
}
