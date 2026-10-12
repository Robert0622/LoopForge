package main

import (
	"log"
	"os"

	"github.com/Tencent/LoopForge/internal/gatemcp"
)

func main() {
	if err := gatemcp.Run(os.Stdin, os.Stdout, os.Getenv("LOOPFORGE_GATE_RESULT_PATH")); err != nil {
		log.Fatal(err)
	}
}
