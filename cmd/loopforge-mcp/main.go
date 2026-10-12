package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/Tencent/LoopForge/internal/mcpserver"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := mcpserver.ServeRemoteStdio(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func init() {
	if os.Getenv("LOOPFORGE_API_URL") == "" {
		fmt.Fprintln(os.Stderr, "loopforge-mcp: set LOOPFORGE_API_URL to the running LoopForge server")
	}
}
