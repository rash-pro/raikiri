#!/bin/bash

echo "Starting Raikiri - Stream Chat Aggregator"
echo "-----------------------------------------"

if [ -f .env ]; then
    source .env
fi

read -p "Enter Twitch Channel(s) (comma separated) [${TWITCH_CHANNELS}]: " input_twitch
TWITCH_CHANNELS=${input_twitch:-$TWITCH_CHANNELS}

read -p "Enter YouTube Video ID (optional) [${YOUTUBE_LIVE_ID}]: " input_yt_live
YOUTUBE_LIVE_ID=${input_yt_live:-$YOUTUBE_LIVE_ID}

if [ -z "$YOUTUBE_LIVE_ID" ]; then
    read -p "Enter YouTube Channel ID (optional, if no Video ID) [${YOUTUBE_CHANNEL_ID}]: " input_yt_chan
    YOUTUBE_CHANNEL_ID=${input_yt_chan:-$YOUTUBE_CHANNEL_ID}
fi

echo "TWITCH_CHANNELS=$TWITCH_CHANNELS" > .env
echo "YOUTUBE_LIVE_ID=$YOUTUBE_LIVE_ID" >> .env
echo "YOUTUBE_CHANNEL_ID=$YOUTUBE_CHANNEL_ID" >> .env

echo "Building and starting container..."
docker-compose up --build -d

echo "-----------------------------------------"
echo "Raikiri is running!"
echo "OBS Overlay URL: http://localhost:30000"
echo "Stop with: docker-compose down"
