package models

import (
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
	Stream string     `json:"stream"`
	Data   TradeData `json:"data"`
}

// TradeData represents the actual trade data
type TradeData struct {
	EventType string `json:"e"`
	EventTime int64  `json:"E"`
	Symbol    string `json:"s"`
	Price     string `json:"p"`
	Quantity  string `json:"q"`
	TradeID   int64  `json:"a"`
	FirstID   int64  `json:"f"`
	LastID    int64  `json:"l"`
	TradeTime int64  `json:"T"`
	IsBuyerMaker bool `json:"m"`
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

// NewCandle creates a new candle from a trade
func NewCandle(trade *Trade) *Candle {
	return &Candle{
		Timestamp:   time.Unix(0, trade.Time),
		OpenPrice:   trade.Price,
		HighPrice:   trade.Price,
		LowPrice:    trade.Price,
		ClosePrice:  trade.Price,
		Volume:      trade.Quantity,
		TradeCount:  1,
	}
}

// UpdateCandle updates a candle with new trade data
func (c *Candle) UpdateCandle(trade *Trade) {
	price, _ := strconv.ParseFloat(trade.Price, 64)
	high, _ := strconv.ParseFloat(c.HighPrice, 64)
	low, _ := strconv.ParseFloat(c.LowPrice, 64)
	volume, _ := strconv.ParseFloat(trade.Quantity, 64)
	existingVolume, _ := strconv.ParseFloat(c.Volume, 64)

	if price > high {
		c.HighPrice = trade.Price
	}
	if price < low {
		c.LowPrice = trade.Price
	}
	c.ClosePrice = trade.Price
	c.Volume = strconv.FormatFloat(volume + existingVolume, 'f', 8, 64)
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