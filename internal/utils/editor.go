package utils

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)


func GetDefaultEditor() string {
	if env := os.Getenv("EDITOR"); env != "" {
		return env
	}
	if env := os.Getenv("VISUAL"); env != "" {
		return env
	}

	switch runtime.GOOS {
	case "windows":
		return "notepad"
	case "darwin":
		
		if IsCommandAvailable("nvim") {
			return "nvim"
		}
		if IsCommandAvailable("vim") {
			return "vim"
		}
		if IsCommandAvailable("nano") {
			return "nano"
		}
		return "vi" 
	case "linux":
		if IsCommandAvailable("nvim") {
			return "nvim"
		}
		if IsCommandAvailable("vim") {
			return "vim"
		}
		if IsCommandAvailable("nano") {
			return "nano"
		}
		return "vi"
	default:
		return "vi"
	}
}


func BuildEditorCmd(filename string) *exec.Cmd {
	editor := GetDefaultEditor()
	
	
	parts := strings.Fields(editor)
	cmdName := parts[0]
	args := parts[1:]
	args = append(args, filename)

	cmd := exec.Command(cmdName, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}


func IsCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}