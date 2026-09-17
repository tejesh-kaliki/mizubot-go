-- +goose Up
-- +goose StatementBegin
ALTER TABLE typesafe_classification_logs ADD COLUMN message_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_typesafe_classification_logs_message
ON typesafe_classification_logs(message_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_typesafe_classification_logs_message;
ALTER TABLE typesafe_classification_logs DROP COLUMN message_id;
-- +goose StatementEnd
