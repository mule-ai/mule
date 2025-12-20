package supabase

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Client represents a Supabase client
type Client struct {
	db     *pgxpool.Pool
	config Config
}

// Config holds the Supabase configuration
type Config struct {
	URL    string
	APIKey string
}

// Event represents a webhook event stored in Supabase
type Event struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	Payload   string `json:"payload"`
	CreatedAt string `json:"created_at"`
	Processed bool   `json:"processed"`
}

// NewClient creates a new Supabase client
func NewClient(config Config) (*Client, error) {
	// Create connection string
	connString := fmt.Sprintf("postgresql://%s@%s:6543/postgres", 
		config.APIKey, 
		config.URL)

	// Create connection pool
	db, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Supabase: %w", err)
	}

	// Test the connection
	if err := db.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping Supabase: %w", err)
	}

	log.Println("Successfully connected to Supabase")
	
	return &Client{
		db:     db,
		config: config,
	}, nil
}

// ListenForEvents subscribes to database changes
func (c *Client) ListenForEvents(ctx context.Context, channel chan<- Event) error {
	// This is a simplified implementation
	// In a production environment, you would use Supabase's real-time features
	// or PostgreSQL's LISTEN/NOTIFY
	
	query := `SELECT id, event_type, payload, created_at, processed 
	          FROM webhook_events 
	          WHERE processed = false 
	          ORDER BY created_at ASC`

	rows, err := c.db.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var event Event
		err := rows.Scan(&event.ID, &event.EventType, &event.Payload, &event.CreatedAt, &event.Processed)
		if err != nil {
			log.Printf("Error scanning event: %v", err)
			continue
		}
		
		// Send event to channel
		select {
		case channel <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return rows.Err()
}

// MarkEventProcessed marks an event as processed
func (c *Client) MarkEventProcessed(ctx context.Context, eventID string) error {
	query := `UPDATE webhook_events SET processed = true WHERE id = $1`
	_, err := c.db.Exec(ctx, query, eventID)
	return err
}

// Close closes the database connection
func (c *Client) Close() {
	if c.db != nil {
		c.db.Close()
	}
}