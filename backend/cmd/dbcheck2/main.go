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

	rows, err := db.Query(`SELECT phone, code, used, created_at FROM otp_records ORDER BY rowid DESC LIMIT 5`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var phone, code, createdAt string
		var used int
		rows.Scan(&phone, &code, &used, &createdAt)
		fmt.Printf("%s code=%s used=%d created_at=%s\n", phone, code, used, createdAt)
	}
}
