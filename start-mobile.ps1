# Obscura Mobile (Expo)
# Android emulator → 10.0.2.2:8090 (otomatik)
# iOS simulator   → localhost:8090   (otomatik)
# Gercek cihaz    → makinenin LAN IP'si:8090

param(
    [ValidateSet("android","ios","web","tunnel","device")]
    [string]$Platform = "android"
)

$mobiledir = "$PSScriptRoot\mobile"
$appjson   = "$mobiledir\app.json"

# Gercek cihaz icin LAN IP'yi bul ve app.json'a yaz
if ($Platform -eq "device") {
    $ip = (Get-NetIPAddress -AddressFamily IPv4 |
           Where-Object { $_.InterfaceAlias -notmatch "Loopback|vEthernet" -and $_.PrefixOrigin -eq "Dhcp" } |
           Select-Object -First 1).IPAddress

    if ($ip) {
        Write-Host "LAN IP tespit edildi: $ip"
        $json = Get-Content $appjson -Raw | ConvertFrom-Json
        $json.expo.extra.apiUrl = "http://${ip}:8090"
        $json.expo.extra.wsUrl  = "ws://${ip}:8090"
        $json | ConvertTo-Json -Depth 10 | Set-Content $appjson -Encoding utf8
        Write-Host "app.json guncellendi → $ip:8090"
    } else {
        Write-Host "UYARI: LAN IP bulunamadi. app.json'u manuel duzenle."
    }
    $Platform = "tunnel"
}

Write-Host ""
Write-Host "Obscura Mobile baslatiliyor — $Platform"
Set-Location $mobiledir

switch ($Platform) {
    "android" { npx expo start --android }
    "ios"     { npx expo start --ios }
    "web"     { npx expo start --web }
    "tunnel"  { npx expo start --tunnel }
}
