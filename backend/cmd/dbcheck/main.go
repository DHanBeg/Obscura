package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", `C:\obscura\backend\data\obscura.db?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=10000&_synchronous=NORMAL`)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	dids := map[string]string{
		"karahanlilar":  "did:obs:9fac080e47e51f61b7a1f5d6857f6c61",
		"maviikrbm":     "did:obs:1f127ceed8f276dc71165774f7772334",
		"yumiumi":       "did:obs:090f0690b17f0fa2953e98d967b1383f",
		"emirin_yaveri": "did:obs:53a3ceeb2b1377af397a1c6cc0cfd406",
	}

	for name, did := range dids {
		var pkCount, opkCount int
		db.QueryRow(`SELECT COUNT(*) FROM prekey_bundles WHERE user_did = ?`, did).Scan(&pkCount)
		db.QueryRow(`SELECT COUNT(*) FROM one_time_prekeys WHERE user_did = ? AND used = 0`, did).Scan(&opkCount)
		fmt.Printf("%s: prekey_bundles=%d one_time_prekeys(unused)=%d\n", name, pkCount, opkCount)
	}
}
