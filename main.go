package main

import (
	"log"
	"os"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/desktop"
)

func main() {
	home, err := defaultDesktopHome()
	if err != nil {
		log.Printf("resolve AgentConfigSync home: %v", err)
		os.Exit(1)
	}
	core, err := desktop.New(home, "")
	if err != nil {
		log.Printf("initialize AgentConfigSync: %v", err)
		os.Exit(1)
	}
	if err := newGUIApplication(core).run(); err != nil {
		log.Printf("run AgentConfigSync: %v", err)
		os.Exit(1)
	}
}

func defaultDesktopHome() (string, error) {
	return cli.Home()
}
