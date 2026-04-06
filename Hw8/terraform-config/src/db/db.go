package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Global connection pool — shared across all handlers
var DB *sql.DB

// InitDB creates the connection pool and runs schema migration.
// Called once at server startup from main().
func InitDB() error {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "admin")
	pass := getEnv("DB_PASSWORD", "password")
	name := getEnv("DB_NAME", "ecommerce")

	// DSN format: user:password@tcp(host:port)/dbname?params
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&interpolateParams=true",
		user, pass, host, port, name)

	var err error
	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("sql.Open failed: %w", err)
	}

	// ── Connection Pool Tuning ──────────────────────────────────
	// MaxOpenConns: cap at 20 per ECS task. With db.t3.micro's 150
	// max_connections, this allows ~7 tasks before pool exhaustion.
	DB.SetMaxOpenConns(20)

	// MaxIdleConns: keep 10 warm connections to avoid TCP handshake
	// overhead on the hot path (cart retrievals).
	DB.SetMaxIdleConns(10)

	// ConnMaxLifetime: recycle connections every 5 min to handle
	// RDS maintenance events and DNS changes gracefully.
	DB.SetConnMaxLifetime(5 * time.Minute)

	// ConnMaxIdleTime: close idle connections after 1 min to release
	// resources during low-traffic periods.
	DB.SetConnMaxIdleTime(1 * time.Minute)

	// Retry connection with backoff — RDS can take a moment after deploy
	for i := 0; i < 30; i++ {
		err = DB.Ping()
		if err == nil {
			log.Printf("Connected to MySQL at %s:%s/%s", host, port, name)
			break
		}
		log.Printf("Waiting for MySQL (attempt %d/30): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("could not connect to MySQL after 30 attempts: %w", err)
	}

	// Run schema migration
	if err := migrate(); err != nil {
		return fmt.Errorf("schema migration failed: %w", err)
	}

	return nil
}

// migrate creates tables if they don't exist.
// Idempotent — safe to run on every startup.
func migrate() error {
	schema := `
	-- ================================================================
	-- Shopping Cart Schema
	-- ================================================================
	-- Design decisions:
	--   • Two tables (shopping_carts + cart_items) for normalized storage.
	--     Keeps cart metadata separate from line items, so retrieving
	--     cart count by customer is a single-table scan.
	--   • AUTO_INCREMENT PKs for fast inserts (sequential B+ tree writes).
	--   • UNIQUE(cart_id, product_id) on cart_items prevents duplicate
	--     product entries — adding an existing product updates quantity.
	--   • INDEX on customer_id for "all carts by customer" queries.
	--   • INDEX on cart_id in cart_items for efficient JOINs on retrieval.
	--   • ON DELETE CASCADE: deleting a cart auto-removes its items,
	--     preventing orphaned rows.
	--   • InnoDB engine for row-level locking (concurrent cart modifications).
	--   • updated_at with ON UPDATE CURRENT_TIMESTAMP for auditing.
	-- ================================================================

	CREATE TABLE IF NOT EXISTS shopping_carts (
		cart_id      INT AUTO_INCREMENT PRIMARY KEY,
		customer_id  INT NOT NULL,
		created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_customer_id (customer_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

	CREATE TABLE IF NOT EXISTS cart_items (
		item_id      INT AUTO_INCREMENT PRIMARY KEY,
		cart_id      INT NOT NULL,
		product_id   INT NOT NULL,
		quantity     INT NOT NULL DEFAULT 1,
		added_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY   uk_cart_product (cart_id, product_id),
		INDEX        idx_cart_id (cart_id),
		CONSTRAINT   fk_cart_items_cart
			FOREIGN KEY (cart_id) REFERENCES shopping_carts(cart_id)
			ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`

	// Execute each statement separately (MySQL driver doesn't support multi-statement by default)
	statements := []string{
		`CREATE TABLE IF NOT EXISTS shopping_carts (
			cart_id      INT AUTO_INCREMENT PRIMARY KEY,
			customer_id  INT NOT NULL,
			created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_customer_id (customer_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS cart_items (
			item_id      INT AUTO_INCREMENT PRIMARY KEY,
			cart_id      INT NOT NULL,
			product_id   INT NOT NULL,
			quantity     INT NOT NULL DEFAULT 1,
			added_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY   uk_cart_product (cart_id, product_id),
			INDEX        idx_cart_id (cart_id),
			CONSTRAINT   fk_cart_items_cart
				FOREIGN KEY (cart_id) REFERENCES shopping_carts(cart_id)
				ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}

	_ = schema // schema string above is for documentation

	for _, stmt := range statements {
		if _, err := DB.Exec(stmt); err != nil {
			return fmt.Errorf("migration statement failed: %w\nSQL: %s", err, stmt)
		}
	}

	log.Println("Schema migration complete")
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
