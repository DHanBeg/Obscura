# Obscura Desktop (Tauri v2)
# Önce: frontend ve backend ayakta olmalı
# start-backend.ps1 + start-frontend.ps1 ayrı terminallerde çalıştır

Write-Host "Obscura Desktop baslatiliyor..."
Write-Host "Backend: http://localhost:8090"
Write-Host "Frontend: http://localhost:3003"
Write-Host ""

Set-Location "$PSScriptRoot\desktop"
npx tauri dev
