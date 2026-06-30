$env:CGO_ENABLED          = "0"
$env:PORT                 = "8090"
$env:DATA_DIR             = "./data"
$env:NODE_ID              = "node-1"
$env:NODE_PEERS           = ""
$env:NODE_INTERNAL_SECRET = "dev-internal-secret"
$env:JWT_SECRET           = "dev-jwt-secret"
$env:SMS_PROVIDER         = "log"
$env:OBSCURA_ENV          = "development"
$env:P2P_ENABLED          = "false"
$env:MLS_CLI_PATH         = "$PSScriptRoot\crypto\target\release\mls-cli.exe"

Set-Location "$PSScriptRoot\backend"
go run ./cmd/node/main.go
