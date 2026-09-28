package main

import (
	"os"
	"syscall"
)

// The Windows build is a GUI program (no console window when double
// clicked): from a terminal, it writes to the terminal's console, where its
// output is not redirected. The terminal does not wait for it though: for
// scripts, the release also has wgen-cli.exe, a console program.
func init() {
	_, errOut := os.Stdout.Stat()
	_, errErr := os.Stderr.Stat()
	if errOut == nil && errErr == nil {
		return // a console program, or redirected
	}
	const attachParentProcess = ^uintptr(0)
	attach := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")
	if ok, _, _ := attach.Call(attachParentProcess); ok == 0 {
		return // not started from a terminal
	}
	f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	if errOut != nil {
		os.Stdout = f
	}
	if errErr != nil {
		os.Stderr = f
	}
}
