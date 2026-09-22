package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"sftp-tui/internal/credentials"

	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "list":
		cmdList(args)
	case "get":
		cmdGet(args)
	case "add":
		cmdAdd(args)
	case "update":
		cmdAdd(args) // same as add (upsert)
	case "delete", "rm":
		cmdDelete(args)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: vault <command> [options]

Manage SFTP credentials stored in an age-encrypted vault.

Commands:
  list                List all saved credential names
  get    <name>       Show details for a credential
  add    <name>       Add or update a credential
  update <name>       Alias for add
  delete <name>       Delete a credential
  help                Show this help

Global flags (apply to all commands):
  --passphrase        Vault passphrase (or set SFTP_TUI_VAULT_PASS)
  --vault             Path to vault file (default: auto-detected)

Add/update flags:
  --host              Server hostname or IP
  --port              SSH port (default: 22)
  --user              Username
  --password          Password
  --key               Path to private key file
  --key-pass          Passphrase for the private key

Get flags:
  --show-secrets      Show passwords instead of masking them

Examples:
  vault add prod --host prod.example.com --user deploy --key ~/.ssh/id_ed25519
  vault list
  vault get prod --show-secrets
  vault delete staging
`)
}

func resolvePassphrase(f *flag.FlagSet) string {
	if p := f.Lookup("passphrase"); p != nil && p.Value.String() != "" {
		return p.Value.String()
	}
	if p := os.Getenv("SFTP_TUI_VAULT_PASS"); p != "" {
		return p
	}
	fmt.Fprint(os.Stderr, "Vault passphrase: ")
	bytePass, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading passphrase: %v\n", err)
		os.Exit(1)
	}
	return string(bytePass)
}

func resolveVault(f *flag.FlagSet) credentials.AgeVault {
	if v := f.Lookup("vault"); v != nil && v.Value.String() != "" {
		return credentials.AgeVault{Path: v.Value.String()}
	}
	return credentials.AgeVault{}
}

func readLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.String("passphrase", "", "vault passphrase")
	fs.String("vault", "", "vault file path")
	fs.Parse(args)

	vault := resolveVault(fs)
	passphrase := resolvePassphrase(fs)

	names, err := vault.List(passphrase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(names) == 0 {
		fmt.Println("No credentials saved.")
		return
	}
	for _, name := range names {
		fmt.Println(name)
	}
}

func cmdGet(args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	fs.String("passphrase", "", "vault passphrase")
	fs.String("vault", "", "vault file path")
	showSecrets := fs.Bool("show-secrets", false, "show passwords in plain text")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "error: credential name required")
		fmt.Fprintln(os.Stderr, "Usage: vault get <name>")
		os.Exit(1)
	}
	name := fs.Arg(0)

	vault := resolveVault(fs)
	passphrase := resolvePassphrase(fs)

	entry, err := vault.Get(passphrase, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Host:\t%s\n", entry.Host)
	fmt.Fprintf(w, "Port:\t%s\n", entry.Port)
	fmt.Fprintf(w, "User:\t%s\n", entry.Username)
	if *showSecrets {
		fmt.Fprintf(w, "Password:\t%s\n", entry.Password)
	} else {
		fmt.Fprintf(w, "Password:\t%s\n", mask(entry.Password))
	}
	if entry.PrivateKey != "" {
		fmt.Fprintf(w, "Key:\t%s\n", entry.PrivateKey)
	}
	if *showSecrets {
		fmt.Fprintf(w, "Key Pass:\t%s\n", entry.Passphrase)
	} else {
		fmt.Fprintf(w, "Key Pass:\t%s\n", mask(entry.Passphrase))
	}
	if !entry.UpdatedAt.IsZero() {
		fmt.Fprintf(w, "Updated:\t%s\n", entry.UpdatedAt.Format("2006-01-02 15:04:05 UTC"))
	}
	w.Flush()
}

func mask(s string) string {
	if s == "" {
		return ""
	}
	return "********"
}

func cmdAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	fs.String("passphrase", "", "vault passphrase")
	fs.String("vault", "", "vault file path")
	host := fs.String("host", "", "server hostname or IP")
	port := fs.String("port", "22", "SSH port")
	user := fs.String("user", "", "username")
	password := fs.String("password", "", "password")
	key := fs.String("key", "", "path to private key file")
	keyPass := fs.String("key-pass", "", "private key passphrase")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "error: credential name required")
		fmt.Fprintln(os.Stderr, "Usage: vault add <name> --host <host> --user <user> [options]")
		os.Exit(1)
	}
	name := fs.Arg(0)

	if *host == "" && *user == "" {
		// Interactive mode for any fields left empty
		reader := bufio.NewReader(os.Stdin)
		if *host == "" {
			fmt.Fprint(os.Stderr, "Host: ")
			*host, _ = readLineOr(reader, *host)
		}
		if *port == "22" {
			fmt.Fprint(os.Stderr, "Port [22]: ")
			if line, _ := readLineOr(reader, ""); line != "" {
				*port = line
			}
		}
		if *user == "" {
			fmt.Fprint(os.Stderr, "User: ")
			*user, _ = readLineOr(reader, *user)
		}
		if *password == "" {
			fmt.Fprint(os.Stderr, "Password (leave empty to skip): ")
			*password, _ = readLineOr(reader, *password)
		}
		if *key == "" {
			fmt.Fprint(os.Stderr, "Private key path (leave empty to skip): ")
			*key, _ = readLineOr(reader, *key)
		}
		if *keyPass == "" && *key != "" {
			fmt.Fprint(os.Stderr, "Key passphrase (leave empty to skip): ")
			*keyPass, _ = readLineOr(reader, *keyPass)
		}
	}

	entry := credentials.Entry{
		Host:       *host,
		Port:       *port,
		Username:   *user,
		Password:   *password,
		PrivateKey: *key,
		Passphrase: *keyPass,
	}

	vault := resolveVault(fs)
	passphrase := resolvePassphrase(fs)

	if err := vault.Save(passphrase, name, entry); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Credential %q saved.\n", name)
}

func readLineOr(reader *bufio.Reader, fallback string) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return fallback, err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func cmdDelete(args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	fs.String("passphrase", "", "vault passphrase")
	fs.String("vault", "", "vault file path")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "error: credential name required")
		fmt.Fprintln(os.Stderr, "Usage: vault delete <name>")
		os.Exit(1)
	}
	name := fs.Arg(0)

	vault := resolveVault(fs)
	passphrase := resolvePassphrase(fs)

	if err := vault.Delete(passphrase, name); err != nil {
		if errors.Is(err, fmt.Errorf("credential %q not found", name)) {
			fmt.Fprintf(os.Stderr, "error: credential %q not found\n", name)
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}
	fmt.Printf("Credential %q deleted.\n", name)
}
