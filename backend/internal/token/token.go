// Package token implements the OBS token off-chain state layer.
//
// This is the transparent in-DB ledger described in ADR-0010. A zk-Rollup will
// later settle to this ledger (separate task); for now all balances and
// transfers live in SQLite tables (obs_accounts, obs_transactions, obs_supply).
//
// Token mechanics (ADR-0010):
//   - Symbol "OBS", 18 decimals, 1,000,000,000 total supply.
//   - Amounts are stored as TEXT decimal strings because 1e9 * 1e18 vastly
//     exceeds int64. All arithmetic uses math/big.Int.
//   - Transfer fee is 0.01 OBS (see TransferFee). Per ADR-0010 the spec fee
//     schedule prices a "file share" micro-op at 0.01 OBS; a basic transfer is
//     the same tier of operation, so we adopt 0.01 OBS as the default flat
//     transfer fee. 50% of every fee is burned, 50% goes to the fee pool
//     account (FeePoolDID) — this mirrors ADR-0010's "50% of fees burned, rest
//     to operators/treasury" split, with the fee pool standing in for the
//     operator/treasury distributor until the rollup settlement layer exists.
//
// Concurrency note: the shared dbi.Querier is configured with MaxOpenConns(1), so
// writes are already serialized. Multi-statement writes still use an explicit
// transaction so a mid-operation failure leaves no partial state.
package token

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/dbi"
)

// FeePoolDID is the well-known account that collects the non-burned half of
// every transfer fee. Until the rollup settlement layer exists this stands in
// for the node-operator / treasury distributor.
const FeePoolDID = "did:obs:feepool"

// opRecorder, her başarıyla commit edilen ledger operasyonundan (Transfer,
// Mint) sonra tx ID'siyle çağrılır (ADR-0017, "sonradan-tasdik" deseni).
// nil ise no-op — BFT henüz aktif değilken (veya testlerde) davranış
// değişmez. token paketi internal/consensus'u DOĞRUDAN import ETMİYOR
// (dependency injection — main.go SetOpRecorder ile bağlar, aynı
// sequencer.SetStakeLookup deseni). Bu hook BAKİYEYE DOKUNMAZ, sadece
// zaten-commit-edilmiş bir tx'in ID'sini dışarı bildirir.
var opRecorder func(txID string)

// SetOpRecorder, opRecorder'ı ayarlar (main.go'dan, süreç başlangıcında bir
// kez çağrılır). nil geçmek kaydı devre dışı bırakır.
func SetOpRecorder(fn func(string)) {
	opRecorder = fn
}

// Decimals is the OBS token precision (ADR-0010: EVM/Aztec standard).
const Decimals = 18

// oneOBS = 10^18 — the smallest-unit value of 1 whole OBS.
var oneOBS = new(big.Int).Exp(big.NewInt(10), big.NewInt(Decimals), nil)

// TransferFee returns the flat fee charged on a basic Transfer: 0.01 OBS.
// Returned as a fresh *big.Int so callers cannot mutate shared state.
func TransferFee() *big.Int {
	// 0.01 OBS = 10^18 / 100 = 10^16
	return new(big.Int).Div(oneOBS, big.NewInt(100))
}

// MaxUint256 is 2^256 - 1; transfer/mint amounts must fit in 256 bits so the
// ledger stays compatible with the future EVM/Aztec settlement layer.
var MaxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

// ErrInsufficientBalance is returned when an account cannot cover amount+fee.
var ErrInsufficientBalance = fmt.Errorf("insufficient balance")

// ErrInvalidAmount is returned for non-positive or oversized amounts.
var ErrInvalidAmount = fmt.Errorf("invalid amount")

// ErrCommitUncertain marks an InternalMove failure where whether the money
// actually moved is UNKNOWN: every other InternalMove error happens before
// tx.Commit() is called, inside a transaction that defer-rolls-back unless
// Commit succeeds, so it's CERTAIN nothing persisted. A Commit() error
// itself is different — the classic distributed-commit ambiguity (did it
// land before failing, or not at all?) that a caller can't resolve from the
// error alone. Escrow release/refund (#31, vault Phase-Status.md
// 2026-08-11, plan commit 28b1527, Adım 4) is the reason this exists: a
// caller that optimistically flips its own state before calling
// InternalMove must NOT undo that flip on this error — undoing it and
// retrying risks the original attempt having actually landed, moving money
// twice. Every other InternalMove error is safe to treat as a clean no-op.
var ErrCommitUncertain = fmt.Errorf("internalMove: commit outcome uncertain")

// ParseAmount parses a base-10 decimal string into a *big.Int and validates it
// is positive and fits in 256 bits. Used by the API layer to validate input.
func ParseAmount(s string) (*big.Int, error) {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("%w: not a base-10 integer", ErrInvalidAmount)
	}
	if v.Sign() <= 0 {
		return nil, fmt.Errorf("%w: must be positive", ErrInvalidAmount)
	}
	if v.Cmp(MaxUint256) > 0 {
		return nil, fmt.Errorf("%w: exceeds 256 bits", ErrInvalidAmount)
	}
	return v, nil
}

// Balance returns the transparent balance of an account. An account with no
// row yet is treated as zero balance (not an error).
func Balance(did string) (*big.Int, error) {
	var s string
	err := db.DB.QueryRow(
		`SELECT transparent_balance FROM obs_accounts WHERE user_did = ?`, did,
	).Scan(&s)
	if err == sql.ErrNoRows {
		return big.NewInt(0), nil
	}
	if err != nil {
		return nil, fmt.Errorf("balance lookup: %w", err)
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("balance for %s is corrupt: %q", did, s)
	}
	return v, nil
}

// txBalance reads a balance inside an open transaction.
// txBalance reads did's balance inside tx, locking the row against
// concurrent readers on Postgres (SELECT ... FOR UPDATE) so the
// read-modify-write these callers all do (read here, setBalance later in
// the same tx) can't interleave with another transaction's read of the same
// row. SQLite has no FOR UPDATE (it's a hard syntax error there, not a
// no-op) and doesn't need one: MaxOpenConns(1) means only one transaction
// can be in flight at all.
//
// A never-before-seen did has no row yet, and FOR UPDATE can't lock a row
// that doesn't exist — so this first ensures the row exists (idempotent,
// ON CONFLICT DO NOTHING) before selecting it. That closes the one gap FOR
// UPDATE alone would leave: two transactions concurrently crediting the same
// brand-new recipient, both racing to INSERT it for the first time.
func txBalance(tx dbi.Querier, did string) (*big.Int, error) {
	if _, err := tx.Exec(
		`INSERT INTO obs_accounts (user_did, transparent_balance, updated_at) VALUES (?, '0', ?) ON CONFLICT(user_did) DO NOTHING`,
		did, dbi.Now(),
	); err != nil {
		return nil, fmt.Errorf("ensure account row %s: %w", did, err)
	}

	query := `SELECT transparent_balance FROM obs_accounts WHERE user_did = ?`
	if dbi.DriverFromEnv() == dbi.DriverPostgres {
		query += ` FOR UPDATE`
	}
	var s string
	err := tx.QueryRow(query, did).Scan(&s)
	if err != nil {
		return nil, fmt.Errorf("balance lookup: %w", err)
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("balance for %s is corrupt: %q", did, s)
	}
	return v, nil
}

// setBalance upserts an account's balance inside a transaction.
func setBalance(tx dbi.Querier, did string, v *big.Int, now string) error {
	_, err := tx.Exec(`
		INSERT INTO obs_accounts (user_did, transparent_balance, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_did) DO UPDATE SET
			transparent_balance = excluded.transparent_balance,
			updated_at          = excluded.updated_at`,
		did, v.String(), now)
	if err != nil {
		return fmt.Errorf("set balance %s: %w", did, err)
	}
	return nil
}

// supply holds the parsed singleton supply row.
type supply struct {
	total       *big.Int
	circulating *big.Int
	burned      *big.Int
}

// readSupply loads the singleton obs_supply row inside a transaction,
// locking it on Postgres (see txBalance's doc comment — same reasoning; the
// row itself always exists, seeded by migration 023_obs_supply, so there's
// no "ensure it exists first" step needed here).
func readSupply(tx dbi.Querier) (*supply, error) {
	query := `SELECT total_supply, circulating, burned FROM obs_supply WHERE id = 1`
	if dbi.DriverFromEnv() == dbi.DriverPostgres {
		query += ` FOR UPDATE`
	}
	var t, c, b string
	err := tx.QueryRow(query).Scan(&t, &c, &b)
	if err != nil {
		return nil, fmt.Errorf("read supply: %w", err)
	}
	tot, ok1 := new(big.Int).SetString(t, 10)
	cir, ok2 := new(big.Int).SetString(c, 10)
	bur, ok3 := new(big.Int).SetString(b, 10)
	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("supply row corrupt: total=%q circ=%q burned=%q", t, c, b)
	}
	return &supply{total: tot, circulating: cir, burned: bur}, nil
}

// writeSupply persists circulating/burned back to the singleton row.
func writeSupply(tx dbi.Querier, s *supply, now string) error {
	_, err := tx.Exec(
		`UPDATE obs_supply SET circulating = ?, burned = ?, last_updated = ? WHERE id = 1`,
		s.circulating.String(), s.burned.String(), now)
	if err != nil {
		return fmt.Errorf("write supply: %w", err)
	}
	return nil
}

// recordTx inserts a row into obs_transactions inside a transaction.
func recordTx(tx dbi.Querier, id, from, to string, amount, fee *big.Int, txType, memo, now string) error {
	_, err := tx.Exec(`
		INSERT INTO obs_transactions
			(id, from_did, to_did, amount, fee, tx_type, memo, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'confirmed', ?)`,
		id, from, to, amount.String(), fee.String(), txType, memo, now)
	if err != nil {
		return fmt.Errorf("record tx: %w", err)
	}
	return nil
}

// Transfer moves `amount` OBS from `from` to `to`, atomically. The sender is
// additionally charged TransferFee(): 50% of the fee is burned and 50% credited
// to FeePoolDID. The whole operation runs in a single DB transaction so a
// failure at any step leaves no partial state. Returns the new transaction id.
//
// Circulating supply is unchanged by a transfer except that the burned portion
// of the fee is removed from circulation and added to `burned`.
func Transfer(ctx context.Context, from, to string, amount *big.Int, memo string) (string, error) {
	if amount == nil || amount.Sign() <= 0 {
		return "", fmt.Errorf("%w: must be positive", ErrInvalidAmount)
	}
	if amount.Cmp(MaxUint256) > 0 {
		return "", fmt.Errorf("%w: exceeds 256 bits", ErrInvalidAmount)
	}
	if from == "" || to == "" {
		return "", fmt.Errorf("%w: from/to required", ErrInvalidAmount)
	}
	if from == to {
		return "", fmt.Errorf("%w: cannot transfer to self", ErrInvalidAmount)
	}

	fee := TransferFee()
	burnPart := new(big.Int).Div(fee, big.NewInt(2)) // 50% burned
	poolPart := new(big.Int).Sub(fee, burnPart)      // remainder to fee pool (handles odd fee)
	totalDebit := new(big.Int).Add(amount, fee)

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	senderBal, err := txBalance(tx, from)
	if err != nil {
		return "", err
	}
	if senderBal.Cmp(totalDebit) < 0 {
		return "", fmt.Errorf("%w: have %s need %s (amount+fee)", ErrInsufficientBalance, senderBal, totalDebit)
	}

	recvBal, err := txBalance(tx, to)
	if err != nil {
		return "", err
	}
	poolBal, err := txBalance(tx, FeePoolDID)
	if err != nil {
		return "", err
	}

	// Apply balance changes.
	newSender := new(big.Int).Sub(senderBal, totalDebit)
	newRecv := new(big.Int).Add(recvBal, amount)
	newPool := new(big.Int).Add(poolBal, poolPart)

	if err := setBalance(tx, from, newSender, now); err != nil {
		return "", err
	}
	if err := setBalance(tx, to, newRecv, now); err != nil {
		return "", err
	}
	if err := setBalance(tx, FeePoolDID, newPool, now); err != nil {
		return "", err
	}

	// Burned half of the fee leaves circulation.
	sup, err := readSupply(tx)
	if err != nil {
		return "", err
	}
	sup.circulating.Sub(sup.circulating, burnPart)
	sup.burned.Add(sup.burned, burnPart)
	if sup.circulating.Sign() < 0 {
		return "", fmt.Errorf("invariant violation: circulating would go negative")
	}
	if err := writeSupply(tx, sup, now); err != nil {
		return "", err
	}

	txID := uuid.New().String()
	if err := recordTx(tx, txID, from, to, amount, fee, "transfer", memo, now); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	if opRecorder != nil {
		opRecorder(txID)
	}
	return txID, nil
}

// InternalMove moves `amount` OBS from `from` to `to` atomically, with NO fee
// and NO change to supply/circulating/burned — a free internal shift, not an
// economic transfer. This is the "internalMove" primitive from the escrow
// design decision (#31, vault Phase-Status.md 2026-08-11, plan commit
// 28b1527, Adım 2): escrow hold/release/refund all move value between a
// buyer/seller account and the well-known escrow account
// (marketplace.MarketplaceEscrowDID) without re-charging TransferFee() on
// top of a fee the buyer already paid once at purchase time.
//
// Deliberately reuses Transfer's proven atomic-tx skeleton unchanged
// (txBalance's row lock / FOR UPDATE-on-Postgres, setBalance, recordTx) and
// strips out exactly the fee and supply/burn logic — see Transfer above for
// what's preserved. txType is the obs_transactions.tx_type value the caller
// wants recorded (e.g. a future "escrow_hold"/"escrow_release"/
// "escrow_refund"), so ledger history can distinguish these from ordinary
// transfers.
//
// Used by internal/marketplace's escrow release (Adım 4) and refund (Adım
// 5) — Purchase()'s own hold leg (Adım 3) still goes through Transfer, see
// that function's doc comment for why.
func InternalMove(ctx context.Context, from, to string, amount *big.Int, txType string) (string, error) {
	if amount == nil || amount.Sign() <= 0 {
		return "", fmt.Errorf("%w: must be positive", ErrInvalidAmount)
	}
	if amount.Cmp(MaxUint256) > 0 {
		return "", fmt.Errorf("%w: exceeds 256 bits", ErrInvalidAmount)
	}
	if from == "" || to == "" {
		return "", fmt.Errorf("%w: from/to required", ErrInvalidAmount)
	}
	if from == to {
		return "", fmt.Errorf("%w: cannot move to self", ErrInvalidAmount)
	}
	if txType == "" {
		return "", fmt.Errorf("%w: txType required", ErrInvalidAmount)
	}

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	senderBal, err := txBalance(tx, from)
	if err != nil {
		return "", err
	}
	if senderBal.Cmp(amount) < 0 {
		return "", fmt.Errorf("%w: have %s need %s", ErrInsufficientBalance, senderBal, amount)
	}

	recvBal, err := txBalance(tx, to)
	if err != nil {
		return "", err
	}

	newSender := new(big.Int).Sub(senderBal, amount)
	newRecv := new(big.Int).Add(recvBal, amount)

	if err := setBalance(tx, from, newSender, now); err != nil {
		return "", err
	}
	if err := setBalance(tx, to, newRecv, now); err != nil {
		return "", err
	}

	txID := uuid.New().String()
	if err := recordTx(tx, txID, from, to, amount, big.NewInt(0), txType, "", now); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("%w: commit tx: %w", ErrCommitUncertain, err)
	}
	if opRecorder != nil {
		opRecorder(txID)
	}
	return txID, nil
}

// Mint creates new OBS and credits it to `to`. Admin-only (caller must enforce
// authorization) — used for genesis allocation and airdrops. Minting increases
// circulating supply but never pushes it above total_supply (ADR-0010: 1B cap).
func Mint(ctx context.Context, to string, amount *big.Int, reason string) (string, error) {
	if amount == nil || amount.Sign() <= 0 {
		return "", fmt.Errorf("%w: must be positive", ErrInvalidAmount)
	}
	if amount.Cmp(MaxUint256) > 0 {
		return "", fmt.Errorf("%w: exceeds 256 bits", ErrInvalidAmount)
	}
	if to == "" {
		return "", fmt.Errorf("%w: to required", ErrInvalidAmount)
	}

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	sup, err := readSupply(tx)
	if err != nil {
		return "", err
	}
	newCirc := new(big.Int).Add(sup.circulating, amount)
	if newCirc.Cmp(sup.total) > 0 {
		return "", fmt.Errorf("mint would exceed total supply: circulating %s + %s > total %s",
			sup.circulating, amount, sup.total)
	}
	sup.circulating = newCirc
	if err := writeSupply(tx, sup, now); err != nil {
		return "", err
	}

	bal, err := txBalance(tx, to)
	if err != nil {
		return "", err
	}
	if err := setBalance(tx, to, new(big.Int).Add(bal, amount), now); err != nil {
		return "", err
	}

	txID := uuid.New().String()
	// Mint has no real "from" account — use a sentinel so history is auditable.
	if err := recordTx(tx, txID, "did:obs:mint", to, amount, big.NewInt(0), "mint", reason, now); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	if opRecorder != nil {
		opRecorder(txID)
	}
	return txID, nil
}

// Burn destroys OBS held by `from`, removing it from circulation permanently.
// Decreases both the account balance and circulating supply, and increases the
// burned counter.
func Burn(ctx context.Context, from string, amount *big.Int, reason string) (string, error) {
	if amount == nil || amount.Sign() <= 0 {
		return "", fmt.Errorf("%w: must be positive", ErrInvalidAmount)
	}
	if amount.Cmp(MaxUint256) > 0 {
		return "", fmt.Errorf("%w: exceeds 256 bits", ErrInvalidAmount)
	}
	if from == "" {
		return "", fmt.Errorf("%w: from required", ErrInvalidAmount)
	}

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	bal, err := txBalance(tx, from)
	if err != nil {
		return "", err
	}
	if bal.Cmp(amount) < 0 {
		return "", fmt.Errorf("%w: have %s need %s", ErrInsufficientBalance, bal, amount)
	}
	if err := setBalance(tx, from, new(big.Int).Sub(bal, amount), now); err != nil {
		return "", err
	}

	sup, err := readSupply(tx)
	if err != nil {
		return "", err
	}
	sup.circulating.Sub(sup.circulating, amount)
	sup.burned.Add(sup.burned, amount)
	if sup.circulating.Sign() < 0 {
		return "", fmt.Errorf("invariant violation: circulating would go negative")
	}
	if err := writeSupply(tx, sup, now); err != nil {
		return "", err
	}

	txID := uuid.New().String()
	if err := recordTx(tx, txID, from, "did:obs:burn", amount, big.NewInt(0), "burn", reason, now); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	if opRecorder != nil {
		opRecorder(txID)
	}
	return txID, nil
}

// SupplyInfo is a read-only snapshot of the obs_supply singleton row.
type SupplyInfo struct {
	Total       string `json:"total"`
	Circulating string `json:"circulating"`
	Burned      string `json:"burned"`
}

// Supply returns the current total/circulating/burned figures.
func Supply() (*SupplyInfo, error) {
	var info SupplyInfo
	err := db.DB.QueryRow(
		`SELECT total_supply, circulating, burned FROM obs_supply WHERE id = 1`,
	).Scan(&info.Total, &info.Circulating, &info.Burned)
	if err != nil {
		return nil, fmt.Errorf("read supply: %w", err)
	}
	return &info, nil
}

// TxRecord is a single row of an account's transaction history.
type TxRecord struct {
	ID        string `json:"id"`
	FromDID   string `json:"from_did"`
	ToDID     string `json:"to_did"`
	Amount    string `json:"amount"`
	Fee       string `json:"fee"`
	TxType    string `json:"tx_type"`
	Memo      string `json:"memo"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// History returns up to `limit` of the most recent transactions where `did` is
// either the sender or the recipient, newest first.
func History(did string, limit int) ([]TxRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := db.DB.Query(`
		SELECT id, from_did, to_did, amount, fee, tx_type, memo, status, created_at
		FROM obs_transactions
		WHERE from_did = ? OR to_did = ?
		ORDER BY created_at DESC
		LIMIT ?`, did, did, limit)
	if err != nil {
		return nil, fmt.Errorf("history query: %w", err)
	}
	defer rows.Close()

	out := make([]TxRecord, 0, limit)
	for rows.Next() {
		var t TxRecord
		if err := rows.Scan(&t.ID, &t.FromDID, &t.ToDID, &t.Amount, &t.Fee,
			&t.TxType, &t.Memo, &t.Status, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("history scan: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
