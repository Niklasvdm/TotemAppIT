-- 0002_add_images.sql — per-animal image + attribution.
-- The image FILES live on disk (served from TOTEM_IMAGE_DIR), NOT in the DB and
-- NOT in the binary. These columns record which file belongs to an animal and
-- the credit that must be shown (CC-BY-SA requires author + licence on display).
-- Populated from data/images/attributions.json by `totem-seed --attributions`.

ALTER TABLE animal ADD COLUMN image_path    TEXT NOT NULL DEFAULT '';
ALTER TABLE animal ADD COLUMN image_author  TEXT NOT NULL DEFAULT '';
ALTER TABLE animal ADD COLUMN image_license TEXT NOT NULL DEFAULT '';
ALTER TABLE animal ADD COLUMN image_source  TEXT NOT NULL DEFAULT '';
