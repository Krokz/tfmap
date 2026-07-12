package main

import (
	"io/fs"
	"log"
	"runtime"

	"github.com/Krokz/tfmap/cmd"
)

func init() {
	// The native webview window must run on the process's first thread
	// (an AppKit requirement on macOS), so pin the main goroutine to it.
	runtime.LockOSThread()
}

func main() {
	distFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Printf("Warning: could not load embedded frontend: %v", err)
	}
	cmd.WebDistFS = distFS
	cmd.Execute()
}
