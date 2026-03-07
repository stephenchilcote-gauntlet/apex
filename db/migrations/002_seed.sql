-- Seed correspondent
INSERT INTO correspondents (id, code, name, omnibus_account_id)
VALUES ('00000000-0000-0000-0000-000000000010', 'ACME', 'ACME Brokerage', NULL);

-- Seed omnibus account
INSERT INTO accounts (id, external_account_id, correspondent_id, account_type, account_name)
VALUES ('00000000-0000-0000-0000-000000000001', 'OMNI-ACME', '00000000-0000-0000-0000-000000000010', 'OMNIBUS', 'ACME Omnibus');

-- Seed fee revenue account
INSERT INTO accounts (id, external_account_id, correspondent_id, account_type, account_name)
VALUES ('00000000-0000-0000-0000-000000000002', 'FEE-REVENUE', '00000000-0000-0000-0000-000000000010', 'FEE_REVENUE', 'Fee Revenue');

-- Seed investor accounts
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
