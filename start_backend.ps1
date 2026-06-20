# Obscura Backend Baslatici
# .env dosyasindan env degiskenlerini yukler, backend'i calistirir ve
# ciktisini hem ekrana hem _backend.log'a yazar.

Set-Location "E:\obscura"

# .env yukle
Get-Content ".env" | Where-Object { $_ -match '^\s*[^#].*=.*' } | ForEach-Object {
    $parts = $_ -split '=', 2
    if ($parts.Count -eq 2) {
        $key   = $parts[0].Trim()
        $value = $parts[1].Trim()
        [System.Environment]::SetEnvironmentVariable($key, $value, 'Process')
    }
}

# P2P devre disi (dev icin — key dosya sorunu yok)
$env:P2P_ENABLED = "false"

Write-Host "=====================================" -ForegroundColor Cyan
Write-Host "  OBSCURA BACKEND — PORT $env:PORT"    -ForegroundColor Cyan
Write-Host "  SMS_PROVIDER: $env:SMS_PROVIDER"      -ForegroundColor Gray
Write-Host "  OBSCURA_ENV : $env:OBSCURA_ENV"       -ForegroundColor Gray
Write-Host "  Ctrl+C ile durdur"                    -ForegroundColor Gray
Write-Host "=====================================" -ForegroundColor Cyan

# Backend'i calistir, ciktisini hem ekrana hem log'a yaz
& ".\backend\obscura-node.exe" 2>&1 | Tee-Object -FilePath "_backend.log" -Append
