package main

import (
	"log"
	"os"
	"strconv"

	"zednine/internal/actions"
	"zednine/internal/transaction_monitor"
	"zednine/internal/ui"
)

func main() {
	// DryRun defaults ON for safety; set DRY_RUN=false to submit real transactions.
	dryRun := os.Getenv("DRY_RUN") != "false"

	config := actions.BuyConfig{
		EnableWebsite:    true,
		EnableTwitter:    true,
		EnableTelegram:   true,
		RPCEndpoint:      mustEnv("RPC_ENDPOINT"),
		PrivateKeyBase58: mustEnv("WALLET_PRIVATE_KEY"),
		DryRun:           dryRun,
		TakeProfitPct:    envFloat("TAKE_PROFIT_PCT", 50),
		StopLossPct:      envFloat("STOP_LOSS_PCT", 40),
		PollSeconds:      envInt("POLL_SECONDS", 3),
	}

	actions.InitializeBuyModule(config)
	actions.StartExitMonitor()

	filteredBuySignalChan := make(chan actions.BuySignal, 100)
	go actions.FilterBuySignals(config, actions.BuySignalChan, filteredBuySignalChan)
	go transaction_monitor.MonitorTransactions()
	go func() {
		for signal := range filteredBuySignalChan {
			if err := actions.ProcessBuySignal(config, signal); err != nil {
				log.Printf("Error processing buy signal: %v", err)
			}
		}
	}()

	// Terminal dashboard — blocks until you type "quit".
	ui.Run(dryRun)
}

// mustEnv returns the value of the named environment variable, or exits with a
// clear message if it is unset. Secrets and endpoints are configured this way so
// nothing sensitive is ever committed to source control (see .env.example).
func mustEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("required environment variable %s is not set (see .env.example)", key)
	}
	return value
}

// envFloat reads a float environment variable, falling back to def if unset/invalid.
func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// envInt reads an integer environment variable, falling back to def if unset/invalid.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
