-- Migration 002: Give investor accounts realistic names for demo purposes.
-- The vendor stub still uses the account ID suffix for scenario routing,
-- so we only update the display name, not the external_account_id.

UPDATE accounts SET account_name = 'James Whitfield'    WHERE external_account_id = 'INV-1001';
UPDATE accounts SET account_name = 'Sarah Chen'         WHERE external_account_id = 'INV-1002';
UPDATE accounts SET account_name = 'Marcus Johnson'     WHERE external_account_id = 'INV-1003';
UPDATE accounts SET account_name = 'Priya Patel'        WHERE external_account_id = 'INV-1004';
UPDATE accounts SET account_name = 'David Okafor'       WHERE external_account_id = 'INV-1005';
UPDATE accounts SET account_name = 'Elena Rodriguez'    WHERE external_account_id = 'INV-1006';
UPDATE accounts SET account_name = 'Thomas Nguyen'      WHERE external_account_id = 'INV-1007';
