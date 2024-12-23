import json
import redis
from datetime import datetime, timedelta
import pandas as pd

class TradeDataClient:
    def __init__(self, host='localhost', port=6379, db=0, password=None):
        self.redis = redis.Redis(
            host=host,
            port=port,
            db=db,
            password=password,
            decode_responses=True
        )
        self.key_prefix = "binance:"

    def get_available_symbols(self):
        """Get all available trading symbols."""
        symbols_key = f"{self.key_prefix}symbols"
        return list(self.redis.smembers(symbols_key))

    def get_latest_trade(self, symbol):
        """Get the latest trade for a symbol."""
        key = f"{self.key_prefix}aggTrade:{symbol}:latest"
        data = self.redis.get(key)
        if data:
            return json.loads(data)
        return None

    def get_trade_history(self, symbol, hours=24):
        """
        Get trade history for a symbol for the last N hours.
        Returns a pandas DataFrame with the trade data.
        """
        history_key = f"{self.key_prefix}aggTrade:{symbol}:history"
        end_time = datetime.now()
        start_time = end_time - timedelta(hours=hours)

        # Get trades from Redis sorted set
        trades = self.redis.zrangebyscore(
            history_key,
            min=int(start_time.timestamp() * 1e9),
            max=int(end_time.timestamp() * 1e9)
        )

        # Parse trades into a list of dictionaries
        parsed_trades = []
        for trade in trades:
            trade_data = json.loads(trade)
            event = trade_data['data']
            parsed_trades.append({
                'symbol': event['s'],
                'price': float(event['p']),
                'quantity': float(event['q']),
                'trade_time': datetime.fromtimestamp(event['T'] / 1000),
                'is_buyer_maker': event['m'],
                'trade_id': event['a']
            })

        # Convert to DataFrame
        if parsed_trades:
            df = pd.DataFrame(parsed_trades)
            df.set_index('trade_time', inplace=True)
            return df
        return pd.DataFrame()

    def get_price_summary(self, symbol, interval='1min'):
        """
        Get OHLCV summary for a symbol at specified interval.
        Returns a pandas DataFrame with OHLCV data.
        """
        df = self.get_trade_history(symbol)
        if df.empty:
            return pd.DataFrame()

        # Resample the data to the specified interval
        ohlcv = df.resample(interval).agg({
            'price': ['first', 'max', 'min', 'last'],
            'quantity': 'sum'
        })

        # Flatten column names
        ohlcv.columns = ['open', 'high', 'low', 'close', 'volume']
        return ohlcv

    def get_volume_profile(self, symbol, price_bins=100):
        """
        Get volume profile (Volume by Price) for a symbol.
        Returns a pandas Series with volume per price level.
        """
        df = self.get_trade_history(symbol)
        if df.empty:
            return pd.Series()

        # Calculate volume-weighted price distribution
        volume_profile = df.groupby(pd.qcut(df['price'], price_bins))['quantity'].sum()
        return volume_profile 