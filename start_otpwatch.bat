@echo off
set TELEGRAM_BOT_TOKEN=<REDACTED-token-yalnizca-VDS-diskinde>
set TELEGRAM_CHAT_ID=5765174249
cd /d C:\obscura\backend
call go run ./cmd/otpwatch > C:\obscura\otpwatch.log 2>&1
