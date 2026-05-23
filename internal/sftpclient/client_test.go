package sftpclient

import "testing"

func TestJoinRemote(t *testing.T) {
	cases := []struct {
		name string
		base string
		part string
		exp  string
	}{
		{name: "root child", base: "/", part: "docs", exp: "/docs"},
		{name: "nested child", base: "/home/user", part: "file.txt", exp: "/home/user/file.txt"},
		{name: "absolute override", base: "/home/user", part: "/tmp/file.txt", exp: "/tmp/file.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := JoinRemote(tc.base, tc.part); got != tc.exp {
				t.Fatalf("got %q want %q", got, tc.exp)
			}
		})
	}
}

func TestParent(t *testing.T) {
	cases := []struct {
		path string
		exp  string
	}{
		{path: "/", exp: "/"},
		{path: "/home/user", exp: "/home"},
		{path: "/home/user/file.txt", exp: "/home/user"},
	}
	for _, tc := range cases {
		if got := Parent(tc.path); got != tc.exp {
			t.Fatalf("parent(%q) = %q want %q", tc.path, got, tc.exp)
		}
	}
}

func TestNetJoinHostPort(t *testing.T) {
	cases := []struct {
		host string
		port string
		exp  string
	}{
		{host: "172.27.110.56", port: "2222", exp: "172.27.110.56:2222"},
		{host: " 172.27.110.56 ", port: " 2222 ", exp: "172.27.110.56:2222"},
		{host: "172.27.110.56:2222", port: "", exp: "172.27.110.56:2222"},
		{host: "\x1b[200~172.27.110.56\x1b[201~", port: "2222\r\n", exp: "172.27.110.56:2222"},
		{host: "2001:db8::1", port: "22", exp: "[2001:db8::1]:22"},
	}
	for _, tc := range cases {
		if got := netJoinHostPort(tc.host, tc.port); got != tc.exp {
			t.Fatalf("netJoinHostPort(%q, %q) = %q want %q", tc.host, tc.port, got, tc.exp)
		}
	}
}
