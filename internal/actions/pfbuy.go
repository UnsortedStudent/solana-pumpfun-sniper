package actions

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
)

// BuySignal struct definition
type BuySignal struct {
	TransactionHash        string
	MintAccount            string
	BondingCurve           string
	AssociatedBondingCurve string
	Metadata               string
	TimeDetected           string
	Name                   string
	Symbol                 string
	Description            string
	Twitter                string
	Telegram               string
	Website                string
	IPFSProcessingTime     time.Duration
}

// BuySignalChan definition
var BuySignalChan = make(chan BuySignal, 100)

type BuyConfig struct {
	EnableWebsite        bool
	EnableTwitter        bool
	EnableTelegram       bool
	RPCEndpoint          string
	SecondaryRPCEndpoint string
	PrivateKeyBase58     string

	// Risk / exit controls (used by the sell side and the dashboard).
	DryRun        bool    // when true, transactions are built but never submitted
	TakeProfitPct float64 // auto-sell when a position is up at least this percent
	StopLossPct   float64 // auto-sell when a position is down at least this percent
	PollSeconds   int     // how often the exit monitor re-checks prices
}

type BuyManager struct {
	config             BuyConfig
	rpcClient          *rpc.Client
	secondaryRpcClient *rpc.Client
	keypair            solana.PrivateKey
	purchasedTokens    sync.Map
	transactionSender  chan *solana.Transaction
	positions          *PositionStore
}

var (
	buyManager *BuyManager
	once       sync.Once
)

func InitializeBuyModule(config BuyConfig) {
	once.Do(func() {
		privateKey, err := solana.PrivateKeyFromBase58(config.PrivateKeyBase58)
		if err != nil {
			log.Fatalf("Failed to load private key: %v", err)
		}

		buyManager = &BuyManager{
			config:             config,
			rpcClient:          rpc.New(config.RPCEndpoint),
			secondaryRpcClient: rpc.New(config.SecondaryRPCEndpoint),
			keypair:            privateKey,
			transactionSender:  make(chan *solana.Transaction, 100),
			positions:          NewPositionStore(),
		}

		go buyManager.processTransactions()
		go processBuySignals()
	})
}

func (bm *BuyManager) processTransactions() {
	for tx := range bm.transactionSender {
		if bm.config.DryRun {
			log.Printf("[dry-run] transaction built but NOT submitted")
			continue
		}

		opts := rpc.TransactionOpts{
			SkipPreflight: true,
		}

		sig, err := bm.rpcClient.SendTransactionWithOpts(context.Background(), tx, opts)
		if err != nil {
			log.Printf("Failed to send transaction: %v", err)
		} else {
			log.Printf("Transaction sent. Signature: %s", sig)
		}
	}
}

func processBuySignals() {
	filteredBuySignalChan := make(chan BuySignal, 100)
	go FilterBuySignals(buyManager.config, BuySignalChan, filteredBuySignalChan)

	for signal := range filteredBuySignalChan {
		if err := ProcessBuySignal(buyManager.config, signal); err != nil {
			log.Printf("Error processing buy signal: %v", err)
		}
	}
}

func encodeDiscriminant(discriminator uint64) []byte {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, discriminator)
	return data
}

func encodeUint64(value uint64) []byte {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, value)
	return data
}

func (bm *BuyManager) createBuyTransaction(signal *BuySignal, numTokens uint64) (*solana.Transaction, error) {
	ctx := context.Background()
	payer := bm.keypair.PublicKey()

	globalAccount, _ := solana.PublicKeyFromBase58("4wTV1YmiEkRvAtNtsSGPtUrqRYQMe5SKy2uB4Jjaxnjf")
	feeRecipient, _ := solana.PublicKeyFromBase58("CebN5WGQ4jvEPvsVU4EoHEpgzq1VV7AbicfhtW4xC9iM")
	mint, _ := solana.PublicKeyFromBase58(signal.MintAccount)
	bondingCurve, _ := solana.PublicKeyFromBase58(signal.BondingCurve)
	associatedBondingCurve, _ := solana.PublicKeyFromBase58(signal.AssociatedBondingCurve)
	pumpProgram, _ := solana.PublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")
	eventAuthority, _ := solana.PublicKeyFromBase58("Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1")

	tokenProgramID := solana.TokenProgramID
	systemProgramID := solana.SystemProgramID
	sysVarRentPubkey := solana.SysVarRentPubkey

	log.Printf("Token Program ID: %s", tokenProgramID.String())

	ata, _, err := solana.FindAssociatedTokenAddress(payer, mint)
	if err != nil {
		log.Printf("Error finding associated token address: %v", err)
		return nil, fmt.Errorf("failed to find associated token address: %w", err)
	}
	log.Printf("Associated Token Address: %s", ata.String())

	discriminator := uint64(16927863322537952870)
	buyInstructionData := append(encodeDiscriminant(discriminator), encodeUint64(numTokens)...)
	buyInstructionData = append(buyInstructionData, encodeUint64(100000000)...)

	buyInstruction := solana.NewInstruction(
		pumpProgram,
		solana.AccountMetaSlice{
			solana.NewAccountMeta(globalAccount, false, false),
			solana.NewAccountMeta(feeRecipient, true, false),
			solana.NewAccountMeta(mint, false, false),
			solana.NewAccountMeta(bondingCurve, true, false),
			solana.NewAccountMeta(associatedBondingCurve, true, false),
			solana.NewAccountMeta(ata, true, false),
			solana.NewAccountMeta(payer, true, true),
			solana.NewAccountMeta(systemProgramID, false, false),
			solana.NewAccountMeta(tokenProgramID, false, false),
			solana.NewAccountMeta(sysVarRentPubkey, false, false),
			solana.NewAccountMeta(eventAuthority, false, false),
			solana.NewAccountMeta(pumpProgram, false, false),
		},
		buyInstructionData,
	)

	recent, err := bm.rpcClient.GetRecentBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		log.Printf("Error getting recent blockhash: %v", err)
		return nil, fmt.Errorf("failed to get recent blockhash: %w", err)
	}
	log.Printf("Recent Blockhash: %s", recent.Value.Blockhash.String())

	computeUnitPrice := computebudget.NewSetComputeUnitPriceInstruction(50020).Build()
	computeUnitLimit := computebudget.NewSetComputeUnitLimitInstruction(100000).Build()

	createAtaInstruction := associatedtokenaccount.NewCreateInstruction(
		payer,
		payer,
		mint,
	)

	ataInst, err := createAtaInstruction.ValidateAndBuild()
	if err != nil {
		log.Printf("Error validating and building ATA instruction: %v", err)
		return nil, fmt.Errorf("failed to validate and build ATA instruction: %w", err)
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			computeUnitPrice,
			computeUnitLimit,
			ataInst,
			buyInstruction,
		},
		recent.Value.Blockhash,
		solana.TransactionPayer(payer),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if payer.Equals(key) {
			return &bm.keypair
		}
		return nil
	})
	if err != nil {
		log.Printf("Error signing transaction: %v", err)
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}

func (bm *BuyManager) attemptConcurrentPurchases(signal *BuySignal, numAttempts int) error {
	var wg sync.WaitGroup
	numUniqueTransactions := 3

	for i := 0; i < numUniqueTransactions; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			numTokens := uint64(11000000 + i)

			for attempt := 1; attempt <= 4; attempt++ {
				tx, err := bm.createBuyTransaction(signal, numTokens)
				if err != nil {
					log.Printf("Failed to create transaction on attempt %d: %v", attempt, err)
					continue
				}

				bm.transactionSender <- tx
				log.Printf("Transaction sent successfully on attempt %d", attempt)
			}
		}(i)
	}

	wg.Wait()
	return nil
}

func (bm *BuyManager) processNewMint(signal BuySignal) error {
	if _, alreadyPurchased := bm.purchasedTokens.Load(signal.MintAccount); !alreadyPurchased {
		log.Printf("Attempting to purchase %s...", signal.MintAccount)
		if err := bm.attemptConcurrentPurchases(&signal, 50); err != nil {
			return fmt.Errorf("failed to attempt purchases: %w", err)
		}
		log.Printf("Purchase transactions sent for %s", signal.MintAccount)
		bm.purchasedTokens.Store(signal.MintAccount, true)

		// Track the position so the exit monitor and dashboard can manage it.
		entryPrice, _ := bm.priceForBondingCurve(signal.BondingCurve)
		bm.positions.Add(Position{
			Mint:                   signal.MintAccount,
			BondingCurve:           signal.BondingCurve,
			AssociatedBondingCurve: signal.AssociatedBondingCurve,
			TokensHeld:             positionTokenSize,
			EntryPriceLamports:     entryPrice,
			OpenedAt:               time.Now(),
		})
		RecordEvent(fmt.Sprintf("BUY %s", short(signal.MintAccount)))
	} else {
		log.Printf("Already purchased %s", signal.MintAccount)
	}
	return nil
}
