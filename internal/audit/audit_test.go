package audit

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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

	content, err := os.ReadFile(filepath.Join(migrationsDir(), "001_init.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(content)); err != nil {
		t.Fatalf("migration: %v", err)
	}
	return db
}

func ptr(s string) *string { return &s }

func TestLogEvent_BasicInsert(t *testing.T) {
	db := newTestDB(t)

	err := LogEvent(db, Event{
		EntityType: "transfer",
		EntityID:   "tx-001",
		ActorType:  "SYSTEM",
		ActorID:    "system",
		EventType:  "STATE_TRANSITION",
		FromState:  ptr("Requested"),
		ToState:    ptr("Validating"),
	})
	if err != nil {
		t.Fatalf("LogEvent: %v", err)
	}

	events, err := GetByEntity(db, "transfer", "tx-001")
	if err != nil {
		t.Fatalf("GetByEntity: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.EntityID != "tx-001" {
		t.Errorf("EntityID = %q, want tx-001", e.EntityID)
	}
	if e.EventType != "STATE_TRANSITION" {
		t.Errorf("EventType = %q, want STATE_TRANSITION", e.EventType)
	}
	if e.FromState == nil || *e.FromState != "Requested" {
		t.Errorf("FromState = %v, want Requested", e.FromState)
	}
	if e.ToState == nil || *e.ToState != "Validating" {
		t.Errorf("ToState = %v, want Validating", e.ToState)
	}
}

func TestLogEvent_AutoAssignsIDAndTimestamp(t *testing.T) {
	db := newTestDB(t)

	before := time.Now().UTC()
	err := LogEvent(db, Event{
		EntityType: "transfer",
		EntityID:   "tx-002",
		ActorType:  "SYSTEM",
		ActorID:    "system",
		EventType:  "CREATED",
		// ID and CreatedAt intentionally omitted
	})
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("LogEvent: %v", err)
	}

	events, err := GetByEntity(db, "transfer", "tx-002")
	if err != nil {
		t.Fatalf("GetByEntity: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.ID == "" {
		t.Error("expected auto-assigned ID, got empty string")
	}
	if e.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if e.CreatedAt.Before(before) || e.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not between %v and %v", e.CreatedAt, before, after)
	}
}

func TestLogEvent_ExplicitIDPreserved(t *testing.T) {
	db := newTestDB(t)

	const fixedID = "aaaabbbb-cccc-dddd-eeee-ffffffffffff"
	err := LogEvent(db, Event{
		ID:         fixedID,
		EntityType: "transfer",
		EntityID:   "tx-003",
		ActorType:  "OPERATOR",
		ActorID:    "op-001",
		EventType:  "APPROVED",
	})
	if err != nil {
		t.Fatalf("LogEvent: %v", err)
	}

	events, err := GetByEntity(db, "transfer", "tx-003")
	if err != nil {
		t.Fatalf("GetByEntity: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != fixedID {
		t.Errorf("ID = %q, want %q", events[0].ID, fixedID)
	}
}

func TestLogEventTx_RollbackDoesNotPersist(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}

	err = LogEventTx(tx, Event{
		EntityType: "transfer",
		EntityID:   "tx-rollback",
		ActorType:  "SYSTEM",
		ActorID:    "system",
		EventType:  "CREATED",
	})
	if err != nil {
		t.Fatalf("LogEventTx: %v", err)
	}

	// Roll back the transaction
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	// Event should NOT be visible
	events, err := GetByEntity(db, "transfer", "tx-rollback")
	if err != nil {
		t.Fatalf("GetByEntity: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events after rollback, got %d", len(events))
	}
}

func TestLogEventTx_CommitPersists(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}

	err = LogEventTx(tx, Event{
		EntityType: "transfer",
		EntityID:   "tx-commit",
		ActorType:  "SYSTEM",
		ActorID:    "system",
		EventType:  "CREATED",
	})
	if err != nil {
		t.Fatalf("LogEventTx: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	events, err := GetByEntity(db, "transfer", "tx-commit")
	if err != nil {
		t.Fatalf("GetByEntity: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event after commit, got %d", len(events))
	}
}

func TestGetByEntity_MultipleEvents_OrderedAscending(t *testing.T) {
	db := newTestDB(t)

	states := []struct{ from, to string }{
		{"Requested", "Validating"},
		{"Validating", "Analyzing"},
		{"Analyzing", "Approved"},
		{"Approved", "FundsPosted"},
	}
	for _, s := range states {
		from, to := s.from, s.to
		if err := LogEvent(db, Event{
			EntityType: "transfer",
			EntityID:   "tx-multi",
			ActorType:  "SYSTEM",
			ActorID:    "system",
			EventType:  "STATE_TRANSITION",
			FromState:  &from,
			ToState:    &to,
		}); err != nil {
			t.Fatalf("LogEvent: %v", err)
		}
	}

	events, err := GetByEntity(db, "transfer", "tx-multi")
	if err != nil {
		t.Fatalf("GetByEntity: %v", err)
	}
	if len(events) != len(states) {
		t.Fatalf("expected %d events, got %d", len(states), len(events))
	}

	// Verify ordering: first event's ToState should be Validating
	if *events[0].ToState != "Validating" {
		t.Errorf("first event ToState = %q, want Validating", *events[0].ToState)
	}
	// Last event's ToState should be FundsPosted
	if *events[len(events)-1].ToState != "FundsPosted" {
		t.Errorf("last event ToState = %q, want FundsPosted", *events[len(events)-1].ToState)
	}
}

func TestGetByEntity_DifferentEntityNotReturned(t *testing.T) {
	db := newTestDB(t)

	if err := LogEvent(db, Event{
		EntityType: "transfer",
		EntityID:   "tx-A",
		ActorType:  "SYSTEM",
		ActorID:    "system",
		EventType:  "CREATED",
	}); err != nil {
		t.Fatal(err)
	}
	if err := LogEvent(db, Event{
		EntityType: "transfer",
		EntityID:   "tx-B",
		ActorType:  "SYSTEM",
		ActorID:    "system",
		EventType:  "CREATED",
	}); err != nil {
		t.Fatal(err)
	}

	events, err := GetByEntity(db, "transfer", "tx-A")
	if err != nil {
		t.Fatalf("GetByEntity: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event for tx-A, got %d", len(events))
	}
	if events[0].EntityID != "tx-A" {
		t.Errorf("unexpected EntityID %q", events[0].EntityID)
	}
}
