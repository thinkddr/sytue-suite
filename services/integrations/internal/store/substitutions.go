// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// Substitution es «para este producto, esta organización usa esta conexión».
type Substitution struct {
	Product    string
	InstanceID string
}

// ListSubstitutions devuelve los productos que esta organización sirve con una app
// conectada. Los que NO aparecen los sirve el producto Sytue: la ausencia de fila ES el
// estado por defecto, no un dato que falte.
func (s *Store) ListSubstitutions(ctx context.Context) ([]Substitution, error) {
	out := make([]Substitution, 0, 8)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT product, instance_id FROM product_substitutions WHERE org_id = current_setting('app.tenant', true) ORDER BY product`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sub Substitution
			if err := rows.Scan(&sub.Product, &sub.InstanceID); err != nil {
				return err
			}
			out = append(out, sub)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar sustituciones: %w", err)
	}
	return out, nil
}

// SetSubstitution designa una conexión como sustituta de un producto (idempotente).
//
// Comprueba en la MISMA transacción que la instancia existe y está activa. Que exista lo
// garantizaría la clave ajena, pero que esté ACTIVA no: designar como sustituto de Sytue
// Mail una conexión desactivada dejaría a la organización sin correo en el lanzador, y
// el fallo aparecería después, lejos de la acción que lo causó.
func (s *Store) SetSubstitution(ctx context.Context, product, instanceID string) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx,
			`SELECT status FROM app_instances WHERE org_id = current_setting('app.tenant', true) AND id = $1`, instanceID).Scan(&status); err != nil {
			if isNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if status != string(domain.StatusActive) {
			return fmt.Errorf("%w: la conexión no está activa", domain.ErrInvalidInput)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO product_substitutions (org_id, product, instance_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (org_id, product)
			DO UPDATE SET instance_id = EXCLUDED.instance_id, updated_at = now()`,
			t.Slug, product, instanceID)
		return err
	})
	if err != nil {
		// ErrNotFound y ErrInvalidInput viajan tal cual: la capa HTTP los mapea a 404 y
		// 400. Envolverlos aquí los convertiría en un 500 opaco.
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrInvalidInput) {
			return err
		}
		return fmt.Errorf("store: fijar sustitución: %w", err)
	}
	return nil
}

// ClearSubstitution devuelve un producto al nativo de Sytue. Borrar algo que no está es
// un no-op: el resultado deseado —«que lo sirva Sytue»— ya se cumple, y devolver
// ErrNotFound obligaría a la UI a distinguir dos casos que para el usuario son el mismo.
func (s *Store) ClearSubstitution(ctx context.Context, product string) error {
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM product_substitutions WHERE org_id = current_setting('app.tenant', true) AND product = $1`, product)
		return err
	})
	if err != nil {
		return fmt.Errorf("store: quitar sustitución: %w", err)
	}
	return nil
}

// SubstitutedProducts devuelve qué productos deja servidos una instancia. Lo usa la baja:
// antes de desconectar una app hay que poder avisar de qué productos vuelven al nativo,
// en vez de que la organización lo descubra al abrir el lanzador.
func (s *Store) SubstitutedProducts(ctx context.Context, instanceID string) ([]string, error) {
	out := make([]string, 0, 4)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT product FROM product_substitutions WHERE org_id = current_setting('app.tenant', true) AND instance_id = $1 ORDER BY product`, instanceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: productos sustituidos: %w", err)
	}
	return out, nil
}
