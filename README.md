# Binance Trade Data Streamer

A real-time cryptocurrency trade data streaming application that collects and processes trade data from Binance.

## Features

- Real-time trade data streaming from Binance WebSocket API
- Multi-layer storage (Redis for recent data, PostgreSQL for historical)
- Trade data aggregation into candles (1-minute intervals)
- Interactive CLI interface for data exploration
- Real-time and historical data visualization
- Configurable symbol selection and filtering

## Installation

1. Clone the repository:
```bash
git clone https://github.com/yourusername/binance-redis-streamer.git
cd binance-redis-streamer
```

2. Install dependencies:
```bash
go mod download
```

3. Build the application:
```bash
go build -o bin/streamer cmd/streamer/main.go
go build -o bin/binance-cli cmd/cli/main.go
```

## Configuration

The application can be configured using environment variables or a `.env` file:

```env
REDIS_URL=redis://localhost:6379/0
DATABASE_URL=postgres://user:password@localhost:5432/dbname
MAX_SYMBOLS=10
RETENTION_DAYS=7
```

## Usage

### Start the Streamer

```bash
./bin/streamer
```

### CLI Commands

1. Watch real-time trade data:
```bash
binance-cli watch BTCUSDT ETHUSDT
binance-cli watch --interval 2  # Update every 2 seconds
```

2. View trade statistics:
```bash
binance-cli stats BTCUSDT --period 1h
binance-cli stats --period 24h  # All symbols
```

3. View interactive charts:
```bash
binance-cli chart BTCUSDT --period 24h
binance-cli chart ETHUSDT --period 7d --port 8081
```

4. View historical data:
```bash
binance-cli history BTCUSDT --period 24h --interval 5m
binance-cli history BTCUSDT --format csv > btc_history.csv
```

5. List available trading pairs:
```bash
binance-cli symbols
binance-cli symbols --format json
```

## CLI Options

### Global Options
- `--help`: Show help for any command

### Watch Command
- `-i, --interval`: Update interval in seconds (default: 1)

### Stats Command
- `-p, --period`: Time period (e.g., 1h, 24h, 7d)

### Chart Command
- `-p, --period`: Time period (e.g., 1h, 24h, 7d)
- `--port`: Port for web interface (default: 8080)

### History Command
- `-p, --period`: Time period (e.g., 1h, 24h, 7d)
- `-i, --interval`: Time interval (e.g., 1m, 5m, 1h)
- `-l, --limit`: Limit number of results
- `-f, --format`: Output format (table or csv)

### Symbols Command
- `-f, --format`: Output format (table, simple, or json)

## Architecture

The application consists of several components:

1. **Data Ingestion**
   - Connects to Binance WebSocket API
   - Processes real-time trade events
   - Publishes to message bus

2. **Storage Layer**
   - Redis for recent trade data
   - PostgreSQL for historical data
   - Automatic data migration

3. **Processing Layer**
   - Aggregates trades into candles
   - Calculates statistics
   - Manages data retention

4. **CLI Interface**
   - Real-time data monitoring
   - Historical data analysis
   - Interactive visualizations

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
