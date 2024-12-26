# Binance Trade Data Streamer

[![Release](https://img.shields.io/github/v/release/AccursedGalaxy/Orders)](https://github.com/AccursedGalaxy/Orders/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/AccursedGalaxy/Orders)](https://goreportcard.com/report/github.com/AccursedGalaxy/Orders)
[![codecov](https://codecov.io/gh/AccursedGalaxy/Orders/branch/main/graph/badge.svg)](https://codecov.io/gh/AccursedGalaxy/Orders)
[![Go Version](https://img.shields.io/github/go-mod/go-version/AccursedGalaxy/Orders)](https://go.dev/)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

A high-performance, real-time cryptocurrency trade data streaming application that collects and processes trade data from Binance. Built with Go, it features a hybrid storage architecture combining Redis for real-time data and PostgreSQL for historical storage, offering both speed and reliability.

## 🚀 Features

- **Optimized Real-time Data Streaming**
  - WebSocket connection to Binance API with automatic reconnection
  - Smart symbol filtering based on volume and importance
  - Configurable trade data compression and retention
  - Memory-optimized storage with automatic cleanup

- **Hybrid Storage Architecture**
  - Redis for high-speed recent data (30-minute window)
  - PostgreSQL for unlimited historical data storage
  - Automatic data migration between layers
  - Efficient data compression and cleanup

- **Advanced Processing**
  - Real-time trade aggregation into 1-minute candles
  - Volume-weighted average price (VWAP) calculation
  - Order book imbalance tracking
  - Trade momentum analysis

- **Interactive CLI Tools**
  - Real-time trade monitoring with customizable intervals
  - Interactive price charts with technical indicators
  - Historical data analysis and export
  - Performance metrics and system monitoring

## 📋 Prerequisites

- Go 1.21.5 or later
- Redis 7.0 or later (with at least 2GB memory)
- PostgreSQL 16.0 or later
- Make (for build automation)

## 🛠 Installation

1. **Clone the Repository**
   ```bash
   git clone https://github.com/AccursedGalaxy/Orders.git
   cd Orders
   ```

2. **Install Dependencies**
   ```bash
   go mod download
   ```

3. **Build the Project**
   ```bash
   make build
   ```

## ⚙️ Configuration

Create a `.env` file in the project root:

```env
# Redis Configuration (Required)
CUSTOM_REDIS_URL=redis://localhost:6379/0
# For Heroku Redis (Optional)
REDIS_URL=rediss://...

# PostgreSQL Configuration (Required)
DATABASE_URL=postgres://user:password@localhost:5432/binance_trades

# Application Settings
DEBUG=false
```

### Advanced Configuration

The application includes smart defaults optimized for both performance and resource usage:

- Redis retention period: 30 minutes
- Cleanup interval: 1 minute
- Max trades per symbol: 1,000
- Data compression: Enabled
- Max tracked symbols: 5
- Min daily volume: 10M USDT

## 🚀 Deployment

### Local Development
```bash
# Start all services
docker-compose up -d

# Run the streamer
./bin/streamer

# Monitor trades
./bin/redis-viewer watch BTCUSDT ETHUSDT
```

### Heroku Deployment
```bash
# Deploy to Heroku
heroku create
git push heroku main

# Add required add-ons
heroku addons:create heroku-redis:premium-0
heroku addons:create heroku-postgresql:standard-0
```

## 🎯 Usage

### Real-time Monitoring
```bash
# Watch live trades with 2-second updates
./bin/redis-viewer watch BTCUSDT ETHUSDT --interval 2

# View interactive chart
./bin/redis-viewer chart BTCUSDT --period 24h --port 8080
```

### Historical Analysis
```bash
# Get 7-day historical data in 5-minute candles
./bin/redis-viewer history BTCUSDT --period 7d --interval 5m

# Export to CSV
./bin/redis-viewer history BTCUSDT --format csv > btc_history.csv
```

## 🏗 Architecture

```mermaid
graph TD
    A[Binance WebSocket] --> B[Data Ingestion Service]
    B --> C[Message Bus]
    C --> D[Redis Cache]
    C --> E[Trade Aggregator]
    E --> F[PostgreSQL]
    D --> G[CLI Interface]
    F --> G
    E --> H[Metrics Exporter]
    H --> I[Prometheus/Grafana]
```

### Key Components

1. **Data Ingestion Service** (`pkg/ingestion`)
   - Manages WebSocket connections and reconnection logic
   - Filters and validates incoming trade data
   - Implements rate limiting and backoff strategies

2. **Trade Aggregator** (`pkg/storage`)
   - Aggregates trades into 1-minute candles
   - Manages data migration between Redis and PostgreSQL
   - Handles data compression and cleanup

3. **Storage Layer**
   - Redis: 30-minute hot data window with compression
   - PostgreSQL: Unlimited historical data storage
   - Automatic cleanup and optimization

4. **CLI Interface** (`pkg/cli`)
   - Real-time monitoring tools
   - Historical data analysis
   - Interactive charting

## 📊 Performance Optimization

The system is optimized for both performance and resource usage:

- **Memory Management**
  - Configurable Redis memory limits
  - Automatic data compression
  - Smart symbol filtering
  - Regular cleanup of old data

- **Processing Efficiency**
  - Batch processing of trades
  - Optimized data structures
  - Efficient PostgreSQL queries
  - Connection pooling

## 🧪 Testing

```bash
# Run all tests
make test

# Run with race detection
make test-race

# Run specific package tests
go test ./pkg/storage/...
```

## 📈 Monitoring

Access metrics at: `http://localhost:2112/metrics`

Available metrics:
- Trade processing latency
- Memory usage
- Storage operations
- WebSocket connection status

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Run tests (`make test`)
4. Commit changes (`git commit -m 'feat: add amazing feature'`)
5. Push to branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

## 📄 License

Licensed under GPLv3 - see [LICENSE.md](LICENSE.md)

## 🙏 Acknowledgments

- [Binance API](https://binance-docs.github.io/apidocs/)
- [Go Redis](https://redis.uptrace.dev/)
- [lib/pq](https://github.com/lib/pq)
- [Cobra](https://github.com/spf13/cobra)

## 📞 Support

- [Issues](https://github.com/AccursedGalaxy/Orders/issues)
- [Discord Community](https://discord.gg/7vY9ZBPdya)
- [Documentation Wiki](https://github.com/AccursedGalaxy/Orders/wiki)
