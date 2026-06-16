package actions

import (
	"fmt"
	"log"
	"strings"
)

func FilterBuySignals(config BuyConfig, input <-chan BuySignal, output chan<- BuySignal) {
	for signal := range input {
		if ShouldProcessSignal(config, signal) {
			output <- signal
		} else {
			log.Printf("Skipping signal for %s due to missing required information", signal.MintAccount)
		}
	}
}

func ShouldProcessSignal(config BuyConfig, signal BuySignal) bool {
	websiteValid := config.EnableWebsite && signal.Website != "" && strings.HasPrefix(signal.Website, "http")
	twitterValid := config.EnableTwitter && signal.Twitter != "" && strings.HasPrefix(signal.Twitter, "https://x.com/")
	telegramValid := config.EnableTelegram && signal.Telegram != "" && strings.HasPrefix(signal.Telegram, "https://t.me/")

	/*
		log.Printf("Validating signal for mint account: %s", signal.MintAccount)
		log.Printf("Config: EnableWebsite: %v, EnableTwitter: %v, EnableTelegram: %v", config.EnableWebsite, config.EnableTwitter, config.EnableTelegram)
		log.Printf("Signal data: Website: '%s', Twitter: '%s', Telegram: '%s'", signal.Website, signal.Twitter, signal.Telegram)
		log.Printf("Validation results: Website: %v, Twitter: %v, Telegram: %v", websiteValid, twitterValid, telegramValid)
	*/
	// Require all valid
	shouldProcess := (websiteValid && twitterValid && telegramValid) && (config.EnableWebsite && config.EnableTwitter && config.EnableTelegram)
	log.Printf("Should process signal: %v", shouldProcess)

	return shouldProcess
}

func ProcessBuySignal(config BuyConfig, signal BuySignal) error {
	if !ShouldProcessSignal(config, signal) {
		log.Printf("Skipping purchase for %s due to failed validation", signal.MintAccount)
		return nil
	}

	// Proceed with the purchase logic
	if _, alreadyPurchased := buyManager.purchasedTokens.Load(signal.MintAccount); !alreadyPurchased {
		log.Printf("Attempting to purchase %s...", signal.MintAccount)
		if err := buyManager.processNewMint(signal); err != nil {
			return fmt.Errorf("failed to process new mint: %w", err)
		}
		log.Printf("Purchase transactions sent for %s", signal.MintAccount)
		buyManager.purchasedTokens.Store(signal.MintAccount, true)
	} else {
		log.Printf("Already purchased %s", signal.MintAccount)
	}
	return nil
}
