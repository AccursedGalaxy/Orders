#!/bin/bash

# Load environment variables from .env file
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Run the Redis viewer with the provided arguments
./bin/redis-viewer "$@" 