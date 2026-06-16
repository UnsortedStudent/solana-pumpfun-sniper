// Package feed streams real pump.fun activity from PumpPortal's free WebSocket API
// (https://pumpportal.fun) into the dashboard's stores. No wallet or API key needed,
// which makes it the default, zero-setup data source.
package feed

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"zednine/internal/actions"
)

const wsURL = "wss://pumpportal.fun/api/data"

type client struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

var c *client

// event is the subset of PumpPortal messages we care about. New-token messages have
// txType "create"; trade messages have "buy"/"sell". All carry marketCapSol.
type event struct {
	TxType       string  `json:"txType"`
	Mint         string  `json:"mint"`
	Name         string  `json:"name"`
	Symbol       string  `json:"symbol"`
	MarketCapSol float64 `json:"marketCapSol"`
}

// Start connects to PumpPortal, subscribes to new-token launches, wires the trade
// (un)subscribe hooks, and pumps events into the actions stores. Reconnects on drop.
func Start() error {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	c = &client{conn: conn}
	c.write(map[string]any{"method": "subscribeNewToken"})

	actions.SubscribeTrades = func(mint string) {
		c.write(map[string]any{"method": "subscribeTokenTrade", "keys": []string{mint}})
	}
	actions.UnsubscribeTrades = func(mint string) {
		c.write(map[string]any{"method": "unsubscribeTokenTrade", "keys": []string{mint}})
	}

	go c.readLoop()
	return nil
}

func (cl *client) write(v any) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.conn != nil {
		_ = cl.conn.WriteJSON(v)
	}
}

func (cl *client) readLoop() {
	for {
		cl.mu.Lock()
		conn := cl.conn
		cl.mu.Unlock()

		_, data, err := conn.ReadMessage()
		if err != nil {
			time.Sleep(2 * time.Second)
			newConn, _, derr := websocket.DefaultDialer.Dial(wsURL, nil)
			if derr != nil {
				continue
			}
			cl.mu.Lock()
			cl.conn = newConn
			cl.mu.Unlock()
			cl.write(map[string]any{"method": "subscribeNewToken"})
			continue
		}

		var e event
		if json.Unmarshal(data, &e) != nil || e.Mint == "" {
			continue
		}
		switch e.TxType {
		case "create":
			actions.RecordLaunch(actions.Launch{
				Mint: e.Mint, Name: e.Name, Symbol: e.Symbol,
				MarketCapSol: e.MarketCapSol, At: time.Now(),
			})
		case "buy", "sell":
			actions.UpdateMarketCap(e.Mint, e.MarketCapSol)
		}
	}
}
