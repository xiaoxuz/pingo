package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: migrate <pg_url> [migration_dir]")
	}
	baseURL := os.Args[1]
	migrationDir := "migrations"
	if len(os.Args) > 2 {
		migrationDir = os.Args[2]
	}

	// 连 postgres 默认库
	pgURL := baseURL + "/postgres"
	if len(os.Args) > 3 {
		pgURL = os.Args[3]
	}

	log.Println("Connecting to PostgreSQL (postgres db)...")
	pgConn, err := pgx.Connect(context.Background(), pgURL)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer pgConn.Close(context.Background())
	log.Println("✓ Connected")

	// 建库
	var exists bool
	err = pgConn.QueryRow(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname='pingo')").Scan(&exists)
	if err != nil {
		log.Fatalf("Check db: %v", err)
	}
	if !exists {
		log.Println("Creating database pingo...")
		_, err = pgConn.Exec(context.Background(), "CREATE DATABASE pingo")
		if err != nil {
			log.Fatalf("Create db: %v", err)
		}
		log.Println("✓ Database pingo created")
	} else {
		log.Println("Database pingo already exists")
	}
	pgConn.Close(context.Background())

	// 连 pingo
	hubURL := baseURL + "/pingo"
	log.Println("Connecting to pingo...")
	conn, err := pgx.Connect(context.Background(), hubURL)
	if err != nil {
		log.Fatalf("Connect to pingo: %v", err)
	}
	defer conn.Close(context.Background())
	log.Println("✓ Connected to pingo")

	// 跑 migration
	if _, err := conn.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMP NOT NULL DEFAULT NOW())`); err != nil {
		log.Fatalf("Initialize migration tracking: %v", err)
	}
	var legacySchema bool
	if err := conn.QueryRow(context.Background(), `SELECT to_regclass('public.agents') IS NOT NULL`).Scan(&legacySchema); err != nil {
		log.Fatalf("Check existing schema: %v", err)
	}
	if legacySchema {
		if _, err := conn.Exec(context.Background(), `INSERT INTO schema_migrations(name) VALUES('001_init.sql') ON CONFLICT DO NOTHING`); err != nil {
			log.Fatalf("Record existing schema: %v", err)
		}
	}
	files := []string{"001_init.sql", "003_admin.sql", "004_admin_delete.sql"}
	for _, f := range files {
		var applied bool
		if err := conn.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, f).Scan(&applied); err != nil {
			log.Fatalf("Check migration %s: %v", f, err)
		}
		if applied {
			log.Printf("Already applied %s", f)
			continue
		}
		path := filepath.Join(migrationDir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("Read migration %s: %v", f, err)
		}
		log.Printf("Running %s...", f)
		transaction, err := conn.Begin(context.Background())
		if err != nil {
			log.Fatalf("Begin migration %s: %v", f, err)
		}
		_, err = transaction.Exec(context.Background(), string(data))
		if err == nil {
			_, err = transaction.Exec(context.Background(), `INSERT INTO schema_migrations(name) VALUES($1)`, f)
		}
		if err == nil {
			err = transaction.Commit(context.Background())
		} else {
			_ = transaction.Rollback(context.Background())
		}
		if err != nil {
			log.Fatalf("Migration %s failed: %v", f, err)
		}
		log.Printf("✓ %s applied", f)
	}

	log.Println("═══════════════════════════════")
	log.Println("  All migrations applied! ✅")
	log.Println("═══════════════════════════════")

	var count int
	err = conn.QueryRow(context.Background(),
		"SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&count)
	if err == nil {
		fmt.Printf("  Total tables: %d\n", count)
	}

	// 列一下所有表
	rows, _ := conn.Query(context.Background(),
		"SELECT table_name FROM information_schema.tables WHERE table_schema='public' ORDER BY table_name")
	defer rows.Close()
	fmt.Println("  Tables:")
	for rows.Next() {
		var name string
		rows.Scan(&name)
		fmt.Printf("    - %s\n", name)
	}
}
