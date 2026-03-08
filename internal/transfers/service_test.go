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
	for _, name := range []string{"001_initial.sql", "002_seed.sql"} {
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
