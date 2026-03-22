package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/amidipayan/kubevision/internal/k8s/client"
	"github.com/amidipayan/kubevision/internal/tui/explorer"

	
	"k8s.io/klog/v2"
)


var version = "dev"

func main() {

	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version") {
		fmt.Printf("Kubevision version %s\n", version)
		os.Exit(0)
	}


	f, err := os.OpenFile("kubevision-debug.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not open debug log: %v\n", err)
	} else {
		defer f.Close()
		log.SetOutput(f)
		klog.SetOutput(f)
		os.Stderr = f
		fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		klog.InitFlags(fs)
		fs.Set("logtostderr", "false")
		fs.Set("alsologtostderr", "false")
	}

	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.Printf("CRITICAL PANIC: %v\n%s", r, stack)
			fmt.Printf("Error: %v\n", r)
			fmt.Printf("Details logged to: kubevision-debug.log\n")

			os.Exit(1)
		}
	}()

	if !isTerminal(os.Stdout) {
		log.Fatal("kubevision requires a terminal to run")
	}

	k8sClient, err := client.NewKubeClient()
	if err != nil {
		log.Printf("Failed to connect to cluster: %v", err)
		fmt.Printf("Failed to connect to cluster: %v\nCheck logs for details.\n", err)
		os.Exit(1)
	}

	model := explorer.NewExplorer(k8sClient)
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		log.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}