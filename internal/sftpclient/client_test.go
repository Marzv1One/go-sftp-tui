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
