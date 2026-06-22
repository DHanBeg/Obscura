package bots

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Init — bots tablolarını oluşturur (idempotent).
func Init(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS bots (
			id          TEXT PRIMARY KEY,
			owner_did   TEXT NOT NULL,
			name        TEXT NOT NULL,
			description TEXT DEFAULT '',
			avatar_url  TEXT DEFAULT '',
			webhook_url TEXT NOT NULL,
			token       TEXT NOT NULL,
			active      INTEGER NOT NULL DEFAULT 1,
			created_at  TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_bots_owner ON bots(owner_did);

		CREATE TABLE IF NOT EXISTS bot_messages (
			id        TEXT PRIMARY KEY,
			bot_id    TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
			from_did  TEXT NOT NULL,
			to_did    TEXT NOT NULL,
			content   TEXT NOT NULL,
			direction TEXT NOT NULL CHECK(direction IN ('inbound','outbound')),
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_bot_messages_bot ON bot_messages(bot_id, created_at DESC);
	`)
	return err
}

func CreateBot(ctx context.Context, db *sql.DB, ownerDID, name, description, avatarURL, webhookURL string) (*Bot, error) {
	bot := &Bot{
		ID:          uuid.New().String(),
		OwnerDID:    ownerDID,
		Name:        name,
		Description: description,
		AvatarURL:   avatarURL,
		WebhookURL:  webhookURL,
		Token:       "bot_" + uuid.New().String(),
		Active:      true,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO bots (id, owner_did, name, description, avatar_url, webhook_url, token, active, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		bot.ID, bot.OwnerDID, bot.Name, bot.Description, bot.AvatarURL,
		bot.WebhookURL, bot.Token, 1, bot.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("bots: create: %w", err)
	}
	return bot, nil
}

func GetBot(ctx context.Context, db *sql.DB, id string) (*Bot, error) {
	b := &Bot{}
	var active int
	err := db.QueryRowContext(ctx,
		`SELECT id, owner_did, name, description, avatar_url, webhook_url, token, active, created_at
		 FROM bots WHERE id = ?`, id,
	).Scan(&b.ID, &b.OwnerDID, &b.Name, &b.Description, &b.AvatarURL,
		&b.WebhookURL, &b.Token, &active, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bots: get: %w", err)
	}
	b.Active = active == 1
	return b, nil
}

func ListUserBots(ctx context.Context, db *sql.DB, ownerDID string) ([]Bot, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, owner_did, name, description, avatar_url, webhook_url, active, created_at
		 FROM bots WHERE owner_did = ? ORDER BY created_at DESC`, ownerDID,
	)
	if err != nil {
		return nil, fmt.Errorf("bots: list: %w", err)
	}
	defer rows.Close()

	var bots []Bot
	for rows.Next() {
		var b Bot
		var active int
		if err := rows.Scan(&b.ID, &b.OwnerDID, &b.Name, &b.Description,
			&b.AvatarURL, &b.WebhookURL, &active, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("bots: scan: %w", err)
		}
		b.Active = active == 1
		bots = append(bots, b)
	}
	return bots, rows.Err()
}

func ListPublicBots(ctx context.Context, db *sql.DB, limit, offset int) ([]Bot, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, owner_did, name, description, avatar_url, active, created_at
		 FROM bots WHERE active = 1 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("bots: discover: %w", err)
	}
	defer rows.Close()

	var bots []Bot
	for rows.Next() {
		var b Bot
		var active int
		if err := rows.Scan(&b.ID, &b.OwnerDID, &b.Name, &b.Description,
			&b.AvatarURL, &active, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("bots: scan: %w", err)
		}
		b.Active = active == 1
		bots = append(bots, b)
	}
	return bots, rows.Err()
}

func UpdateBot(ctx context.Context, db *sql.DB, id, ownerDID, name, description, webhookURL string, active bool) error {
	activeInt := 0
	if active {
		activeInt = 1
	}
	_, err := db.ExecContext(ctx,
		`UPDATE bots SET name = ?, description = ?, webhook_url = ?, active = ?
		 WHERE id = ? AND owner_did = ?`,
		name, description, webhookURL, activeInt, id, ownerDID,
	)
	if err != nil {
		return fmt.Errorf("bots: update: %w", err)
	}
	return nil
}

func RegenerateToken(ctx context.Context, db *sql.DB, id, ownerDID string) (string, error) {
	newToken := "bot_" + uuid.New().String()
	_, err := db.ExecContext(ctx,
		`UPDATE bots SET token = ? WHERE id = ? AND owner_did = ?`,
		newToken, id, ownerDID,
	)
	if err != nil {
		return "", fmt.Errorf("bots: regen-token: %w", err)
	}
	return newToken, nil
}

func DeleteBot(ctx context.Context, db *sql.DB, id, ownerDID string) error {
	_, err := db.ExecContext(ctx,
		`DELETE FROM bots WHERE id = ? AND owner_did = ?`, id, ownerDID,
	)
	if err != nil {
		return fmt.Errorf("bots: delete: %w", err)
	}
	return nil
}

func LogBotMessage(ctx context.Context, db *sql.DB, botID, fromDID, toDID, content, direction string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO bot_messages (id, bot_id, from_did, to_did, content, direction, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), botID, fromDID, toDID, content, direction,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("bots: log-msg: %w", err)
	}
	return nil
}
