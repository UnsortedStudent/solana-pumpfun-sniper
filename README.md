# Solana pump.fun Sniper (Go)

A low-latency Go service that detects newly created [pump.fun](https://pump.fun) tokens on Solana the moment they launch, filters them by on-chain social signals, buys within milliseconds, then **manages each position with take-profit / stop-loss exits** — all from a live **terminal dashboard**.

It streams transaction data from a validator over the **Geyser gRPC** interface (rather than polling RPC), pulls each new token's metadata from **IPFS**, and only acts on launches that meet configurable quality filters.

> **Safety:** `DRY_RUN` defaults to **on** — the bot builds and prices everything but submits **no** real transactions until you explicitly set `DRY_RUN=false`.

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
                               │  BuySignal
                               ▼
      ┌──────────────────────────────────────────────┐
      │              actions / filters                 │
      │  require website + X (Twitter) + Telegram      │
      └──────────────────────────────────────────────┘
                               │  filtered BuySignal
                               ▼
      ┌──────────────────────────────────────────────┐
      │              actions / buy manager             │
      │  • build pump.fun buy instruction              │
      │  • priority fee + concurrent attempts          │
      │  • record an open Position                     │
      └──────────────────────────────────────────────┘
                               │  Position
                               ▼
      ┌──────────────────────────────────────────────┐
      │          actions / exit monitor + sell         │
      │  • re-price each position from bonding curve   │
      │  • auto-sell on take-profit / stop-loss        │
      │  • manual sell from the dashboard              │
      └──────────────────────────────────────────────┘
                               │
                               ▼
                     terminal dashboard (ui)
```

1. **Monitor** (`internal/transaction_monitor`) opens a Geyser gRPC subscription to the pump.fun program, identifies token-creation events, decodes the mint/bonding-curve/metadata accounts, extracts the IPFS hash, and fetches the token metadata (local IPFS node first, then public gateways concurrently).
2. **Filter** (`internal/actions/filters.go`) only forwards launches that publish a website, an `x.com` profile, and a `t.me` channel (each toggleable).
3. **Buy** (`internal/actions/pfbuy.go`) builds the pump.fun `buy` instruction (priority fee + compute-unit limit + ATA creation), submits concurrent `skip_preflight` attempts, de-duplicates mints, and **records an open position**.
4. **Manage / Sell** (`internal/actions/sell.go`) re-prices every open position from its bonding-curve reserves and **auto-sells on a configurable take-profit or stop-loss**, building the pump.fun `sell` instruction (the mirror of the buy). Positions can also be sold manually from the dashboard.
5. **Dashboard** (`internal/ui/tui.go`) renders open positions with live P/L and a recent-activity feed, and accepts a `sell <#|mint>` command.

## Tech stack

- **Language:** Go (goroutines + channels for the concurrent pipeline)
- **Streaming:** gRPC (`google.golang.org/grpc`) against a Geyser endpoint; generated bindings in `proto/`
- **Solana:** [`github.com/gagliardetto/solana-go`](https://github.com/gagliardetto/solana-go) for keys, instructions, and RPC
- **Other:** `mr-tron/base58`, standard-library HTTP/JSON for IPFS, stdlib-only terminal UI

## Project structure

```
cmd/main.go                              # entry point: wires the pipeline + dashboard
internal/transaction_monitor/
  monitor.go                             # Geyser subscription + token-creation detection
  ipfs.go                                # IPFS metadata fetching (local node + gateways)
internal/actions/
  filters.go                             # social-signal eligibility filter
  pfbuy.go                               # pump.fun buy construction & submission
  positions.go                           # concurrency-safe open-position store + activity feed
  sell.go                                # pump.fun sell + take-profit/stop-loss exit monitor
internal/ui/
  tui.go                                 # terminal dashboard (positions, P/L, manual sell)
proto/                                   # generated Geyser gRPC bindings
```

## Configuration

All endpoints and secrets come from environment variables — nothing is hardcoded. See `.env.example`:

| Variable             | Default | Description                                          |
| -------------------- | ------- | ---------------------------------------------------- |
| `RPC_ENDPOINT`       | —       | Solana RPC endpoint for submitting transactions      |
| `GEYSER_GRPC_URL`    | —       | Geyser gRPC streaming endpoint (`host:port`)         |
| `WALLET_PRIVATE_KEY` | —       | Base58-encoded wallet secret key (use a test wallet) |
| `DRY_RUN`            | `true`  | Build but never submit transactions (safety default) |
| `TAKE_PROFIT_PCT`    | `50`    | Auto-sell when a position is up at least this %       |
| `STOP_LOSS_PCT`      | `40`    | Auto-sell when a position is down at least this %     |
| `POLL_SECONDS`       | `3`     | How often the exit monitor re-prices positions       |

**Never commit a real key.** Use a dedicated, low-value wallet — treat anything configured here as disposable.

## Try the dashboard (no setup)

To see the terminal dashboard immediately — no wallet, RPC, or Geyser endpoint — run it in **demo mode**, which seeds sample positions and simulates live price movement, take-profit / stop-loss exits, and new detections:

```bash
DEMO=true go run ./cmd        # Windows cmd:  set DEMO=true && go run ./cmd
```

Type `sell 1` (or `sell <mint>`) to exit a position by hand, or `quit` to stop.

## Build & run

```bash
go build ./...

export RPC_ENDPOINT="https://your-rpc"
export GEYSER_GRPC_URL="your-geyser-host:10000"
export WALLET_PRIVATE_KEY="your_base58_secret_key"
# DRY_RUN defaults to true — leave it until you've tested.

go run ./cmd
```

The dashboard takes over the terminal. Type `sell 1` (or `sell <mint>`) to exit a position manually, or `quit` to stop. Requires Go 1.23+ and a reachable Geyser gRPC endpoint (low-latency providers expose one; most public RPCs do not).

## Scope & limitations

The full **detect → filter → buy → manage/sell** loop plus a dashboard is implemented. Known simplifications (kept intentionally honest):

- **Position size on sell** uses a fixed token amount rather than querying the wallet's exact token-account balance after the buy lands.
- **Slippage:** the sell sets `min_sol_output = 0` (accepts any output). Add a slippage floor before live use.
- **Account layouts** for the pump.fun `buy`/`sell` instructions are pinned to a known program version; verify them against the current on-chain program before trading real funds.
- No persistence — positions live in memory for the session.

## ⚠️ Disclaimer

Shared for **educational and portfolio purposes.** Automated trading of highly volatile on-chain assets carries a real risk of total loss, and "sniping" markets are adversarial. Nothing here is financial advice. Run it only with funds you can afford to lose entirely, and comply with the laws and platform terms that apply to you.

## License

[MIT](LICENSE)
