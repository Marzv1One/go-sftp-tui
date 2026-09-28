# go-sftp-tui

A terminal UI (TUI) for browsing and managing files over SFTP, with a dual-pane
view of your local filesystem and a remote server. Connection details can be
saved in an age-encrypted local vault so you never have to retype credentials.

Built in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

- **Dual-pane file browser** — local and remote panes side by side; switch with
  `Tab`, open directories with `Enter`, go up with `Backspace`/`h`.
- **Multi-select transfers** — select multiple files/directories with `Space`,
  then upload (`u`) or download (`d`) the whole selection.
- **Remote file management** — rename (`n`), delete (`x`, with confirmation),
  create directories (`m`), and jump to a path (`g`) in either pane.
- **Search** — press `/` in a pane to search recursively; `Esc` clears the
  search and returns to the directory listing.
- **Encrypted credential vault** — save named connection profiles encrypted
  with a passphrase using [age](https://github.com/FiloSottile/age). Load a
  saved secret on the connect screen by entering its name and the vault
  passphrase; any fields you type yourself override the saved values.
- **Password or key authentication** — password, private key file (optionally
  passphrase-protected), or both. Default port is 22 with a 15-second timeout.
- **Clipboard paste** — `Ctrl+V` / `Ctrl+Shift+V` pastes into the focused
  connect-screen field or prompt.

## Installation

Requires Go 1.25+.

```sh
go build -o sftptui ./cmd/sftptui
go build -o vault   ./cmd/vault
```

Note: `go.mod` contains a `replace` directive pointing Bubble Tea v2 at a local
checkout. Adjust or remove it before building in your own environment.

## Usage

Start the UI (optionally setting the initial local directory):

```sh
./sftptui
./sftptui -local-cwd /path/to/folder
```

On the connect screen, fill in host, port, username, and a password or private
key path, then select **Connect**. To reuse saved credentials, enter the
**Secret name** and **Vault passphrase** and select **Connect**; select **Save
secret** to store (or update) the current settings under that name.

### Browser keys

| Key | Action |
| --- | --- |
| `Tab` / `Shift+Tab` | Switch active pane |
| `↑`/`↓` or `k`/`j` | Move cursor |
| `PgUp` / `PgDn` | Move 10 entries |
| `Home` / `End` | Jump to first/last entry |
| `Space` | Select/deselect entry |
| `Enter` | Open directory |
| `Backspace` / `h` | Parent directory |
| `u` / `d` | Upload / download selection |
| `x` | Delete selection (asks for confirmation) |
| `n` | Rename entry |
| `m` | Create directory |
| `g` | Go to path |
| `/` | Search; `Esc` clears search |
| `r` / `R` | Refresh active pane / both panes |
| `c` | Disconnect (back to connect screen) |
| `q`, `Esc`, `Ctrl+C` | Quit |

## Credential vault CLI

Saved credentials live in an age-encrypted vault file (in your user config
directory under `sftp-tui/credentials.age`, falling back to
`~/.sftp-tui/credentials.age`). Manage them from the command line:

```sh
vault list
vault add prod --host prod.example.com --user deploy --key ~/.ssh/id_ed25519
vault get prod [--show-secrets]
vault update prod --host new.example.com
vault delete prod
```

The vault passphrase can be supplied with `--passphrase`, the
`SFTP_TUI_VAULT_PASS` environment variable, or an interactive prompt. Use
`--vault` to point at a non-default vault file. Passwords are masked in `get`
output unless `--show-secrets` is passed.

## Development

Run the test suite:

```sh
go test ./...
```

## Project layout

```
cmd/sftptui/       TUI entry point
cmd/vault/         vault CLI entry point
internal/app/      Bubble Tea application (screens, panes, key handling)
internal/sftpclient/  SSH/SFTP connection and file operations
internal/credentials/ age-encrypted credential vault
```
