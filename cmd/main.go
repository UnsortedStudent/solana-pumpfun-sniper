package main

import (
	"log"
	"os"

	"zednine/internal/actions"
	"zednine/internal/feed"
	"zednine/internal/transaction_monitor"
	"zednine/internal/ui"
)

func main() {
	// Keep framework logs out of the dashboard by sending them to a file.
	if f, err := os.OpenFile("sniper.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(f)
	}

	// DEMO mode: simulated data, zero setup -> `DEMO=true go run ./cmd`.
	if os.Getenv("DEMO") == "true" {
		actions.StartDemo()
		ui.Run("DEMO - simulated data (no wallet/RPC/Geyser needed)")
		return
	}

	actions.StartSession()

	// Data source. Default: PumpPortal's free live feed (real launches, no setup).
	// SOURCE=geyser streams from your own low-latency Geyser gRPC endpoint instead
	// (set GEYSER_GRPC_URL) - see README and .env.example.
	var mode string
	switch os.Getenv("SOURCE") {
	case "geyser":
		go transaction_monitor.MonitorTransactions()
		go func() {
			for sig := range actions.BuySignalChan {
				actions.RecordLaunch(actions.Launch{Mint: sig.MintAccount, Name: sig.Name, Symbol: sig.Symbol})
			}
		}()
		mode = "LIVE - Geyser gRPC source (set GEYSER_GRPC_URL)"
	default:
		if err := feed.Start(); err != nil {
			log.Printf("PumpPortal connect failed: %v", err)
			mode = "LIVE - PumpPortal unreachable (check your connection)"
		} else {
			mode = "LIVE - real launches via PumpPortal (free, no setup)"
		}
	}

	ui.Run(mode)
}
