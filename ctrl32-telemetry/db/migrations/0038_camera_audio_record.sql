-- Sound only live (spec 020, 0.56.0): a camera's sound can be heard without being recorded, to save disk space.
-- Cameras whose sound was already on keep recording it.
ALTER TABLE core.camera ADD COLUMN audio_record boolean NOT NULL DEFAULT true;
