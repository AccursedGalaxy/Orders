# Binance Trade Data Streamer

[![Release](https://img.shields.io/github/v/release/AccursedGalaxy/Orders)](https://github.com/AccursedGalaxy/Orders/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/AccursedGalaxy/Orders)](https://goreportcard.com/report/github.com/AccursedGalaxy/Orders)
[![codecov](https://codecov.io/gh/AccursedGalaxy/Orders/branch/main/graph/badge.svg)](https://codecov.io/gh/AccursedGalaxy/Orders)
[![Go Version](https://img.shields.io/github/go-mod/go-version/AccursedGalaxy/Orders)](https://go.dev/)
[![License](https://img.shields.io/github/license/AccursedGalaxy/Orders)](LICENSE)

A high-performance, real-time cryptocurrency trade data streaming application that collects and processes trade data from Binance. Built with Go, it offers multi-layer storage, real-time processing, and interactive visualization capabilities.

## 🚀 Features

- **Real-time Data Streaming**
  - WebSocket connection to Binance API
  - Automatic reconnection and error handling
  - Configurable symbol selection and filtering

- **Multi-layer Storage**
  - Redis for high-speed recent data access
  - PostgreSQL for reliable historical data storage
  - Automatic data migration and cleanup

- **Advanced Processing**
  - Real-time trade aggregation into candles
  - Customizable time intervals (1m, 5m, 15m, etc.)
  - Volume-weighted average price (VWAP) calculation

- **Interactive CLI**
  - Real-time trade monitoring
  - Historical data analysis
  - Interactive price charts
  - Comprehensive statistics

## 📋 Prerequisites

- Go 1.21.5 or later
- Redis 7.0 or later
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

3. **Build the Binaries**
   ```bash
   make build
   ```

## ⚙️ Configuration

Create a `.env` file in the project root:

```env
# Redis Configuration
CUSTOM_REDIS_URL=redis://localhost:6379/0
REDIS_KEY_PREFIX=binance:

# PostgreSQL Configuration
DATABASE_URL=postgres://user:password@localhost:5432/dbname

# Binance API Configuration
MAX_SYMBOLS=10
MIN_DAILY_VOLUME=1000000
MAIN_SYMBOLS=BTCUSDT,ETHUSDT

# Application Settings
DEBUG=false
RETENTION_DAYS=7
CLEANUP_INTERVAL=1h
```

## 🚦 Usage

### Start the Streamer

```bash
./bin/streamer
```

### CLI Commands

#### Real-time Monitoring
```bash
# Watch multiple symbols
./bin/redis-viewer watch BTCUSDT ETHUSDT --interval 2

# View interactive chart
./bin/redis-viewer chart BTCUSDT --period 24h --port 8080
```

#### Data Analysis
```bash
# Get historical data
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
    C --> E[Processing Service]
    E --> F[PostgreSQL]
    D --> G[CLI Interface]
    F --> G
```

### Components

1. **Data Ingestion Service**
   - Handles WebSocket connections
   - Implements automatic reconnection
   - Manages connection pooling

2. **Processing Service**
   - Aggregates trade data
   - Calculates technical indicators
   - Manages data retention

3. **Storage Layer**
   - Redis for real-time data
   - PostgreSQL for historical data
   - Automatic data migration

## 📊 Performance

- Handles 1000+ trades per second
- Sub-millisecond data processing
- Efficient memory usage with configurable limits
- Automatic cleanup of old data

## 🧪 Testing

Run the test suite:

```bash
make test
```

Run with race detection:

```bash
make test-race
```

## 📈 Monitoring

The application exposes metrics for Prometheus:

- Trade processing latency
- WebSocket connection status
- Storage operation metrics
- System resource usage

Access metrics at: `http://localhost:2112/metrics`

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Run tests (`make test`)
4. Commit your changes (`git commit -m 'feat: add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

### Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` New features
- `fix:` Bug fixes
- `chore:` Maintenance tasks
- `docs:` Documentation updates
- `test:` Test updates
- `refactor:` Code refactoring
- `style:` Code style updates
- `perf:` Performance improvements

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [Binance API](https://binance-docs.github.io/apidocs/) for the WebSocket feed
- [Go Redis](https://redis.uptrace.dev/) for the Redis client
- [lib/pq](https://github.com/lib/pq) for PostgreSQL support
- [Cobra](https://github.com/spf13/cobra) for CLI interface

## 📞 Support

For support, please:
1. Check the [Issues](https://github.com/AccursedGalaxy/Orders/issues) page
2. Open a new issue if needed
3. Join our [Discord](https://discord.gg/your-invite) community
