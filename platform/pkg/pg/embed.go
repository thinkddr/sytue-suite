// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RateLimitMigrationsFS expone las migraciones del limitador de la plataforma
// para que un servicio las combine con las suyas (o las aplique con MigrateUp).
func RateLimitMigrationsFS() fs.FS {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		// Nunca ocurre: el path está embebido en tiempo de compilación.
		panic(err)
	}
	return sub
}

// PurgeRateLimits elimina buckets ociosos más antiguos que olderThan, para
// acotar el tamaño de la tabla (ejecutar periódicamente vía cron/river).
func PurgeRateLimits(ctx context.Context, q Querier, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		olderThan = time.Hour
	}
	cutoff := time.Now().UTC().Add(-olderThan)
	tag, err := q.Exec(ctx, "DELETE FROM platform_rate_limits WHERE updated_at < $1", cutoff)
	if err != nil {
		return 0, fmt.Errorf("pg: purgar rate limits: %w", err)
	}
	return tag.RowsAffected(), nil
}
