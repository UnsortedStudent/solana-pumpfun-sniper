package actions

import (
	"fmt"
	"math/rand"
	"time"
)

// StartDemo seeds sample launches and positions and simulates live P/L movement,
// take-profit / stop-loss exits, and new detections — so the dashboard can be viewed
// instantly with no wallet, RPC, or Geyser endpoint (DEMO=true).
func StartDemo() {
	StartSession()
	now := time.Now()
	seed := []struct {
		mint, name, symbol string
		pnl                float64
	}{
		{"7AcbX9qY2vKpZr4tWfHpump", "Holy Sheesh", "HOLY", 8},
		{"9XqRkBnk5mNvD2sLrQ4moon", "Moon Doge", "MOONDOGE", -3},
		{"3PdFm7Zk8Rq2vLtWcXbpump", "Banana Coin", "NANA", 21},
	}
	for i, s := range seed {
		entry := 30.0
		RecordLaunch(Launch{Mint: s.mint, Name: s.name, Symbol: s.symbol, MarketCapSol: entry, At: now.Add(time.Duration(i) * time.Second)})
		activeStore.Add(Position{
			Mint:               s.mint,
			Name:               s.name,
			Symbol:             s.symbol,
			TokensHeld:         positionTokenSize,
			EntryPriceLamports: entry,
			LastPriceLamports:  entry * (1 + s.pnl/100),
			PnLPct:             s.pnl,
			OpenedAt:           now.Add(time.Duration(i) * time.Second),
		})
		RecordEvent(fmt.Sprintf("BUY %s", s.symbol))
	}
	go demoLoop()
}

func demoLoop() {
	const takeProfit, stopLoss = 50.0, 40.0
	names := []string{"PEPE", "WIF", "BONK2", "GIGA", "MOG", "TURBO", "SLERF"}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		for _, p := range activeStore.List() {
			pnl := p.PnLPct + (rand.Float64()-0.45)*12 // slight upward random walk
			activeStore.UpdateMetrics(p.Mint, p.EntryPriceLamports*(1+pnl/100), pnl)
			switch {
			case pnl >= takeProfit:
				RecordEvent(fmt.Sprintf("take-profit %s (%.0f%%)", labelFor(p.Symbol, p.Mint), pnl))
				activeStore.Remove(p.Mint)
			case pnl <= -stopLoss:
				RecordEvent(fmt.Sprintf("stop-loss %s (%.0f%%)", labelFor(p.Symbol, p.Mint), pnl))
				activeStore.Remove(p.Mint)
			}
		}
		if rand.Intn(3) == 0 { // simulate a fresh launch arriving in the feed
			sym := names[rand.Intn(len(names))]
			mint := fmt.Sprintf("Dmo%03d%sSampleMint0000", rand.Intn(1000), sym)
			RecordLaunch(Launch{Mint: mint, Name: sym + " Coin", Symbol: sym, MarketCapSol: 25 + rand.Float64()*40})
			RecordEvent(fmt.Sprintf("launch %s", sym))
		}
	}
}
