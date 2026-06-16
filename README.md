# Solana pump.fun Sniper (Go)

A real-time [pump.fun](https://pump.fun) launch monitor and trading dashboard in Go. It streams newly created tokens the moment they launch, shows them in a live terminal dashboard, and lets you open/close positions and track P/L — with two interchangeable data sources and the full on-chain buy/sell engine included.

> **Zero setup:** by default it streams **real launches** from PumpPortal's free WebSocket — no wallet, RPC, or paid endpoint needed. Point it at your own **Geyser gRPC** endpoint for the low-latency path when you want it.

---

## Quick start

```bash
go run ./cmd
```

That connects to the free live feed and opens the dashboard. In it:

```
COMMANDS:  buy <#|addr>   sell <#|addr>   help   quit
```

- `buy 1` — open a position in launch #1 (or `buy <mint-address>` to paste any token)
- `sell 1` — close position #1 (or `sell <mint-address>`)
- `quit` — exit

No internet/Go handy? `DEMO=true go run ./cmd` runs it on simulated data.

## Data sources

| `SOURCE` | What it does |
| -------- | ------------ |
| `pumpportal` *(default)* | Streams real pump.fun launches from PumpPortal's free WebSocket. No wallet, RPC, or key required. |
| `geyser` | Streams from your own **Geyser gRPC** endpoint (`GEYSER_GRPC_URL`) — the low-latency, production path. Most public RPCs don't expose Geyser; low-latency providers do. |

```bash
SOURCE=geyser GEYSER_GRPC_URL=your-geyser-host:10000 go run ./cmd
```

## Dashboard

```
==================================================================
  Solana pump.fun Sniper
  LIVE - real launches via PumpPortal (free, no setup)
==================================================================
  COMMANDS:  buy <#|addr>   sell <#|addr>   help   quit
------------------------------------------------------------------
LIVE LAUNCHES (newest first - buy by #)
  #   SYMBOL     NAME                  MCAP(SOL)   AGE
  1   HUMANS     Return To Humans           28.0    0s
  ...
OPEN POSITIONS (sell by #)
  #   SYMBOL     MINT                  P/L
  1   ORYK       2297..pump          +12.4%
RECENT ACTIVITY
  02:07:50  BUY ORYK
```

Open positions track **live P/L** from real market-cap updates (the feed subscribes to each held token's trades). Dashboard buy/sell is paper-tracked on live data; real on-chain execution runs through the configured trading path below.

## On-chain trading engine

The repo also contains the full low-latency **buy/sell engine** (the Geyser path):

1. **Monitor** (`internal/transaction_monitor`) — Geyser gRPC subscription to the pump.fun program; decodes mint/bonding-curve accounts and pulls token metadata from **IPFS**.
2. **Filter** (`internal/actions/filters.go`) — only forwards launches with a website + `x.com` + `t.me` (each toggleable).
3. **Buy** (`internal/actions/pfbuy.go`) — builds the pump.fun `buy` instruction (priority fee + compute-unit limit + ATA creation) and submits concurrent `skip_preflight` attempts.
4. **Manage / Sell** (`internal/actions/sell.go`) — re-prices positions from bonding-curve reserves and auto-sells on take-profit / stop-loss, building the pump.fun `sell` instruction (mirror of the buy).

`DRY_RUN` defaults to **on** — transactions are built but never submitted until you set `DRY_RUN=false` with a funded wallet + RPC.

## Tech stack

- **Language:** Go (goroutines + channels)
- **Live feed:** `gorilla/websocket` against PumpPortal's API
- **Streaming (Geyser):** `google.golang.org/grpc`; generated bindings in `proto/`
- **Solana:** [`github.com/gagliardetto/solana-go`](https://github.com/gagliardetto/solana-go)
- **UI:** stdlib-only terminal dashboard (ANSI), Windows VT support

## Project structure

```
cmd/main.go                  # entry point: mode + data-source selection, dashboard
internal/feed/
  pumpportal.go              # free live launch feed (WebSocket) — default source
internal/transaction_monitor/
  monitor.go, ipfs.go        # Geyser gRPC source + IPFS metadata
internal/actions/
  session.go                 # launches store, paper buy/sell, live P/L
  positions.go               # concurrency-safe position store + activity feed
  filters.go                 # social-signal eligibility filter
  pfbuy.go, sell.go          # on-chain pump.fun buy/sell + exit monitor
  demo.go                    # simulated data for DEMO mode
internal/ui/
  tui.go                     # terminal dashboard + commands
proto/                       # generated Geyser gRPC bindings
```

## Configuration

Everything is configured via environment variables — nothing is hardcoded. See `.env.example`:

| Variable             | Default        | Description                                                     |
| -------------------- | -------------- | -------------------------------------------------------------- |
| `SOURCE`             | `pumpportal`   | Live data source: `pumpportal` (free) or `geyser`              |
| `DEMO`               | `false`        | Run on simulated data (no network needed)                      |
| `GEYSER_GRPC_URL`    | —              | Geyser gRPC endpoint (`host:port`) when `SOURCE=geyser`         |
| `RPC_ENDPOINT`       | —              | Solana RPC endpoint (for the on-chain trading engine)          |
| `WALLET_PRIVATE_KEY` | —              | Base58 wallet secret key — use a dedicated, disposable wallet  |
| `DRY_RUN`            | `true`         | Build but never submit transactions (safety default)           |
| `TAKE_PROFIT_PCT`    | `50`           | Auto-sell when a position is up at least this %                 |
| `STOP_LOSS_PCT`      | `40`           | Auto-sell when a position is down at least this %               |
| `POLL_SECONDS`       | `3`            | How often the exit monitor re-prices positions                 |

**Never commit a real key.**

## Scope & limitations (kept honest)

- The dashboard's buy/sell **paper-tracks** positions on live market-cap data; **real on-chain execution** runs through the Geyser + wallet path (instruction builders in `pfbuy.go`/`sell.go`, `DRY_RUN` on by default).
- Sell sizing uses a fixed token amount rather than querying the exact token-account balance; `min_sol_output` is `0` (no slippage floor) — set one before live trading.
- pump.fun `buy`/`sell` account layouts are pinned to a known program version — verify against the current on-chain program before trading real funds.
- No persistence — state lives in memory for the session.

## ⚠️ Disclaimer

Shared for **educational and portfolio purposes.** Automated trading of highly volatile on-chain assets carries a real risk of total loss, and "sniping" markets are adversarial. Nothing here is financial advice. Run it only with funds you can afford to lose entirely, and comply with the laws and platform terms that apply to you.

## License

[MIT](LICENSE)
