package transfers

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func migrationsDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..", "db", "migrations")
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file::memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	mDir := migrationsDir()
	for _, name := range []string{"001_init.sql"} {
		content, err := os.ReadFile(filepath.Join(mDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatalf("migration %s: %v", name, err)
		}
	}
	return db
}

func seedTransfer(t *testing.T, db *sql.DB, state State) *Transfer {
	t.Helper()
	svc := &TransferService{}
	tr := &Transfer{
		InvestorAccountID: "00000000-0000-0000-0000-000000001001",
		CorrespondentID:   "00000000-0000-0000-0000-000000000010",
		OmnibusAccountID:  "00000000-0000-0000-0000-000000000001",
		AmountCents:       10000,
		Currency:          "USD",
		State:             state,
	}
	if err := svc.Create(db, tr); err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestTransferService_Transition_Valid(t *testing.T) {
	db := newTestDB(t)
	svc := &TransferService{}
	tr := seedTransfer(t, db, StateRequested)

	err := svc.Transition(db, tr.ID, StateValidating, "SYSTEM", "test")
	if err != nil {
		t.Fatalf("valid transition failed: %v", err)
	}

	got, err := svc.GetByID(db, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateValidating {
		t.Errorf("state = %s, want Validating", got.State)
	}

	// Verify audit event
	var count int
	var fromState, toState, actorType, actorID string
	err = db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE entity_id = ? AND event_type = 'STATE_TRANSITION'`, tr.ID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("audit events = %d, want 1", count)
	}
	err = db.QueryRow(`SELECT from_state, to_state, actor_type, actor_id FROM audit_events WHERE entity_id = ? AND event_type = 'STATE_TRANSITION'`, tr.ID).
		Scan(&fromState, &toState, &actorType, &actorID)
	if err != nil {
		t.Fatal(err)
	}
	if fromState != "Requested" || toState != "Validating" {
		t.Errorf("audit from/to = %s/%s, want Requested/Validating", fromState, toState)
	}
	if actorType != "SYSTEM" || actorID != "test" {
		t.Errorf("audit actor = %s/%s, want SYSTEM/test", actorType, actorID)
	}
}

func TestTransferService_Transition_Invalid(t *testing.T) {
	db := newTestDB(t)
	svc := &TransferService{}
	tr := seedTransfer(t, db, StateRequested)

	err := svc.Transition(db, tr.ID, StateApproved, "SYSTEM", "test")
	if err == nil {
		t.Fatal("expected error for invalid transition")
	}

	got, err := svc.GetByID(db, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateRequested {
		t.Errorf("state = %s, want Requested (unchanged)", got.State)
	}

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE entity_id = ?`, tr.ID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("audit events = %d, want 0 (no side effects)", count)
	}
}

func ptrState(s State) *State { return &s }
func ptrBool(b bool) *bool    { return &b }
func ptrStr(s string) *string { return &s }

func TestTransferService_Create_AutoAssignsID(t *testing.T) {
	db := newTestDB(t)
	svc := &TransferService{}

	tr := &Transfer{
		InvestorAccountID: "00000000-0000-0000-0000-000000001001",
		CorrespondentID:   "00000000-0000-0000-0000-000000000010",
		OmnibusAccountID:  "00000000-0000-0000-0000-000000000001",
		AmountCents:       15000,
		Currency:          "USD",
		State:             StateRequested,
	}
	if err := svc.Create(db, tr); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tr.ID == "" {
		t.Error("expected auto-assigned ID, got empty string")
	}

	got, err := svc.GetByID(db, tr.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.AmountCents != 15000 {
		t.Errorf("AmountCents = %d, want 15000", got.AmountCents)
	}
}

func TestTransferService_List_StateFilter(t *testing.T) {
	db := newTestDB(t)
	svc := &TransferService{}

	// Seed data has 0 Requested transfers; create 2 fresh Requested ones
	seedTransfer(t, db, StateRequested)
	seedTransfer(t, db, StateRequested)
	// Also create one FundsPosted to confirm it won't appear in Requested filter
	seedTransfer(t, db, StateFundsPosted)

	state := StateRequested
	list, err := svc.List(db, TransferFilters{State: &state})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 Requested transfers, got %d", len(list))
	}
	for _, tr := range list {
		if tr.State != StateRequested {
			t.Errorf("unexpected state %s in Requested filter results", tr.State)
		}
	}
}

func TestTransferService_Count_StateFilter(t *testing.T) {
	db := newTestDB(t)
	svc := &TransferService{}

	// Seed data has 0 Requested transfers; create 3
	seedTransfer(t, db, StateRequested)
	seedTransfer(t, db, StateRequested)
	seedTransfer(t, db, StateRequested)

	state := StateRequested
	count, err := svc.Count(db, TransferFilters{State: &state})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 3 {
		t.Errorf("Count(Requested) = %d, want 3", count)
	}
}

func TestTransferService_List_LimitOffset(t *testing.T) {
	db := newTestDB(t)
	svc := &TransferService{}

	// Create 5 Requested transfers
	for i := 0; i < 5; i++ {
		seedTransfer(t, db, StateRequested)
	}

	state := StateRequested
	// Fetch first 2
	page1, err := svc.List(db, TransferFilters{State: &state, Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("page1 len = %d, want 2", len(page1))
	}

	// Fetch next 2
	page2, err := svc.List(db, TransferFilters{State: &state, Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page2 len = %d, want 2", len(page2))
	}

	// IDs should be different across pages
	if page1[0].ID == page2[0].ID {
		t.Error("page1[0] and page2[0] have the same ID")
	}
}

func TestTransferService_List_ReviewRequiredFilter(t *testing.T) {
	db := newTestDB(t)
	svc := &TransferService{}

	// Create a transfer and mark it review_required via direct DB insert
	tr := seedTransfer(t, db, StateAnalyzing)
	_, err := db.Exec(`UPDATE transfers SET review_required=1, review_status='PENDING' WHERE id=?`, tr.ID)
	if err != nil {
		t.Fatalf("update review_required: %v", err)
	}

	// Also create a non-review Analyzing transfer
	seedTransfer(t, db, StateAnalyzing)

	// Filter: state=Analyzing AND review_required=true
	state := StateAnalyzing
	reviewRequired := true
	list, err := svc.List(db, TransferFilters{
		State:          &state,
		ReviewRequired: &reviewRequired,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Should find at least our transfer (plus any from seed with review_required=1)
	for _, t2 := range list {
		if !t2.ReviewRequired {
			t.Errorf("transfer %s has ReviewRequired=false in filtered list", t2.ID)
		}
	}
	// Our specific transfer should be in the results
	found := false
	for _, t2 := range list {
		if t2.ID == tr.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected transfer %s in review_required=true list", tr.ID)
	}
}
