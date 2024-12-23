# Binance Futures Trade Streamer

This application streams aggregated trades from Binance USDS-Margined Futures WebSocket and stores them in Redis in real-time.

## Prerequisites

- Go 1.20 or later
- Redis server running on localhost:6379
- Internet connection to access Binance API

## Installation

1. Install Go:
```bash
# Ubuntu/Debian
sudo apt install golang-go

# Or download from https://golang.org/dl/
```

2. Install Redis:
```bash
sudo apt install redis-server
```

3. Clone the repository and install dependencies:
```bash
git clone <repository-url>
cd binance-redis-streamer
go mod download
```

## Usage

1. Make sure Redis is running:
```bash
sudo systemctl start redis
```

2. Run the application:
```bash
go run main.go
```

The application will:
- Fetch available symbols from Binance Futures
- Connect to WebSocket stream for the first 3 symbols (configurable in code)
- Store trade data in Redis in two formats:
  - Latest trade data in hash: `binance:aggTrade:<symbol>`
  - Historical trades in list: `binance:aggTrade:history:<symbol>` (last 1000 trades)

## Redis Data Structure

1. Latest trade (Hash):
   - Key: `binance:aggTrade:<symbol>`
   - Fields:
     - price: Latest trade price
     - quantity: Trade quantity
     - tradeId: Binance trade ID
     - time: Trade timestamp

2. Trade history (List):
   - Key: `binance:aggTrade:history:<symbol>`
   - Values: Raw JSON messages of last 1000 trades

## Monitoring

You can monitor the stored data using Redis CLI:

```bash
# Get latest trade for a symbol
redis-cli HGETALL binance:aggTrade:btcusdt

# Get last 10 trades from history
redis-cli LRANGE binance:aggTrade:history:btcusdt 0 9
```