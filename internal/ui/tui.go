// Package ui renders a simple terminal dashboard for the sniper: open positions
// with live P/L, a recent-activity feed, and a manual "sell" command.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"zednine/internal/actions"
)

// Run starts the dashboard: a render loop in the background and a blocking command
// reader on the foreground. It returns only when the user quits.
func Run(mode string) {
	enableVT()    // make ANSI escape codes work in the Windows console
	render(mode)  // paint the first frame immediately
	go renderLoop(mode)
	readCommands()
}

func renderLoop(mode string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		render(mode)
	}
}

func render(mode string) {
	var b strings.Builder
	b.WriteString("\033[2J\033[H") // clear screen + move cursor home

	b.WriteString("==============================================================\n")
	b.WriteString("  Solana pump.fun Sniper\n")
	b.WriteString("  Mode: " + mode + "\n")
	b.WriteString("==============================================================\n\n")

	b.WriteString("OPEN POSITIONS\n")
	b.WriteString(fmt.Sprintf("  %-3s %-14s %-12s %-8s\n", "#", "MINT", "HELD", "P/L"))
	positions := actions.Positions()
	var list []actions.Position
	if positions != nil {
		list = positions.List()
	}
	if len(list) == 0 {
		b.WriteString("  (none yet — waiting for the monitor to detect a launch)\n")
	} else {
		for i, p := range list {
			b.WriteString(fmt.Sprintf("  %-3d %-14s %-12d %+7.1f%%\n",
				i+1, short(p.Mint), p.TokensHeld, p.PnLPct))
		}
	}

	b.WriteString("\nRECENT ACTIVITY\n")
	events := actions.RecentEvents()
	if len(events) == 0 {
		b.WriteString("  (nothing yet)\n")
	} else {
		for _, e := range events {
			b.WriteString("  " + e + "\n")
		}
	}

	b.WriteString("\nCommands:  sell <#|mint>   |   quit\n> ")
	fmt.Print(b.String())
}

func readCommands() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fields := strings.Fields(strings.TrimSpace(scanner.Text()))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "quit", "exit", "q":
			fmt.Println("Shutting down.")
			os.Exit(0)
		case "sell":
			if len(fields) < 2 {
				fmt.Println("usage: sell <#|mint>")
				continue
			}
			mint := resolveMint(fields[1])
			if mint == "" {
				fmt.Println("no matching open position")
				continue
			}
			if err := actions.Sell(mint); err != nil {
				fmt.Printf("sell failed: %v\n", err)
			} else {
				fmt.Printf("sell submitted for %s\n", mint)
			}
		default:
			fmt.Printf("unknown command: %s\n", fields[0])
		}
	}
}

// resolveMint accepts either a 1-based row number from the dashboard or a full mint
// address and returns the matching open position's mint (empty if none).
func resolveMint(target string) string {
	positions := actions.Positions()
	if positions == nil {
		return ""
	}
	list := positions.List()
	if n, err := strconv.Atoi(target); err == nil {
		if n >= 1 && n <= len(list) {
			return list[n-1].Mint
		}
		return ""
	}
	for _, p := range list {
		if p.Mint == target {
			return p.Mint
		}
	}
	return ""
}

func short(s string) string {
	if len(s) <= 10 {
		return s
	}
	return s[:4] + ".." + s[len(s)-4:]
}
