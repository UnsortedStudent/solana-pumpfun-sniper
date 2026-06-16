package main

import (
	"fmt"
	"log"
	"os"

	"zednine/internal/actions"
	"zednine/internal/transaction_monitor"
)

func main() {
	config := actions.BuyConfig{
		EnableWebsite:    true,
		EnableTwitter:    true,
		EnableTelegram:   true,
		RPCEndpoint:      mustEnv("RPC_ENDPOINT"),
		PrivateKeyBase58: mustEnv("WALLET_PRIVATE_KEY"),
	}

	actions.InitializeBuyModule(config)

	filteredBuySignalChan := make(chan actions.BuySignal, 100)

	go actions.FilterBuySignals(config, actions.BuySignalChan, filteredBuySignalChan)
	go transaction_monitor.MonitorTransactions()

	fmt.Println("Application started. Monitoring transactions and processing buy signals...")

	for signal := range filteredBuySignalChan {
		if err := actions.ProcessBuySignal(config, signal); err != nil {
			log.Printf("Error processing buy signal: %v", err)
		}
	}
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
