-- +goose Up

-- One copy of a video per queue; checked by the database so two
-- adds at once cannot both get in.
CREATE UNIQUE INDEX queue_items_one_per_video ON queue_items (account_id, video_id);

-- Moving songs shifts many positions in one statement; check uniqueness when the transaction ends.
ALTER TABLE queue_items DROP CONSTRAINT queue_items_account_id_position_key;
ALTER TABLE queue_items ADD CONSTRAINT queue_items_account_id_position_key
    UNIQUE (account_id, position) DEFERRABLE INITIALLY DEFERRED;

-- +goose Down

ALTER TABLE queue_items DROP CONSTRAINT queue_items_account_id_position_key;
ALTER TABLE queue_items ADD CONSTRAINT queue_items_account_id_position_key UNIQUE (account_id, position);
DROP INDEX queue_items_one_per_video;
