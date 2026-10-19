-- +goose Up

CREATE TABLE accounts (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rooms (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    handle             text NOT NULL UNIQUE CHECK (handle ~ '^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$'),
    rev                bigint NOT NULL DEFAULT 1 CHECK (rev >= 1),
    skip_policy        smallint NOT NULL DEFAULT 1 CHECK (skip_policy IN (1, 2)),
    skip_votes         smallint NOT NULL DEFAULT 50 CHECK (skip_votes BETWEEN 1 AND 100),
    last_token         bigint NOT NULL DEFAULT 0 CHECK (last_token >= 0),
    booth_head_leaving boolean NOT NULL DEFAULT false,
    created_at         timestamptz NOT NULL DEFAULT now(),
    -- Restoring a room counts away members' grace from here, when the server last saved it.
    saved_at           timestamptz NOT NULL DEFAULT now()
);

-- Roles 2 member, 3 DJ, 4 owner, as room.Role. Guests are never stored.
CREATE TABLE memberships (
    room_id    bigint NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    account_id bigint NOT NULL REFERENCES accounts (id),
    role       smallint NOT NULL CHECK (role IN (2, 3, 4)),
    joined_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, account_id)
);

CREATE UNIQUE INDEX memberships_one_owner ON memberships (room_id) WHERE role = 4;
CREATE INDEX memberships_account ON memberships (account_id);

CREATE TABLE room_bans (
    room_id    bigint NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    account_id bigint NOT NULL REFERENCES accounts (id),
    banned_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, account_id)
);

-- A song queue belongs to the account and is shared by every room it DJs in.
CREATE TABLE queue_items (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id  bigint NOT NULL REFERENCES accounts (id),
    video_id    text NOT NULL CHECK (video_id ~ '^[A-Za-z0-9_-]{11}$'),
    title       text NOT NULL,
    channel     text NOT NULL DEFAULT '',
    thumbnail   text NOT NULL DEFAULT '',
    duration_ms bigint CHECK (duration_ms > 0),
    position    bigint NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account_id, position)
);

-- Only members sit in the booth; leaving the room clears the seat.
CREATE TABLE booth_seats (
    room_id    bigint NOT NULL,
    position   integer NOT NULL CHECK (position >= 0),
    account_id bigint NOT NULL,
    PRIMARY KEY (room_id, position),
    UNIQUE (room_id, account_id),
    FOREIGN KEY (room_id, account_id) REFERENCES memberships (room_id, account_id) ON DELETE CASCADE
);

-- One row per room, the stage as room.Phase: 1 idle, 2 choosing, 3 playing, 4 resting.
-- The track is copied in, so a play goes on even if its queue item is deleted.
CREATE TABLE room_stage (
    room_id     bigint PRIMARY KEY REFERENCES rooms (id) ON DELETE CASCADE,
    phase       smallint NOT NULL CHECK (phase BETWEEN 1 AND 4),
    dj          bigint REFERENCES accounts (id),
    passed      integer NOT NULL DEFAULT 0 CHECK (passed >= 0),
    failed      bigint[] NOT NULL DEFAULT '{}',
    votes       bigint[] NOT NULL DEFAULT '{}',
    play_token  bigint,
    item_id     bigint,
    video_id    text CHECK (video_id ~ '^[A-Za-z0-9_-]{11}$'),
    title       text,
    channel     text,
    thumbnail   text,
    duration_ms bigint CHECK (duration_ms > 0),
    rest_reason smallint CHECK (rest_reason IN (1, 2)),
    rest_until  timestamptz,
    CHECK (CASE phase
        WHEN 1 THEN dj IS NULL AND play_token IS NULL AND rest_reason IS NULL
        WHEN 2 THEN dj IS NOT NULL AND play_token IS NULL AND rest_reason IS NULL
        WHEN 3 THEN dj IS NOT NULL AND play_token IS NOT NULL AND item_id IS NOT NULL
                    AND video_id IS NOT NULL AND title IS NOT NULL AND rest_reason IS NULL
        WHEN 4 THEN dj IS NOT NULL AND play_token IS NULL AND rest_reason IS NOT NULL AND rest_until IS NOT NULL
    END)
);

-- Reasons 1 finished, 2 skipped, 3 voted out, 4 unplayable, as room.EndReason.
-- One row per play token, so saving the same end twice cannot record it twice.
CREATE TABLE play_history (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id     bigint NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    play_token  bigint NOT NULL,
    dj          bigint NOT NULL REFERENCES accounts (id),
    item_id     bigint,
    video_id    text NOT NULL CHECK (video_id ~ '^[A-Za-z0-9_-]{11}$'),
    title       text NOT NULL,
    duration_ms bigint CHECK (duration_ms > 0),
    reason      smallint NOT NULL CHECK (reason BETWEEN 1 AND 4),
    ended_at    timestamptz NOT NULL,
    UNIQUE (room_id, play_token)
);

CREATE INDEX play_history_recent ON play_history (room_id, ended_at DESC);

-- Chat is kept forever; nothing deletes old messages.
CREATE TABLE chat_messages (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id    bigint NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    account_id bigint NOT NULL REFERENCES accounts (id),
    body       text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 300),
    said_at    timestamptz NOT NULL
);

CREATE INDEX chat_messages_recent ON chat_messages (room_id, said_at DESC);

-- The app writes rooms and queues but can only add to history and chat, never rewrite them.
GRANT SELECT, INSERT ON accounts TO synqsong_app;
GRANT SELECT, INSERT, UPDATE ON rooms TO synqsong_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON memberships, room_bans, queue_items, booth_seats, room_stage TO synqsong_app;
GRANT SELECT, INSERT ON play_history, chat_messages TO synqsong_app;

-- +goose Down

DROP TABLE chat_messages, play_history, room_stage, booth_seats, queue_items, room_bans, memberships, rooms, accounts;
