@echo off
set OBSCURA_ENV=development
set PORT=8090
cd /d C:\obscura\backend
call go run ./cmd/node/main.go > C:\obscura\backend_start.log 2>&1
