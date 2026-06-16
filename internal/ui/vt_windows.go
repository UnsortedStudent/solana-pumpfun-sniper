//go:build windows

package ui

import "golang.org/x/sys/windows"

// enableVT turns on virtual-terminal (ANSI) processing so the dashboard's escape
// codes render correctly in the Windows console instead of printing as raw text.
func enableVT() {
	h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return
	}
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return
	}
	windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}
