-- Schema for the slots service (PostgreSQL).
-- Matches what GORM AutoMigrate generates from models/user.go, models/slot.go and models/slot_history.go.

CREATE TABLE IF NOT EXISTS users (
    id    BIGSERIAL    PRIMARY KEY,
    name  VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users (email);

CREATE TABLE IF NOT EXISTS slots (
    id         BIGSERIAL   PRIMARY KEY,
    date       DATE        NOT NULL,
    "from"     TIMESTAMPTZ NOT NULL,
    "to"       TIMESTAMPTZ NOT NULL,
    status     SMALLINT    NOT NULL DEFAULT 0, -- 0 = not booked, 50 = on hold, 100 = booked
    booked_by  BIGINT,                         -- users.id of the booker or holder; NULL when not booked
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,

    CONSTRAINT fk_slots_user FOREIGN KEY (booked_by)
        REFERENCES users (id) ON UPDATE CASCADE ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_slots_date      ON slots (date);
CREATE INDEX IF NOT EXISTS idx_slots_status    ON slots (status);
CREATE INDEX IF NOT EXISTS idx_slots_booked_by ON slots (booked_by);

-- One row per booking change made by a user.
-- For a reschedule, slot_id is the slot moved to and previous_slot_id the slot moved from;
-- for booked/unbooked, previous_slot_id is NULL.
CREATE TABLE IF NOT EXISTS slot_history (
    id               BIGSERIAL   PRIMARY KEY,
    user_id          BIGINT      NOT NULL,
    slot_id          BIGINT      NOT NULL,
    previous_slot_id BIGINT,
    action           SMALLINT    NOT NULL, -- 1 = booked, 2 = unbooked, 3 = rescheduled, 4 = held
    created_at       TIMESTAMPTZ,

    CONSTRAINT fk_slot_history_user FOREIGN KEY (user_id)
        REFERENCES users (id) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT fk_slot_history_slot FOREIGN KEY (slot_id)
        REFERENCES slots (id) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT fk_slot_history_previous_slot FOREIGN KEY (previous_slot_id)
        REFERENCES slots (id) ON UPDATE CASCADE ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_slot_history_user_created     ON slot_history (user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_slot_history_slot_id          ON slot_history (slot_id);
CREATE INDEX IF NOT EXISTS idx_slot_history_previous_slot_id ON slot_history (previous_slot_id);
