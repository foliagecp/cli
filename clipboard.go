package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		switch {
		case commandExists("xclip"):
			cmd = exec.Command("xclip", "-selection", "clipboard")
		case commandExists("xsel"):
			cmd = exec.Command("xsel", "--clipboard", "--input")
		case commandExists("wl-copy"):
			cmd = exec.Command("wl-copy")
		default:
			return fmt.Errorf("no clipboard tool found (install xclip, xsel, or wl-clipboard)")
		}
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
