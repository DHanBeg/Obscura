@echo off
"C:\cloudflared.exe" tunnel --url http://localhost:8090 > C:\obscura\_cloudflared_8090.log 2>&1
