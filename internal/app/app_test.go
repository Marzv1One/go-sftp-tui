package app

import "testing"

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
