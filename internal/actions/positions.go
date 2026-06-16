package actions

import (
	"sort"
	"sync"
	"time"
)

// Position is an open holding the bot is tracking so it can manage an exit
// (take-profit / stop-loss) and display it on the dashboard.
type Position struct {
	Mint                   string
	BondingCurve           string
	AssociatedBondingCurve string
	TokensHeld             uint64
	EntryPriceLamports     float64 // lamports of SOL per token at entry
	LastPriceLamports      float64 // most recent observed price (updated by the exit monitor)
	PnLPct                 float64 // most recent profit/loss percent vs entry
	OpenedAt               time.Time
}

// PositionStore is a concurrency-safe set of open positions keyed by mint.
type PositionStore struct {
	mu        sync.RWMutex
	positions map[string]*Position
}

func NewPositionStore() *PositionStore {
	return &PositionStore{positions: make(map[string]*Position)}
}

// Add records a new open position (a copy is stored).
func (s *PositionStore) Add(p Position) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := p
	s.positions[p.Mint] = &cp
}

// Remove drops a position (e.g. after it has been sold).
func (s *PositionStore) Remove(mint string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.positions, mint)
}

// Get returns a snapshot copy of a position.
func (s *PositionStore) Get(mint string) (Position, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.positions[mint]
	if !ok {
		return Position{}, false
	}
	return *p, true
}

// List returns snapshot copies of all open positions, oldest first (stable order
// so dashboard row numbers don't jump around).
func (s *PositionStore) List() []Position {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Position, 0, len(s.positions))
	for _, p := range s.positions {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	return out
}

// UpdateMetrics records the latest observed price and P/L for a position.
func (s *PositionStore) UpdateMetrics(mint string, lastPrice, pnlPct float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.positions[mint]; ok {
		p.LastPriceLamports = lastPrice
		p.PnLPct = pnlPct
	}
}

// --- lightweight recent-activity feed for the dashboard ---

var (
	feedMu sync.Mutex
	feed   []string
)

// RecordEvent appends a timestamped line to the activity feed (capped to the last 8).
func RecordEvent(msg string) {
	feedMu.Lock()
	defer feedMu.Unlock()
	feed = append(feed, time.Now().Format("15:04:05")+"  "+msg)
	if len(feed) > 8 {
		feed = feed[len(feed)-8:]
	}
}

// RecentEvents returns a copy of the recent activity feed.
func RecentEvents() []string {
	feedMu.Lock()
	defer feedMu.Unlock()
	out := make([]string, len(feed))
	copy(out, feed)
	return out
}
