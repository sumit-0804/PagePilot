package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect opens Postgres. The schema is owned by supabase/migrations, so nothing is auto-migrated here.
func Connect(databaseURL string) (*gorm.DB, error) {
	// Simple protocol keeps the connection usable through Supabase's transaction pooler.
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  databaseURL,
		PreferSimpleProtocol: true,
	}), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("postgres not reachable: %w", err)
	}

	var hasVector bool
	if err := db.WithContext(ctx).Raw("select exists (select 1 from pg_extension where extname = 'vector')").Scan(&hasVector).Error; err != nil {
		return nil, fmt.Errorf("checking pgvector extension: %w", err)
	}
	if !hasVector {
		return nil, fmt.Errorf("pgvector extension missing: apply migrations (supabase db reset / supabase db push)")
	}
	return db, nil
}
