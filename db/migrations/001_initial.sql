PRAGMA foreign_keys = ON;

-- correspondents
CREATE TABLE correspondents (
    id TEXT PRIMARY KEY,
    code TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    omnibus_account_id TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- accounts
CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    external_account_id TEXT UNIQUE NOT NULL,
    correspondent_id TEXT REFERENCES correspondents(id),
    account_type TEXT NOT NULL CHECK(account_type IN ('INVESTOR','OMNIBUS','FEE_REVENUE')),
    account_name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK(status IN ('ACTIVE','BLOCKED','CLOSED')),
    currency TEXT NOT NULL DEFAULT 'USD',
    contribution_type_default TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- transfers
CREATE TABLE transfers (
    id TEXT PRIMARY KEY,
    client_request_id TEXT UNIQUE,
    investor_account_id TEXT NOT NULL REFERENCES accounts(id),
    correspondent_id TEXT NOT NULL REFERENCES correspondents(id),
    omnibus_account_id TEXT NOT NULL REFERENCES accounts(id),
    state TEXT NOT NULL DEFAULT 'Requested' CHECK(state IN ('Requested','Validating','Analyzing','Approved','FundsPosted','Completed','Rejected','Returned')),
    amount_cents INTEGER NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    contribution_type TEXT,
    review_required INTEGER NOT NULL DEFAULT 0,
    review_status TEXT CHECK(review_status IN ('PENDING','APPROVED','REJECTED')),
    business_date_ct DATE,
    rejection_code TEXT,
    rejection_message TEXT,
    return_reason_code TEXT,
    return_fee_cents INTEGER DEFAULT 3000,
    duplicate_fingerprint TEXT,
    submitted_at DATETIME,
    approved_at DATETIME,
    posted_at DATETIME,
    completed_at DATETIME,
    returned_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- transfer_images
CREATE TABLE transfer_images (
    id TEXT PRIMARY KEY,
    transfer_id TEXT NOT NULL REFERENCES transfers(id),
    side TEXT NOT NULL CHECK(side IN ('FRONT','BACK')),
    file_path TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(transfer_id, side)
);

-- vendor_results
CREATE TABLE vendor_results (
    id TEXT PRIMARY KEY,
    transfer_id TEXT NOT NULL UNIQUE REFERENCES transfers(id),
    vendor_transaction_id TEXT NOT NULL,
    decision TEXT NOT NULL CHECK(decision IN ('PASS','FAIL','REVIEW')),
    iqa_status TEXT NOT NULL,
    micr_routing_number TEXT,
    micr_account_number TEXT,
    micr_check_number TEXT,
    micr_confidence REAL,
    ocr_amount_cents INTEGER,
    amount_matches INTEGER NOT NULL DEFAULT 0,
    duplicate_detected INTEGER NOT NULL DEFAULT 0,
    risk_score INTEGER NOT NULL DEFAULT 0,
    manual_review_required INTEGER NOT NULL DEFAULT 0,
    raw_response_json TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- rule_evaluations
CREATE TABLE rule_evaluations (
    id TEXT PRIMARY KEY,
    transfer_id TEXT NOT NULL REFERENCES transfers(id),
    rule_name TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK(outcome IN ('PASS','FAIL','WARN')),
    details_json TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- operator_actions
CREATE TABLE operator_actions (
    id TEXT PRIMARY KEY,
    transfer_id TEXT NOT NULL REFERENCES transfers(id),
    operator_id TEXT NOT NULL,
    action TEXT NOT NULL CHECK(action IN ('APPROVE','REJECT','OVERRIDE_CONTRIBUTION')),
    notes TEXT,
    override_contribution_type TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- audit_events
CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    actor_type TEXT NOT NULL CHECK(actor_type IN ('SYSTEM','OPERATOR','API')),
    actor_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    from_state TEXT,
    to_state TEXT,
    details_json TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- ledger_journals
CREATE TABLE ledger_journals (
    id TEXT PRIMARY KEY,
    transfer_id TEXT NOT NULL REFERENCES transfers(id),
    journal_type TEXT NOT NULL CHECK(journal_type IN ('DEPOSIT_POSTING','RETURN_REVERSAL','RETURN_FEE')),
    memo TEXT,
    effective_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- ledger_entries
CREATE TABLE ledger_entries (
    id TEXT PRIMARY KEY,
    journal_id TEXT NOT NULL REFERENCES ledger_journals(id),
    account_id TEXT NOT NULL REFERENCES accounts(id),
    signed_amount_cents INTEGER NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    line_type TEXT NOT NULL CHECK(line_type IN ('PRINCIPAL','FEE')),
    source_application_id TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- settlement_batches
CREATE TABLE settlement_batches (
    id TEXT PRIMARY KEY,
    business_date_ct DATE NOT NULL,
    cutoff_at_ct DATETIME,
    file_format TEXT NOT NULL DEFAULT 'X9_ICL',
    file_path TEXT,
    status TEXT NOT NULL DEFAULT 'GENERATED' CHECK(status IN ('GENERATED','SUBMITTED','ACKNOWLEDGED','FAILED')),
    total_items INTEGER NOT NULL DEFAULT 0,
    total_amount_cents INTEGER NOT NULL DEFAULT 0,
    ack_reference TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    submitted_at DATETIME,
    acknowledged_at DATETIME
);

-- settlement_batch_items
CREATE TABLE settlement_batch_items (
    id TEXT PRIMARY KEY,
    batch_id TEXT NOT NULL REFERENCES settlement_batches(id),
    transfer_id TEXT NOT NULL UNIQUE REFERENCES transfers(id),
    sequence_number INTEGER NOT NULL,
    amount_cents INTEGER NOT NULL,
    micr_snapshot_json TEXT,
    front_image_path TEXT,
    back_image_path TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- return_notifications
CREATE TABLE return_notifications (
    id TEXT PRIMARY KEY,
    transfer_id TEXT NOT NULL REFERENCES transfers(id),
    reason_code TEXT NOT NULL,
    reason_text TEXT,
    fee_cents INTEGER NOT NULL DEFAULT 3000,
    raw_payload_json TEXT,
    received_at DATETIME NOT NULL DEFAULT (datetime('now')),
    processed_at DATETIME
);

-- notifications_outbox
CREATE TABLE notifications_outbox (
    id TEXT PRIMARY KEY,
    transfer_id TEXT NOT NULL REFERENCES transfers(id),
    channel TEXT NOT NULL DEFAULT 'EMAIL',
    recipient_ref TEXT,
    template_code TEXT NOT NULL,
    payload_json TEXT,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','SENT')),
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    sent_at DATETIME
);

-- Indices
CREATE INDEX idx_transfers_state ON transfers(state);
CREATE INDEX idx_transfers_review ON transfers(state, review_required, review_status);
CREATE INDEX idx_transfers_business_date ON transfers(business_date_ct);
CREATE INDEX idx_transfers_investor ON transfers(investor_account_id);
CREATE INDEX idx_transfers_fingerprint ON transfers(duplicate_fingerprint);
CREATE INDEX idx_audit_entity ON audit_events(entity_type, entity_id);
CREATE INDEX idx_audit_created ON audit_events(created_at);
CREATE INDEX idx_ledger_entries_account ON ledger_entries(account_id);
CREATE INDEX idx_ledger_entries_journal ON ledger_entries(journal_id);
