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
    return_fee_cents INTEGER,
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
    fee_cents INTEGER NOT NULL,
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

-- ===========================================================================
-- SEED DATA
-- ===========================================================================

-- Correspondent
INSERT INTO correspondents (id, code, name, omnibus_account_id)
VALUES ('00000000-0000-0000-0000-000000000010', 'ACME', 'ACME Brokerage', NULL);

-- Omnibus account
INSERT INTO accounts (id, external_account_id, correspondent_id, account_type, account_name)
VALUES ('00000000-0000-0000-0000-000000000001', 'OMNI-ACME', '00000000-0000-0000-0000-000000000010', 'OMNIBUS', 'ACME Omnibus');

-- Fee revenue account
INSERT INTO accounts (id, external_account_id, correspondent_id, account_type, account_name)
VALUES ('00000000-0000-0000-0000-000000000002', 'FEE-REVENUE', '00000000-0000-0000-0000-000000000010', 'FEE_REVENUE', 'Fee Revenue');

-- Investor accounts
INSERT INTO accounts (id, external_account_id, correspondent_id, account_type, account_name, contribution_type_default)
VALUES ('00000000-0000-0000-0000-000000001001', 'INV-1001', '00000000-0000-0000-0000-000000000010', 'INVESTOR', 'Test Investor 1001', 'INDIVIDUAL');

INSERT INTO accounts (id, external_account_id, correspondent_id, account_type, account_name, contribution_type_default)
VALUES ('00000000-0000-0000-0000-000000001002', 'INV-1002', '00000000-0000-0000-0000-000000000010', 'INVESTOR', 'Test Investor 1002', 'INDIVIDUAL');

INSERT INTO accounts (id, external_account_id, correspondent_id, account_type, account_name, contribution_type_default)
VALUES ('00000000-0000-0000-0000-000000001003', 'INV-1003', '00000000-0000-0000-0000-000000000010', 'INVESTOR', 'Test Investor 1003', 'INDIVIDUAL');

INSERT INTO accounts (id, external_account_id, correspondent_id, account_type, account_name, contribution_type_default)
VALUES ('00000000-0000-0000-0000-000000001004', 'INV-1004', '00000000-0000-0000-0000-000000000010', 'INVESTOR', 'Test Investor 1004', 'INDIVIDUAL');

INSERT INTO accounts (id, external_account_id, correspondent_id, account_type, account_name, contribution_type_default)
VALUES ('00000000-0000-0000-0000-000000001005', 'INV-1005', '00000000-0000-0000-0000-000000000010', 'INVESTOR', 'Test Investor 1005', 'INDIVIDUAL');

INSERT INTO accounts (id, external_account_id, correspondent_id, account_type, account_name, contribution_type_default)
VALUES ('00000000-0000-0000-0000-000000001006', 'INV-1006', '00000000-0000-0000-0000-000000000010', 'INVESTOR', 'Test Investor 1006', 'INDIVIDUAL');

INSERT INTO accounts (id, external_account_id, correspondent_id, account_type, account_name, contribution_type_default)
VALUES ('00000000-0000-0000-0000-000000001007', 'INV-1007', '00000000-0000-0000-0000-000000000010', 'INVESTOR', 'Test Investor 1007', 'INDIVIDUAL');

-- Link correspondent to omnibus account
UPDATE correspondents
SET omnibus_account_id = '00000000-0000-0000-0000-000000000001'
WHERE id = '00000000-0000-0000-0000-000000000010';

-- ===========================================================================
-- DEMO TRANSFERS
-- Pre-populated transfers covering every meaningful state so a reviewer
-- opening the app sees a realistic system rather than empty tables.
--
-- Accounts:
--   INV-1001  00000000-0000-0000-0000-000000001001  clean_pass
--   INV-1002  00000000-0000-0000-0000-000000001002  iqa_blur
--   INV-1004  00000000-0000-0000-0000-000000001004  micr_failure
--   INV-1006  00000000-0000-0000-0000-000000001006  amount_mismatch
-- ===========================================================================

-- T1: Completed — happy path, settled 2026-03-07
INSERT INTO transfers (id, investor_account_id, correspondent_id, omnibus_account_id,
    state, amount_cents, currency, contribution_type, business_date_ct,
    duplicate_fingerprint, submitted_at, approved_at, posted_at, completed_at,
    created_at, updated_at)
VALUES (
    '00000000-seed-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000001001',
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000001',
    'Completed', 50000, 'USD', 'INDIVIDUAL', '2026-03-07',
    'aabbcc001', '2026-03-07 14:00:00', '2026-03-07 14:00:02',
    '2026-03-07 14:00:02', '2026-03-07 18:05:00',
    '2026-03-07 14:00:00', '2026-03-07 18:05:00'
);

-- T2: Completed — settled same batch as T1
INSERT INTO transfers (id, investor_account_id, correspondent_id, omnibus_account_id,
    state, amount_cents, currency, contribution_type, business_date_ct,
    duplicate_fingerprint, submitted_at, approved_at, posted_at, completed_at,
    created_at, updated_at)
VALUES (
    '00000000-seed-0000-0000-000000000002',
    '00000000-0000-0000-0000-000000001001',
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000001',
    'Completed', 120000, 'USD', 'INDIVIDUAL', '2026-03-07',
    'aabbcc002', '2026-03-07 15:30:00', '2026-03-07 15:30:02',
    '2026-03-07 15:30:02', '2026-03-07 18:05:00',
    '2026-03-07 15:30:00', '2026-03-07 18:05:00'
);

-- T3: Returned — settled then bounced (NSF)
INSERT INTO transfers (id, investor_account_id, correspondent_id, omnibus_account_id,
    state, amount_cents, currency, contribution_type, business_date_ct,
    duplicate_fingerprint, return_reason_code, return_fee_cents,
    submitted_at, approved_at, posted_at, completed_at, returned_at,
    created_at, updated_at)
VALUES (
    '00000000-seed-0000-0000-000000000003',
    '00000000-0000-0000-0000-000000001001',
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000001',
    'Returned', 75000, 'USD', 'INDIVIDUAL', '2026-03-07',
    'aabbcc003', 'NSF', 3000,
    '2026-03-07 16:00:00', '2026-03-07 16:00:02',
    '2026-03-07 16:00:02', '2026-03-07 18:05:00', '2026-03-08 09:10:00',
    '2026-03-07 16:00:00', '2026-03-08 09:10:00'
);

-- T4: FundsPosted — today, awaiting settlement
INSERT INTO transfers (id, investor_account_id, correspondent_id, omnibus_account_id,
    state, amount_cents, currency, contribution_type, business_date_ct,
    duplicate_fingerprint, submitted_at, approved_at, posted_at,
    created_at, updated_at)
VALUES (
    '00000000-seed-0000-0000-000000000004',
    '00000000-0000-0000-0000-000000001001',
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000001',
    'FundsPosted', 250000, 'USD', 'INDIVIDUAL', '2026-03-09',
    'aabbcc004', '2026-03-09 10:15:00', '2026-03-09 10:15:02',
    '2026-03-09 10:15:02',
    '2026-03-09 10:15:00', '2026-03-09 10:15:02'
);

-- T5: Analyzing, review_required — amount mismatch (INV-1006)
INSERT INTO transfers (id, investor_account_id, correspondent_id, omnibus_account_id,
    state, amount_cents, currency, business_date_ct,
    review_required, review_status,
    submitted_at, created_at, updated_at)
VALUES (
    '00000000-seed-0000-0000-000000000005',
    '00000000-0000-0000-0000-000000001006',
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000001',
    'Analyzing', 45000, 'USD', '2026-03-09',
    1, 'PENDING',
    '2026-03-09 11:00:00', '2026-03-09 11:00:00', '2026-03-09 11:00:00'
);

-- T6: Analyzing, review_required — MICR failure (INV-1004)
INSERT INTO transfers (id, investor_account_id, correspondent_id, omnibus_account_id,
    state, amount_cents, currency, business_date_ct,
    review_required, review_status,
    submitted_at, created_at, updated_at)
VALUES (
    '00000000-seed-0000-0000-000000000006',
    '00000000-0000-0000-0000-000000001004',
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000001',
    'Analyzing', 80000, 'USD', '2026-03-09',
    1, 'PENDING',
    '2026-03-09 11:30:00', '2026-03-09 11:30:00', '2026-03-09 11:30:00'
);

-- T7: Rejected — IQA blur (INV-1002)
INSERT INTO transfers (id, investor_account_id, correspondent_id, omnibus_account_id,
    state, amount_cents, currency, business_date_ct,
    rejection_code, rejection_message,
    submitted_at, created_at, updated_at)
VALUES (
    '00000000-seed-0000-0000-000000000007',
    '00000000-0000-0000-0000-000000001002',
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000001',
    'Rejected', 20000, 'USD', '2026-03-09',
    'VENDOR_REJECT', 'Vendor decision FAIL: IQA=BLUR duplicate=false',
    '2026-03-09 12:00:00', '2026-03-09 12:00:00', '2026-03-09 12:00:00'
);

-- ===========================================================================
-- TRANSFER IMAGES
-- ===========================================================================

INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000001', '00000000-seed-0000-0000-000000000001', 'FRONT', 'data/images/seed/FRONT.jpg', 'aabbcc0001front', 'image/jpeg');
INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000002', '00000000-seed-0000-0000-000000000001', 'BACK',  'data/images/seed/BACK.jpg',  'aabbcc0001back',  'image/jpeg');

INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000003', '00000000-seed-0000-0000-000000000002', 'FRONT', 'data/images/seed/FRONT.jpg', 'aabbcc0002front', 'image/jpeg');
INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000004', '00000000-seed-0000-0000-000000000002', 'BACK',  'data/images/seed/BACK.jpg',  'aabbcc0002back',  'image/jpeg');

INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000005', '00000000-seed-0000-0000-000000000003', 'FRONT', 'data/images/seed/FRONT.jpg', 'aabbcc0003front', 'image/jpeg');
INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000006', '00000000-seed-0000-0000-000000000003', 'BACK',  'data/images/seed/BACK.jpg',  'aabbcc0003back',  'image/jpeg');

INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000007', '00000000-seed-0000-0000-000000000004', 'FRONT', 'data/images/seed/FRONT.jpg', 'aabbcc0004front', 'image/jpeg');
INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000008', '00000000-seed-0000-0000-000000000004', 'BACK',  'data/images/seed/BACK.jpg',  'aabbcc0004back',  'image/jpeg');

INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000009', '00000000-seed-0000-0000-000000000005', 'FRONT', 'data/images/seed/FRONT.jpg', 'aabbcc0005front', 'image/jpeg');
INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000010', '00000000-seed-0000-0000-000000000005', 'BACK',  'data/images/seed/BACK.jpg',  'aabbcc0005back',  'image/jpeg');

INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000011', '00000000-seed-0000-0000-000000000006', 'FRONT', 'data/images/seed/FRONT.jpg', 'aabbcc0006front', 'image/jpeg');
INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000012', '00000000-seed-0000-0000-000000000006', 'BACK',  'data/images/seed/BACK.jpg',  'aabbcc0006back',  'image/jpeg');

INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000013', '00000000-seed-0000-0000-000000000007', 'FRONT', 'data/images/seed/FRONT.jpg', 'aabbcc0007front', 'image/jpeg');
INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type)
VALUES ('00000000-seed-0001-0000-000000000014', '00000000-seed-0000-0000-000000000007', 'BACK',  'data/images/seed/BACK.jpg',  'aabbcc0007back',  'image/jpeg');

-- ===========================================================================
-- VENDOR RESULTS
-- ===========================================================================

-- T1: PASS
INSERT INTO vendor_results (id, transfer_id, vendor_transaction_id, decision, iqa_status,
    micr_routing_number, micr_account_number, micr_check_number, micr_confidence,
    ocr_amount_cents, amount_matches, duplicate_detected, risk_score, manual_review_required, raw_response_json)
VALUES ('00000000-seed-0002-0000-000000000001', '00000000-seed-0000-0000-000000000001',
    'vtxn-seed-001', 'PASS', 'PASS',
    '021000089', '123456789', '1001', 0.98,
    50000, 1, 0, 12, 0, '{"decision":"PASS"}');

-- T2: PASS
INSERT INTO vendor_results (id, transfer_id, vendor_transaction_id, decision, iqa_status,
    micr_routing_number, micr_account_number, micr_check_number, micr_confidence,
    ocr_amount_cents, amount_matches, duplicate_detected, risk_score, manual_review_required, raw_response_json)
VALUES ('00000000-seed-0002-0000-000000000002', '00000000-seed-0000-0000-000000000002',
    'vtxn-seed-002', 'PASS', 'PASS',
    '021000089', '123456789', '1002', 0.97,
    120000, 1, 0, 8, 0, '{"decision":"PASS"}');

-- T3: PASS (returned post-settlement, not vendor fault)
INSERT INTO vendor_results (id, transfer_id, vendor_transaction_id, decision, iqa_status,
    micr_routing_number, micr_account_number, micr_check_number, micr_confidence,
    ocr_amount_cents, amount_matches, duplicate_detected, risk_score, manual_review_required, raw_response_json)
VALUES ('00000000-seed-0002-0000-000000000003', '00000000-seed-0000-0000-000000000003',
    'vtxn-seed-003', 'PASS', 'PASS',
    '021000089', '123456789', '1003', 0.96,
    75000, 1, 0, 15, 0, '{"decision":"PASS"}');

-- T4: PASS
INSERT INTO vendor_results (id, transfer_id, vendor_transaction_id, decision, iqa_status,
    micr_routing_number, micr_account_number, micr_check_number, micr_confidence,
    ocr_amount_cents, amount_matches, duplicate_detected, risk_score, manual_review_required, raw_response_json)
VALUES ('00000000-seed-0002-0000-000000000004', '00000000-seed-0000-0000-000000000004',
    'vtxn-seed-004', 'PASS', 'PASS',
    '021000089', '987654321', '2001', 0.99,
    250000, 1, 0, 5, 0, '{"decision":"PASS"}');

-- T5: REVIEW — amount mismatch (OCR read $440, investor entered $450)
INSERT INTO vendor_results (id, transfer_id, vendor_transaction_id, decision, iqa_status,
    micr_routing_number, micr_account_number, micr_check_number, micr_confidence,
    ocr_amount_cents, amount_matches, duplicate_detected, risk_score, manual_review_required, raw_response_json)
VALUES ('00000000-seed-0002-0000-000000000005', '00000000-seed-0000-0000-000000000005',
    'vtxn-seed-005', 'REVIEW', 'PASS',
    '021000089', '555000111', '3001', 0.95,
    44000, 0, 0, 22, 1, '{"decision":"REVIEW","reason":"amount_mismatch"}');

-- T6: REVIEW — MICR failure (low confidence, no account number parsed)
INSERT INTO vendor_results (id, transfer_id, vendor_transaction_id, decision, iqa_status,
    micr_routing_number, micr_account_number, micr_check_number, micr_confidence,
    ocr_amount_cents, amount_matches, duplicate_detected, risk_score, manual_review_required, raw_response_json)
VALUES ('00000000-seed-0002-0000-000000000006', '00000000-seed-0000-0000-000000000006',
    'vtxn-seed-006', 'REVIEW', 'PASS',
    NULL, NULL, NULL, 0.21,
    80000, 1, 0, 35, 1, '{"decision":"REVIEW","reason":"micr_failure"}');

-- T7: FAIL — IQA blur
INSERT INTO vendor_results (id, transfer_id, vendor_transaction_id, decision, iqa_status,
    micr_routing_number, micr_account_number, micr_check_number, micr_confidence,
    ocr_amount_cents, amount_matches, duplicate_detected, risk_score, manual_review_required, raw_response_json)
VALUES ('00000000-seed-0002-0000-000000000007', '00000000-seed-0000-0000-000000000007',
    'vtxn-seed-007', 'FAIL', 'BLUR',
    NULL, NULL, NULL, NULL,
    NULL, 0, 0, 0, 0, '{"decision":"FAIL","iqa_status":"BLUR"}');

-- ===========================================================================
-- RULE EVALUATIONS (not run for T7 which was rejected by vendor before rules)
-- ===========================================================================

-- T1 rules
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0001-000000000001', '00000000-seed-0000-0000-000000000001', 'ACCOUNT_ELIGIBLE',        'PASS', '{"details":"account is ACTIVE"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0001-000000000002', '00000000-seed-0000-0000-000000000001', 'MAX_DEPOSIT_LIMIT',       'PASS', '{"details":"amount 50000 cents within limit"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0001-000000000003', '00000000-seed-0000-0000-000000000001', 'CONTRIBUTION_TYPE_DEFAULT','PASS', '{"details":"set contribution_type to account default INDIVIDUAL"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0001-000000000004', '00000000-seed-0000-0000-000000000001', 'INTERNAL_DUPLICATE',      'PASS', '{"details":"no duplicate found"}');

-- T2 rules
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0002-000000000001', '00000000-seed-0000-0000-000000000002', 'ACCOUNT_ELIGIBLE',        'PASS', '{"details":"account is ACTIVE"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0002-000000000002', '00000000-seed-0000-0000-000000000002', 'MAX_DEPOSIT_LIMIT',       'PASS', '{"details":"amount 120000 cents within limit"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0002-000000000003', '00000000-seed-0000-0000-000000000002', 'CONTRIBUTION_TYPE_DEFAULT','PASS', '{"details":"set contribution_type to account default INDIVIDUAL"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0002-000000000004', '00000000-seed-0000-0000-000000000002', 'INTERNAL_DUPLICATE',      'PASS', '{"details":"no duplicate found"}');

-- T3 rules
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0003-000000000001', '00000000-seed-0000-0000-000000000003', 'ACCOUNT_ELIGIBLE',        'PASS', '{"details":"account is ACTIVE"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0003-000000000002', '00000000-seed-0000-0000-000000000003', 'MAX_DEPOSIT_LIMIT',       'PASS', '{"details":"amount 75000 cents within limit"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0003-000000000003', '00000000-seed-0000-0000-000000000003', 'CONTRIBUTION_TYPE_DEFAULT','PASS', '{"details":"set contribution_type to account default INDIVIDUAL"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0003-000000000004', '00000000-seed-0000-0000-000000000003', 'INTERNAL_DUPLICATE',      'PASS', '{"details":"no duplicate found"}');

-- T4 rules
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0004-000000000001', '00000000-seed-0000-0000-000000000004', 'ACCOUNT_ELIGIBLE',        'PASS', '{"details":"account is ACTIVE"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0004-000000000002', '00000000-seed-0000-0000-000000000004', 'MAX_DEPOSIT_LIMIT',       'PASS', '{"details":"amount 250000 cents within limit"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0004-000000000003', '00000000-seed-0000-0000-000000000004', 'CONTRIBUTION_TYPE_DEFAULT','PASS', '{"details":"set contribution_type to account default INDIVIDUAL"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0004-000000000004', '00000000-seed-0000-0000-000000000004', 'INTERNAL_DUPLICATE',      'PASS', '{"details":"no duplicate found"}');

-- T5 rules (vendor REVIEW, rules still run)
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0005-000000000001', '00000000-seed-0000-0000-000000000005', 'ACCOUNT_ELIGIBLE',        'PASS', '{"details":"account is ACTIVE"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0005-000000000002', '00000000-seed-0000-0000-000000000005', 'MAX_DEPOSIT_LIMIT',       'PASS', '{"details":"amount 45000 cents within limit"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0005-000000000003', '00000000-seed-0000-0000-000000000005', 'CONTRIBUTION_TYPE_DEFAULT','PASS', '{"details":"no default contribution_type configured"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0005-000000000004', '00000000-seed-0000-0000-000000000005', 'INTERNAL_DUPLICATE',      'PASS', '{"details":"no duplicate found"}');

-- T6 rules (MICR failure — INTERNAL_DUPLICATE skips due to no MICR data)
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0006-000000000001', '00000000-seed-0000-0000-000000000006', 'ACCOUNT_ELIGIBLE',        'PASS', '{"details":"account is ACTIVE"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0006-000000000002', '00000000-seed-0000-0000-000000000006', 'MAX_DEPOSIT_LIMIT',       'PASS', '{"details":"amount 80000 cents within limit"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0006-000000000003', '00000000-seed-0000-0000-000000000006', 'CONTRIBUTION_TYPE_DEFAULT','PASS', '{"details":"no default contribution_type configured"}');
INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json) VALUES ('00000000-seed-0003-0006-000000000004', '00000000-seed-0000-0000-000000000006', 'INTERNAL_DUPLICATE',      'PASS', '{"details":"no MICR data available, skipping duplicate check"}');

-- ===========================================================================
-- AUDIT EVENTS
-- ===========================================================================

-- T1: Requested → Validating → Analyzing → Approved → FundsPosted → Completed
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0001-000000000001', 'transfer', '00000000-seed-0000-0000-000000000001', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Requested',   'Validating',  '2026-03-07 14:00:00');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0001-000000000002', 'transfer', '00000000-seed-0000-0000-000000000001', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Validating',  'Analyzing',   '2026-03-07 14:00:01');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0001-000000000003', 'transfer', '00000000-seed-0000-0000-000000000001', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Analyzing',   'Approved',    '2026-03-07 14:00:01');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0001-000000000004', 'transfer', '00000000-seed-0000-0000-000000000001', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Approved',    'FundsPosted', '2026-03-07 14:00:02');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0001-000000000005', 'transfer', '00000000-seed-0000-0000-000000000001', 'SYSTEM', 'settlement',      'STATE_TRANSITION', 'FundsPosted', 'Completed',   '2026-03-07 18:05:00');

-- T2: same happy path
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0002-000000000001', 'transfer', '00000000-seed-0000-0000-000000000002', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Requested',   'Validating',  '2026-03-07 15:30:00');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0002-000000000002', 'transfer', '00000000-seed-0000-0000-000000000002', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Validating',  'Analyzing',   '2026-03-07 15:30:01');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0002-000000000003', 'transfer', '00000000-seed-0000-0000-000000000002', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Analyzing',   'Approved',    '2026-03-07 15:30:01');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0002-000000000004', 'transfer', '00000000-seed-0000-0000-000000000002', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Approved',    'FundsPosted', '2026-03-07 15:30:02');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0002-000000000005', 'transfer', '00000000-seed-0000-0000-000000000002', 'SYSTEM', 'settlement',      'STATE_TRANSITION', 'FundsPosted', 'Completed',   '2026-03-07 18:05:00');

-- T3: happy path → Completed → Returned
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0003-000000000001', 'transfer', '00000000-seed-0000-0000-000000000003', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Requested',   'Validating',  '2026-03-07 16:00:00');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0003-000000000002', 'transfer', '00000000-seed-0000-0000-000000000003', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Validating',  'Analyzing',   '2026-03-07 16:00:01');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0003-000000000003', 'transfer', '00000000-seed-0000-0000-000000000003', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Analyzing',   'Approved',    '2026-03-07 16:00:01');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0003-000000000004', 'transfer', '00000000-seed-0000-0000-000000000003', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Approved',    'FundsPosted', '2026-03-07 16:00:02');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0003-000000000005', 'transfer', '00000000-seed-0000-0000-000000000003', 'SYSTEM', 'settlement',      'STATE_TRANSITION', 'FundsPosted', 'Completed',   '2026-03-07 18:05:00');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0003-000000000006', 'transfer', '00000000-seed-0000-0000-000000000003', 'SYSTEM', 'returns',         'STATE_TRANSITION', 'Completed',   'Returned',    '2026-03-08 09:10:00');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, details_json,                    created_at) VALUES ('00000000-seed-0004-0003-000000000007', 'transfer', '00000000-seed-0000-0000-000000000003', 'SYSTEM', 'returns',         'RETURN_PROCESSED', NULL, '2026-03-08 09:10:00');

-- T4: Requested → ... → FundsPosted
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0004-000000000001', 'transfer', '00000000-seed-0000-0000-000000000004', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Requested',   'Validating',  '2026-03-09 10:15:00');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0004-000000000002', 'transfer', '00000000-seed-0000-0000-000000000004', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Validating',  'Analyzing',   '2026-03-09 10:15:01');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0004-000000000003', 'transfer', '00000000-seed-0000-0000-000000000004', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Analyzing',   'Approved',    '2026-03-09 10:15:01');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0004-000000000004', 'transfer', '00000000-seed-0000-0000-000000000004', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Approved',    'FundsPosted', '2026-03-09 10:15:02');

-- T5: → Analyzing (review queue)
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0005-000000000001', 'transfer', '00000000-seed-0000-0000-000000000005', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Requested',  'Validating', '2026-03-09 11:00:00');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0005-000000000002', 'transfer', '00000000-seed-0000-0000-000000000005', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Validating', 'Analyzing',  '2026-03-09 11:00:01');

-- T6: → Analyzing (review queue)
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0006-000000000001', 'transfer', '00000000-seed-0000-0000-000000000006', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Requested',  'Validating', '2026-03-09 11:30:00');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0006-000000000002', 'transfer', '00000000-seed-0000-0000-000000000006', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Validating', 'Analyzing',  '2026-03-09 11:30:01');

-- T7: → Validating → Rejected
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0007-000000000001', 'transfer', '00000000-seed-0000-0000-000000000007', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Requested',  'Validating', '2026-03-09 12:00:00');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0007-000000000002', 'transfer', '00000000-seed-0000-0000-000000000007', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Validating', 'Analyzing',  '2026-03-09 12:00:01');
INSERT INTO audit_events (id, entity_type, entity_id, actor_type, actor_id, event_type, from_state, to_state, created_at) VALUES ('00000000-seed-0004-0007-000000000003', 'transfer', '00000000-seed-0000-0000-000000000007', 'SYSTEM', 'deposit-service', 'STATE_TRANSITION', 'Analyzing',  'Rejected',   '2026-03-09 12:00:01');

-- ===========================================================================
-- LEDGER — journals and entries
-- Double-entry: positive = credit, negative = debit. Must sum to zero.
-- ===========================================================================

-- T1 DEPOSIT_POSTING
INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at) VALUES ('00000000-seed-0005-0001-000000000001', '00000000-seed-0000-0000-000000000001', 'DEPOSIT_POSTING', 'Check deposit posting', '2026-03-07 14:00:02');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0001-000000000001', '00000000-seed-0005-0001-000000000001', '00000000-0000-0000-0000-000000001001',  50000, 'USD', 'PRINCIPAL');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0001-000000000002', '00000000-seed-0005-0001-000000000001', '00000000-0000-0000-0000-000000000001', -50000, 'USD', 'PRINCIPAL');

-- T2 DEPOSIT_POSTING
INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at) VALUES ('00000000-seed-0005-0002-000000000001', '00000000-seed-0000-0000-000000000002', 'DEPOSIT_POSTING', 'Check deposit posting', '2026-03-07 15:30:02');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0002-000000000001', '00000000-seed-0005-0002-000000000001', '00000000-0000-0000-0000-000000001001',  120000, 'USD', 'PRINCIPAL');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0002-000000000002', '00000000-seed-0005-0002-000000000001', '00000000-0000-0000-0000-000000000001', -120000, 'USD', 'PRINCIPAL');

-- T3 DEPOSIT_POSTING + RETURN_REVERSAL + RETURN_FEE
INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at) VALUES ('00000000-seed-0005-0003-000000000001', '00000000-seed-0000-0000-000000000003', 'DEPOSIT_POSTING',  'Check deposit posting', '2026-03-07 16:00:02');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0003-000000000001', '00000000-seed-0005-0003-000000000001', '00000000-0000-0000-0000-000000001001',  75000, 'USD', 'PRINCIPAL');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0003-000000000002', '00000000-seed-0005-0003-000000000001', '00000000-0000-0000-0000-000000000001', -75000, 'USD', 'PRINCIPAL');

INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at) VALUES ('00000000-seed-0005-0003-000000000002', '00000000-seed-0000-0000-000000000003', 'RETURN_REVERSAL',  'Return reversal',       '2026-03-08 09:10:00');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0003-000000000003', '00000000-seed-0005-0003-000000000002', '00000000-0000-0000-0000-000000001001', -75000, 'USD', 'PRINCIPAL');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0003-000000000004', '00000000-seed-0005-0003-000000000002', '00000000-0000-0000-0000-000000000001',  75000, 'USD', 'PRINCIPAL');

INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at) VALUES ('00000000-seed-0005-0003-000000000003', '00000000-seed-0000-0000-000000000003', 'RETURN_FEE',       'Return fee $30',        '2026-03-08 09:10:00');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0003-000000000005', '00000000-seed-0005-0003-000000000003', '00000000-0000-0000-0000-000000001001', -3000, 'USD', 'FEE');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0003-000000000006', '00000000-seed-0005-0003-000000000003', '00000000-0000-0000-0000-000000000002',  3000, 'USD', 'FEE');

-- T4 DEPOSIT_POSTING
INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at) VALUES ('00000000-seed-0005-0004-000000000001', '00000000-seed-0000-0000-000000000004', 'DEPOSIT_POSTING', 'Check deposit posting', '2026-03-09 10:15:02');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0004-000000000001', '00000000-seed-0005-0004-000000000001', '00000000-0000-0000-0000-000000001001',  250000, 'USD', 'PRINCIPAL');
INSERT INTO ledger_entries  (id, journal_id, account_id, signed_amount_cents, currency, line_type) VALUES ('00000000-seed-0006-0004-000000000002', '00000000-seed-0005-0004-000000000001', '00000000-0000-0000-0000-000000000001', -250000, 'USD', 'PRINCIPAL');

-- ===========================================================================
-- SETTLEMENT BATCH — acknowledged batch for 2026-03-07
-- ===========================================================================

INSERT INTO settlement_batches (id, business_date_ct, file_format, file_path, status,
    total_items, total_amount_cents, ack_reference, created_at, submitted_at, acknowledged_at)
VALUES (
    '00000000-seed-0007-0000-000000000001',
    '2026-03-07', 'X9_ICL',
    'reports/settlement/2026-03-07_seed-batch.x9',
    'ACKNOWLEDGED',
    3, 245000,
    'ACK-20260307-seed001',
    '2026-03-07 18:00:00',
    '2026-03-07 18:01:00',
    '2026-03-07 18:05:00'
);

INSERT INTO settlement_batch_items (id, batch_id, transfer_id, sequence_number, amount_cents, micr_snapshot_json)
VALUES ('00000000-seed-0008-0000-000000000001', '00000000-seed-0007-0000-000000000001', '00000000-seed-0000-0000-000000000001', 1, 50000,  '{"routingNumber":"021000089","accountNumber":"123456789","checkNumber":"1001"}');
INSERT INTO settlement_batch_items (id, batch_id, transfer_id, sequence_number, amount_cents, micr_snapshot_json)
VALUES ('00000000-seed-0008-0000-000000000002', '00000000-seed-0007-0000-000000000001', '00000000-seed-0000-0000-000000000002', 2, 120000, '{"routingNumber":"021000089","accountNumber":"123456789","checkNumber":"1002"}');
INSERT INTO settlement_batch_items (id, batch_id, transfer_id, sequence_number, amount_cents, micr_snapshot_json)
VALUES ('00000000-seed-0008-0000-000000000003', '00000000-seed-0007-0000-000000000001', '00000000-seed-0000-0000-000000000003', 3, 75000,  '{"routingNumber":"021000089","accountNumber":"123456789","checkNumber":"1003"}');

-- ===========================================================================
-- RETURN NOTIFICATIONS
-- ===========================================================================

INSERT INTO return_notifications (id, transfer_id, reason_code, reason_text, fee_cents, received_at, processed_at)
VALUES (
    '00000000-seed-0009-0000-000000000001',
    '00000000-seed-0000-0000-000000000003',
    'NSF', 'Insufficient funds', 3000,
    '2026-03-08 09:05:00', '2026-03-08 09:10:00'
);

-- ===========================================================================
-- NOTIFICATIONS OUTBOX
-- ===========================================================================

INSERT INTO notifications_outbox (id, transfer_id, channel, template_code, status,
    payload_json, created_at, sent_at)
VALUES (
    '00000000-seed-000a-0000-000000000001',
    '00000000-seed-0000-0000-000000000003',
    'EMAIL', 'RETURNED_CHECK', 'SENT',
    '{"investor":"INV-1001","amount":"$750.00","reason":"NSF"}',
    '2026-03-08 09:10:00', '2026-03-08 09:10:05'
);
