package app

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"sftp-tui/internal/credentials"
)

type pickerStore struct {
	credentials.Store
	passphrase string
	err        error
}

func (store *pickerStore) List(passphrase string) ([]string, error) {
	store.passphrase = passphrase
	return []string{"development", "production", "staging"}, store.err
}

func press(code rune) tea.KeyPressMsg      { return tea.KeyPressMsg{Code: code} }
func typeText(text string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(text)[0], Text: text} }

func TestCredentialPicker(t *testing.T) {
	app := New()
	store := &pickerStore{}
	app.vault = store
	app.browseCredentials()
	if app.screen != screenPrompt || !app.prompt.masked {
		t.Fatal("expected masked unlock prompt")
	}
	app.prompt.value = " passphrase "
	if strings.Contains(app.promptView(), "passphrase _") {
		t.Fatal("passphrase exposed")
	}
	cmd := app.handlePromptKey(press(tea.KeyEnter))
	if cmd == nil || !app.busy {
		t.Fatal("expected asynchronous vault load")
	}
	app.Update(cmd())
	if store.passphrase != " passphrase " || app.screen != screenCredentials {
		t.Fatal("unlock failed")
	}
	app.handleCredentialKey(typeText("prod"))
	app.handleCredentialKey(press(tea.KeyDown))
	app.handleCredentialKey(press(tea.KeyEnter))
	if app.connect.values[connectSecretName] != "production" || app.connect.active != connectAction || app.screen != screenConnect {
		t.Fatal("profile not selected")
	}
	if len(app.profiles) != 0 {
		t.Fatal("profile list retained after closing")
	}
}

func TestCredentialPickerFailureAndCancel(t *testing.T) {
	app := New()
	app.browseCredentials()
	app.handlePromptKey(press(tea.KeyEscape))
	if app.screen != screenConnect {
		t.Fatal("cancel did not return to connection")
	}
	app.vault = &pickerStore{err: errors.New("wrong passphrase")}
	app.connect.values[connectVaultPassphrase] = "wrong"
	app.Update(app.browseCredentials()())
	if app.busy || app.screen != screenConnect || app.status != "wrong passphrase" {
		t.Fatal("vault error not handled")
	}
	app.Update(profilesLoadedMsg{})
	if app.screen != screenConnect || !strings.Contains(app.status, "No saved profiles") {
		t.Fatal("empty vault not handled")
	}
	app.Update(profilesLoadedMsg{names: []string{"prod"}})
	app.handleCredentialKey(typeText("missing"))
	app.handleCredentialKey(press(tea.KeyEnter))
	if app.screen != screenCredentials {
		t.Fatal("empty match selected")
	}
	app.handleCredentialKey(press(tea.KeyEscape))
	if app.screen != screenConnect {
		t.Fatal("picker cancel failed")
	}
}

func TestSearchAndPathCompletion(t *testing.T) {
	for _, remote := range []bool{false, true} {
		app := New()
		if remote {
			app.active = paneRemote
		}
		pane := app.activePane()
		pane.path = "/work"
		pane.items = []fileItem{{Name: "report.txt", Path: "/work/report.txt"}, {Name: "reports", Path: "/work/reports", IsDir: true}, {Name: "report.txt", Path: "/work/sub/report.txt"}}
		app.promptSearch()
		app.handlePromptKey(typeText("rep"))
		if len(matchingChoices(app.prompt.choices, app.prompt.value)) != 2 {
			t.Fatal("expected deduplicated suggestions")
		}
		app.handlePromptKey(press(tea.KeyDown))
		app.handlePromptKey(press(tea.KeyTab))
		if app.prompt.value != "reports" {
			t.Fatalf("completion = %q", app.prompt.value)
		}
		if app.handlePromptKey(press(tea.KeyEnter)) == nil || !app.busy {
			t.Fatal("search did not execute")
		}
		app.busy = false
		app.promptGoTo()
		app.handlePromptKey(press(tea.KeyTab))
		if app.prompt.value != "/work/reports" {
			t.Fatal("directory completion failed")
		}
	}
}

func TestConnectionAcceptsQ(t *testing.T) {
	app := New()
	app.connect.active = connectPassword
	if app.handleConnectKey(typeText("q")) != nil || app.connect.values[connectPassword] != "q" {
		t.Fatal("q must be editable, not quit")
	}
}

func TestResponsiveBrowser(t *testing.T) {
	for _, width := range []int{40, 79, 80, 120} {
		app := New()
		app.width, app.height = width, 24
		app.local.items = []fileItem{{Name: "日本語資料.txt", Path: "/file"}}
		rendered := app.browserView()
		if lipgloss.Width(rendered) > width {
			t.Fatalf("width %d rendered %d", width, lipgloss.Width(rendered))
		}
		if lipgloss.Height(rendered) > app.height {
			t.Fatalf("height %d rendered %d", app.height, lipgloss.Height(rendered))
		}
		if !utf8.ValidString(rendered) {
			t.Fatal("invalid UTF-8")
		}
	}
}

func TestDisplayAndMasking(t *testing.T) {
	if got := displayText("\x1b[31mfile\n\x00"); got != "file" {
		t.Fatalf("unsafe display %q", got)
	}
	for width := 1; width < 12; width++ {
		got := truncate("日本語資料.txt", width)
		if !utf8.ValidString(got) || lipgloss.Width(got) > width {
			t.Fatalf("bad truncation %q", got)
		}
	}
	app := New()
	for _, field := range []int{connectPassword, connectPassphrase, connectVaultPassphrase} {
		app.connect.values[field] = "private-secret"
	}
	if strings.Contains(app.connectView(), "private-secret") {
		t.Fatal("secret exposed")
	}
}
