# Obscura OTP Monitor
# Kullanim: .\otp_monitor.ps1
# Backend logundan [OTP-MONITOR] satirlarini filtreler ve temiz cikar.
# Durdurmak icin Ctrl+C.

$logFile = "$PSScriptRoot\_backend.log"
$lastSize = 0

Write-Host "=====================================" -ForegroundColor Cyan
Write-Host "  OBSCURA OTP MONITOR" -ForegroundColor Cyan
Write-Host "  Log: $logFile" -ForegroundColor Gray
Write-Host "  Ctrl+C ile durdur" -ForegroundColor Gray
Write-Host "=====================================" -ForegroundColor Cyan
Write-Host ""
Write-Host ("{0,-20} {1,-20} {2,-10}" -f "IP ADRESI", "TELEFON", "KOD") -ForegroundColor Yellow
Write-Host ("-" * 55) -ForegroundColor Yellow

while ($true) {
    if (-not (Test-Path $logFile)) {
        Write-Host "Log dosyasi bekleniyor: $logFile" -ForegroundColor DarkGray
        Start-Sleep -Seconds 2
        continue
    }

    $currentSize = (Get-Item $logFile).Length
    if ($currentSize -gt $lastSize) {
        $stream = [System.IO.File]::Open($logFile, 'Open', 'Read', 'ReadWrite')
        $stream.Seek($lastSize, 'Begin') | Out-Null
        $reader = New-Object System.IO.StreamReader($stream)
        while (-not $reader.EndOfStream) {
            $line = $reader.ReadLine()
            if ($line -match '\[OTP-MONITOR\] IP:(.+?) \| Tel:(.+?) \| Kod:(.+)$') {
                $ip   = $matches[1].Trim()
                $tel  = $matches[2].Trim()
                $kod  = $matches[3].Trim()
                $saat = (Get-Date).ToString("HH:mm:ss")
                Write-Host ("[{0}] " -f $saat) -ForegroundColor DarkGray -NoNewline
                Write-Host ("{0,-20}" -f $ip)  -ForegroundColor Green  -NoNewline
                Write-Host ("{0,-20}" -f $tel)  -ForegroundColor White  -NoNewline
                Write-Host ("{0,-10}" -f $kod)  -ForegroundColor Cyan
            }
        }
        $reader.Close()
        $stream.Close()
        $lastSize = $currentSize
    }
    Start-Sleep -Milliseconds 500
}
