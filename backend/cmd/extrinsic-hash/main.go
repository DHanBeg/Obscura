// extrinsic-hash — one-off CLI: compute the canonical blake2b-256 tx hash
// for a raw signed extrinsic hex (bridge.ExtrinsicHash), for manual
// reconciliation of a "ready_to_submit"/"failed" bridge_transfers row after
// a submit-side websocket drop where the extrinsic still landed on-chain.
//
//	go run ./cmd/extrinsic-hash <0x-hex>
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"obscura.network/core/internal/bridge"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: extrinsic-hash <0x-hex>")
		os.Exit(1)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(os.Args[1], "0x"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "hex decode: %v\n", err)
		os.Exit(1)
	}
	h := bridge.ExtrinsicHash(raw)
	fmt.Printf("0x%x\n", h)
}
