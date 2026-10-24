-- +goose Up
ALTER TABLE short_links
    ADD COLUMN title text NOT NULL DEFAULT '',
    ADD COLUMN tags jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD CONSTRAINT short_links_tags_array CHECK (jsonb_typeof(tags) = 'array');

-- +goose Down
ALTER TABLE short_links
    DROP CONSTRAINT IF EXISTS short_links_tags_array,
    DROP COLUMN IF EXISTS tags,
    DROP COLUMN IF EXISTS title;
