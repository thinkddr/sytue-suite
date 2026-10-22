// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// MarcarPlantillaOrg marca un documento como plantilla de la organización y registra su
// primera versión (la instantánea `versionID`, ya hecha). Si ya estaba marcado solo
// cambia el nombre y la descripción y NO publica nada: publicar es PublicarPlantillaOrg.
// Devuelve si la marca es nueva.
func (s *Store) MarcarPlantillaOrg(ctx context.Context, docID string, kind domain.Kind, nombre, descripcion, sub, versionID string) (bool, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return false, err
	}
	var nueva bool
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var vivo bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM documents WHERE id = $1 AND deleted_at IS NULL)`, docID).Scan(&vivo); err != nil {
			return err
		}
		if !vivo {
			return domain.ErrNotFound
		}
		// xmax = 0 solo en la fila recién insertada: distingue marcar de renombrar sin
		// una lectura aparte que otra petición podría cambiar entre medias.
		if err := tx.QueryRow(ctx, `
			INSERT INTO doc_org_templates (document_id, tenant, kind, name, description, marked_by)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (document_id) DO UPDATE
			   SET name = EXCLUDED.name, description = EXCLUDED.description, updated_at = now()
			RETURNING (xmax = 0)`, docID, t.Slug, kind, nombre, descripcion, sub).Scan(&nueva); err != nil {
			return err
		}
		if !nueva {
			return logPlantilla(ctx, tx, t.Slug, docID, domain.AccionPlantillaRenombrada, sub, nil, nil)
		}
		if versionID == "" {
			return fmt.Errorf("sin instantánea que publicar")
		}
		// Quitar y volver a marcar sigue la numeración: las versiones viejas se conservan.
		n, err := numeroSiguiente(ctx, tx, docID)
		if err != nil {
			return err
		}
		return logPlantilla(ctx, tx, t.Slug, docID, domain.AccionPlantillaMarcada, sub, &versionID, &n)
	})
	if err != nil {
		return false, fmt.Errorf("store: marcar plantilla: %w", err)
	}
	return nueva, nil
}

// MarcarBloqueOrg dice si una plantilla ya marcada se ofrece como bloque para insertar
// (true) o como plantilla para crear (false). ErrNotFound si el documento no está marcado.
func (s *Store) MarcarBloqueOrg(ctx context.Context, docID string, bloque bool) error {
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE doc_org_templates SET es_bloque = $2 WHERE document_id = $1`, docID, bloque)
		if err == nil && tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return err
	})
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("store: marcar bloque: %w", err)
	}
	return err
}

// numeroSiguiente bloquea la marca (serializa dos publicaciones a la vez) y da el
// número que toca a la próxima versión.
func numeroSiguiente(ctx context.Context, tx pgx.Tx, docID string) (int, error) {
	var marcada bool
	if err := tx.QueryRow(ctx,
		`SELECT true FROM doc_org_templates WHERE document_id = $1 FOR UPDATE`, docID).Scan(&marcada); err != nil {
		if isNoRows(err) {
			return 0, domain.ErrNotFound
		}
		return 0, err
	}
	var n int
	err := tx.QueryRow(ctx,
		`SELECT coalesce(max(version_no), 0) + 1 FROM doc_org_template_log WHERE document_id = $1`, docID).Scan(&n)
	return n, err
}

// PublicarPlantillaOrg registra `versionID` (una instantánea ya hecha del documento)
// como nueva versión vigente de la plantilla y devuelve su número.
func (s *Store) PublicarPlantillaOrg(ctx context.Context, docID, sub, versionID string) (int, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return 0, err
	}
	var n int
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		if n, err = numeroSiguiente(ctx, tx, docID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE doc_org_templates SET updated_at = now() WHERE document_id = $1`, docID); err != nil {
			return err
		}
		return logPlantilla(ctx, tx, t.Slug, docID, domain.AccionPlantillaPublicada, sub, &versionID, &n)
	})
	if err != nil {
		return 0, fmt.Errorf("store: publicar plantilla: %w", err)
	}
	return n, nil
}

// QuitarPlantillaOrg quita la marca. El documento y sus versiones se quedan como están.
func (s *Store) QuitarPlantillaOrg(ctx context.Context, docID, sub string) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM doc_org_templates WHERE document_id = $1`, docID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return logPlantilla(ctx, tx, t.Slug, docID, domain.AccionPlantillaQuitada, sub, nil, nil)
	})
	if err != nil {
		return fmt.Errorf("store: quitar plantilla: %w", err)
	}
	return nil
}

// RegistrarUsoPlantillaOrg anota que alguien creó un documento desde la plantilla.
func (s *Store) RegistrarUsoPlantillaOrg(ctx context.Context, docID, sub string, versionNo int) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		return logPlantilla(ctx, tx, t.Slug, docID, domain.AccionPlantillaUsada, sub, nil, &versionNo)
	})
}

func logPlantilla(ctx context.Context, tx pgx.Tx, tenant, docID, accion, sub string, versionID *string, no *int) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO doc_org_template_log (tenant, document_id, action, actor_sub, version_id, version_no)
		VALUES ($1, $2, $3, $4, $5, $6)`, tenant, docID, accion, sub, versionID, no)
	return err
}

// ListarPlantillasOrg devuelve las plantillas de la organización cuyo documento sigue
// vivo (`kind` vacío = todas), con sus versiones. Un documento en la papelera deja de
// ofrecerse sin perder la marca: al restaurarlo vuelve.
func (s *Store) ListarPlantillasOrg(ctx context.Context, kind domain.Kind) ([]domain.PlantillaOrg, error) {
	out := []domain.PlantillaOrg{}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.document_id, p.kind, p.name, p.description, p.marked_by, p.updated_at, p.es_bloque
			FROM doc_org_templates p
			JOIN documents d ON d.id = p.document_id AND d.deleted_at IS NULL
			WHERE ($1 = '' OR p.kind::text = $1)
			ORDER BY lower(p.name), p.document_id`, string(kind))
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var p domain.PlantillaOrg
			if err := rows.Scan(&p.DocumentID, &p.Kind, &p.Name, &p.Description, &p.MarkedBy, &p.UpdatedAt, &p.Block); err != nil {
				rows.Close()
				return err
			}
			p.Key = domain.PrefijoPlantillaOrg + p.DocumentID
			p.Versions = []domain.VersionPlantilla{}
			out = append(out, p)
			ids = append(ids, p.DocumentID)
		}
		rows.Close()
		if err := rows.Err(); err != nil || len(out) == 0 {
			return err
		}
		porDoc, err := versionesDe(ctx, tx, ids)
		if err != nil {
			return err
		}
		for i := range out {
			out[i].Versions = append(out[i].Versions, porDoc[out[i].DocumentID]...)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar plantillas: %w", err)
	}
	return out, nil
}

// GetPlantillaOrg devuelve una plantilla viva con sus versiones, o ErrNotFound.
func (s *Store) GetPlantillaOrg(ctx context.Context, docID string) (*domain.PlantillaOrg, error) {
	var p domain.PlantillaOrg
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT p.document_id, p.kind, p.name, p.description, p.marked_by, p.updated_at, p.es_bloque
			FROM doc_org_templates p
			JOIN documents d ON d.id = p.document_id AND d.deleted_at IS NULL
			WHERE p.document_id = $1`, docID).
			Scan(&p.DocumentID, &p.Kind, &p.Name, &p.Description, &p.MarkedBy, &p.UpdatedAt, &p.Block); err != nil {
			return err
		}
		p.Key = domain.PrefijoPlantillaOrg + p.DocumentID
		porDoc, err := versionesDe(ctx, tx, []string{docID})
		p.Versions = append([]domain.VersionPlantilla{}, porDoc[docID]...)
		return err
	})
	if isNoRows(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: obtener plantilla: %w", err)
	}
	return &p, nil
}

// versionesDe agrupa las versiones publicadas de varios documentos, la más reciente primero.
func versionesDe(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]domain.VersionPlantilla, error) {
	rows, err := tx.Query(ctx, `
		SELECT l.document_id, l.version_no, l.version_id, v.snapshot_id, l.actor_sub, l.created_at
		FROM doc_org_template_log l
		JOIN doc_versions v ON v.id = l.version_id
		WHERE l.document_id = ANY($1::uuid[]) AND l.version_id IS NOT NULL
		ORDER BY l.document_id, l.version_no DESC`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]domain.VersionPlantilla{}
	for rows.Next() {
		var doc string
		var v domain.VersionPlantilla
		if err := rows.Scan(&doc, &v.No, &v.VersionID, &v.SnapshotID, &v.AuthorSub, &v.CreatedAt); err != nil {
			return nil, err
		}
		out[doc] = append(out[doc], v)
	}
	return out, rows.Err()
}

// RegistroPlantillaOrg es el registro de lo hecho con una plantilla, lo más reciente primero.
func (s *Store) RegistroPlantillaOrg(ctx context.Context, docID string, limite int) ([]domain.EntradaRegistroPlantilla, error) {
	limite = min(max(limite, 1), 500)
	out := []domain.EntradaRegistroPlantilla{}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT action, actor_sub, version_no, created_at
			FROM doc_org_template_log WHERE document_id = $1
			ORDER BY created_at DESC, id LIMIT $2`, docID, limite)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e domain.EntradaRegistroPlantilla
			if err := rows.Scan(&e.Action, &e.ActorSub, &e.VersionNo, &e.At); err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: registro de plantilla: %w", err)
	}
	return out, nil
}
