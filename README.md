# go-sftp-tui

A terminal UI (TUI) for browsing and managing files over SFTP, with a dual-pane
view of your local filesystem and a remote server. Connection details can be
saved in an age-encrypted local vault so you never have to retype credentials.

Built in Go with Charm's Bubble Tea v2 and Lip Gloss v2 (`charm.land`), with rounded panels, highlighted selection, and responsive single-pane mode in narrow terminals.

## Features

- **Dual-pane file browser** — local and remote panes side by side; switch with
  `Tab`, open directories with `Enter`, go up with `Backspace`/`h`.
- **Multi-select transfers** — select multiple files/directories with `Space`,
  then upload (`u`) or download (`d`) the whole selection.
- **Remote file management** — rename (`n`), delete (`x`, with confirmation),
  create directories (`m`), and jump to a path (`g`) in either pane.
- **Search and completion**: press `/` to search recursively. Suggestions come
  from the current pane's loaded entries; use up/down to choose, `Tab` to complete,
  and `Enter` to search. `Ctrl+U` clears the input. `g` offers directory-path
  completion from loaded directories. `Esc` cancels a prompt or clears results.
- **Encrypted credential vault** — save named connection profiles encrypted
  with a passphrase using [age](https://github.com/FiloSottile/age). Load a
  saved profile with `Ctrl+L` or `Enter` on **Secret name**. Unlock the vault,
  type to filter, use up/down to select, and press `Enter` to use it.
  Any connection fields you type yourself override the saved values.
- **Password or key authentication** — password, private key file (optionally
  passphrase-protected), or both. Default port is 22 with a 15-second timeout.
- **Clipboard paste** — `Ctrl+V` / `Ctrl+Shift+V` pastes into the focused
  connect-screen field or prompt.

## Installation

Requires Go 1.26+.

```sh
go build -o sftptui ./cmd/sftptui
go build -o vault   ./cmd/vault
```

Dependencies use published Charm modules; no local checkout is required.

## Usage

Start the UI (optionally setting the initial local directory):

```sh
./sftptui
./sftptui -local-cwd /path/to/folder
```

On the connect screen, fill in host, port, username, and a password or private
key path, then select **Connect**. To reuse saved credentials, press `Ctrl+L`,
unlock the vault, and choose a profile from the filterable list. Press `Enter`
again on **Connect** to connect. To save a profile, type a **Secret name**, fill
in the connection settings and **Vault passphrase**, then choose **Save secret**.
`Esc` cancels the picker or unlock prompt; on the connect screen it quits.
Typed connection fields override saved values, so clear them to use the saved
settings unchanged.

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
| `q`, `Ctrl+C` | Quit |

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

## Local SFTP server backed by S3

`compose.yaml` runs SFTPGo v2.7.6 against an existing S3-compatible service,
including Floci running on the host on port 4566. It does not start Floci.
Docker Compose v2 or newer is required.

With Floci running and your AWS environment variables loaded, create the bucket
once, then start SFTPGo:

```sh
aws --endpoint-url http://localhost.floci.io:4566 s3api create-bucket --bucket sftpgo
docker compose up -d --wait
```

Connect the TUI to `127.0.0.1`, port `2022`, username `demo`, password
`local-sftp-password`. The user's SFTP root maps to the `sftpgo` bucket.
The WebAdmin is at <http://localhost:8080/web/admin>, with username `admin`
and password `local-admin-password`. These are local-development credentials
only; both published ports are bound to loopback.

Compose forwards `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and optionally
`AWS_SESSION_TOKEN` from your shell. The local defaults are `test` / `test`,
with `AWS_DEFAULT_REGION` defaulting to `us-east-1`. SFTPGo uses the AWS SDK
credential chain; the user's S3 access-key fields are intentionally empty.

Your host's `AWS_ENDPOINT_URL=http://localhost.floci.io:4566` is deliberately
not forwarded: inside a container, loopback points to that container, not to
your host. Instead, the S3 endpoint defaults to
`http://host.docker.internal:4566`, with path-style requests enabled. Floci
must accept connections from Docker, not only from host loopback. If it runs
in another container, publish its port 4566 on the host or use a shared Docker
network and configure its reachable service address.

Set these optional variables in your shell or a private Compose `--env-file`:

| Variable | Default | Purpose |
| --- | --- | --- |
| `SFTPGO_S3_ENDPOINT` | `http://host.docker.internal:4566` | Endpoint reachable from inside Docker |
| `SFTPGO_S3_BUCKET` | `sftpgo` | Existing bucket to expose |
| `SFTPGO_S3_KEY_PREFIX` | Empty | Restrict the user to a bucket prefix, e.g. `demo/` |
| `SFTPGO_S3_FORCE_PATH_STYLE` | `true` | Path-style S3 requests |
| `SFTPGO_USERNAME` / `SFTPGO_PASSWORD` | `demo` / `local-sftp-password` | Initial SFTP user |
| `SFTPGO_ADMIN_USERNAME` / `SFTPGO_ADMIN_PASSWORD` | `admin` / `local-admin-password` | Initial administrator |

For real AWS S3, use real credentials, the bucket's region, an existing bucket,
`SFTPGO_S3_ENDPOINT` set to an empty string, and
`SFTPGO_S3_FORCE_PATH_STYLE=false`. Do not commit credentials or private env files.

Named volumes persist the SQLite configuration database, SSH host keys, and
local temporary-file directories required by SFTPGo's S3 backend. The user and
administrator are created only if absent: changing bootstrap variables does
not overwrite existing accounts. Update existing users' passwords and storage
settings through WebAdmin. AWS credential environment changes take effect when
Compose recreates the container.

```sh
docker compose logs -f sftpgo
docker compose down
```

`down` preserves the volumes and S3 objects. `docker compose down -v` deletes
local accounts, configuration, and host keys, but does not delete S3 objects.
The healthcheck verifies SFTPGo's HTTP service, not S3 connectivity.
S3 is not a full POSIX filesystem: symlinks and truncation are unsupported,
renames are copy/delete operations, and non-empty directories cannot be renamed.

References: [official Docker setup](https://docs.sftpgo.com/latest/docker/),
[S3 backend](https://docs.sftpgo.com/latest/s3/), and
[S3 user configuration](https://docs.sftpgo.com/latest/tutorials/postgresql-s3/).

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
