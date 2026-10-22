// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// AvisosHistoricos son los avisos de «compartido contigo» de la organización del
// contexto que la campana no llegó a guardar, con la hora en que se compartió.
//
// Solo lo que la persona SIGUE pudiendo abrir: comparticiones con ella (no enlaces ni
// grupos, como en vivo), sin revocar ni caducar, de un fichero o carpeta que no está
// en la papelera, y nunca a quien lo compartió. Sin sesión no hay nombre de quien
// compartió, así que el título no lo nombra.
func AvisosHistoricos(ctx context.Context, pool *pg.Pool, publicURL string) ([]hubnotify.Historico, error) {
	t := tenancy.MustFrom(ctx)
	var out []hubnotify.Historico
	err := pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetMaintenance(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT sh.created_at, sh.id::text, sh.subject, coalesce(f.id, d.id)::text, coalesce(f.name, d.name)
			FROM shares sh
			LEFT JOIN files f ON f.tenant = sh.tenant AND f.id = sh.file_id AND f.deleted_at IS NULL
			LEFT JOIN folders d ON d.tenant = sh.tenant AND d.id = sh.folder_id AND d.deleted_at IS NULL
			WHERE sh.tenant = $1 AND sh.kind = 'user' AND sh.revoked_at IS NULL
			  AND (sh.expires_at IS NULL OR sh.expires_at > now())
			  AND sh.subject <> sh.created_by
			  AND coalesce(f.id, d.id) IS NOT NULL
			ORDER BY sh.created_at, sh.id`, t.Slug)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var h hubnotify.Historico
			var id, para, nodo, nombre string
			if err := rows.Scan(&h.Fecha, &id, &para, &nodo, &nombre); err != nil {
				return err
			}
			h.Notification = avisoDeCompartido(publicURL, t.Slug, para, nodo, nombre, "")
			h.Clave = "drive:share:" + id
			out = append(out, h)
		}
		return rows.Err()
	})
	return out, err
}
