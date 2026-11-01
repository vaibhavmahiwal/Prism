package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/vaibhavmahiwal/Prism/internal/executor"
)

func main() { os.Exit(run()) }

func run() int {
	ex, err := executor.NewDocker()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create docker executor:", err)
		return 2
	}
	defer ex.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	code, err := ex.Run(ctx, executor.Spec{
		Image:   "alpine",
		Command: strings.Join(os.Args[1:], " "),
	}, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to run command:", err)
		return 2
	}
	fmt.Println("exit code:", code)
	return code
}
