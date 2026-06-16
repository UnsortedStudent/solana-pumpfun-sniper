package actions

import (
	"fmt"
	"sync"
	"time"
)

// activeStore holds the dashboard's positions in live/paper or demo mode.
var activeStore *PositionStore

// Hooks set by the feed package so a paper buy/sell can (un)subscribe to a token's
// live trades for P/L updates. Kept as hooks to avoid an import cycle (feed → actions).
var (
	SubscribeTrades   func(mint string)
	UnsubscribeTrades func(mint string)
)

// StartSession initializes the dashboard's position store.
func StartSession() {
	activeStore = NewPositionStore()
}

// ---------------------------------------------------------------------------
// Live launches (newly detected tokens from the active source)
// ---------------------------------------------------------------------------

// Launch is a newly detected pump.fun token.
type Launch struct {
	Mint         string
	Name         string
	Symbol       string
	MarketCapSol float64
	At           time.Time
}

var (
	launchMu sync.Mutex
	launches []Launch // oldest first
)

// RecordLaunch adds a detected token to the recent-launches list (capped at 12).
func RecordLaunch(l Launch) {
	if l.At.IsZero() {
		l.At = time.Now()
	}
	launchMu.Lock()
	defer launchMu.Unlock()
	launches = append(launches, l)
	if len(launches) > 12 {
		launches = launches[len(launches)-12:]
	}
}

// RecentLaunches returns recent launches, newest first.
func RecentLaunches() []Launch {
	launchMu.Lock()
	defer launchMu.Unlock()
	out := make([]Launch, len(launches))
	copy(out, launches)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func lookupLaunch(mint string) (name, symbol string, mcap float64) {
	launchMu.Lock()
	defer launchMu.Unlock()
	for _, l := range launches {
		if l.Mint == mint {
			return l.Name, l.Symbol, l.MarketCapSol
		}
	}
	return "", "", 0
}

// UpdateMarketCap refreshes an open position's market cap and recomputes its P/L.
// Called by the feed when a real trade arrives for a held token.
func UpdateMarketCap(mint string, mcap float64) {
	if activeStore == nil || mcap <= 0 {
		return
	}
	pos, ok := activeStore.Get(mint)
	if !ok || pos.EntryPriceLamports <= 0 {
		return
	}
	pnl := (mcap - pos.EntryPriceLamports) / pos.EntryPriceLamports * 100
	activeStore.UpdateMetrics(mint, mcap, pnl)
}

// ---------------------------------------------------------------------------
// Manual (paper) buy / sell from the dashboard
// ---------------------------------------------------------------------------

// Buy opens a position for a token — either a launch row or a pasted mint address.
// Positions are tracked on live market-cap data; on-chain execution runs through the
// configured trading path (wallet + RPC) when enabled.
func Buy(mint string) error {
	if activeStore == nil {
		return fmt.Errorf("session not started")
	}
	if mint == "" {
		return fmt.Errorf("empty mint address")
	}
	if _, ok := activeStore.Get(mint); ok {
		return fmt.Errorf("already holding %s", short(mint))
	}
	name, symbol, mcap := lookupLaunch(mint)
	activeStore.Add(Position{
		Mint:               mint,
		Name:               name,
		Symbol:             symbol,
		TokensHeld:         positionTokenSize,
		EntryPriceLamports: mcap,
		LastPriceLamports:  mcap,
		OpenedAt:           time.Now(),
	})
	RecordEvent(fmt.Sprintf("BUY %s", labelFor(symbol, mint)))
	if SubscribeTrades != nil {
		SubscribeTrades(mint)
	}
	return nil
}

// SellHeld closes an open position by mint.
func SellHeld(mint string) error {
	if activeStore == nil {
		return fmt.Errorf("session not started")
	}
	pos, ok := activeStore.Get(mint)
	if !ok {
		return fmt.Errorf("no open position for %s", short(mint))
	}
	activeStore.Remove(mint)
	RecordEvent(fmt.Sprintf("SELL %s (%+.0f%%)", labelFor(pos.Symbol, mint), pos.PnLPct))
	if UnsubscribeTrades != nil {
		UnsubscribeTrades(mint)
	}
	return nil
}

func labelFor(symbol, mint string) string {
	if symbol != "" {
		return symbol
	}
	return short(mint)
}
