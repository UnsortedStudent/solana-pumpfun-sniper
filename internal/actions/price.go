package actions

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

var (
	priceMu sync.RWMutex
	solUSD  = 150.0 // fallback until the first successful fetch
)

// SolUSD returns the most recent SOL/USD price (a fallback value until the first fetch).
func SolUSD() float64 {
	priceMu.RLock()
	defer priceMu.RUnlock()
	return solUSD
}

func setSolUSD(v float64) {
	priceMu.Lock()
	solUSD = v
	priceMu.Unlock()
}

// StartPriceFeed refreshes the SOL/USD price from CoinGecko once a minute so the
// dashboard can show market caps and position sizes in USD.
func StartPriceFeed() {
	go func() {
		for {
			if p := fetchSolUSD(); p > 0 {
				setSolUSD(p)
			}
			time.Sleep(60 * time.Second)
		}
	}()
}

func fetchSolUSD() float64 {
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://api.coingecko.com/api/v3/simple/price?ids=solana&vs_currencies=usd")
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var data map[string]map[string]float64
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return 0
	}
	return data["solana"]["usd"]
}
