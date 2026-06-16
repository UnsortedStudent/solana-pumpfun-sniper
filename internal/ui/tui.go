// Package ui renders the terminal dashboard: a live launches feed, open positions
// with P/L, a recent-activity log, and buy/sell commands.
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

// Run starts the dashboard: an immediate paint, a 1s render loop, and a blocking
// command reader. It returns only when the user quits.
func Run(mode string) {
	enableVT()   // make ANSI escape codes work in the Windows console
	render(mode) // paint the first frame immediately
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
	b.WriteString("\033[2J\033[H") // clear screen + cursor home

	b.WriteString("==================================================================\n")
	b.WriteString("  Solana pump.fun Sniper\n")
	b.WriteString("  " + mode + "\n")
	b.WriteString("==================================================================\n")
	b.WriteString("  COMMANDS:  buy <#|addr>   sell <#|addr>   help   quit\n")
	b.WriteString("------------------------------------------------------------------\n")

	// --- live launches ---
	b.WriteString("\nLIVE LAUNCHES (newest first - buy by #)\n")
	b.WriteString(fmt.Sprintf("  %-3s %-10s %-20s %10s %5s\n", "#", "SYMBOL", "NAME", "MCAP(SOL)", "AGE"))
	launches := actions.RecentLaunches()
	if len(launches) == 0 {
		b.WriteString("  (waiting for launches...)\n")
	} else {
		for i, l := range launches {
			if i >= 8 {
				break
			}
			b.WriteString(fmt.Sprintf("  %-3d %-10s %-20s %10.1f %4ds\n",
				i+1, cell(l.Symbol, 10), cell(l.Name, 20), l.MarketCapSol, int(time.Since(l.At).Seconds())))
		}
	}

	// --- open positions ---
	b.WriteString("\nOPEN POSITIONS (sell by #)\n")
	b.WriteString(fmt.Sprintf("  %-3s %-10s %-16s %8s\n", "#", "SYMBOL", "MINT", "P/L"))
	var list []actions.Position
	if p := actions.Positions(); p != nil {
		list = p.List()
	}
	if len(list) == 0 {
		b.WriteString("  (none - buy a launch above, or: buy <address>)\n")
	} else {
		for i, p := range list {
			sym := p.Symbol
			if sym == "" {
				sym = "-"
			}
			b.WriteString(fmt.Sprintf("  %-3d %-10s %-16s %+7.1f%%\n",
				i+1, cell(sym, 10), short(p.Mint), p.PnLPct))
		}
	}

	// --- activity ---
	b.WriteString("\nRECENT ACTIVITY\n")
	events := actions.RecentEvents()
	if len(events) == 0 {
		b.WriteString("  (nothing yet)\n")
	} else {
		for _, e := range events {
			b.WriteString("  " + e + "\n")
		}
	}

	b.WriteString("\n> ")
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
		case "help", "h":
			fmt.Println("commands:  buy <launch#|address>  |  sell <position#|address>  |  quit")
		case "buy":
			if len(fields) < 2 {
				fmt.Println("usage: buy <launch#|mint-address>")
				continue
			}
			mint := resolveLaunch(fields[1])
			if mint == "" {
				mint = fields[1] // treat as a pasted address
			}
			if err := actions.Buy(mint); err != nil {
				fmt.Printf("buy failed: %v\n", err)
			} else {
				fmt.Printf("opened position in %s\n", short(mint))
			}
		case "sell":
			if len(fields) < 2 {
				fmt.Println("usage: sell <position#|mint-address>")
				continue
			}
			mint := resolvePosition(fields[1])
			if mint == "" {
				mint = fields[1]
			}
			if err := actions.SellHeld(mint); err != nil {
				fmt.Printf("sell failed: %v\n", err)
			} else {
				fmt.Printf("closed position in %s\n", short(mint))
			}
		default:
			fmt.Printf("unknown command: %s (try 'help')\n", fields[0])
		}
	}
}

// resolveLaunch maps a 1-based launch row number or a mint address to a mint.
func resolveLaunch(target string) string {
	list := actions.RecentLaunches()
	if n, err := strconv.Atoi(target); err == nil {
		if n >= 1 && n <= len(list) {
			return list[n-1].Mint
		}
		return ""
	}
	for _, l := range list {
		if l.Mint == target {
			return l.Mint
		}
	}
	return ""
}

// resolvePosition maps a 1-based position row number or a mint address to a mint.
func resolvePosition(target string) string {
	p := actions.Positions()
	if p == nil {
		return ""
	}
	list := p.List()
	if n, err := strconv.Atoi(target); err == nil {
		if n >= 1 && n <= len(list) {
			return list[n-1].Mint
		}
		return ""
	}
	for _, pos := range list {
		if pos.Mint == target {
			return pos.Mint
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

// cell strips control characters and truncates to n runes for safe column display
// (token names are user-supplied and may contain junk).
func cell(s string, n int) string {
	out := make([]rune, 0, n)
	for _, r := range s {
		if r < 32 || r == 127 {
			continue
		}
		out = append(out, r)
		if len(out) >= n {
			break
		}
	}
	return string(out)
}
