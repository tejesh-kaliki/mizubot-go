-- +goose Up
-- +goose StatementBegin
ALTER TABLE typesafe_classification_logs ADD COLUMN matched_flags TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE typesafe_classification_logs DROP COLUMN matched_flags;
-- +goose StatementEnd
