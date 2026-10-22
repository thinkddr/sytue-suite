// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store es la persistencia del Sytue Integration Gateway sobre PostgreSQL
// (pgx). Todo acceso a datos por-org pasa por una transacción con `app.tenant`
// fijado y el rol de aplicación (integrations_app) activado, de modo que las
// políticas RLS deny-by-default aíslan por organización SIEMPRE — incluso si el
// pool se conecta con un superusuario (dev/CI). Consultas SIEMPRE parametrizadas.
//
// Los VALORES de secretos se sellan con platform/pkg/keycrypto (AES-256-GCM,
// sobre "encv1:") y NUNCA se serializan en lecturas de instancia: solo
// LoadInstanceWithSecrets los abre, en memoria, para fabricar el Runtime.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// appRole es el rol NO superusuario y SIN BYPASSRLS bajo el que corren todas las
// transacciones por-org. Fijarlo con SET LOCAL ROLE garantiza que RLS se aplique
// SIEMPRE. Debe coincidir con el rol creado en la migración 0001.
const appRole = "integrations_app"

// SecretKeyBearer es la clave convencional del vault (instance_secrets) cuyo valor
// la pasarela inyecta como "Authorization: Bearer …" en el egress al SaaS. Fuente
// única compartida por el despacho HTTP (Test) y el de sync (Provisioner) para no
// divergir. El driver nunca lee el secreto: el núcleo lo inyecta en Runtime.HTTP.
const SecretKeyBearer = "bearer"

// ErrNoCipher indica que se intentó sellar secretos sin KEK configurada. Guardar
// un secreto en claro nunca es aceptable: sin clave, escribir secretos se rechaza.
var ErrNoCipher = errors.New("store: cifrado de secretos no configurado (falta la KEK)")

// Store agrupa el acceso a datos del gateway.
type Store struct {
	rol    *pg.AppRole
	pool   *pg.Pool
	cipher *keycrypto.Cipher
}

// New construye un Store sobre el pool dado. cipher puede ser nil (sin KEK): en
// ese caso, escribir secretos devuelve ErrNoCipher (arranque OK, secretos no).
func New(pool *pg.Pool, cipher *keycrypto.Cipher) *Store {
	return &Store{rol: pg.NewAppRole(appRole), pool: pool, cipher: cipher}
}

// Healthz verifica la conectividad con PostgreSQL (sondas /readyz).
//
// Ya NO comprueba aquí el rol de aplicación: eso lo decide ResolveAppRole una vez
// en el arranque, y si el aislamiento no se puede garantizar el servicio no llega
// a servir. Repetirlo en cada sonda solo añadía una transacción cada diez
// segundos para volver a responder lo mismo.
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// ResolveAppRole decide cómo se sostiene el aislamiento por organización y
// verifica que se sostiene de verdad. Se llama en el arranque, tras las
// migraciones y ANTES de servir tráfico; si devuelve error, el servicio no debe
// arrancar. Ver platform/pkg/pg.AppRole para el razonamiento completo.
func (s *Store) ResolveAppRole(ctx context.Context) (string, error) {
	return s.rol.Resolve(ctx, s.pool)
}

// tenantTx ejecuta fn en una transacción con `app.tenant` (del contexto) fijado y
// el rol integrations_app activado (SET LOCAL ROLE): RLS aísla por org siempre.
func (s *Store) tenantTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// El rol solo se asume si Resolve comprobó que se PUEDE. Cuando no, el
		// aislamiento lo sostiene la RLS forzada sobre un rol sin superusuario ni
		// BYPASSRLS, que Resolve verifica antes de permitirlo; ver pg.AppRole.
		if s.rol.Activo() {
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+appRole); err != nil {
				return fmt.Errorf("store: fijar rol de aplicación: %w", err)
			}
		}
		return fn(tx)
	})
}

// isNoRows normaliza el "sin filas" de pgx.
func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// execAffecting ejecuta una sentencia de escritura en tenantTx y devuelve
// domain.ErrNotFound si no afectó ninguna fila (recurso ausente en la org).
func (s *Store) execAffecting(ctx context.Context, what, sql string, args ...any) error {
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, sql, args...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("store: %s: %w", what, err)
	}
	return nil
}

// ListTemplates devuelve el catálogo de plantillas habilitadas (global; no
// por-org). El orden es estable por Key.
func (s *Store) ListTemplates(ctx context.Context) ([]domain.AppTemplate, error) {
	out := make([]domain.AppTemplate, 0, 32)
	// SIN tenant, y por eso no usa tenantTx: `app_templates` es el catálogo de la
	// PLATAFORMA —no tiene columna de organización ni política RLS— y quien lo
	// pide desde la consola global no trae ninguna.
	//
	// Con `tenantTx` esto fallaba con «tenancy: no hay tenant en el contexto», un
	// 500 que la pantalla de conexiones se tragaba entera: el catálogo llegaba
	// vacío, así que el botón «añadir proveedor» salía DESACTIVADO sin decir por
	// qué. Un error de servidor convertido en un botón gris.
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if s.rol.Activo() {
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+appRole); err != nil {
				return fmt.Errorf("store: fijar rol de aplicación: %w", err)
			}
		}
		rows, err := tx.Query(ctx,
			`SELECT key, manifest, tier, enabled FROM app_templates WHERE enabled = true ORDER BY key ASC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t domain.AppTemplate
			if err := rows.Scan(&t.Key, &t.Manifest, &t.Tier, &t.Enabled); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar plantillas: %w", err)
	}
	return out, nil
}

// RecordEgress inserta una fila de egress_audit. Se invoca desde la cadena de
// políticas (fuera de una transacción de petición), así que fija el tenant
// EXPLÍCITAMENTE desde rec.OrgID (no del contexto). Cumple policy.AuditSink.
func (s *Store) RecordEgress(ctx context.Context, rec domain.EgressAudit) error {
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, rec.OrgID); err != nil {
			return err
		}
		if s.rol.Activo() {
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+appRole); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO egress_audit
			    (org_id, instance_id, host, method, status, req_bytes, resp_bytes, latency_ms, auth, err, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			rec.OrgID, nullUUID(rec.InstanceID), rec.Host, rec.Method, rec.Status,
			rec.ReqBytes, rec.RespBytes, rec.LatencyMS, rec.Auth, rec.Err, rec.OccurredAt)
		return err
	})
	if err != nil {
		return fmt.Errorf("store: auditar egress: %w", err)
	}
	return nil
}

// nullUUID mapea una cadena vacía a NULL para una columna uuid (instance_id puede
// faltar en un egress no ligado a instancia).
func nullUUID(id string) any {
	if id == "" {
		return nil
	}
	return id
}
