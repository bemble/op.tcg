package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	db *sql.DB
}

type Owner struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

// Item is one possession of a card. OwnerID is optional (nil = unspecified).
// Card is not stored here — it is filled in from the catalogue by the server.
type Item struct {
	ID        int64  `json:"id"`
	CardID    string `json:"cardId"`
	OwnerID   *int64 `json:"ownerId"`
	OwnerName string `json:"ownerName,omitempty"`
	Quantity  int    `json:"quantity"`
	Language  string `json:"language"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Card      *Card  `json:"card,omitempty"`
}

func openStore(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // sqlite: serialize writes, simplest correct default
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ---- owners ----

func (s *Store) ListOwners() ([]Owner, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at FROM owners ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Owner{}
	for rows.Next() {
		var o Owner
		if err := rows.Scan(&o.ID, &o.Name, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) AddOwner(name string) (*Owner, error) {
	res, err := s.db.Exec(`INSERT INTO owners (name) VALUES (?)
		ON CONFLICT (name) DO NOTHING`, name)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// already existed — return the existing row
		var o Owner
		err := s.db.QueryRow(`SELECT id, name, created_at FROM owners WHERE name=?`, name).
			Scan(&o.ID, &o.Name, &o.CreatedAt)
		return &o, err
	}
	id, _ := res.LastInsertId()
	var o Owner
	err = s.db.QueryRow(`SELECT id, name, created_at FROM owners WHERE id=?`, id).
		Scan(&o.ID, &o.Name, &o.CreatedAt)
	return &o, err
}

// DeleteOwner removes an owner. Possessions referencing it keep existing with
// owner_id set to NULL (ON DELETE SET NULL), so no card is lost.
func (s *Store) DeleteOwner(id int64) error {
	_, err := s.db.Exec(`DELETE FROM owners WHERE id=?`, id)
	return err
}

// ---- settings (app-level key/value) ----

func (s *Store) GetSetting(key, def string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return def, nil
	}
	if err != nil {
		return def, err
	}
	return v, nil
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// ---- collection ----

// AddItem merges/creates a possession. Possessions are keyed by
// (card_id, owner_id, language); adding the same combination again increments
// the quantity. The card itself lives in the catalogue, not here.
func (s *Store) AddItem(cardID string, it Item) (*Item, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	id, err := upsertItemTx(tx, cardID, it)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetItem(id)
}

// BatchEntry is one (card, possession) pair for AddItems.
type BatchEntry struct {
	CardID string
	Item   Item
}

// AddItems inserts/updates many possessions in a single transaction: it's
// all-or-nothing (any error rolls the whole batch back) and far cheaper than N
// separate AddItem calls. Returns the resulting items in input order.
func (s *Store) AddItems(entries []BatchEntry) ([]Item, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ids := make([]int64, 0, len(entries))
	for _, e := range entries {
		id, err := upsertItemTx(tx, e.CardID, e.Item)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	out := make([]Item, 0, len(ids))
	for _, id := range ids {
		it, err := s.GetItem(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	return out, nil
}

// upsertItemTx adds quantity to a matching possession (same card, language and
// NULL-aware owner) or inserts a new row, within the given transaction. Returns
// the affected row id.
func upsertItemTx(tx *sql.Tx, cardID string, it Item) (int64, error) {
	if it.Quantity <= 0 {
		it.Quantity = 1
	}

	var id int64
	row := tx.QueryRow(`SELECT id FROM collection_items
		WHERE card_id=? AND language=? AND owner_id IS ?`,
		cardID, it.Language, it.OwnerID)
	switch err := row.Scan(&id); err {
	case nil:
		if _, err := tx.Exec(`UPDATE collection_items
			SET quantity=quantity+?, notes=CASE WHEN ?!='' THEN ? ELSE notes END, updated_at=datetime('now')
			WHERE id=?`, it.Quantity, it.Notes, it.Notes, id); err != nil {
			return 0, err
		}
	case sql.ErrNoRows:
		res, err := tx.Exec(`INSERT INTO collection_items (card_id, owner_id, quantity, language, notes)
			VALUES (?, ?, ?, ?, ?)`,
			cardID, it.OwnerID, it.Quantity, it.Language, it.Notes)
		if err != nil {
			return 0, err
		}
		id, _ = res.LastInsertId()
	default:
		return 0, err
	}
	return id, nil
}

func (s *Store) GetItem(id int64) (*Item, error) {
	rows, err := s.queryItems(`WHERE i.id=?`, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	return &rows[0], nil
}

// ListItems returns possessions with their owner name. card_id matching (by
// name) happens at the server layer against the catalogue; here q filters by
// card_id text and ownerID (>0) filters by owner.
func (s *Store) ListItems(ownerID int64) ([]Item, error) {
	where := "WHERE 1=1"
	var args []any
	if ownerID > 0 {
		where += " AND i.owner_id=?"
		args = append(args, ownerID)
	}
	where += " ORDER BY i.card_id, i.id"
	return s.queryItems(where, args...)
}

func (s *Store) queryItems(where string, args ...any) ([]Item, error) {
	rows, err := s.db.Query(`
		SELECT i.id, i.card_id, i.owner_id, COALESCE(o.name,''), i.quantity, i.language, i.notes, i.created_at, i.updated_at
		FROM collection_items i
		LEFT JOIN owners o ON o.id=i.owner_id `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.CardID, &it.OwnerID, &it.OwnerName, &it.Quantity, &it.Language, &it.Notes,
			&it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// UpdateItem patches mutable fields. quantity<=0 deletes the row.
func (s *Store) UpdateItem(id int64, in Item) (*Item, error) {
	if in.Quantity <= 0 {
		if _, err := s.db.Exec(`DELETE FROM collection_items WHERE id=?`, id); err != nil {
			return nil, err
		}
		return nil, nil
	}
	_, err := s.db.Exec(`UPDATE collection_items
		SET owner_id=?, quantity=?, language=?, notes=?, updated_at=datetime('now')
		WHERE id=?`, in.OwnerID, in.Quantity, in.Language, in.Notes, id)
	if err != nil {
		return nil, err
	}
	return s.GetItem(id)
}

func (s *Store) DeleteItem(id int64) error {
	_, err := s.db.Exec(`DELETE FROM collection_items WHERE id=?`, id)
	return err
}

// RemapCardIDs rewrites owned possessions whose card_id changed (e.g. PRB DON
// golds moving from _p1 to _p2). Returns the number of rows updated. New ids are
// freshly minted so collisions with the (card_id, language, owner_id) unique
// index don't occur in practice; runs idempotently (no-op once migrated).
func (s *Store) RemapCardIDs(remap map[string]string) (int, error) {
	if len(remap) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for oldID, newID := range remap {
		if oldID == newID {
			continue
		}
		res, err := tx.Exec(`UPDATE collection_items
			SET card_id=?, updated_at=datetime('now') WHERE card_id=?`, newID, oldID)
		if err != nil {
			return n, err
		}
		c, _ := res.RowsAffected()
		n += int(c)
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

// ---- curated cards ----

// ListCuratedCards returns the user-added cards, newest first.
func (s *Store) ListCuratedCards() ([]curatedCard, error) {
	rows, err := s.db.Query(`SELECT card_id, code, name, rarity, product_id
		FROM curated_cards ORDER BY created_at DESC, card_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []curatedCard{}
	for rows.Next() {
		var c curatedCard
		if err := rows.Scan(&c.cardID, &c.code, &c.name, &c.rarity, &c.productID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddCuratedCard inserts a curated card (fails if its card_id already exists).
func (s *Store) AddCuratedCard(c curatedCard) error {
	_, err := s.db.Exec(`INSERT INTO curated_cards (card_id, code, name, rarity, product_id)
		VALUES (?, ?, ?, ?, ?)`, c.cardID, c.code, c.name, c.rarity, c.productID)
	return err
}

// DeleteCuratedCard removes a curated card by its card_id.
func (s *Store) DeleteCuratedCard(cardID string) error {
	_, err := s.db.Exec(`DELETE FROM curated_cards WHERE card_id=?`, cardID)
	return err
}

// ---- stats ----

type OwnerStat struct {
	OwnerID     *int64 `json:"ownerId"`
	Name        string `json:"name"`
	UniqueCards int    `json:"uniqueCards"`
	TotalCards  int    `json:"totalCards"`
}

type Stats struct {
	UniqueCards int         `json:"uniqueCards"` // distinct cards owned at all
	TotalCards  int         `json:"totalCards"`  // sum of quantities
	ByOwner     []OwnerStat `json:"byOwner"`
}

func (s *Store) Stats() (Stats, error) {
	var st Stats
	st.ByOwner = []OwnerStat{}

	if err := s.db.QueryRow(`SELECT COUNT(DISTINCT card_id), COALESCE(SUM(quantity),0)
		FROM collection_items`).Scan(&st.UniqueCards, &st.TotalCards); err != nil {
		return st, err
	}

	rows, err := s.db.Query(`
		SELECT i.owner_id, COALESCE(o.name,''), COUNT(DISTINCT i.card_id), COALESCE(SUM(i.quantity),0)
		FROM collection_items i
		LEFT JOIN owners o ON o.id=i.owner_id
		GROUP BY i.owner_id
		ORDER BY o.name COLLATE NOCASE`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var os OwnerStat
		if err := rows.Scan(&os.OwnerID, &os.Name, &os.UniqueCards, &os.TotalCards); err != nil {
			return st, err
		}
		if os.Name == "" {
			os.Name = "Non attribué"
		}
		st.ByOwner = append(st.ByOwner, os)
	}
	return st, rows.Err()
}
