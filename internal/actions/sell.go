package actions

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
)

// pump.fun "sell" instruction discriminator (first 8 bytes of sha256("global:sell"),
// little-endian) — the mirror of the buy discriminator used in pfbuy.go.
const sellDiscriminator uint64 = 12502976635542562355

// positionTokenSize is the token amount recorded per position. The buy side fires
// fixed-size buys; a production build would query the wallet's token-account balance
// after the buy lands and sell exactly that. Kept simple here on purpose.
const positionTokenSize uint64 = 11000000

// associatedTokenProgramID is the SPL Associated Token Account program.
var associatedTokenProgramID = solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")

// priceForBondingCurve reads a pump.fun bonding-curve account and returns the current
// price in lamports of SOL per token (virtual SOL reserves / virtual token reserves).
func (bm *BuyManager) priceForBondingCurve(bondingCurve string) (float64, error) {
	bc, err := solana.PublicKeyFromBase58(bondingCurve)
	if err != nil {
		return 0, err
	}
	info, err := bm.rpcClient.GetAccountInfo(context.Background(), bc)
	if err != nil {
		return 0, err
	}
	if info == nil || info.Value == nil {
		return 0, fmt.Errorf("bonding curve account not found")
	}
	data := info.Value.Data.GetBinary()
	// Layout: [0:8] anchor discriminator, [8:16] virtualTokenReserves, [16:24] virtualSolReserves, ...
	if len(data) < 24 {
		return 0, fmt.Errorf("unexpected bonding curve data length: %d", len(data))
	}
	virtualTokenReserves := binary.LittleEndian.Uint64(data[8:16])
	virtualSolReserves := binary.LittleEndian.Uint64(data[16:24])
	if virtualTokenReserves == 0 {
		return 0, fmt.Errorf("zero token reserves")
	}
	return float64(virtualSolReserves) / float64(virtualTokenReserves), nil
}

// createSellTransaction builds a pump.fun sell transaction for an open position.
// It mirrors createBuyTransaction (same accounts/program), using the sell
// discriminator and a (amount, minSolOutput) argument pair.
func (bm *BuyManager) createSellTransaction(pos *Position) (*solana.Transaction, error) {
	ctx := context.Background()
	payer := bm.keypair.PublicKey()

	globalAccount, _ := solana.PublicKeyFromBase58("4wTV1YmiEkRvAtNtsSGPtUrqRYQMe5SKy2uB4Jjaxnjf")
	feeRecipient, _ := solana.PublicKeyFromBase58("CebN5WGQ4jvEPvsVU4EoHEpgzq1VV7AbicfhtW4xC9iM")
	mint, err := solana.PublicKeyFromBase58(pos.Mint)
	if err != nil {
		return nil, fmt.Errorf("invalid mint: %w", err)
	}
	bondingCurve, _ := solana.PublicKeyFromBase58(pos.BondingCurve)
	associatedBondingCurve, _ := solana.PublicKeyFromBase58(pos.AssociatedBondingCurve)
	pumpProgram, _ := solana.PublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")
	eventAuthority, _ := solana.PublicKeyFromBase58("Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1")

	ata, _, err := solana.FindAssociatedTokenAddress(payer, mint)
	if err != nil {
		return nil, fmt.Errorf("failed to find associated token address: %w", err)
	}

	// data = sell discriminator + amount + min_sol_output.
	// min_sol_output = 0 accepts any output; set a slippage-protected floor for production.
	sellData := append(encodeDiscriminant(sellDiscriminator), encodeUint64(pos.TokensHeld)...)
	sellData = append(sellData, encodeUint64(0)...)

	sellInstruction := solana.NewInstruction(
		pumpProgram,
		solana.AccountMetaSlice{
			solana.NewAccountMeta(globalAccount, false, false),
			solana.NewAccountMeta(feeRecipient, true, false),
			solana.NewAccountMeta(mint, false, false),
			solana.NewAccountMeta(bondingCurve, true, false),
			solana.NewAccountMeta(associatedBondingCurve, true, false),
			solana.NewAccountMeta(ata, true, false),
			solana.NewAccountMeta(payer, true, true),
			solana.NewAccountMeta(solana.SystemProgramID, false, false),
			solana.NewAccountMeta(associatedTokenProgramID, false, false),
			solana.NewAccountMeta(solana.TokenProgramID, false, false),
			solana.NewAccountMeta(eventAuthority, false, false),
			solana.NewAccountMeta(pumpProgram, false, false),
		},
		sellData,
	)

	recent, err := bm.rpcClient.GetRecentBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	computeUnitPrice := computebudget.NewSetComputeUnitPriceInstruction(50020).Build()
	computeUnitLimit := computebudget.NewSetComputeUnitLimitInstruction(100000).Build()

	tx, err := solana.NewTransaction(
		[]solana.Instruction{computeUnitPrice, computeUnitLimit, sellInstruction},
		recent.Value.Blockhash,
		solana.TransactionPayer(payer),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create sell transaction: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if payer.Equals(key) {
			return &bm.keypair
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign sell transaction: %w", err)
	}
	return tx, nil
}

// SellPosition builds and submits a sell for the given mint, then drops the position.
// Submission goes through the shared transactionSender, which honors DryRun.
func (bm *BuyManager) SellPosition(mint string) error {
	pos, ok := bm.positions.Get(mint)
	if !ok {
		return fmt.Errorf("no open position for %s", mint)
	}
	tx, err := bm.createSellTransaction(&pos)
	if err != nil {
		return err
	}
	bm.transactionSender <- tx
	bm.positions.Remove(mint)
	RecordEvent(fmt.Sprintf("SELL submitted %s", short(mint)))
	return nil
}

// MonitorExits periodically re-prices every open position and auto-sells when the
// configured take-profit or stop-loss threshold is hit.
func (bm *BuyManager) MonitorExits() {
	interval := time.Duration(bm.config.PollSeconds) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		for _, pos := range bm.positions.List() {
			if pos.EntryPriceLamports <= 0 {
				continue
			}
			price, err := bm.priceForBondingCurve(pos.BondingCurve)
			if err != nil {
				continue
			}
			pnl := (price - pos.EntryPriceLamports) / pos.EntryPriceLamports * 100
			bm.positions.UpdateMetrics(pos.Mint, price, pnl)

			switch {
			case bm.config.TakeProfitPct > 0 && pnl >= bm.config.TakeProfitPct:
				RecordEvent(fmt.Sprintf("take-profit %s (%.0f%%)", short(pos.Mint), pnl))
				_ = bm.SellPosition(pos.Mint)
			case bm.config.StopLossPct > 0 && pnl <= -bm.config.StopLossPct:
				RecordEvent(fmt.Sprintf("stop-loss %s (%.0f%%)", short(pos.Mint), pnl))
				_ = bm.SellPosition(pos.Mint)
			}
		}
	}
}

// short abbreviates a long base58 address for compact display.
func short(s string) string {
	if len(s) <= 10 {
		return s
	}
	return s[:4] + ".." + s[len(s)-4:]
}

// --- exported helpers used by the dashboard / entry point ---

// Positions returns the live position store (nil if the buy module isn't initialized).
func Positions() *PositionStore {
	if demoStore != nil {
		return demoStore
	}
	if buyManager == nil {
		return nil
	}
	return buyManager.positions
}

// Sell triggers a manual sell of an open position by mint.
func Sell(mint string) error {
	if demoStore != nil {
		if _, ok := demoStore.Get(mint); !ok {
			return fmt.Errorf("no open position for %s", mint)
		}
		demoStore.Remove(mint)
		RecordEvent(fmt.Sprintf("SELL submitted %s", short(mint)))
		return nil
	}
	if buyManager == nil {
		return fmt.Errorf("buy module not initialized")
	}
	return buyManager.SellPosition(mint)
}

// StartExitMonitor launches the take-profit / stop-loss monitor loop.
func StartExitMonitor() {
	if buyManager != nil {
		go buyManager.MonitorExits()
	}
}
