-- Remove test-only scenario columns from production tables
ALTER TABLE transfers DROP COLUMN vendor_scenario;
ALTER TABLE vendor_results DROP COLUMN scenario;
