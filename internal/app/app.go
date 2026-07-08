package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"sftp-tui/internal/credentials"
	"sftp-tui/internal/sftpclient"

	tea "charm.land/bubbletea/v2"
)

type screen int

const (
	screenConnect screen = iota
	screenBrowser
	screenPrompt
	screenConfirm
)

type paneKind int

const (
	paneLocal paneKind = iota
	paneRemote
)

const (
	connectSecretName = iota
	connectHost
	connectPort
	connectUsername
	connectPassword
	connectPrivateKey
	connectPassphrase
	connectVaultPassphrase
	connectAction
	connectSaveAction
)

type App struct {
	client *sftpclient.Client
	config sftpclient.Config
	vault  credentials.Store

	screen screen
	width  int
	height int
	busy   bool
	status string

	connect connectForm
	local   filePane
	remote  filePane
	active  paneKind

	prompt  promptState
	confirm confirmState
}

type connectForm struct {
	labels []string
	values []string
	active int
}

type filePane struct {
	title       string
	path        string
	items       []fileItem
	cursor      int
	selected    map[string]bool
	searchQuery string
	searchMode  bool
}

type fileItem struct {
	Path  string
	Name  string
	Size  int64
	IsDir bool
}

type promptState struct {
	title  string
	label  string
	value  string
	done   func(*App, string) tea.Cmd
	cancel func(*App) tea.Cmd
}

type confirmState struct {
	message string
	yes     func(*App) tea.Cmd
	no      func(*App) tea.Cmd
}

type connectedMsg struct {
	client *sftpclient.Client
	cfg    sftpclient.Config
	cwd    string
	err    error
}

type localLoadedMsg struct {
	path   string
	items  []fileItem
	query  string
	search bool
	err    error
}

type remoteLoadedMsg struct {
	path   string
	items  []fileItem
	query  string
	search bool
	err    error
}

type operationDoneMsg struct {
	message       string
	err           error
	refreshLocal  bool
	refreshRemote bool
}

type Options struct {
	LocalCWD string
}

func New() *App {
	app, err := NewWithOptions(Options{})
	if err != nil {
		app, _ = NewWithOptions(Options{LocalCWD: "."})
	}
	return app
}

func NewWithOptions(options Options) (*App, error) {
	cwd, err := initialLocalPath(options.LocalCWD)
	if err != nil {
		return nil, err
	}
	return &App{
		screen: screenConnect,
		status: "Fill connection settings and press enter on Connect.",
		connect: connectForm{
			labels: []string{"Secret name", "Host", "Port", "Username", "Password", "Private key", "Passphrase", "Vault passphrase", "Connect", "Save secret"},
			values: []string{"", "", "", "", "", "", "", "", "", ""},
		},
		vault: credentials.AgeVault{Path: credentials.DefaultVaultPath()},
		local: filePane{
			title:    "Local",
			path:     cwd,
			selected: map[string]bool{},
		},
		remote: filePane{
			title:    "Server",
			path:     "/",
			selected: map[string]bool{},
		},
	}, nil
}

func initialLocalPath(localCWD string) (string, error) {
	if strings.TrimSpace(localCWD) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return ".", nil
		}
		return cwd, nil
	}
	abs, err := filepath.Abs(localCWD)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("local cwd is not a directory: %s", abs)
	}
	return abs, nil
}

func (a *App) Run() error {
	_, err := tea.NewProgram(a).Run()
	return err
}

func (a *App) Init() tea.Cmd {
	return loadLocalCmd(a.local.path, "")
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
	case tea.KeyPressMsg:
		return a.handleKey(msg)
	case tea.PasteMsg:
		a.pasteText(msg.Content)
	case tea.ClipboardMsg:
		a.pasteText(msg.Content)
	case connectedMsg:
		a.busy = false
		if msg.err != nil {
			a.status = msg.err.Error()
			return a, nil
		}
		if a.client != nil {
			_ = a.client.Close()
		}
		a.client = msg.client
		a.config = msg.cfg
		a.screen = screenBrowser
		a.remote.path = msg.cwd
		a.remote.cursor = 0
		a.remote.selected = map[string]bool{}
		a.remote.searchQuery = ""
		a.remote.searchMode = false
		a.status = "Connected. Tab switches panes; space selects; u uploads; d downloads."
		return a, loadRemoteCmd(a.client, a.remote.path, "")
	case localLoadedMsg:
		a.busy = false
		if msg.err != nil {
			a.status = msg.err.Error()
			return a, nil
		}
		a.local.path = msg.path
		a.local.items = msg.items
		a.local.searchQuery = msg.query
		a.local.searchMode = msg.search
		a.local.clampCursor()
		a.status = fmt.Sprintf("Local: %d entries", len(msg.items))
	case remoteLoadedMsg:
		a.busy = false
		if msg.err != nil {
			a.status = msg.err.Error()
			return a, nil
		}
		a.remote.path = msg.path
		a.remote.items = msg.items
		a.remote.searchQuery = msg.query
		a.remote.searchMode = msg.search
		a.remote.clampCursor()
		a.status = fmt.Sprintf("Server: %d entries", len(msg.items))
	case operationDoneMsg:
		a.busy = false
		if msg.err != nil {
			a.status = msg.err.Error()
			return a, nil
		}
		a.status = msg.message
		cmds := make([]tea.Cmd, 0, 2)
		if msg.refreshLocal {
			cmds = append(cmds, loadLocalCmd(a.local.path, a.local.searchQuery))
		}
		if msg.refreshRemote && a.client != nil {
			cmds = append(cmds, loadRemoteCmd(a.client, a.remote.path, a.remote.searchQuery))
		}
		return a, tea.Batch(cmds...)
	}
	return a, nil
}

func (a *App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return a, tea.Quit
	}
	if (msg.String() == "ctrl+shift+v" || msg.String() == "ctrl+v") && (a.screen == screenConnect || a.screen == screenPrompt) {
		return a, func() tea.Msg { return tea.ReadClipboard() }
	}
	if a.busy {
		return a, nil
	}
	switch a.screen {
	case screenConnect:
		return a, a.handleConnectKey(msg)
	case screenPrompt:
		return a, a.handlePromptKey(msg)
	case screenConfirm:
		return a, a.handleConfirmKey(msg)
	case screenBrowser:
		return a, a.handleBrowserKey(msg)
	}
	return a, nil
}

func (a *App) handleConnectKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "q", "esc":
		return tea.Quit
	case "tab", "down":
		a.connect.active = (a.connect.active + 1) % len(a.connect.labels)
	case "shift+tab", "up":
		a.connect.active--
		if a.connect.active < 0 {
			a.connect.active = len(a.connect.labels) - 1
		}
	case "enter":
		if a.connect.active == connectAction {
			return a.connectCmd()
		}
		if a.connect.active == connectSaveAction {
			return a.saveSecretCmd()
		}
		a.connect.active = (a.connect.active + 1) % len(a.connect.labels)
	case "backspace":
		a.trimConnectField()
	default:
		if msg.Text != "" && a.connect.active < connectAction {
			a.connect.values[a.connect.active] += msg.Text
		}
	}
	return nil
}

func (a *App) handleBrowserKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "q":
		return tea.Quit
	case "tab", "shift+tab":
		a.switchPane()
	case "up", "k":
		a.activePane().move(-1)
	case "down", "j":
		a.activePane().move(1)
	case "pgup":
		a.activePane().move(-10)
	case "pgdown":
		a.activePane().move(10)
	case "home":
		a.activePane().cursor = 0
	case "end":
		pane := a.activePane()
		pane.cursor = len(pane.items) - 1
		pane.clampCursor()
	case "space":
		a.activePane().toggleSelected()
	case "enter":
		return a.openSelected()
	case "backspace", "h":
		return a.openParent()
	case "r":
		return a.refreshActive()
	case "R":
		return tea.Batch(loadLocalCmd(a.local.path, a.local.searchQuery), loadRemoteCmd(a.client, a.remote.path, a.remote.searchQuery))
	case "u":
		return a.uploadSelected()
	case "d":
		return a.downloadSelected()
	case "x":
		return a.confirmDelete()
	case "n":
		return a.promptRename()
	case "m":
		return a.promptMakeDirectory()
	case "g":
		return a.promptGoTo()
	case "/":
		return a.promptSearch()
	case "esc":
		return a.clearSearch()
	case "c":
		a.disconnect()
	}
	return nil
}

func (a *App) handlePromptKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		cancel := a.prompt.cancel
		a.screen = screenBrowser
		if cancel != nil {
			return cancel(a)
		}
	case "enter":
		done := a.prompt.done
		value := a.prompt.value
		a.screen = screenBrowser
		if done != nil {
			return done(a, strings.TrimSpace(value))
		}
	case "backspace":
		a.prompt.value = trimLastRune(a.prompt.value)
	default:
		if msg.Text != "" {
			a.prompt.value += msg.Text
		}
	}
	return nil
}

func (a *App) pasteText(content string) {
	content = strings.ReplaceAll(content, "\x00", "")
	content = strings.TrimRight(content, "\r\n")
	if content == "" {
		return
	}
	switch a.screen {
	case screenConnect:
		if a.connect.active < connectAction {
			a.connect.values[a.connect.active] += content
			a.status = "Pasted into " + a.connect.labels[a.connect.active] + "."
		}
	case screenPrompt:
		a.prompt.value += content
	}
}

func (a *App) handleConfirmKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "y", "Y", "enter":
		yes := a.confirm.yes
		a.screen = screenBrowser
		if yes != nil {
			return yes(a)
		}
	case "n", "N", "esc":
		no := a.confirm.no
		a.screen = screenBrowser
		if no != nil {
			return no(a)
		}
	}
	return nil
}

func (a *App) View() tea.View {
	var content string
	switch a.screen {
	case screenConnect:
		content = a.connectView()
	case screenPrompt:
		content = a.browserView() + "\n" + a.promptView()
	case screenConfirm:
		content = a.browserView() + "\n" + a.confirmView()
	default:
		content = a.browserView()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (a *App) connectCmd() tea.Cmd {
	typedCfg := a.formConfig()
	secretName := strings.TrimSpace(a.connect.values[connectSecretName])
	vaultPassphrase := a.connect.values[connectVaultPassphrase]
	if secretName == "" && (typedCfg.Host == "" || typedCfg.Username == "") {
		a.status = "Host and username are required, or provide a secret name."
		return nil
	}
	if secretName != "" && vaultPassphrase == "" {
		a.status = "Vault passphrase is required to load a secret."
		return nil
	}
	a.busy = true
	a.status = "Connecting..."
	return func() tea.Msg {
		cfg := typedCfg
		if secretName != "" {
			entry, err := a.vault.Get(vaultPassphrase, secretName)
			if err != nil {
				return connectedMsg{cfg: cfg, err: err}
			}
			cfg = credentials.MergeConfig(entry.Config(), typedCfg)
		}
		client, err := sftpclient.Connect(context.Background(), cfg)
		if err != nil {
			return connectedMsg{client: client, cfg: cfg, err: err}
		}
		cwd, err := client.Getwd()
		if err != nil {
			_ = client.Close()
			return connectedMsg{cfg: cfg, err: err}
		}
		return connectedMsg{client: client, cfg: cfg, cwd: cwd, err: nil}
	}
}

func (a *App) saveSecretCmd() tea.Cmd {
	secretName := strings.TrimSpace(a.connect.values[connectSecretName])
	vaultPassphrase := a.connect.values[connectVaultPassphrase]
	cfg := a.formConfig()
	if secretName == "" {
		a.status = "Secret name is required."
		return nil
	}
	if vaultPassphrase == "" {
		a.status = "Vault passphrase is required."
		return nil
	}
	if cfg.Host == "" || cfg.Username == "" {
		a.status = "Host and username are required to save a secret."
		return nil
	}
	a.busy = true
	a.status = "Saving secret..."
	return func() tea.Msg {
		entry := credentials.EntryFromConfig(cfg)
		err := a.vault.Save(vaultPassphrase, secretName, entry)
		return operationDoneMsg{message: "Secret saved to age vault.", err: err}
	}
}

func (a *App) formConfig() sftpclient.Config {
	return sftpclient.Config{
		Host:       strings.TrimSpace(a.connect.values[connectHost]),
		Port:       strings.TrimSpace(a.connect.values[connectPort]),
		Username:   strings.TrimSpace(a.connect.values[connectUsername]),
		Password:   a.connect.values[connectPassword],
		PrivateKey: strings.TrimSpace(a.connect.values[connectPrivateKey]),
		Passphrase: a.connect.values[connectPassphrase],
		Timeout:    15 * time.Second,
	}
}

func loadLocalCmd(localPath, query string) tea.Cmd {
	return func() tea.Msg {
		items, loadedPath, search, err := loadLocalItems(localPath, query)
		return localLoadedMsg{path: loadedPath, items: items, query: query, search: search, err: err}
	}
}

func loadRemoteCmd(client *sftpclient.Client, remotePath, query string) tea.Cmd {
	return func() tea.Msg {
		items, loadedPath, search, err := loadRemoteItems(client, remotePath, query)
		return remoteLoadedMsg{path: loadedPath, items: items, query: query, search: search, err: err}
	}
}

func loadLocalItems(localPath, query string) ([]fileItem, string, bool, error) {
	if localPath == "" {
		localPath = "."
	}
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return nil, localPath, false, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, abs, false, err
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	if strings.TrimSpace(query) != "" {
		items, err := searchLocal(abs, query)
		return items, abs, true, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, abs, false, err
	}
	items := make([]fileItem, 0, len(entries))
	for _, entry := range entries {
		entryInfo, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, localFileItem(filepath.Join(abs, entry.Name()), entryInfo))
	}
	sortFileItems(items)
	return items, abs, false, nil
}

func loadRemoteItems(client *sftpclient.Client, remotePath, query string) ([]fileItem, string, bool, error) {
	if client == nil {
		return nil, remotePath, false, errors.New("not connected")
	}
	if remotePath == "" {
		remotePath = "/"
	}
	info, err := client.Stat(remotePath)
	if err != nil {
		return nil, remotePath, false, err
	}
	if !info.IsDir() {
		remotePath = sftpclient.Parent(remotePath)
	}
	remotePath = sftpclient.JoinRemote(remotePath, "")
	if strings.TrimSpace(query) != "" {
		matches, err := client.Walk(remotePath, func(remotePath string, info os.FileInfo) bool {
			return remotePath != "/" && strings.Contains(strings.ToLower(path.Base(remotePath)), strings.ToLower(query))
		})
		if err != nil {
			return nil, remotePath, true, err
		}
		items := make([]fileItem, 0, len(matches))
		for _, match := range matches {
			items = append(items, remoteFileItem(match.Path, match.Info))
		}
		sortFileItems(items)
		return items, remotePath, true, nil
	}
	entries, err := client.ReadDir(remotePath)
	if err != nil {
		return nil, remotePath, false, err
	}
	items := make([]fileItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, remoteFileItem(sftpclient.JoinRemote(remotePath, entry.Name()), entry))
	}
	sortFileItems(items)
	return items, remotePath, false, nil
}

func searchLocal(root, query string) ([]fileItem, error) {
	query = strings.ToLower(query)
	items := []fileItem{}
	err := filepath.WalkDir(root, func(itemPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if itemPath == root || !strings.Contains(strings.ToLower(entry.Name()), query) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		items = append(items, localFileItem(itemPath, info))
		return nil
	})
	sortFileItems(items)
	return items, err
}

func localFileItem(itemPath string, info os.FileInfo) fileItem {
	return fileItem{Path: itemPath, Name: info.Name(), Size: info.Size(), IsDir: info.IsDir()}
}

func remoteFileItem(itemPath string, info os.FileInfo) fileItem {
	return fileItem{Path: itemPath, Name: path.Base(itemPath), Size: info.Size(), IsDir: info.IsDir()}
}

func sortFileItems(items []fileItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
}

func (a *App) activePane() *filePane {
	if a.active == paneRemote {
		return &a.remote
	}
	return &a.local
}

func (a *App) inactivePane() *filePane {
	if a.active == paneRemote {
		return &a.local
	}
	return &a.remote
}

func (a *App) switchPane() {
	if a.active == paneLocal {
		a.active = paneRemote
	} else {
		a.active = paneLocal
	}
}

func (a *App) openSelected() tea.Cmd {
	pane := a.activePane()
	item := pane.current()
	if item == nil || !item.IsDir {
		return nil
	}
	a.busy = true
	if a.active == paneLocal {
		return loadLocalCmd(item.Path, "")
	}
	return loadRemoteCmd(a.client, item.Path, "")
}

func (a *App) openParent() tea.Cmd {
	a.busy = true
	if a.active == paneLocal {
		return loadLocalCmd(filepath.Dir(a.local.path), "")
	}
	return loadRemoteCmd(a.client, sftpclient.Parent(a.remote.path), "")
}

func (a *App) refreshActive() tea.Cmd {
	a.busy = true
	if a.active == paneLocal {
		return loadLocalCmd(a.local.path, a.local.searchQuery)
	}
	return loadRemoteCmd(a.client, a.remote.path, a.remote.searchQuery)
}

func (a *App) clearSearch() tea.Cmd {
	pane := a.activePane()
	if !pane.searchMode {
		return nil
	}
	pane.searchQuery = ""
	pane.searchMode = false
	a.busy = true
	if a.active == paneLocal {
		return loadLocalCmd(a.local.path, "")
	}
	return loadRemoteCmd(a.client, a.remote.path, "")
}

func (a *App) promptSearch() tea.Cmd {
	pane := a.activePane()
	a.screen = screenPrompt
	a.prompt = promptState{
		title: "Search " + pane.title,
		label: "name contains",
		value: pane.searchQuery,
		done: func(app *App, value string) tea.Cmd {
			app.busy = true
			if app.active == paneLocal {
				return loadLocalCmd(app.local.path, value)
			}
			return loadRemoteCmd(app.client, app.remote.path, value)
		},
	}
	return nil
}

func (a *App) promptGoTo() tea.Cmd {
	pane := a.activePane()
	a.screen = screenPrompt
	a.prompt = promptState{
		title: "Go to " + pane.title,
		label: "path",
		value: pane.path,
		done: func(app *App, value string) tea.Cmd {
			if value == "" {
				return nil
			}
			app.busy = true
			if app.active == paneLocal {
				return loadLocalCmd(value, "")
			}
			return loadRemoteCmd(app.client, value, "")
		},
	}
	return nil
}

func (a *App) promptRename() tea.Cmd {
	pane := a.activePane()
	item := pane.current()
	if item == nil {
		a.status = "No item selected."
		return nil
	}
	a.screen = screenPrompt
	a.prompt = promptState{
		title: "Rename " + item.Name,
		label: "new name",
		value: item.Name,
		done: func(app *App, value string) tea.Cmd {
			if value == "" {
				return nil
			}
			from := item.Path
			to := value
			active := app.active
			if active == paneLocal && !filepath.IsAbs(to) {
				to = filepath.Join(filepath.Dir(from), to)
			}
			if active == paneRemote && !strings.Contains(to, "/") {
				to = sftpclient.JoinRemote(sftpclient.Parent(from), to)
			}
			app.busy = true
			return func() tea.Msg {
				var err error
				if active == paneLocal {
					err = os.Rename(from, to)
				} else {
					err = app.client.Rename(from, to)
				}
				return operationDoneMsg{message: "Renamed.", err: err, refreshLocal: active == paneLocal, refreshRemote: active == paneRemote}
			}
		},
	}
	return nil
}

func (a *App) promptMakeDirectory() tea.Cmd {
	pane := a.activePane()
	a.screen = screenPrompt
	a.prompt = promptState{
		title: "Create " + strings.ToLower(pane.title) + " directory",
		label: "directory name or path",
		done: func(app *App, value string) tea.Cmd {
			if value == "" {
				return nil
			}
			active := app.active
			localPath := localDirectoryTarget(app.local.path, value)
			remotePath := remoteDirectoryTarget(app.remote.path, value)
			app.busy = true
			if active == paneLocal {
				app.status = "Creating local directory..."
			} else {
				app.status = "Creating server directory..."
			}
			return func() tea.Msg {
				if active == paneLocal {
					err := os.Mkdir(localPath, 0o755)
					return operationDoneMsg{message: "Created local directory: " + localPath, err: err, refreshLocal: true}
				}
				err := app.client.Mkdir(remotePath)
				return operationDoneMsg{message: "Created server directory: " + remotePath, err: err, refreshRemote: true}
			}
		},
	}
	return nil
}

func localDirectoryTarget(baseDir, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(baseDir, value)
}

func remoteDirectoryTarget(baseDir, value string) string {
	return sftpclient.JoinRemote(baseDir, value)
}

func (a *App) uploadSelected() tea.Cmd {
	items := a.local.selectedItems()
	if len(items) == 0 {
		if item := a.local.current(); item != nil {
			items = []fileItem{*item}
		}
	}
	if len(items) == 0 {
		a.status = "Select local items to upload."
		return nil
	}
	a.busy = true
	a.status = "Uploading..."
	remoteDir := a.remote.path
	client := a.client
	return func() tea.Msg {
		err := uploadItems(client, items, remoteDir)
		return operationDoneMsg{message: fmt.Sprintf("Uploaded %d item(s).", len(items)), err: err, refreshRemote: true}
	}
}

func (a *App) downloadSelected() tea.Cmd {
	items := a.remote.selectedItems()
	if len(items) == 0 {
		if item := a.remote.current(); item != nil {
			items = []fileItem{*item}
		}
	}
	if len(items) == 0 {
		a.status = "Select server items to download."
		return nil
	}
	a.busy = true
	a.status = "Downloading..."
	localDir := a.local.path
	client := a.client
	return func() tea.Msg {
		err := downloadItems(client, items, localDir)
		return operationDoneMsg{message: fmt.Sprintf("Downloaded %d item(s).", len(items)), err: err, refreshLocal: true}
	}
}

func (a *App) confirmDelete() tea.Cmd {
	pane := a.activePane()
	items := pane.selectedItems()
	if len(items) == 0 {
		if item := pane.current(); item != nil {
			items = []fileItem{*item}
		}
	}
	if len(items) == 0 {
		a.status = "No item selected."
		return nil
	}
	active := a.active
	a.screen = screenConfirm
	a.confirm = confirmState{
		message: fmt.Sprintf("Delete %d item(s) from %s? y/N", len(items), pane.title),
		yes: func(app *App) tea.Cmd {
			app.busy = true
			client := app.client
			return func() tea.Msg {
				var err error
				if active == paneLocal {
					err = deleteLocalItems(items)
				} else {
					err = deleteRemoteItems(client, items)
				}
				return operationDoneMsg{message: "Deleted.", err: err, refreshLocal: active == paneLocal, refreshRemote: active == paneRemote}
			}
		},
		no: func(app *App) tea.Cmd {
			app.status = "Delete canceled."
			return nil
		},
	}
	return nil
}

func (a *App) disconnect() {
	if a.client != nil {
		_ = a.client.Close()
		a.client = nil
	}
	a.screen = screenConnect
	a.status = "Disconnected."
}

func uploadItems(client *sftpclient.Client, items []fileItem, remoteDir string) error {
	if client == nil {
		return errors.New("not connected")
	}
	for _, item := range items {
		remoteTarget := sftpclient.JoinRemote(remoteDir, filepath.Base(item.Path))
		if item.IsDir {
			if err := uploadDir(client, item.Path, remoteTarget); err != nil {
				return err
			}
			continue
		}
		if err := client.Put(item.Path, remoteTarget); err != nil {
			return err
		}
	}
	return nil
}

func uploadDir(client *sftpclient.Client, localRoot, remoteRoot string) error {
	if err := ensureRemoteDir(client, remoteRoot); err != nil {
		return err
	}
	return filepath.WalkDir(localRoot, func(localPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(localRoot, localPath)
		if err != nil || rel == "." {
			return err
		}
		remotePath := sftpclient.JoinRemote(remoteRoot, filepath.ToSlash(rel))
		if entry.IsDir() {
			return ensureRemoteDir(client, remotePath)
		}
		return client.Put(localPath, remotePath)
	})
}

func ensureRemoteDir(client *sftpclient.Client, remotePath string) error {
	info, err := client.Stat(remotePath)
	if err == nil {
		if info.IsDir() {
			return nil
		}
		return fmt.Errorf("remote path exists and is not a directory: %s", remotePath)
	}
	return client.Mkdir(remotePath)
}

func downloadItems(client *sftpclient.Client, items []fileItem, localDir string) error {
	if client == nil {
		return errors.New("not connected")
	}
	for _, item := range items {
		localTarget := filepath.Join(localDir, path.Base(item.Path))
		if item.IsDir {
			if err := downloadDir(client, item.Path, localTarget); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(localTarget), 0o755); err != nil {
			return err
		}
		if err := client.Get(item.Path, localTarget); err != nil {
			return err
		}
	}
	return nil
}

func downloadDir(client *sftpclient.Client, remoteRoot, localRoot string) error {
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		return err
	}
	items, err := client.Walk(remoteRoot, nil)
	if err != nil {
		return err
	}
	for _, item := range items {
		rel := strings.TrimPrefix(strings.TrimPrefix(item.Path, remoteRoot), "/")
		if rel == "" {
			continue
		}
		localPath := filepath.Join(localRoot, filepath.FromSlash(rel))
		if item.Info.IsDir() {
			if err := os.MkdirAll(localPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			return err
		}
		if err := client.Get(item.Path, localPath); err != nil {
			return err
		}
	}
	return nil
}

func deleteLocalItems(items []fileItem) error {
	for _, item := range items {
		if err := os.RemoveAll(item.Path); err != nil {
			return err
		}
	}
	return nil
}

func deleteRemoteItems(client *sftpclient.Client, items []fileItem) error {
	if client == nil {
		return errors.New("not connected")
	}
	for _, item := range items {
		if item.IsDir {
			entries, err := client.Walk(item.Path, nil)
			if err != nil {
				return err
			}
			sort.Slice(entries, func(i, j int) bool {
				return len(entries[i].Path) > len(entries[j].Path)
			})
			for _, entry := range entries {
				if entry.Info.IsDir() {
					if err := client.RemoveDirectory(entry.Path); err != nil {
						return err
					}
				} else if err := client.Remove(entry.Path); err != nil {
					return err
				}
			}
			continue
		}
		if err := client.Remove(item.Path); err != nil {
			return err
		}
	}
	return nil
}

func (p *filePane) current() *fileItem {
	if len(p.items) == 0 || p.cursor < 0 || p.cursor >= len(p.items) {
		return nil
	}
	return &p.items[p.cursor]
}

func (p *filePane) move(delta int) {
	p.cursor += delta
	p.clampCursor()
}

func (p *filePane) clampCursor() {
	if len(p.items) == 0 {
		p.cursor = 0
		return
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.items) {
		p.cursor = len(p.items) - 1
	}
}

func (p *filePane) toggleSelected() {
	item := p.current()
	if item == nil {
		return
	}
	if p.selected == nil {
		p.selected = map[string]bool{}
	}
	if p.selected[item.Path] {
		delete(p.selected, item.Path)
		return
	}
	p.selected[item.Path] = true
}

func (p *filePane) selectedItems() []fileItem {
	items := []fileItem{}
	for _, item := range p.items {
		if p.selected[item.Path] {
			items = append(items, item)
		}
	}
	return items
}

func (a *App) connectView() string {
	var b strings.Builder
	b.WriteString("SFTP TUI\n\n")
	for i, label := range a.connect.labels {
		prefix := "  "
		if i == a.connect.active {
			prefix = "> "
		}
		if i == connectAction || i == connectSaveAction {
			b.WriteString(prefix + "[ " + label + " ]\n")
			continue
		}
		value := a.connect.values[i]
		if i == connectPassword || i == connectPassphrase || i == connectVaultPassphrase {
			value = strings.Repeat("*", utf8.RuneCountInString(value))
		}
		b.WriteString(fmt.Sprintf("%s%-18s %s\n", prefix, label+":", value))
	}
	b.WriteString("\nSecret name + vault passphrase loads an age-encrypted credential. Empty port uses saved/default 22.\n")
	b.WriteString("Tab/down moves, Ctrl+Shift+V pastes, enter activates, q quits.\n")
	b.WriteString("Vault: " + credentials.DefaultVaultPath() + "\n")
	b.WriteString("\n" + a.status)
	return b.String()
}

func (a *App) browserView() string {
	width := a.width
	if width <= 0 {
		width = 100
	}
	paneWidth := (width - 3) / 2
	if paneWidth < 30 {
		paneWidth = 30
	}
	paneHeight := a.height - 6
	if paneHeight < 8 {
		paneHeight = 14
	}
	left := renderPane(a.local, a.active == paneLocal, paneWidth, paneHeight)
	right := renderPane(a.remote, a.active == paneRemote, paneWidth, paneHeight)
	lines := make([]string, 0, paneHeight+5)
	lines = append(lines, "SFTP TUI  tab switch | enter open | space select | u upload | d download | m mkdir | / search | g goto | n rename | x delete | r refresh | q quit")
	for i := 0; i < len(left) || i < len(right); i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lines = append(lines, padRight(l, paneWidth)+" | "+padRight(r, paneWidth))
	}
	if a.busy {
		lines = append(lines, "Working... "+a.status)
	} else {
		lines = append(lines, a.status)
	}
	return strings.Join(lines, "\n")
}

func renderPane(p filePane, active bool, width, height int) []string {
	marker := " "
	if active {
		marker = "*"
	}
	title := fmt.Sprintf("%s %s: %s", marker, p.title, p.path)
	if p.searchMode {
		title += " [search: " + p.searchQuery + "]"
	}
	lines := []string{truncate(title, width)}
	start := 0
	if p.cursor >= height {
		start = p.cursor - height + 1
	}
	for row := 0; row < height; row++ {
		idx := start + row
		if idx >= len(p.items) {
			lines = append(lines, "")
			continue
		}
		item := p.items[idx]
		cursor := " "
		if idx == p.cursor {
			cursor = ">"
		}
		selected := " "
		if p.selected[item.Path] {
			selected = "x"
		}
		kind := " "
		if item.IsDir {
			kind = "/"
		}
		name := item.Name
		if p.searchMode {
			name = item.Path
		}
		size := ""
		if !item.IsDir {
			size = humanSize(item.Size)
		}
		lines = append(lines, truncate(fmt.Sprintf("%s[%s]%s %-40s %9s", cursor, selected, kind, name, size), width))
	}
	return lines
}

func (a *App) promptView() string {
	return fmt.Sprintf("\n%s\n%s: %s_\nenter confirms, esc cancels", a.prompt.title, a.prompt.label, a.prompt.value)
}

func (a *App) confirmView() string {
	return "\n" + a.confirm.message
}

func (a *App) trimConnectField() {
	if a.connect.active >= connectAction {
		return
	}
	a.connect.values[a.connect.active] = trimLastRune(a.connect.values[a.connect.active])
}

func trimLastRune(value string) string {
	if value == "" {
		return ""
	}
	_, size := utf8.DecodeLastRuneInString(value)
	return value[:len(value)-size]
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return truncate(value, width)
	}
	return value + strings.Repeat(" ", width-len(value))
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	return value[:width-1] + "…"
}

func humanSize(size int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(size)
	index := 0
	for value >= 1024 && index < len(units)-1 {
		value /= 1024
		index++
	}
	if index == 0 {
		return fmt.Sprintf("%d %s", size, units[index])
	}
	return fmt.Sprintf("%.1f %s", value, units[index])
}
