package models

import (
	"encoding/json"
	"strconv"
	"time"
)

// Symbol represents a trading symbol
type Symbol struct {
	Symbol string `json:"symbol"`
	Status string `json:"status"`
}

// ExchangeInfo represents the exchange information response
type ExchangeInfo struct {
	Symbols []Symbol `json:"symbols"`
}

// AggTradeEvent represents an aggregated trade event from WebSocket
type AggTradeEvent struct {
	Stream string    `json:"stream"`
	Data   TradeData `json:"data"`
	Raw    []byte    `json:"-"` // Raw message data
}

// UnmarshalJSON implements custom JSON unmarshaling for AggTradeEvent
func (e *AggTradeEvent) UnmarshalJSON(data []byte) error {
	type Alias AggTradeEvent
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	e.Raw = data
	return nil
}

// TradeData represents the actual trade data
type TradeData struct {
	EventType    string `json:"e"`
	EventTime    int64  `json:"E"`
	Symbol       string `json:"s"`
	Price        string `json:"p"`
	Quantity     string `json:"q"`
	TradeID      int64  `json:"a"`
	FirstID      int64  `json:"f"`
	LastID       int64  `json:"l"`
	TradeTime    int64  `json:"T"`
	IsBuyerMaker bool   `json:"m"`
}

// Trade represents a processed trade ready for storage
type Trade struct {
	Symbol    string
	Price     string
	Quantity  string
	TradeID   int64
	Time      int64
	EventTime int64
}

// ToTrade converts an AggTradeEvent to a Trade
func (e *AggTradeEvent) ToTrade() *Trade {
	return &Trade{
		Symbol:    e.Data.Symbol,
		Price:     e.Data.Price,
		Quantity:  e.Data.Quantity,
		TradeID:   e.Data.TradeID,
		Time:      e.Data.TradeTime,
		EventTime: e.Data.EventTime,
	}
}

// Candle represents aggregated trade data for a time period
type Candle struct {
	Timestamp   time.Time
	OpenPrice   string
	HighPrice   string
	LowPrice    string
	ClosePrice  string
	Volume      string
	TradeCount  int64
}

// NewCandle creates a new candle for a given timestamp
func NewCandle(timestamp time.Time) *Candle {
	return &Candle{
		Timestamp:   timestamp,
		OpenPrice:   "",
		HighPrice:   "",
		LowPrice:    "",
		ClosePrice:  "",
		Volume:      "0",
		TradeCount:  0,
	}
}

// UpdateFromTrade updates the candle with data from a new trade
func (c *Candle) UpdateFromTrade(trade *Trade) {
	if c.OpenPrice == "" {
		c.OpenPrice = trade.Price
	}
	if c.HighPrice == "" || trade.Price > c.HighPrice {
		c.HighPrice = trade.Price
	}
	if c.LowPrice == "" || trade.Price < c.LowPrice {
		c.LowPrice = trade.Price
	}
	c.ClosePrice = trade.Price

	// Update volume
	currentVolume, _ := strconv.ParseFloat(c.Volume, 64)
	tradeVolume, _ := strconv.ParseFloat(trade.Quantity, 64)
	newVolume := currentVolume + tradeVolume
	c.Volume = strconv.FormatFloat(newVolume, 'f', -1, 64)

	c.TradeCount++
}

// ToTrade converts TradeData to Trade
func (td *TradeData) ToTrade() *Trade {
	return &Trade{
		Symbol:    td.Symbol,
		Price:     td.Price,
		Quantity:  td.Quantity,
		TradeID:   td.TradeID,
		Time:      td.TradeTime,
		EventTime: td.EventTime,
	}
} 