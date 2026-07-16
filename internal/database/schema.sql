-- Accounts. A user may sign in with a password, with Google, or both, so each
-- credential column is nullable: password_hash is NULL for Google-only
-- accounts, google_id is NULL until a Google account is linked.
--
-- Emails are stored lowercased by the application, which is what makes the
-- UNIQUE constraint behave case-insensitively.
CREATE TABLE IF NOT EXISTS users (
    id            SERIAL PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    name          TEXT        NOT NULL DEFAULT '',
    password_hash TEXT,
    google_id     TEXT        UNIQUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Sessions issued at login. Only the SHA-256 of each token is stored: a leaked
-- database dump therefore yields no usable session tokens.
CREATE TABLE IF NOT EXISTS sessions (
    token_hash BYTEA       PRIMARY KEY,
    user_id    INTEGER     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Supports the cascade on user deletion and the periodic expiry sweep.
CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE IF NOT EXISTS tasks (
    id        SERIAL PRIMARY KEY,
    title     TEXT    NOT NULL DEFAULT '',
    completed BOOLEAN NOT NULL DEFAULT FALSE
);

-- Tasks became user-owned when authentication was added. Existing tasks predate
-- the column and have no owner, so it cannot simply be declared NOT NULL.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;

-- Enforce ownership as soon as the data allows it: on a fresh database that is
-- immediately, and on one carrying pre-auth tasks it is once those orphans are
-- adopted or removed. Until then the rows are simply invisible — every query is
-- filtered by user_id — rather than deleted, so no data is lost silently.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM tasks WHERE user_id IS NULL) THEN
        ALTER TABLE tasks ALTER COLUMN user_id SET NOT NULL;
    END IF;
END $$;

-- Every task query filters by owner.
CREATE INDEX IF NOT EXISTS tasks_user_id_idx ON tasks (user_id);
