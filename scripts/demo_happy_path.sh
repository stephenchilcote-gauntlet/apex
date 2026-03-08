#!/usr/bin/env bash
# demo_happy_path.sh — Full happy-path demo for Mobile Check Deposit System
# Requires: curl, jq, running app (localhost:8080) and vendor stub (localhost:8081)
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
FRONT_IMG=$(mktemp /tmp/front_XXXX.png)
BACK_IMG=$(mktemp /tmp/back_XXXX.png)
trap 'rm -f "$FRONT_IMG" "$BACK_IMG"' EXIT

dd if=/dev/urandom of="$FRONT_IMG" bs=1024 count=4 2>/dev/null
dd if=/dev/urandom of="$BACK_IMG" bs=1024 count=4 2>/dev/null

echo "============================================"
echo " Mobile Check Deposit — Happy Path Demo"
echo "============================================"
echo ""

# Step 0: Reset data
echo ">>> Step 0: Resetting test data..."
curl -sf -X POST "$BASE_URL/api/v1/test/reset" | jq .
echo ""

# Step 1: Submit deposit
echo ">>> Step 1: Submitting check deposit (INV-1001, \$250.00, clean_pass)..."
DEPOSIT_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/deposits" \
  -F "investorAccountId=INV-1001" \
  -F "amount=250.00" \
  -F "vendorScenario=clean_pass" \
  -F "frontImage=@$FRONT_IMG" \
  -F "backImage=@$BACK_IMG")
echo "$DEPOSIT_RESP" | jq .

TRANSFER_ID=$(echo "$DEPOSIT_RESP" | jq -r '.transferId')
STATE=$(echo "$DEPOSIT_RESP" | jq -r '.state')
echo "    Transfer ID: $TRANSFER_ID"
echo "    State: $STATE"
echo ""

# Step 2: Verify state is FundsPosted
echo ">>> Step 2: Verifying deposit state (expect FundsPosted)..."
curl -sf "$BASE_URL/api/v1/deposits/$TRANSFER_ID" | jq '.transfer.state'
echo ""

# Step 3: View decision trace
echo ">>> Step 3: Decision trace..."
curl -sf "$BASE_URL/api/v1/deposits/$TRANSFER_ID/decision-trace" | jq .
echo ""

# Step 4: View ledger journals for this transfer
echo ">>> Step 4: Ledger journals for this deposit..."
curl -sf "$BASE_URL/api/v1/ledger/journals?transferId=$TRANSFER_ID" | jq .
echo ""

# Step 5: Generate settlement batch
echo ">>> Step 5: Generating settlement batch..."
BATCH_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/settlement/batches/generate" \
  -H "Content-Type: application/json" \
  -d '{}')
echo "$BATCH_RESP" | jq .

BATCH_ID=$(echo "$BATCH_RESP" | jq -r '.id // .batchId // empty')
if [ -z "$BATCH_ID" ]; then
  echo "    (Extracting batchId from response...)"
  BATCH_ID=$(echo "$BATCH_RESP" | jq -r 'keys[] as $k | select($k | test("id"; "i")) | .[$k]' 2>/dev/null | head -1)
fi
echo "    Batch ID: $BATCH_ID"
echo ""

# Step 6: Acknowledge settlement batch
echo ">>> Step 6: Acknowledging settlement batch..."
curl -sf -X POST "$BASE_URL/api/v1/settlement/batches/$BATCH_ID/ack" \
  -H "Content-Type: application/json" \
  -d "{\"ackReference\": \"ACK-$(date +%s)\"}" | jq .
echo ""

# Step 7: Verify final state is Completed
echo ">>> Step 7: Final deposit state (expect Completed)..."
FINAL=$(curl -sf "$BASE_URL/api/v1/deposits/$TRANSFER_ID")
echo "$FINAL" | jq '.transfer.state'
echo ""

echo "============================================"
echo " Happy Path Demo Complete!"
echo "============================================"
echo " Deposit $TRANSFER_ID: Submitted → FundsPosted → Completed"
