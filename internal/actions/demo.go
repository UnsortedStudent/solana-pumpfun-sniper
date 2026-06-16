package actions

import (
	"fmt"
	"math/rand"
	"time"
)

// demoStore, when non-nil, makes the dashboard show simulated data instead of live
// positions. Enabled with DEMO=true — it needs no wallet, RPC, or Geyser endpoint.
var demoStore *PositionStore

// StartDemo seeds sample positions and simulates live price movement plus
// take-profit / stop-loss exits, so the terminal dashboard can be viewed instantly.
func StartDemo() {
	demoStore = NewPositionStore()
	now := time.Now()
	seed := []struct {
		mint string
		pnl  float64
	}{
		{"7AcbX9qY2vKpZr4tWfHpump", 8},
		{"9XqRkBnk5mNvD2sLrQ4moon", -3},
		{"3PdFm7Zk8Rq2vLtWcXbpump", 21},
	}
	for i, s := range seed {
		demoStore.Add(Position{
			Mint:               s.mint,
			TokensHeld:         11000000,
			EntryPriceLamports: 1,
			LastPriceLamports:  1 + s.pnl/100,
			PnLPct:             s.pnl,
			OpenedAt:           now.Add(time.Duration(i) * time.Second),
		})
		RecordEvent(fmt.Sprintf("BUY %s", short(s.mint)))
	}
	go demoLoop()
}

func demoLoop() {
	const takeProfit, stopLoss = 50.0, 40.0
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		for _, p := range demoStore.List() {
			pnl := p.PnLPct + (rand.Float64()-0.45)*12 // slight upward random walk
			demoStore.UpdateMetrics(p.Mint, 1+pnl/100, pnl)
			switch {
			case pnl >= takeProfit:
				RecordEvent(fmt.Sprintf("take-profit %s (%.0f%%)", short(p.Mint), pnl))
				demoStore.Remove(p.Mint)
			case pnl <= -stopLoss:
				RecordEvent(fmt.Sprintf("stop-loss %s (%.0f%%)", short(p.Mint), pnl))
				demoStore.Remove(p.Mint)
			}
		}
		if rand.Intn(5) == 0 { // occasionally "detect" a fresh launch
			mint := fmt.Sprintf("Dmo%03dpumpSampleMint00000", rand.Intn(1000))
			demoStore.Add(Position{
				Mint: mint, TokensHeld: 11000000, EntryPriceLamports: 1, OpenedAt: time.Now(),
			})
			RecordEvent(fmt.Sprintf("BUY %s", short(mint)))
		}
	}
}
