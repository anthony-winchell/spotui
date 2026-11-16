package spotify

import (
	"os/exec"
	"runtime"
)

// openBrowser opens u in the user's default browser
func openBrowser(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default: // linux, *bsd
		cmd = exec.Command("xdg-open", u)
	}

	return cmd.Start()
}
