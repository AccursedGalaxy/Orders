# Trade Data Analysis

This package provides tools for analyzing trade data stored in Redis by the Binance trade streamer.

## Setup

1. Create a Python virtual environment:
```bash
python -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate
```

2. Install dependencies:
```bash
pip install -r requirements.txt
```

3. Start Jupyter:
```bash
jupyter notebook notebooks/
```

## Usage

The main components are:

1. `tradinglib/redis_client.py` - Contains the `TradeDataClient` class for interacting with Redis data
2. `notebooks/trade_analysis.ipynb` - Example Jupyter notebook showing various analysis techniques

The `TradeDataClient` provides these main functions:

- `get_available_symbols()` - List all trading symbols
- `get_latest_trade(symbol)` - Get most recent trade for a symbol
- `get_trade_history(symbol, hours=24)` - Get historical trades as pandas DataFrame
- `get_price_summary(symbol, interval='1min')` - Get OHLCV data at specified interval
- `get_volume_profile(symbol, price_bins=100)` - Get volume distribution across price levels

## Example Analyses

The example notebook demonstrates:

1. Basic price analysis and visualization
2. Volume profile analysis
3. OHLCV candlestick charts
4. Trade size distribution analysis

You can use these as starting points for your own analysis. 