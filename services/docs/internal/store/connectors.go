// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/connectors"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Persistencia del conector de datos externos. Toda operación va por tenantTx
// (RLS + rol docs_app): el aislamiento por tenant lo refuerza la base de datos.
// Las credenciales se guardan YA cifradas (bytea) y NUNCA se devuelven en las
// listas/gets: solo LoadConnectionSecret las lee, para descifrarlas en memoria al
// ejecutar.

// orNilBytes convierte un slice vacío en NULL para la columna bytea (una conexión
// sin credenciales guarda NULL, no un bytea vacío).
func orNilBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

const connCols = `id, name, kind, config, (credentials IS NOT NULL), created_by, created_at, updated_at`

// scanConnection escanea una fila de conexión (sin credenciales) a un DTO.
func scanConnection(row pgx.Row, c *connectors.DataConnection) error {
	var kind string
	if err := row.Scan(&c.ID, &c.Name, &kind, &c.Config, &c.HasCredentials,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return err
	}
	c.Kind = connectors.Kind(kind)
	return nil
}

// CreateConnection inserta una conexión y devuelve su forma pública (sin creds).
func (s *Store) CreateConnection(ctx context.Context, name string, kind connectors.Kind, config json.RawMessage, sealed []byte, createdBy string) (*connectors.DataConnection, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	var c connectors.DataConnection
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		return scanConnection(tx.QueryRow(ctx, `
			INSERT INTO data_connections (tenant, name, kind, config, credentials, created_by)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING `+connCols,
			t.Slug, name, string(kind), config, orNilBytes(sealed), createdBy), &c)
	})
	if err != nil {
		return nil, fmt.Errorf("store: crear conexión: %w", err)
	}
	return &c, nil
}

// ListConnections lista las conexiones del tenant (sin credenciales).
func (s *Store) ListConnections(ctx context.Context) ([]connectors.DataConnection, error) {
	out := make([]connectors.DataConnection, 0, 16)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+connCols+` FROM data_connections ORDER BY name ASC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c connectors.DataConnection
			if err := scanConnection(rows, &c); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar conexiones: %w", err)
	}
	return out, nil
}

// GetConnection carga una conexión por id (sin credenciales). ErrNotFound si no existe.
func (s *Store) GetConnection(ctx context.Context, id string) (*connectors.DataConnection, error) {
	var c connectors.DataConnection
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return scanConnection(tx.QueryRow(ctx, `SELECT `+connCols+` FROM data_connections WHERE id = $1`, id), &c)
	})
	if isNoRows(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: obtener conexión: %w", err)
	}
	return &c, nil
}

// UpdateConnection actualiza nombre/config y, si replaceCreds, también la
// credencial cifrada (dejarla intacta en caso contrario). ErrNotFound si no existe.
func (s *Store) UpdateConnection(ctx context.Context, id, name string, config json.RawMessage, sealed []byte, replaceCreds bool) error {
	if replaceCreds {
		return s.execAffecting(ctx, "actualizar conexión",
			`UPDATE data_connections SET name = $2, config = $3, credentials = $4, updated_at = now()
			 WHERE id = $1`, id, name, config, orNilBytes(sealed))
	}
	return s.execAffecting(ctx, "actualizar conexión",
		`UPDATE data_connections SET name = $2, config = $3, updated_at = now()
		 WHERE id = $1`, id, name, config)
}

// DeleteConnection borra una conexión (y en cascada sus consultas). ErrNotFound
// si no existe.
func (s *Store) DeleteConnection(ctx context.Context, id string) error {
	return s.execAffecting(ctx, "borrar conexión",
		`DELETE FROM data_connections WHERE id = $1`, id)
}

// LoadConnectionSecret lee kind + config + credencial CIFRADA de una conexión,
// para descifrarla en memoria al ejecutar. Es la ÚNICA lectura que toca
// `credentials`. sealed puede ser nil (conexión sin secreto).
func (s *Store) LoadConnectionSecret(ctx context.Context, id string) (connectors.Kind, json.RawMessage, []byte, error) {
	var (
		kind   string
		config json.RawMessage
		sealed []byte
	)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT kind, config, credentials FROM data_connections WHERE id = $1`, id).
			Scan(&kind, &config, &sealed)
	})
	if isNoRows(err) {
		return "", nil, nil, domain.ErrNotFound
	}
	if err != nil {
		return "", nil, nil, fmt.Errorf("store: cargar credencial de conexión: %w", err)
	}
	return connectors.Kind(kind), config, sealed, nil
}

// --- Consultas guardadas (data_queries) ---

const queryCols = `id, connection_id, name, spec, created_by, created_at, updated_at`

func scanQuery(row pgx.Row, q *connectors.DataQuery) error {
	return row.Scan(&q.ID, &q.ConnectionID, &q.Name, &q.Spec, &q.CreatedBy, &q.CreatedAt, &q.UpdatedAt)
}

// CreateQuery inserta una consulta sobre una conexión del tenant. Verifica que la
// conexión sea visible (mismo tenant) antes de insertar: el FK de PostgreSQL
// ignora RLS, así que sin este chequeo se podría referenciar una conexión de otro
// tenant (la ejecución igualmente la ocultaría por RLS, pero evitamos el enlace
// colgante). ErrNotFound si la conexión no existe en el tenant.
func (s *Store) CreateQuery(ctx context.Context, connID, name string, spec json.RawMessage, createdBy string) (*connectors.DataQuery, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	var q connectors.DataQuery
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM data_connections WHERE id = $1)`, connID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return domain.ErrNotFound
		}
		return scanQuery(tx.QueryRow(ctx, `
			INSERT INTO data_queries (tenant, connection_id, name, spec, created_by)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING `+queryCols,
			t.Slug, connID, name, spec, createdBy), &q)
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("store: crear consulta: %w", err)
	}
	return &q, nil
}

// ListQueries lista las consultas de una conexión.
func (s *Store) ListQueries(ctx context.Context, connID string) ([]connectors.DataQuery, error) {
	out := make([]connectors.DataQuery, 0, 16)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+queryCols+` FROM data_queries WHERE connection_id = $1 ORDER BY name ASC`, connID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var q connectors.DataQuery
			if err := scanQuery(rows, &q); err != nil {
				return err
			}
			out = append(out, q)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar consultas: %w", err)
	}
	return out, nil
}

// UpdateQuery actualiza nombre/spec de una consulta. ErrNotFound si no existe.
func (s *Store) UpdateQuery(ctx context.Context, id, name string, spec json.RawMessage) error {
	return s.execAffecting(ctx, "actualizar consulta",
		`UPDATE data_queries SET name = $2, spec = $3, updated_at = now() WHERE id = $1`,
		id, name, spec)
}

// DeleteQuery borra una consulta. ErrNotFound si no existe.
func (s *Store) DeleteQuery(ctx context.Context, id string) error {
	return s.execAffecting(ctx, "borrar consulta",
		`DELETE FROM data_queries WHERE id = $1`, id)
}

// LoadQueryForExecute lee, para una consulta guardada, la spec + kind/config/
// credencial cifrada de su conexión (join bajo RLS: solo del tenant activo).
// ErrNotFound si la consulta no existe en el tenant.
func (s *Store) LoadQueryForExecute(ctx context.Context, queryID string) (kind connectors.Kind, config json.RawMessage, sealed []byte, spec json.RawMessage, err error) {
	var k string
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT c.kind, c.config, c.credentials, q.spec
			FROM data_queries q
			JOIN data_connections c ON c.id = q.connection_id
			WHERE q.id = $1`, queryID).
			Scan(&k, &config, &sealed, &spec)
	})
	if isNoRows(err) {
		return "", nil, nil, nil, domain.ErrNotFound
	}
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("store: cargar consulta: %w", err)
	}
	return connectors.Kind(k), config, sealed, spec, nil
}
