package legacy

func pauseTitle(paused bool) string {
	if paused {
		return "Resume"
	}
	return "Pause"
}

// openCommand returns the command + args to open a URL or folder on goos.
func openCommand(goos, target string) (string, []string) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}
	case "darwin":
		return "open", []string{target}
	default:
		return "xdg-open", []string{target}
	}
}
