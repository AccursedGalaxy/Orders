package models

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