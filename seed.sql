-- Seed data: 1 user + 20 open slots.
-- 10 one-hour slots per day (09:00–19:00 IST) on 2026-10-09 and 2026-10-10.
-- Run after db.sql:  psql -h localhost -U postgres -d slots -f seed.sql

BEGIN;

INSERT INTO users (name, email)
VALUES ('Akshay Herle', 'akshayherle2000@gmail.com')
ON CONFLICT (email) DO NOTHING;

INSERT INTO slots (date, "from", "to", status, booked_by, created_at, updated_at)
SELECT
    d::date,
    (d::date + make_interval(hours => h))     AT TIME ZONE 'Asia/Kolkata',
    (d::date + make_interval(hours => h + 1)) AT TIME ZONE 'Asia/Kolkata',
    0,      -- not booked
    NULL,
    now(),
    now()
FROM generate_series(DATE '2026-10-09', DATE '2026-10-10', INTERVAL '1 day') AS d,
     generate_series(9, 18) AS h
ORDER BY d, h;

COMMIT;
