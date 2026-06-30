# Obscura Desktop — Production Build
# Tauri için Next.js'i static export modunda derler, sonra Tauri bundle olusturur.

$frontendDir = "$PSScriptRoot\frontend"
$desktopDir  = "$PSScriptRoot\desktop"
$configFile  = "$frontendDir\next.config.js"
$backup      = "$frontendDir\next.config.js.bak"

Write-Host "[1/4] Next.js config -> export moduna geciliyor..."
Copy-Item $configFile $backup
(Get-Content $configFile) -replace '"standalone"', '"export"' | Set-Content $configFile

try {
    Write-Host "[2/4] Frontend build (static export)..."
    Set-Location $frontendDir
    npm run build
    if (-not $?) { throw "Frontend build basarisiz" }

    Write-Host "[3/4] Tauri bundle olusturuluyor..."
    Set-Location $desktopDir
    npx tauri build
    if (-not $?) { throw "Tauri build basarisiz" }

    Write-Host "[4/4] Tamamlandi!"
    Write-Host "Binary: $desktopDir\src-tauri\target\release\obscura-desktop.exe"
} finally {
    Write-Host "Next.js config geri yukleniyor..."
    Copy-Item $backup $configFile -Force
    Remove-Item $backup -ErrorAction SilentlyContinue
}
