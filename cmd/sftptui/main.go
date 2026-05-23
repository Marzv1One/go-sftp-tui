package main

import (
	"fmt"
	"os"

	"sftp-tui/internal/app"
)

func main() {
	if err := app.New().Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
