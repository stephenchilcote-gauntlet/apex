#!/usr/bin/env bash
# demo_all_scenarios.sh — Exercise ALL vendor scenarios, business rules, operator
# review, settlement, and return/reversal flows.
# Requires: curl, jq, running app (localhost:8080) and vendor stub (localhost:8081)
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
FRONT_IMG=$(mktemp /tmp/front_XXXX.png)
BACK_IMG=$(mktemp /tmp/back_XXXX.png)
trap 'rm -f "$FRONT_IMG" "$BACK_IMG"' EXIT

dd if=/dev/urandom of="$FRONT_IMG" bs=1024 count=4 2>/dev/null
dd if=/dev/urandom of="$BACK_IMG" bs=1024 count=4 2>/dev/null

submit_deposit() {
  local account="$1" amount="$2" scenario="$3"
  curl -sf -X POST "$BASE_URL/api/v1/deposits" \
    -F "investorAccountId=$account" \
    -F "amount=$amount" \
    -F "vendorScenario=$scenario" \
    -F "frontImage=@$FRONT_IMG" \
    -F "backImage=@$BACK_IMG"
}

header() {
  echo ""
  echo "================================================================"
  echo " $1"
  echo "================================================================"
}

PASS=0
FAIL=0

check_state() {
  local label="$1" transfer_id="$2" expected="$3"
  local actual
  actual=$(curl -sf "$BASE_URL/api/v1/deposits/$transfer_id" | jq -r '.transfer.state')
  if [ "$actual" = "$expected" ]; then
    echo "    ✅ $label: state=$actual (expected $expected)"
    PASS=$((PASS + 1))
  else
    echo "    ❌ $label: state=$actual (expected $expected)"
    FAIL=$((FAIL + 1))
  fi
}

check_field() {
  local label="$1" json="$2" field="$3" expected="$4"
  local actual
  actual=$(echo "$json" | jq -r "$field")
  if [ "$actual" = "$expected" ]; then
    echo "    ✅ $label: $field=$actual"
    PASS=$((PASS + 1))
  else
    echo "    ❌ $label: $field=$actual (expected $expected)"
    FAIL=$((FAIL + 1))
  fi
}

echo "============================================================"
echo " Mobile Check Deposit — All Scenarios Demo"
echo "============================================================"

# Reset
echo ""
echo ">>> Resetting test data..."
curl -sf -X POST "$BASE_URL/api/v1/test/reset" | jq .

# ---------------------------------------------------------------
header "Scenario 1: clean_pass (INV-1001, \$250.00)"
# ---------------------------------------------------------------
RESP=$(submit_deposit "INV-1001" "250.00" "clean_pass")
echo "$RESP" | jq .
TID_CLEAN=$(echo "$RESP" | jq -r '.transferId')
check_field "clean_pass" "$RESP" '.state' 'FundsPosted'
check_field "clean_pass" "$RESP" '.reviewRequired' 'false'

# ---------------------------------------------------------------
header "Scenario 2: iqa_blur (INV-1002, \$100.00)"
# ---------------------------------------------------------------
RESP=$(submit_deposit "INV-1002" "100.00" "iqa_blur")
echo "$RESP" | jq .
TID_BLUR=$(echo "$RESP" | jq -r '.transferId')
check_field "iqa_blur" "$RESP" '.state' 'Rejected'

# ---------------------------------------------------------------
header "Scenario 3: iqa_glare (INV-1003, \$100.00)"
# ---------------------------------------------------------------
RESP=$(submit_deposit "INV-1003" "100.00" "iqa_glare")
echo "$RESP" | jq .
TID_GLARE=$(echo "$RESP" | jq -r '.transferId')
check_field "iqa_glare" "$RESP" '.state' 'Rejected'

# ---------------------------------------------------------------
header "Scenario 4: micr_failure (INV-1004, \$300.00)"
# ---------------------------------------------------------------
RESP=$(submit_deposit "INV-1004" "300.00" "micr_failure")
echo "$RESP" | jq .
TID_MICR=$(echo "$RESP" | jq -r '.transferId')
check_field "micr_failure" "$RESP" '.state' 'Analyzing'
check_field "micr_failure" "$RESP" '.reviewRequired' 'true'

# ---------------------------------------------------------------
header "Scenario 5: duplicate_detected (INV-1005, \$200.00)"
# ---------------------------------------------------------------
RESP=$(submit_deposit "INV-1005" "200.00" "duplicate_detected")
echo "$RESP" | jq .
TID_DUP=$(echo "$RESP" | jq -r '.transferId')
check_field "duplicate_detected" "$RESP" '.state' 'Rejected'

# ---------------------------------------------------------------
header "Scenario 6: amount_mismatch (INV-1006, \$400.00)"
# ---------------------------------------------------------------
RESP=$(submit_deposit "INV-1006" "400.00" "amount_mismatch")
echo "$RESP" | jq .
TID_MISMATCH=$(echo "$RESP" | jq -r '.transferId')
check_field "amount_mismatch" "$RESP" '.state' 'Analyzing'
check_field "amount_mismatch" "$RESP" '.reviewRequired' 'true'

# ---------------------------------------------------------------
header "Scenario 7: iqa_pass_review (INV-1007, \$500.00)"
# ---------------------------------------------------------------
RESP=$(submit_deposit "INV-1007" "500.00" "iqa_pass_review")
echo "$RESP" | jq .
TID_REVIEW=$(echo "$RESP" | jq -r '.transferId')
check_field "iqa_pass_review" "$RESP" '.state' 'Analyzing'
check_field "iqa_pass_review" "$RESP" '.reviewRequired' 'true'

# ---------------------------------------------------------------
header "Scenario 8: Over-limit rejection (\$6000.00 > \$5000 limit)"
# ---------------------------------------------------------------
RESP=$(submit_deposit "INV-1001" "6000.00" "clean_pass")
echo "$RESP" | jq .
check_field "over-limit" "$RESP" '.state' 'Rejected'

# ---------------------------------------------------------------
header "Scenario 9: Operator review queue"
# ---------------------------------------------------------------
echo ">>> Fetching review queue..."
QUEUE=$(curl -sf "$BASE_URL/api/v1/operator/review-queue")
QUEUE_COUNT=$(echo "$QUEUE" | jq 'length')
echo "    Items in review queue: $QUEUE_COUNT"
echo "$QUEUE" | jq '.[].id'

if [ "$QUEUE_COUNT" -gt 0 ]; then
  echo ""
  echo "    ✅ Review queue has $QUEUE_COUNT item(s)"
  PASS=$((PASS + 1))
else
  echo "    ❌ Review queue is empty"
  FAIL=$((FAIL + 1))
fi

# ---------------------------------------------------------------
header "Scenario 10: Operator approves micr_failure transfer"
# ---------------------------------------------------------------
echo ">>> Approving transfer $TID_MICR..."
APPROVE_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/operator/transfers/$TID_MICR/approve" \
  -H "Content-Type: application/json" \
  -d '{"operatorId": "OP-001", "notes": "MICR manually verified"}')
echo "$APPROVE_RESP" | jq .
check_state "operator-approve" "$TID_MICR" "FundsPosted"

# ---------------------------------------------------------------
header "Scenario 11: Operator rejects amount_mismatch transfer"
# ---------------------------------------------------------------
echo ">>> Rejecting transfer $TID_MISMATCH..."
REJECT_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/operator/transfers/$TID_MISMATCH/reject" \
  -H "Content-Type: application/json" \
  -d '{"operatorId": "OP-001", "notes": "Amount discrepancy too large"}')
echo "$REJECT_RESP" | jq .
check_state "operator-reject" "$TID_MISMATCH" "Rejected"

# ---------------------------------------------------------------
header "Scenario 12: Settlement batch generation & acknowledgment"
# ---------------------------------------------------------------
echo ">>> Generating settlement batch..."
BATCH_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/settlement/batches/generate" \
  -H "Content-Type: application/json" \
  -d '{}')
echo "$BATCH_RESP" | jq .
BATCH_ID=$(echo "$BATCH_RESP" | jq -r '.id // .batchId // empty')
echo "    Batch ID: $BATCH_ID"

echo ""
echo ">>> Acknowledging batch..."
ACK_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/settlement/batches/$BATCH_ID/ack" \
  -H "Content-Type: application/json" \
  -d "{\"ackReference\": \"ACK-DEMO-$(date +%s)\"}")
echo "$ACK_RESP" | jq .

# Verify settled transfers moved to Completed
check_state "settlement-clean" "$TID_CLEAN" "Completed"
check_state "settlement-micr-approved" "$TID_MICR" "Completed"

# ---------------------------------------------------------------
header "Scenario 13: Return/reversal on completed transfer"
# ---------------------------------------------------------------
echo ">>> Processing return on transfer $TID_CLEAN..."
RETURN_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/returns" \
  -H "Content-Type: application/json" \
  -d "{\"transferId\": \"$TID_CLEAN\", \"reasonCode\": \"R01\", \"reasonText\": \"Insufficient funds\"}")
echo "$RETURN_RESP" | jq .
check_state "return" "$TID_CLEAN" "Returned"

# ---------------------------------------------------------------
header "Scenario 14: Ledger balances after all operations"
# ---------------------------------------------------------------
echo ">>> Ledger account balances..."
curl -sf "$BASE_URL/api/v1/ledger/accounts" | jq .

echo ""
echo ">>> Ledger journals for returned transfer $TID_CLEAN..."
curl -sf "$BASE_URL/api/v1/ledger/journals?transferId=$TID_CLEAN" | jq .

# ---------------------------------------------------------------
header "Summary"
# ---------------------------------------------------------------
TOTAL=$((PASS + FAIL))
echo ""
echo "    Passed: $PASS / $TOTAL"
echo "    Failed: $FAIL / $TOTAL"
echo ""
if [ "$FAIL" -eq 0 ]; then
  echo "    🎉 All scenarios passed!"
else
  echo "    ⚠️  Some scenarios failed — review output above."
fi
echo ""
echo "============================================================"
echo " All Scenarios Demo Complete"
echo "============================================================"
