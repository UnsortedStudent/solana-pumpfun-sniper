// Package ui renders the terminal dashboard: a live launches feed, open positions
// with P/L, a recent-activity log, and buy/sell commands. Command output goes to a
// persistent status line so the 1s repaint never wipes it.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"zednine/internal/actions"
)

var (
	statusMu sync.Mutex
	status   = "type 'help' for commands"
)

func setStatus(s string) {
	statusMu.Lock()
	status = s
	statusMu.Unlock()
}

func getStatus() string {
	statusMu.Lock()
	defer statusMu.Unlock()
	return status
}

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
	b.WriteString(fmt.Sprintf("  %s   (SOL $%.2f)\n", mode, actions.SolUSD()))
	b.WriteString("==================================================================\n")
	b.WriteString("  COMMANDS:  buy <#|addr> [sol]   sell <#|addr>   help   quit\n")
	b.WriteString("------------------------------------------------------------------\n")

	// --- live launches ---
	b.WriteString("\nLIVE LAUNCHES (newest first - buy by #)\n")
	b.WriteString(fmt.Sprintf("  %-3s %-10s %-20s %10s %5s\n", "#", "SYMBOL", "NAME", "MCAP(USD)", "AGE"))
	launches := actions.RecentLaunches()
	if len(launches) == 0 {
		b.WriteString("  (waiting for launches...)\n")
	} else {
		sol := actions.SolUSD()
		for i, l := range launches {
			if i >= 8 {
				break
			}
			b.WriteString(fmt.Sprintf("  %-3d %-10s %-20s %10s %4ds\n",
				i+1, cell(l.Symbol, 10), cell(l.Name, 20), usd(l.MarketCapSol*sol), int(time.Since(l.At).Seconds())))
		}
	}

	// --- open positions ---
	b.WriteString("\nOPEN POSITIONS (sell by #)\n")
	b.WriteString(fmt.Sprintf("  %-3s %-10s %-14s %6s %8s\n", "#", "SYMBOL", "MINT", "SOL", "P/L"))
	var list []actions.Position
	if p := actions.Positions(); p != nil {
		list = p.List()
	}
	if len(list) == 0 {
		b.WriteString("  (none - buy a launch above, or: buy <address> <sol>)\n")
	} else {
		for i, p := range list {
			sym := p.Symbol
			if sym == "" {
				sym = "-"
			}
			b.WriteString(fmt.Sprintf("  %-3d %-10s %-14s %6.2f %+7.1f%%\n",
				i+1, cell(sym, 10), short(p.Mint), p.SolSpent, p.PnLPct))
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

	// persistent status line + prompt
	b.WriteString("\n  " + getStatus() + "\n> ")
	fmt.Print(b.String())
}

func readCommands() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "\uFEFF") // tolerate a leading BOM
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "quit", "exit", "q":
			fmt.Println("Shutting down.")
			os.Exit(0)
		case "help", "h":
			setStatus("buy <#|addr> [sol]  (e.g. 'buy <address> 5' = 5 SOL)  |  sell <#|addr>  |  quit")
		case "buy":
			if len(fields) < 2 {
				setStatus("usage: buy <launch#|address> [sol-amount]")
				continue
			}
			mint, ok := resolveTarget(fields[1], resolveLaunch, "launch")
			if !ok {
				continue
			}
			sol := 1.0
			if len(fields) >= 3 {
				if v, err := strconv.ParseFloat(fields[2], 64); err == nil && v > 0 {
					sol = v
				}
			}
			if err := actions.Buy(mint, sol); err != nil {
				setStatus("buy failed: " + err.Error())
			} else {
				setStatus(fmt.Sprintf("opened %.2f SOL position in %s", sol, short(mint)))
			}
		case "sell":
			if len(fields) < 2 {
				setStatus("usage: sell <position#|address>")
				continue
			}
			mint, ok := resolveTarget(fields[1], resolvePosition, "position")
			if !ok {
				continue
			}
			if err := actions.SellHeld(mint); err != nil {
				setStatus("sell failed: " + err.Error())
			} else {
				setStatus("closed position in " + short(mint))
			}
		default:
			setStatus("unknown command: " + fields[0] + " (try 'help')")
		}
	}
}

// resolveTarget interprets a command argument as either a row number (resolved via
// `resolver`) or a pasted mint address, setting a status message and returning
// ok=false when it's neither a valid row nor a plausible address.
func resolveTarget(target string, resolver func(string) string, kind string) (string, bool) {
	if _, err := strconv.Atoi(target); err == nil {
		mint := resolver(target)
		if mint == "" {
			setStatus("no " + kind + " #" + target)
			return "", false
		}
		return mint, true
	}
	if len(target) < 32 {
		setStatus("not a " + kind + " # or a valid address: " + target)
		return "", false
	}
	return target, true
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

// usd formats a dollar amount compactly ($31, $2.1K, $4.5M).
func usd(v float64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("$%.1fM", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("$%.1fK", v/1_000)
	default:
		return fmt.Sprintf("$%.0f", v)
	}
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
