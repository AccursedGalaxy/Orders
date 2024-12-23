# Orders

## Goal
Use Binance Websockets To Continuously Stream All Live Aggregate Trades Into A Redis Database.
-> Build in a way where we can quite easily integrate other Live Order Streams If Needed.

## System Design

### Architecture Flow
1. **WebSocket Client**
   - Connects to Binance WebSocket API
   - Subscribes to aggregate trade streams (`<symbol>@aggTrade`) for all trading pairs
   - Handles reconnection and error scenarios
   - Validates incoming data
   - Handles 100ms update frequency per symbol

2. **Data Processor**
   - Normalizes trade data into standard format
   - Handles trade aggregation (100ms window)
   - Filters out non-market trades (insurance fund trades and ADL trades)
   - Prepares data for storage

3. **Redis Storage**
   - Real-time trade storage
   - Historical aggregations
   - Performance metrics

### Redis Schema

#### 1. Live Trades
Key pattern and example:

    # Latest trades (sorted set)
    key: trades:{symbol}:latest
    value: {
        "event_type": "aggTrade",
        "event_time": 123456789,
        "symbol": "BTCUSDT",
        "agg_trade_id": "5933014",
        "price": "0.001",
        "quantity": "100",
        "first_trade_id": 100,
        "last_trade_id": 105,
        "trade_time": 123456785,
        "is_buyer_maker": true
    }
    score: {trade_time}

#### 2. Trade Statistics (per symbol)
Key pattern and example:

    # Hash storing current statistics (100ms aggregation window)
    key: stats:{symbol}:current
    fields:
        last_update_time: 123456789
        volume: "100.50"
        trades_count: "150"
        high_price: "0.002"
        low_price: "0.001"
        first_trade_id: "100"
        last_trade_id: "105"
        is_buyer_maker_count: "75"  # Number of maker buys in window

    # Symbol metadata hash
    key: symbol:{symbol}:info
    fields:
        base_asset: "BTC"
        quote_asset: "USDT"
        status: "TRADING"
        last_trade_time: 123456785

## Implementation Details

### Data Retention
- Live trades: 24 hours rolling window
- Statistics: Keep 100ms, 1m, 5m, 15m, 1h, 4h, 1d aggregations
- Raw trade data archived to cold storage (optional)

### Performance Considerations
- Use Redis Streams for high-throughput ingestion (100ms updates per symbol)
- Implement batch processing for statistics updates
- Use pipelining for multiple Redis operations
- Consider Redis cluster for scaling
- Plan for high write throughput due to 100ms update frequency

### Monitoring
- WebSocket connection status
- Trade ingestion rate (expect updates every 100ms per symbol)
- Redis memory usage
- Processing latency (critical to handle 100ms updates)
- Aggregation window accuracy
- Missing trades detection

## Example Data

Aggregate Trade Streams Endpoint Response (`<symbol>@aggTrade`):

    {
      "e": "aggTrade",  // Event type
      "E": 123456789,   // Event time
      "s": "BTCUSDT",   // Symbol
      "a": 5933014,     // Aggregate trade ID
      "p": "0.001",     // Price
      "q": "100",       // Quantity
      "f": 100,         // First trade ID
      "l": 105,         // Last trade ID
      "T": 123456785,   // Trade time
      "m": true         // Is the buyer the market maker?
    }

This design provides several advantages:

1. **Scalability**: The schema can handle multiple trading pairs with 100ms updates.
2. **Accuracy**: Properly handles trade aggregation windows as per Binance specs.
3. **Performance**: Optimized for high-frequency updates and real-time access.
4. **Monitoring**: Built-in ways to track system health and data accuracy.
5. **Historical Data**: Maintains both real-time and historical views of the data.