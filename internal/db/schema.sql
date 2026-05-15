-- IOU schema.
-- Applied idempotently on startup; tables are added per build phase.

-- Phase 1: magic-link authentication.
CREATE TABLE IF NOT EXISTS users (
    id           TEXT PRIMARY KEY,
    email        TEXT UNIQUE NOT NULL,
    venmo_handle TEXT,
    created_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS magic_links (
    token      TEXT PRIMARY KEY,
    email      TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    used       INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);

-- Phase 2: bill creation and receipt parsing.
-- A service charge is a mandatory restaurant fee, separate from tip. Its
-- kind controls how it splits: 'percent' prorates over item subtotals (like
-- tax), 'fixed' splits evenly across service_charge_headcount diners. See the
-- service_charge_* columns; 'none' means there is no service charge.
CREATE TABLE IF NOT EXISTS bills (
    id                       TEXT PRIMARY KEY,
    host_user_id             TEXT NOT NULL REFERENCES users(id),
    restaurant               TEXT,
    currency                 TEXT NOT NULL DEFAULT 'USD',
    tax_cents                INTEGER NOT NULL DEFAULT 0,
    tip_cents                INTEGER NOT NULL DEFAULT 0,
    service_charge_kind      TEXT NOT NULL DEFAULT 'none',
    service_charge_rate_bps  INTEGER NOT NULL DEFAULT 0,
    service_charge_cents     INTEGER NOT NULL DEFAULT 0,
    service_charge_headcount INTEGER NOT NULL DEFAULT 0,
    status                   TEXT NOT NULL DEFAULT 'draft',
    split_mode               TEXT NOT NULL DEFAULT 'claim',
    audio_transcript         TEXT,
    friend_token             TEXT NOT NULL UNIQUE,
    created_at               INTEGER NOT NULL
);

-- Each row is one claimable unit; multi-quantity receipt lines are expanded
-- into separate rows at parse time so each unit can be claimed individually.
CREATE TABLE IF NOT EXISTS items (
    id          TEXT PRIMARY KEY,
    bill_id     TEXT NOT NULL REFERENCES bills(id),
    name        TEXT NOT NULL,
    price_cents INTEGER NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0
);

-- Phase 3: friend split flow.
CREATE TABLE IF NOT EXISTS participants (
    id                TEXT PRIMARY KEY,
    bill_id           TEXT NOT NULL REFERENCES bills(id),
    display_name      TEXT NOT NULL,
    participant_token TEXT NOT NULL UNIQUE,
    host_managed      INTEGER NOT NULL DEFAULT 0,
    is_host           INTEGER NOT NULL DEFAULT 0,
    created_at        INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS claims (
    item_id        TEXT NOT NULL REFERENCES items(id),
    participant_id TEXT NOT NULL REFERENCES participants(id),
    PRIMARY KEY (item_id, participant_id)
);

-- Phase 4: Venmo payments.
-- A payment tracks one friend settling their share. recipient is the host's
-- Venmo handle; status is 'pending' until the friend self-reports or the host
-- confirms it 'paid'. The provider and tx_ref columns are vestigial from the
-- earlier USDC design — Venmo gives the app no on-chain reference — and are
-- always written as 'venmo' / NULL respectively.
CREATE TABLE IF NOT EXISTS payments (
    id             TEXT PRIMARY KEY,
    bill_id        TEXT NOT NULL REFERENCES bills(id),
    participant_id TEXT NOT NULL UNIQUE REFERENCES participants(id),
    amount_cents   INTEGER NOT NULL,
    currency       TEXT NOT NULL,
    recipient      TEXT NOT NULL,
    status         TEXT NOT NULL,
    provider       TEXT NOT NULL,
    tx_ref         TEXT,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
);
