package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHumanSize(t *testing.T) {
	cases := []struct {
		size int64
		exp  string
	}{
		{size: 0, exp: "0 B"},
		{size: 512, exp: "512 B"},
		{size: 1536, exp: "1.5 KB"},
		{size: 5 * 1024 * 1024, exp: "5.0 MB"},
	}
	for _, tc := range cases {
		if got := humanSize(tc.size); got != tc.exp {
			t.Fatalf("humanSize(%d) = %q want %q", tc.size, got, tc.exp)
		}
	}
}

func TestNewConnectFormLeavesPortEmpty(t *testing.T) {
	app := New()
	if got := app.connect.values[connectPort]; got != "" {
		t.Fatalf("default port = %q want empty", got)
	}
}

func TestNewWithOptionsUsesLocalCWD(t *testing.T) {
	localCWD := t.TempDir()
	app, err := NewWithOptions(Options{LocalCWD: localCWD})
	if err != nil {
		t.Fatalf("new app with local cwd: %v", err)
	}
	expected, err := filepath.Abs(localCWD)
	if err != nil {
		t.Fatalf("abs local cwd: %v", err)
	}
	if app.local.path != expected {
		t.Fatalf("local path = %q want %q", app.local.path, expected)
	}
}

func TestNewWithOptionsRejectsFileLocalCWD(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if _, err := NewWithOptions(Options{LocalCWD: filePath}); err == nil {
		t.Fatal("expected file local cwd to fail")
	}
}

func TestPasteTextConnectForm(t *testing.T) {
	app := New()
	app.connect.active = connectPassword
	app.pasteText("secret\r\n")
	if got := app.connect.values[connectPassword]; got != "secret" {
		t.Fatalf("pasted password = %q", got)
	}
}

func TestPasteTextPrompt(t *testing.T) {
	app := New()
	app.screen = screenPrompt
	app.prompt.value = "base"
	app.pasteText("-suffix")
	if got := app.prompt.value; got != "base-suffix" {
		t.Fatalf("prompt value = %q", got)
	}
}
