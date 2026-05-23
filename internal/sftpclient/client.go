package sftpclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Config struct {
	Host       string
	Port       string
	Username   string
	Password   string
	PrivateKey string
	Passphrase string
	Timeout    time.Duration
}

type Client struct {
	sshClient  *ssh.Client
	sftpClient *sftp.Client
}

func Connect(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	host, port := normalizeEndpoint(cfg.Host, cfg.Port)
	cfg.Host = host
	cfg.Port = port
	cfg.Username = cleanTextField(cfg.Username)
	address := netJoinHostPort(cfg.Host, cfg.Port)
	client, err := dialSSH(ctx, address, cfg)
	if err != nil {
		return nil, err
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Client{sshClient: client, sftpClient: sftpClient}, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	if c.sftpClient != nil {
		_ = c.sftpClient.Close()
	}
	if c.sshClient != nil {
		return c.sshClient.Close()
	}
	return nil
}

func (c *Client) ReadDir(remotePath string) ([]os.FileInfo, error) {
	return c.sftpClient.ReadDir(remotePath)
}

func (c *Client) Stat(remotePath string) (os.FileInfo, error) {
	return c.sftpClient.Stat(remotePath)
}

func (c *Client) Getwd() (string, error) {
	return c.sftpClient.Getwd()
}

func (c *Client) Get(remotePath, localPath string) error {
	remoteFile, err := c.sftpClient.Open(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	localFile, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	_, err = io.Copy(localFile, remoteFile)
	return err
}

func (c *Client) Put(localPath, remotePath string) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	remoteFile, err := c.sftpClient.Create(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	_, err = io.Copy(remoteFile, localFile)
	return err
}

func (c *Client) Rename(oldPath, newPath string) error {
	return c.sftpClient.Rename(oldPath, newPath)
}

func (c *Client) Remove(remotePath string) error {
	return c.sftpClient.Remove(remotePath)
}

func (c *Client) RemoveDirectory(remotePath string) error {
	return c.sftpClient.RemoveDirectory(remotePath)
}

func (c *Client) Mkdir(remotePath string) error {
	return c.sftpClient.Mkdir(remotePath)
}

func (c *Client) Walk(root string, matcher func(string, os.FileInfo) bool) ([]RemoteEntry, error) {
	var entries []RemoteEntry
	walker := c.sftpClient.Walk(root)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return nil, err
		}
		info := walker.Stat()
		if info == nil {
			continue
		}
		remotePath := walker.Path()
		if matcher != nil && !matcher(remotePath, info) {
			continue
		}
		entries = append(entries, RemoteEntry{Path: remotePath, Info: info})
	}
	return entries, nil
}

type RemoteEntry struct {
	Path string
	Info os.FileInfo
}

func dialSSH(ctx context.Context, address string, cfg Config) (*ssh.Client, error) {
	authMethods, err := authMethods(cfg)
	if err != nil {
		return nil, err
	}
	sshConfig := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         cfg.Timeout,
	}
	type result struct {
		client *ssh.Client
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		client, err := ssh.Dial("tcp", address, sshConfig)
		ch <- result{client: client, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.client, res.err
	}
}

func authMethods(cfg Config) ([]ssh.AuthMethod, error) {
	methods := make([]ssh.AuthMethod, 0, 2)
	if cfg.Password != "" {
		methods = append(methods, ssh.Password(cfg.Password))
	}
	if cfg.PrivateKey != "" {
		keyBytes, err := os.ReadFile(cfg.PrivateKey)
		if err != nil {
			return nil, err
		}
		signer, err := signerFromKey(keyBytes, cfg.Passphrase)
		if err != nil {
			return nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("provide password or private key")
	}
	return methods, nil
}

func signerFromKey(keyBytes []byte, passphrase string) (ssh.Signer, error) {
	if passphrase == "" {
		return ssh.ParsePrivateKey(keyBytes)
	}
	return ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
}

func netJoinHostPort(host, port string) string {
	host, port = normalizeEndpoint(host, port)
	return net.JoinHostPort(host, port)
}

func normalizeEndpoint(host, port string) (string, string) {
	host = cleanTextField(host)
	port = cleanTextField(port)
	if strings.Contains(host, "://") {
		parts := strings.SplitN(host, "://", 2)
		host = parts[1]
	}
	if splitHost, splitPort, err := net.SplitHostPort(host); err == nil {
		host = splitHost
		if port == "" || port == "22" {
			port = splitPort
		}
	} else if strings.Count(host, ":") == 1 {
		left, right, ok := strings.Cut(host, ":")
		if ok && right != "" {
			host = left
			if port == "" || port == "22" {
				port = right
			}
		}
	}
	if port == "" {
		port = "22"
	}
	return strings.Trim(host, "[]"), port
}

func cleanTextField(value string) string {
	value = strings.ReplaceAll(value, "\x1b[200~", "")
	value = strings.ReplaceAll(value, "\x1b[201~", "")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, value)
}

func JoinRemote(baseDir, name string) string {
	if baseDir == "" || baseDir == "." {
		baseDir = "/"
	}
	if name == "" {
		return cleanRemote(baseDir)
	}
	if path.IsAbs(name) {
		return cleanRemote(name)
	}
	return cleanRemote(path.Join(baseDir, name))
}

func Parent(remotePath string) string {
	cleaned := cleanRemote(remotePath)
	if cleaned == "/" {
		return "/"
	}
	parent := path.Dir(cleaned)
	if parent == "." || parent == "" {
		return "/"
	}
	return parent
}

func cleanRemote(remotePath string) string {
	if remotePath == "" {
		return "/"
	}
	cleaned := path.Clean(remotePath)
	if !path.IsAbs(cleaned) {
		cleaned = "/" + strings.TrimPrefix(cleaned, "/")
	}
	if cleaned == "." {
		return "/"
	}
	return cleaned
}
