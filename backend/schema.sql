-- One Piece TCG. The card catalogue lives in catalogue.json (in memory at
-- runtime); SQLite holds only the user's collection and co-owners.

-- Co-owners of the shared collection. Managed (free-text names) in the
-- Preferences screen, then optionally picked per possession.
CREATE TABLE IF NOT EXISTS owners (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- One row per possession of a card. A card can be possessed several times,
-- by one or several owners (owner_id may be NULL = unspecified). card_id
-- references a card in catalogue.json (looked up at the application layer).
CREATE TABLE IF NOT EXISTS collection_items (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    card_id    TEXT NOT NULL,
    owner_id   INTEGER,
    quantity   INTEGER NOT NULL DEFAULT 1,
    language   TEXT NOT NULL DEFAULT 'EN',
    notes      TEXT NOT NULL DEFAULT '',
    -- 'owned' (physical copy), 'ordered' (bought, not yet in hand) or 'wishlist'.
    status     TEXT NOT NULL DEFAULT 'owned',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (owner_id) REFERENCES owners (id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_items_card ON collection_items (card_id);
CREATE INDEX IF NOT EXISTS idx_items_owner ON collection_items (owner_id);

-- App-level key/value settings (e.g. the collection goal).
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

-- User-added cards our automated sources miss (e.g. TCGplayer-only promo alt
-- arts). Merged into the in-memory catalogue as "extra" cards; image comes from
-- the TCGplayer product id. card_id is the (possibly suffixed) catalogue id.
CREATE TABLE IF NOT EXISTS curated_cards (
    card_id    TEXT PRIMARY KEY,
    code       TEXT NOT NULL,
    name       TEXT NOT NULL,
    rarity     TEXT NOT NULL DEFAULT 'PR',
    product_id INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
