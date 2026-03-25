package cmd

import (
	"errors"
	"os"

	"github.com/ncruces/zenity"
)

var errBrowseCancelled = errors.New("directory selection cancelled")

func browseForDirectory() (string, error) {
	cwd, _ := os.Getwd()
	path, err := zenity.SelectFile(
		zenity.Directory(),
		zenity.Title("Select a Terraform project directory"),
		zenity.Filename(cwd+"/"),
	)
	if err == zenity.ErrCanceled {
		return "", errBrowseCancelled
	}
	return path, err
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
