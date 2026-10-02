-- Community feedback: new-animal suggestions and reports on existing animals.
-- Both deduplicate and keep a count, so repeated submissions bump a counter
-- instead of creating rows. The SysAdmin reviews pending entries via totem-admin.

CREATE TABLE suggestion (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,                       -- as submitted (trimmed, validated)
    name_norm  TEXT NOT NULL UNIQUE,                -- lower/space-collapsed dedup key
    note       TEXT NOT NULL DEFAULT '',            -- optional "why", from the first submitter
    count      INTEGER NOT NULL DEFAULT 1,          -- how many people suggested it
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (status IN ('pending', 'accepted', 'rejected'))
);

CREATE INDEX idx_suggestion_status ON suggestion(status, count DESC);

CREATE TABLE report (
    id         INTEGER PRIMARY KEY,
    animal_id  INTEGER NOT NULL REFERENCES animal(id) ON DELETE CASCADE,
    reason     TEXT NOT NULL,                       -- whitelisted enum (see store)
    note       TEXT NOT NULL DEFAULT '',
    count      INTEGER NOT NULL DEFAULT 1,          -- how many people reported this (animal, reason)
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (animal_id, reason),
    CHECK (reason IN ('incorrect', 'unknown', 'poor_description', 'other')),
    CHECK (status IN ('pending', 'accepted', 'rejected'))
);

CREATE INDEX idx_report_status ON report(status, count DESC);
