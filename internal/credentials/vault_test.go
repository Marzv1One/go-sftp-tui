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

func TestMergeConfigPrefersTypedValues(t *testing.T) {
	stored := sftpclient.Config{Host: "stored", Port: "22", Username: "stored-user", Password: "stored-pass"}
	typed := sftpclient.Config{Host: "typed", Username: "typed-user"}
	got := MergeConfig(stored, typed)
	if got.Host != "typed" || got.Username != "typed-user" || got.Password != "stored-pass" || got.Port != "22" {
		t.Fatalf("merged config = %#v", got)
	}
}
