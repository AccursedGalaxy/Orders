# Binance Trade Streamer

A Go application that streams trade data from Binance, stores recent data in Redis, and archives historical data in PostgreSQL with time-series optimization.

## Features

- Real-time trade streaming for major cryptocurrency pairs
- Redis-based hot storage for recent trades (2 hours)
- PostgreSQL/TimescaleDB for historical data storage
- Automatic data aggregation into 1-minute candles
- Configurable symbol selection and data retention
- Metrics collection and monitoring

## Requirements

- Go 1.21 or later
- Redis 6.x or later
- PostgreSQL 12.x or later with TimescaleDB extension
- Heroku CLI (for deployment)

## Local Setup

1. Clone the repository:
```bash
git clone https://github.com/yourusername/binance-redis-streamer.git
cd binance-redis-streamer
```

2. Install dependencies:
```bash
go mod download
```

3. Copy the environment template:
```bash
cp .env.example .env
```

4. Set up local databases:
```bash
# Start Redis
docker run -d -p 6379:6379 redis:6

# Start PostgreSQL with TimescaleDB
docker run -d -p 5432:5432 \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=binance_trades \
  timescale/timescaledb:latest-pg12
```

5. Build and run:
```bash
go build -o bin/streamer cmd/streamer/main.go
./bin/streamer
```

## Heroku Deployment

1. Create a new Heroku app:
```bash
heroku create your-app-name
```

2. Add required add-ons:
```bash
heroku addons:create heroku-redis:hobby-dev
heroku addons:create timescale
```

3. Configure environment variables:
```bash
heroku config:set MAX_SYMBOLS=3
heroku config:set RETENTION_DAYS=90
```

4. Deploy:
```bash
git push heroku main
```

5. Start the worker:
```bash
heroku ps:scale worker=1
```

## Configuration

The application can be configured through environment variables:

- `REDIS_URL`: Redis connection URL (set by Heroku Redis)
- `DATABASE_URL`: PostgreSQL connection URL (set by TimescaleDB)
- `MAX_SYMBOLS`: Maximum number of symbols to track (default: 3)
- `RETENTION_DAYS`: Days to keep historical data (default: 90)
- `LOG_LEVEL`: Logging level (default: info)

## Architecture

1. **Data Flow**:
   - Binance WebSocket → Redis (recent trades)
   - Redis → PostgreSQL (historical aggregation)
   - PostgreSQL TimescaleDB (time-series storage)

2. **Storage Strategy**:
   - Redis: Last 2 hours of raw trade data
   - PostgreSQL: 1-minute candles with 90-day retention
   - TimescaleDB: Automatic partitioning and optimization

## Monitoring

The application exports metrics for:
- Trade volume per symbol
- Trade count per minute
- Storage usage
- Connection status

## License

MIT License
