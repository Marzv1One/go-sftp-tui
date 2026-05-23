package main

import (
	"flag"
	"fmt"
	"os"

	"sftp-tui/internal/app"
)

func main() {
	localCWD := flag.String("local-cwd", "", "initial local pane directory")
	flag.Parse()

	application, err := app.NewWithOptions(app.Options{LocalCWD: *localCWD})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := application.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
