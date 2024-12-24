# Binance Redis Streamer

A high-performance Go application that streams real-time cryptocurrency trade data from Binance and stores it in Redis.

## Features

- Real-time streaming of Binance trade data
- Efficient Redis storage with configurable retention
- Automatic reconnection and error handling
- Metrics collection and monitoring
- Support for multiple trading pairs
- Redis data viewer utility

## Prerequisites

- Go 1.19 or higher
- Redis server
- Binance API access

## Installation

```bash
git clone <repository-url>
cd Orders
go mod download
```

## Configuration

The application uses environment variables for configuration. Create a `.env` file in the root directory:

```env
REDIS_URL=redis://localhost:6379/0
CUSTOM_REDIS_URL=  # Optional: Override Redis URL for development
```

## Usage

### Starting the Streamer

```bash
go run cmd/streamer/main.go
```

### Using the Redis Viewer

```bash
go run scripts/redis-viewer.go --cmd list  # List all keys
go run scripts/redis-viewer.go --cmd symbols  # List available symbols
go run scripts/redis-viewer.go --cmd latest --symbol btcusdt  # Get latest trade
go run scripts/redis-viewer.go --cmd history --symbol btcusdt  # Get trade history
```

## Project Structure

```
.
├── cmd/
│   └── streamer/          # Main application entry point
├── internal/
│   └── models/            # Internal data models
├── pkg/
│   ├── binance/          # Binance API client
│   ├── config/           # Configuration management
│   ├── metrics/          # Metrics collection
│   └── storage/          # Redis storage implementation
└── scripts/              # Utility scripts
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.