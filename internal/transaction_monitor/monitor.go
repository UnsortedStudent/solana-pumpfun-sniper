package transaction_monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mr-tron/base58"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"zednine/internal/actions"
	pb "zednine/proto" // Make sure this import path is correct for your project
)

const programAddress = "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"

// GetRPCURL returns the Geyser gRPC endpoint (host:port), configured via the
// GEYSER_GRPC_URL environment variable. Most public RPCs do not expose Geyser;
// a low-latency provider does.
func GetRPCURL() string {
	if url := os.Getenv("GEYSER_GRPC_URL"); url != "" {
		return url
	}
	return "127.0.0.1:10000"
}

func MonitorTransactions() {
	for {
		if err := connectAndMonitor(); err != nil {
			log.Printf("Error: %v. Reconnecting...", err)
		}
	}
}

func connectAndMonitor() error {
	rpcURL := GetRPCURL()

	// Use grpc.NewClient to establish a connection
	conn, err := grpc.NewClient(rpcURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to create gRPC client: %v", err)
	}
	defer conn.Close()

	client := pb.NewGeyserClient(conn)
	log.Println("Successfully connected to gRPC server")

	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	defer monitorCancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- SubscribeToProgramUpdates(monitorCtx, client)
	}()

	go func() {
		for {
			state := conn.GetState()
			if state == connectivity.TransientFailure || state == connectivity.Shutdown {
				log.Printf("Connection state changed to %s, canceling context...", state)
				monitorCancel()
				return
			}
			time.Sleep(time.Second)
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-monitorCtx.Done():
		return monitorCtx.Err()
	}
}

func SubscribeToProgramUpdates(ctx context.Context, client pb.GeyserClient) error {
	falseValue := false
	commitment := pb.CommitmentLevel_PROCESSED

	memcmpFilter := &pb.SubscribeRequestFilterAccountsFilter{
		Filter: &pb.SubscribeRequestFilterAccountsFilter_Memcmp{
			Memcmp: &pb.SubscribeRequestFilterAccountsFilterMemcmp{
				Offset: 0,
				Data: &pb.SubscribeRequestFilterAccountsFilterMemcmp_Base58{
					Base58: "2zt6UC",
				},
			},
		},
	}

	subscribeRequest := &pb.SubscribeRequest{
		Transactions: map[string]*pb.SubscribeRequestFilterTransactions{
			"program_transactions": {
				AccountRequired: []string{programAddress},
				Vote:            &falseValue,
				Failed:          &falseValue,
			},
		},
		Accounts: map[string]*pb.SubscribeRequestFilterAccounts{
			"inner_instruction_accounts": {
				Filters: []*pb.SubscribeRequestFilterAccountsFilter{memcmpFilter},
			},
		},
		Commitment: &commitment,
	}

	stream, err := client.Subscribe(ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe: %v", err)
	}

	err = stream.Send(subscribeRequest)
	if err != nil {
		return fmt.Errorf("failed to send subscribe request: %v", err)
	}

	log.Println("Successfully subscribed, waiting for updates...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			update, err := stream.Recv()
			if err != nil {
				return fmt.Errorf("subscription error: %v", err)
			}

			processUpdate(update)
		}
	}
}

func processUpdate(update *pb.SubscribeUpdate) {
	startTime := time.Now()

	if txnUpdate, ok := update.UpdateOneof.(*pb.SubscribeUpdate_Transaction); ok {
		if txnUpdate.Transaction != nil && txnUpdate.Transaction.Transaction != nil {
			transactionInfo := txnUpdate.Transaction.Transaction

			if transactionInfo.Transaction != nil &&
				transactionInfo.Transaction.Message != nil &&
				transactionInfo.Meta != nil &&
				transactionInfo.Meta.InnerInstructions != nil {

				message := transactionInfo.Transaction.Message
				accountKeys := message.AccountKeys

				for _, innerInstrSet := range transactionInfo.Meta.InnerInstructions {
					if len(innerInstrSet.Instructions) > 8 {
						instr8 := innerInstrSet.Instructions[8]

						if len(instr8.Accounts) == 6 {
							txHash := base58.Encode(transactionInfo.Signature)
							timeDetected := time.Now().Format("2006-01-02T15:04:05.9999-07:00")
							if len(accountKeys) > 5 {
								userAccount := base58.Encode(accountKeys[0])
								mintAccount := base58.Encode(accountKeys[1])
								bondingCurve := base58.Encode(accountKeys[3])
								associatedBondingCurve := base58.Encode(accountKeys[4])
								metadata := base58.Encode(accountKeys[5])

								decodedData := base58.Encode(instr8.Data)
								decodedBytes, err := base58.Decode(decodedData)
								if err == nil {
									decodedStr := string(decodedBytes)
									ipfsHash := extractIPFSHash(decodedStr)
									if ipfsHash != "" {
										ipfsStartTime := time.Now()
										content, err := resolveIPFSContent(ipfsHash)
										ipfsProcessingTime := time.Since(ipfsStartTime)

										if err == nil {
											jsonContent, err := decodeIPFSContent(content)
											if err == nil {
												var ipfsData map[string]interface{}
												if err := json.Unmarshal([]byte(jsonContent), &ipfsData); err == nil {
													// Create buy signal
													buySignal := actions.BuySignal{
														TransactionHash:        txHash,
														MintAccount:            mintAccount,
														BondingCurve:           bondingCurve,
														AssociatedBondingCurve: associatedBondingCurve,
														Metadata:               metadata,
														TimeDetected:           timeDetected,
														IPFSProcessingTime:     ipfsProcessingTime,
														Name:                   getString(ipfsData, "name"),
														Symbol:                 getString(ipfsData, "symbol"),
														Description:            getString(ipfsData, "description"),
														Twitter:                getString(ipfsData, "twitter"),
														Telegram:               getString(ipfsData, "telegram"),
														Website:                getString(ipfsData, "website"),
													}

													// Log information locally
													totalDuration := time.Since(startTime)
													fmt.Printf("Received buy signal at %s:\n", timeDetected)
													fmt.Printf("  Transaction Hash: %s\n", txHash)
													fmt.Printf("  User: %s\n", userAccount)
													fmt.Printf("  Mint Account: %s\n", mintAccount)
													fmt.Printf("  Bonding Curve: %s\n", bondingCurve)
													fmt.Printf("  Associated Bonding Curve: %s\n", associatedBondingCurve)
													fmt.Printf("  Metadata: %s\n", metadata)
													fmt.Printf("  Time Detected: %s\n", timeDetected)
													fmt.Printf("  Name: %s\n", buySignal.Name)
													fmt.Printf("  Symbol: %s\n", buySignal.Symbol)
													fmt.Printf("  Description: %s\n", buySignal.Description)
													fmt.Printf("  Twitter: %s\n", buySignal.Twitter)
													fmt.Printf("  Telegram: %s\n", buySignal.Telegram)
													fmt.Printf("  Website: %s\n", buySignal.Website)
													fmt.Printf("  IPFS Processing Time: %v\n", ipfsProcessingTime)
													fmt.Printf("Total processing time: %d ms\n", totalDuration.Milliseconds())
													fmt.Println("------------------------")

													// Send signal to buy module (only once)
													select {
													case actions.BuySignalChan <- buySignal:
														// Signal sent successfully
													default:
														// Channel is full, log this event
														fmt.Println("Warning: BuySignalChan is full, signal not sent")
													}
												}
											}
										}
									}
								}
							}
							return
						}
					}
				}
			}
		}
	}
}

func getString(data map[string]interface{}, key string) string {
	if value, ok := data[key].(string); ok {
		return value
	}
	return ""
}
