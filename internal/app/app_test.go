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

func TestPromptMakeDirectoryOpensLocalPrompt(t *testing.T) {
	app := New()
	app.active = paneLocal
	cmd := app.promptMakeDirectory()
	if cmd != nil {
		t.Fatal("expected no command")
	}
	if app.screen != screenPrompt {
		t.Fatalf("screen = %v want prompt", app.screen)
	}
	if app.prompt.title != "Create local directory" {
		t.Fatalf("prompt title = %q", app.prompt.title)
	}
	if app.prompt.label != "directory name or path" {
		t.Fatalf("prompt label = %q", app.prompt.label)
	}
}

func TestPromptMakeDirectoryOpensRemotePrompt(t *testing.T) {
	app := New()
	app.active = paneRemote
	cmd := app.promptMakeDirectory()
	if cmd != nil {
		t.Fatal("expected no command")
	}
	if app.screen != screenPrompt {
		t.Fatalf("screen = %v want prompt", app.screen)
	}
	if app.prompt.title != "Create server directory" {
		t.Fatalf("prompt title = %q", app.prompt.title)
	}
	if app.prompt.label != "directory name or path" {
		t.Fatalf("prompt label = %q", app.prompt.label)
	}
}

func TestPromptMakeDirectoryCreatesLocalDirectory(t *testing.T) {
	localDir := t.TempDir()
	app := New()
	app.active = paneLocal
	app.local.path = localDir
	_ = app.promptMakeDirectory()
	cmd := app.prompt.done(app, "downloads")
	if cmd == nil {
		t.Fatal("expected create directory command")
	}
	msg := cmd().(operationDoneMsg)
	if msg.err != nil {
		t.Fatalf("create local directory: %v", msg.err)
	}
	if !msg.refreshLocal {
		t.Fatal("expected local refresh")
	}
	if _, err := os.Stat(filepath.Join(localDir, "downloads")); err != nil {
		t.Fatalf("stat created directory: %v", err)
	}
}

func TestDirectoryTargets(t *testing.T) {
	if got := localDirectoryTarget(filepath.Join("C:", "work"), "downloads"); got != filepath.Join("C:", "work", "downloads") {
		t.Fatalf("relative local directory = %q", got)
	}
	if got := remoteDirectoryTarget("/home/user", "new-dir"); got != "/home/user/new-dir" {
		t.Fatalf("relative remote directory = %q", got)
	}
	if got := remoteDirectoryTarget("/home/user", "/var/tmp/new-dir"); got != "/var/tmp/new-dir" {
		t.Fatalf("absolute remote directory = %q", got)
	}
}
