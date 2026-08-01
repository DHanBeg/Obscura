// store.go — commit edilen BFT bloklarının kalıcılaştırılması (ADIM 6).
//
// Engine kendi başına *sql.DB tutmaz (transport/proposer/mempool ile aynı
// dependency-injection deseni — main.go bu paketin fonksiyonlarını çağırıp
// sonucu Engine'e closure olarak enjekte eder, bkz. NewEngine'in
// parentHashFn parametresi). Bu dosyadaki fonksiyonlar salt SQL yardımcıları,
// bağlantı yönetimi yapmaz.
package consensus

import "database/sql"

// GenesisParentHash, height=1 için parent hash yerine kullanılan sabit
// (bft.go'daki eski davranışla birebir aynı sabit — height=1'in anlamı
// değişmedi, sadece height>1 artık gerçek DB'den okunuyor).
const GenesisParentHash = "0000000000000000000000000000000000000000000000000000000000000000"

// SaveBlock, commit edilen bir bloğu consensus_blocks tablosuna yazar.
// height PRIMARY KEY olduğu için aynı height iki kez INSERT edilmeye
// çalışılırsa (retry/duplicate commit) sessizce yok sayılır — idempotent.
func SaveBlock(db *sql.DB, b Block, committedAt string) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO consensus_blocks
			(height, round, parent_hash, tx_root, proposer, block_hash, block_ts, committed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		b.Height, b.Round, b.ParentHash, b.TxRoot, b.Proposer, b.Hash, b.Timestamp, committedAt,
	)
	return err
}

// LatestBlockHash, en yüksek height'e sahip commit edilmiş bloğun hash'ini
// döner. Hiç blok yoksa (height=0, GenesisParentHash, nil) döner.
func LatestBlockHash(db *sql.DB) (height uint64, hash string, err error) {
	row := db.QueryRow(`SELECT height, block_hash FROM consensus_blocks ORDER BY height DESC LIMIT 1`)
	err = row.Scan(&height, &hash)
	if err == sql.ErrNoRows {
		return 0, GenesisParentHash, nil
	}
	if err != nil {
		return 0, "", err
	}
	return height, hash, nil
}

// SaveBlockOps — ADIM 7 (ADR-0017, "sonradan-tasdik" deseni). Commit edilen
// bir bloğun Ops listesini audit-log'a (consensus_block_ops) yazar. BAKİYEYE
// HİÇ DOKUNMAZ — token.Transfer/Mint zaten senkron uygulanmıştır, bu sadece
// "bu op'lar şu blokta mutabık kalındı" kaydıdır.
//
// op_id PRIMARY KEY olduğu için: (a) aynı op aynı çağrıda iki kez geçse bile
// idempotent, (b) bir op FARKLI bir height'te tekrar tasdik edilmeye
// çalışılırsa (replay) sessizce yok sayılır — veritabanı seviyesinde
// replay-guard. Tek tek INSERT yapılır (SQLite'ta değişken sayıda VALUES
// için tek sorguda placeholder üretmek yerine — küçük batch'ler için yeterli
// ve daha basit); tamamı ÇAĞIRANIN transaction'ı içinde çalışır (db burada
// zaten bir *sql.DB olduğu için tek tek Exec — çağıran isterse kendi
// transaction'ını sarabilir).
func SaveBlockOps(db *sql.DB, height uint64, ops []string, recordedAt string) error {
	for _, op := range ops {
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO consensus_block_ops (op_id, height, recorded_at) VALUES (?, ?, ?)`,
			op, height, recordedAt,
		); err != nil {
			return err
		}
	}
	return nil
}

// HasOp, bir op-ID'nin daha önce (herhangi bir height'te) audit-log'a
// yazılıp yazılmadığını döner — replay kontrolü için sorgulanabilir.
func HasOp(db *sql.DB, opID string) (bool, error) {
	var exists int
	err := db.QueryRow(`SELECT 1 FROM consensus_block_ops WHERE op_id = ?`, opID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// OpsForBlock, verilen height için audit-log'a yazılmış op-ID'leri döner
// (testler ve ileride sorgu/görüntüleme için).
func OpsForBlock(db *sql.DB, height uint64) ([]string, error) {
	rows, err := db.Query(`SELECT op_id FROM consensus_block_ops WHERE height = ? ORDER BY op_id`, height)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var op string
		if err := rows.Scan(&op); err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, rows.Err()
}
