package models

import (
	"testing"
	"time"
)

func TestNewCandle(t *testing.T) {
	timestamp := time.Now().Truncate(time.Minute)
	trade := &Trade{
		Symbol:    "BTCUSDT",
		Price:     "50000.00",
		Quantity:  "1.5",
		TradeID:   12345,
		Time:      timestamp,
		EventTime: timestamp,
	}

	candle := NewCandle(timestamp)
	candle.UpdateFromTrade(trade)

	tests := []struct {
		name  string
		field string

		got      string
		expected string
	}{
		{"Price", "OpenPrice", candle.OpenPrice, "50000.00"},
		{"High", "HighPrice", candle.HighPrice, "50000.00"},
		{"Low", "LowPrice", candle.LowPrice, "50000.00"},
		{"Close", "ClosePrice", candle.ClosePrice, "50000.00"},
		{"Volume", "Volume", candle.Volume, "1.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.field, tt.got, tt.expected)
			}
		})
	}

	if candle.TradeCount != 1 {
		t.Errorf("TradeCount = %v, want 1", candle.TradeCount)
	}
}

func TestCandleUpdate(t *testing.T) {
	timestamp := time.Now().Truncate(time.Minute)
	candle := NewCandle(timestamp)

	trades := []*Trade{
		{
			Symbol:    "BTCUSDT",
			Price:     "50000.00",
			Quantity:  "1.0",
			TradeID:   1,
			Time:      timestamp,
			EventTime: timestamp,
		},
		{
			Symbol:    "BTCUSDT",
			Price:     "51000.00",
			Quantity:  "2.0",
			TradeID:   2,
			Time:      timestamp,
			EventTime: timestamp,
		},
		{
			Symbol:    "BTCUSDT",
			Price:     "49000.00",
			Quantity:  "1.5",
			TradeID:   3,
			Time:      timestamp,
			EventTime: timestamp,
		},
	}

	for _, trade := range trades {
		candle.UpdateFromTrade(trade)
	}

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"OpenPrice", candle.OpenPrice, "50000.00"},
		{"HighPrice", candle.HighPrice, "51000.00"},
		{"LowPrice", candle.LowPrice, "49000.00"},
		{"ClosePrice", candle.ClosePrice, "49000.00"},
		{"Volume", candle.Volume, "4.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}

	if candle.TradeCount != 3 {
		t.Errorf("TradeCount = %v, want 3", candle.TradeCount)
	}
}
