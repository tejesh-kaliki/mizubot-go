-- +goose Up
-- +goose StatementBegin
-- The default backfills every existing row as a tool/flag classification,
-- which is all this table held before other judgements were logged here.
ALTER TABLE typesafe_classification_logs ADD COLUMN kind TEXT NOT NULL DEFAULT 'classifier';
-- +goose StatementEnd

-- +goose StatementBegin
-- Rows from the anime pick and embed filter judges, which used to label
-- themselves in selected_tools before this column existed.
UPDATE typesafe_classification_logs SET kind = 'anilist_pick' WHERE selected_tools LIKE '["anilist_pick"%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE typesafe_classification_logs SET kind = 'embed_filter' WHERE selected_tools LIKE '["embed_filter"%';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE typesafe_classification_logs DROP COLUMN kind;
-- +goose StatementEnd
