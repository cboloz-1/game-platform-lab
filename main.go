package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

const eventQueueSize = 100

var eventQueue chan GameEvent

// GameEvent defines the structure of incoming data.
type GameEvent struct {
	PlayerID        string    `json:"player_id"`
	EventID         string    `json:"event_id"`
	EventType       string    `json:"event_type"`
	ClientTimestamp int64     `json:"timestamp"`
	ServerTimestamp time.Time `json:"-"`
}

func main() {

	// Load environment variables from .env file for database connection
	if err := godotenv.Load(); err != nil {
		log.Fatalf("No .env file found: %v", err)
	}

	dbname := os.Getenv("POSTGRES_DB")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	host := os.Getenv("POSTGRES_HOST")
	port := os.Getenv("POSTGRES_PORT")
	sslmode := os.Getenv("SSL_MODE")

	// the connection string for PostgreSQL
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	// Open a connection to the PostgreSQL database
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to the database: %v", err)
	}
	defer db.Close()

	// Test the database connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping the database: %v", err)
	}

	log.Println("Successfully connected to the database.")

	// Create a new database driver for migrations
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatalf("Failed to create database driver: %v", err)
	}

	// Create a new migration instance
	m, err := migrate.NewWithDatabaseInstance(
		"file://./cmd/migrate/migrations",
		"postgres",
		driver,
	)

	if err != nil {
		log.Fatal(err)
	}

	// Determine the command from the last argument
	cmd := os.Args[len(os.Args)-1]
	if cmd == "up" {
		// Apply all up migrations
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("Migration up failed: %v", err)
		}
		fmt.Println("Database migration up completed successfully.")
		return
	}

	if cmd == "down" {
		// Apply all down migrations
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("Migration down failed: %v", err)
		}
		fmt.Println("Database migration down completed successfully.")
		return
	}

	// Load the New York time zone once when the server starts.
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		log.Fatalf("Failed to load Eastern Time location: %v", err)
	}

	// Create a queue so requests are accepted quickly and processed in order.
	eventQueue = make(chan GameEvent, eventQueueSize)
	go processEvents(eventQueue, db, location)

	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		getEventsHandler(w, r, db)
	})
	fmt.Println("Server is running on port 8080...")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// getEventsHandler accepts incoming game events.
func getEventsHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	// Ensure clients send POST requests.
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode the request's JSON fields into a GameEvent.
	var event GameEvent
	err := json.NewDecoder(r.Body).Decode(&event)
	// Invalid JSON cannot be processed.
	if err != nil {
		http.Error(w, "Bad request: invalid JSON", http.StatusBadRequest)
		return
	}
	// Valid JSON still needs the fields required for a usable event.
	if strings.TrimSpace(event.PlayerID) == "" || strings.TrimSpace(event.EventID) == "" || strings.TrimSpace(event.EventType) == "" || event.ClientTimestamp <= 0 {
		http.Error(w, "Bad request: player_id, event_id, event_type, and a positive timestamp are required", http.StatusBadRequest)
		return
	}

	// Save the server's receive time in UTC, the standard format for server logs.
	event.ServerTimestamp = time.Now().UTC()

	// Insert the event into the database immediately to ensure it is saved before responding to the client
	_, err = db.Exec(
		`INSERT INTO events (event_id, player_id, event_type, client_timestamp, server_timestamp) VALUES ($1, $2, $3, $4, $5)`,
		event.EventID,
		event.PlayerID,
		event.EventType,
		event.ClientTimestamp,
		event.ServerTimestamp,
	)
	if err != nil {
		http.Error(w, "Service unavailable: unable to save event", http.StatusServiceUnavailable)
		return
	}

	// Respond after the event has been saved to the database
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status":"success","message":"Event received"}`))
}

func getNextEvent(db *sql.DB) (GameEvent, bool, error) {
	var event GameEvent

	// SQL statement to atomically select and update the next pending event
	err := db.QueryRow(`
	UPDATE events
	SET status = 'processing',
		attempt_count = attempt_count + 1,
	WHERE event_id = (
		SELECT event_id FROM events
		WHERE status = 'pending'
		ORDER BY 
			AND (next_attempt_at IS NULL OR next_attempt_at <= NOW())
		ORDER BY server_timestamp ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	)
	RETURNING event_id, player_id, event_type, client_timestamp, server_timestamp
	`).Scan(
		// Scan the returned row into the event struct
		&event.EventID,
		&event.PlayerID,
		&event.EventType,
		&event.ClientTimestamp,
		&event.ServerTimestamp)

	if errors.Is(err, sql.ErrNoRows) {
		return GameEvent{}, false, nil // No pending events
	}
	if err != nil {
		return GameEvent{}, false, fmt.Errorf("claim next event: %w", err)
	}

	return event, true, nil
}

func processEvents(eventQueue <-chan GameEvent, db *sql.DB, location *time.Location) {
	// Read and process events one at a time from the queue.
	for event := range eventQueue {
		processEvent(event, db, location)
	}
}

func processEvent(event GameEvent, db *sql.DB, location *time.Location) {
	// Insert the event into the database.
	_, err := db.Exec(
		`INSERT INTO events (event_id, player_id, event_type, client_timestamp, server_timestamp) VALUES ($1, $2, $3, $4, $5)`,
		event.EventID,
		event.PlayerID,
		event.EventType,
		event.ClientTimestamp,
		event.ServerTimestamp,
	)
	if err != nil {
		log.Printf("Failed to insert event [%s] for player [%s]: %v", event.EventID, event.PlayerID, err)
		return
	}

	// Simulate downstream processing
	time.Sleep(2 * time.Second)

	// Convert the client's Unix timestamp to New York's local time.
	eventTime := time.Unix(event.ClientTimestamp, 0).In(location)
	// Log the event with both the client time and the server receive time.
	log.Printf(
		"Event processed for player [%s] with event type [%s] and event ID [%s]; client event time [%s], server received time [%s]",
		event.PlayerID,
		event.EventType,
		event.EventID,
		eventTime.Format(time.RFC3339),
		event.ServerTimestamp.Format(time.RFC3339),
	)
}
