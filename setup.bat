@echo off
echo Starting Raikiri - Stream Chat Aggregator
echo -----------------------------------------

IF EXIST .env (
    FOR /F "tokens=*" %%i IN (.env) DO SET %%i
)

set /p input_twitch="Enter Twitch Channel(s) (comma separated) [%TWITCH_CHANNELS%]: "
if not "%input_twitch%"=="" set TWITCH_CHANNELS=%input_twitch%

set /p input_yt_live="Enter YouTube Video ID (optional) [%YOUTUBE_LIVE_ID%]: "
if not "%input_yt_live%"=="" set YOUTUBE_LIVE_ID=%input_yt_live%

if "%YOUTUBE_LIVE_ID%"=="" (
    set /p input_yt_chan="Enter YouTube Channel ID (optional, if no Video ID) [%YOUTUBE_CHANNEL_ID%]: "
    if not "%input_yt_chan%"=="" set YOUTUBE_CHANNEL_ID=%input_yt_chan%
)

echo TWITCH_CHANNELS=%TWITCH_CHANNELS% > .env
echo YOUTUBE_LIVE_ID=%YOUTUBE_LIVE_ID% >> .env
echo YOUTUBE_CHANNEL_ID=%YOUTUBE_CHANNEL_ID% >> .env

echo Building and starting container...
docker-compose up --build -d

echo -----------------------------------------
echo Raikiri is running!
echo OBS Overlay URL: http://localhost:30000
echo Stop with: docker-compose down
pause
