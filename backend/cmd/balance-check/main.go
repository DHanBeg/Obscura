// balance-check — one-off CLI to read a Paseo SS58 address's free balance.
// Used to verify bridge lock/unlock transfers end-to-end (Bridge madde 4).
//
//	go run ./cmd/balance-check <ss58-address>
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"obscura.network/core/internal/bridge"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: balance-check <ss58-address>")
		os.Exit(1)
	}
	rpcURL := os.Getenv("DOT_RPC_URL")
	if rpcURL == "" {
		fmt.Fprintln(os.Stderr, "DOT_RPC_URL not set")
		os.Exit(1)
	}
	client := bridge.New(bridge.Config{PolkadotRPC: rpcURL, RPCTimeout: 15 * time.Second, RPCMaxRetries: 3})
	balance, err := client.FetchFreeBalance(context.Background(), os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("address=%s free_balance_planck=%s\n", os.Args[1], balance.String())
}
