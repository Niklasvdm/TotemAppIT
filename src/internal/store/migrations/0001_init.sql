-- 0001_init.sql — core catalogue schema.
-- Single source of truth for both the Phase-1 Python seeder and the Phase-2 Go store.
-- Kept Postgres-portable: the only SQLite-specific piece is INTEGER PRIMARY KEY
-- (becomes GENERATED ALWAYS AS IDENTITY on Postgres). Full-text search lives in
-- 0002_fts.sql because it is the one genuinely DB-specific part.

CREATE TABLE animal (
    id         INTEGER PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE,
    name_nl    TEXT NOT NULL,              -- canonical Dutch name (source language)
    alt_names  TEXT NOT NULL DEFAULT '',   -- free-text alternate names, may be empty
    source_url TEXT NOT NULL DEFAULT ''
);

CREATE TABLE trait (
    id     INTEGER PRIMARY KEY,
    key_nl TEXT NOT NULL UNIQUE            -- canonical Dutch trait, the dedupe key
);

CREATE TABLE animal_trait (
    animal_id INTEGER NOT NULL REFERENCES animal(id) ON DELETE CASCADE,
    trait_id  INTEGER NOT NULL REFERENCES trait(id)  ON DELETE CASCADE,
    PRIMARY KEY (animal_id, trait_id)
);

CREATE TABLE translation (
    animal_id   INTEGER NOT NULL REFERENCES animal(id) ON DELETE CASCADE,
    lang        TEXT NOT NULL CHECK (lang IN ('nl', 'it', 'en')),
    name        TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (animal_id, lang)
);

CREATE TABLE trait_translation (
    trait_id INTEGER NOT NULL REFERENCES trait(id) ON DELETE CASCADE,
    lang     TEXT NOT NULL CHECK (lang IN ('nl', 'it', 'en')),
    value    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (trait_id, lang)
);

CREATE INDEX idx_animal_trait_trait ON animal_trait (trait_id);
CREATE INDEX idx_translation_lang   ON translation (lang);
CREATE INDEX idx_trait_tr_lang      ON trait_translation (lang);
