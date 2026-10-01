package main

import (
	"context"
	"fmt"
	"os"
)

func run(ctx context.Context) error {
	return nil
}

func main() {
	err := run(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "Server: ", err)
		os.Exit(1)
	}
}
