package audit

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID          string
	EntityType  string
	EntityID    string
	ActorType   string
	ActorID     string
	EventType   string
	FromState   *string
	ToState     *string
	DetailsJSON *string
	CreatedAt   time.Time
}

func LogEvent(db *sql.DB, e Event) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}

	_, err := db.Exec(`
		INSERT INTO audit_events (
			id, entity_type, entity_id, actor_type, actor_id,
			event_type, from_state, to_state, details_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.EntityType, e.EntityID, e.ActorType, e.ActorID,
		e.EventType, e.FromState, e.ToState, e.DetailsJSON, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func LogEventTx(tx *sql.Tx, e Event) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}

	_, err := tx.Exec(`
		INSERT INTO audit_events (
			id, entity_type, entity_id, actor_type, actor_id,
			event_type, from_state, to_state, details_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.EntityType, e.EntityID, e.ActorType, e.ActorID,
		e.EventType, e.FromState, e.ToState, e.DetailsJSON, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func GetByEntity(db *sql.DB, entityType, entityID string) ([]Event, error) {
	rows, err := db.Query(`
		SELECT id, entity_type, entity_id, actor_type, actor_id,
			event_type, from_state, to_state, details_json, created_at
		FROM audit_events
		WHERE entity_type = ? AND entity_id = ?
		ORDER BY created_at ASC`, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("query audit events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		err := rows.Scan(
			&e.ID, &e.EntityType, &e.EntityID, &e.ActorType, &e.ActorID,
			&e.EventType, &e.FromState, &e.ToState, &e.DetailsJSON, &e.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
