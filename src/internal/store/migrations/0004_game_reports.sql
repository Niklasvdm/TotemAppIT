-- Community reports about a game (a bug, a rule implemented wrong, something
-- unclear). Mirrors the animal `report` table: deduplicated per (game, reason)
-- with a running count, moderated via totem-admin. There is no foreign key: a
-- game is a slug owned by the code (internal/game), validated at the API layer.

CREATE TABLE game_report (
    id         INTEGER PRIMARY KEY,
    game       TEXT NOT NULL,                       -- game slug (validated against the registry)
    reason     TEXT NOT NULL,                       -- whitelisted enum (see store)
    note       TEXT NOT NULL DEFAULT '',
    count      INTEGER NOT NULL DEFAULT 1,          -- how many people reported this (game, reason)
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (game, reason),
    CHECK (reason IN ('bug', 'rules', 'unclear', 'other')),
    CHECK (status IN ('pending', 'accepted', 'rejected'))
);

CREATE INDEX idx_game_report_status ON game_report(status, count DESC);
