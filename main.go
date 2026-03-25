package main

import (
	"io/fs"
	"log"

	"github.com/Krokz/tfmap/cmd"
)

func main() {
	distFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Printf("Warning: could not load embedded frontend: %v", err)
	}
	cmd.WebDistFS = distFS
	cmd.Execute()
}
