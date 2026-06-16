# Solana pump.fun Sniper (Go)

A low-latency Go service that detects newly created [pump.fun](https://pump.fun) tokens on Solana the moment they launch, filters them by on-chain social signals, and submits buy transactions within milliseconds.

It streams live transaction data from a validator over the **Geyser gRPC** interface (rather than polling RPC), pulls each new token's metadata from **IPFS**, and only acts on launches that meet configurable quality filters.

> **Status:** working detection → filter → buy pipeline. Exit/sell logic and a UI are intentionally out of scope here — see [Scope & limitations](#scope--limitations).

---

## Pipeline

```
                         Solana Validator
                               │
                   Geyser gRPC stream (transactions)
                               │
                               ▼
      ┌──────────────────────────────────────────────┐
      │              transaction_monitor               │
      │  • subscribe to the pump.fun program           │
      │  • find token-creation transactions            │
      │  • decode mint + bonding-curve accounts        │
      │  • extract IPFS hash, fetch token metadata     │
      └──────────────────────────────────────────────┘
                               │  BuySignal (channel)
                               ▼
      ┌──────────────────────────────────────────────┐
      │              actions / filters                 │
      │  require website + X (Twitter) + Telegram      │
      │  before a launch is eligible to buy            │
      └──────────────────────────────────────────────┘
                               │  filtered BuySignal
                               ▼
      ┌──────────────────────────────────────────────┐
      │              actions / buy manager             │
      │  • build pump.fun buy instruction              │
      │  • set priority fee + compute-unit limit       │
      │  • fire concurrent attempts, skip-preflight    │
      │  • de-duplicate already-purchased mints        │
      └──────────────────────────────────────────────┘
```

1. **Monitor** (`internal/transaction_monitor`) opens a Geyser gRPC subscription to the pump.fun program, inspects inner instructions to identify a token-creation event, and decodes the mint, bonding-curve, and metadata accounts straight from the transaction. It extracts the token's IPFS hash and fetches the metadata JSON (trying a local IPFS node first, then public gateways concurrently).
2. **Filter** (`internal/actions/filters.go`) gates each detected launch on social legitimacy — it only forwards tokens that publish a website, an `x.com` profile, and a `t.me` channel. These checks are toggleable via config.
3. **Buy** (`internal/actions/pfbuy.go`) constructs the pump.fun `buy` instruction (with an explicit compute-unit price/limit for landing priority and associated-token-account creation), signs it, and submits several concurrent attempts with `skip_preflight` for speed. Mints already bought are tracked in a concurrent map so they aren't re-purchased.

## Tech stack

- **Language:** Go (goroutines + channels for the concurrent pipeline)
- **Streaming:** gRPC (`google.golang.org/grpc`) against a Geyser endpoint; generated bindings in `proto/`
- **Solana:** [`github.com/gagliardetto/solana-go`](https://github.com/gagliardetto/solana-go) for keys, instructions, and RPC
- **Other:** `mr-tron/base58`, standard-library HTTP/JSON for IPFS

## Project structure

```
cmd/main.go                              # entry point: wires monitor → filter → buy
internal/transaction_monitor/
  monitor.go                             # Geyser subscription + token-creation detection
  ipfs.go                                # IPFS metadata fetching (local node + gateways)
internal/actions/
  filters.go                             # social-signal eligibility filter
  pfbuy.go                               # pump.fun buy-instruction construction & submission
proto/                                   # generated Geyser gRPC bindings
```

## Configuration

All endpoints and secrets come from environment variables — nothing is hardcoded. See `.env.example`:

| Variable             | Description                                          |
| -------------------- | ---------------------------------------------------- |
| `RPC_ENDPOINT`       | Solana RPC endpoint for submitting transactions      |
| `GEYSER_GRPC_URL`    | Geyser gRPC streaming endpoint (`host:port`)         |
| `WALLET_PRIVATE_KEY` | Base58-encoded wallet secret key (use a test wallet) |

**Never commit a real key.** Use a dedicated, low-value wallet — treat anything configured here as disposable.

## Build & run

```bash
go build ./...

export RPC_ENDPOINT="https://your-rpc"
export GEYSER_GRPC_URL="your-geyser-host:10000"
export WALLET_PRIVATE_KEY="your_base58_secret_key"

go run ./cmd
```

Requires Go 1.23+ and a reachable Geyser gRPC endpoint (low-latency providers expose one; most public RPCs do not).

## Scope & limitations

This repo is the **detection → filter → buy** engine. Deliberately not included:

- **Sell / exit strategy** — take-profit, stop-loss, and the pump.fun `sell` path are not implemented; exits are manual/external.
- **Position sizing & risk controls** beyond the fixed buy parameters.
- **UI / dashboard.**

## ⚠️ Disclaimer

Shared for **educational and portfolio purposes.** Automated trading of highly volatile on-chain assets carries a real risk of total loss, and "sniping" markets are adversarial. Nothing here is financial advice. Run it only with funds you can afford to lose entirely, and comply with the laws and platform terms that apply to you.

## License

[MIT](LICENSE)
