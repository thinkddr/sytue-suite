// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SealPlaintextSecrets cifra los secretos de firma que quedaron EN CLARO de antes
// de que Rooms cifrara en reposo (`outgoing_webhooks.signing_secret` y
// `slash_commands.signing_secret`).
//
// Hace falta porque la lectura es deliberadamente tolerante: keycrypto devuelve
// tal cual lo que no lleva el prefijo `encv1:`, para que una integración creada
// ayer siga firmando hoy. Esa tolerancia mantiene el servicio en pie, pero deja
// el secreto legible para quien lea la base — que es justo lo que se quería
// evitar. Esto lo cierra de una pasada.
//
// Cruza organizaciones por `InMaintenanceTx` (política PERMISSIVE `app.maintenance`,
// migración 0025), sin BYPASSRLS y como el resto de tareas de operación del
// servicio. Por eso se ejecuta desde el subcomando `rooms seal-secrets`, jamás
// desde una petición.
//
// Idempotente: solo toca lo que aún no está sellado, así que se puede correr en
// cada despliegue sin pensarlo. Devuelve cuántas filas selló de cada tabla.
func (s *Store) SealPlaintextSecrets(ctx context.Context) (webhooks, comandos int, err error) {
	if !s.cipher.Enabled() {
		return 0, 0, ErrNoCipher
	}
	tablas := []struct {
		nombre string
		n      *int
	}{
		{"outgoing_webhooks", &webhooks},
		{"slash_commands", &comandos},
	}

	for _, t := range tablas {
		type fila struct{ id, secret string }
		var pendientes []fila

		// Lectura y escritura en transacciones separadas a propósito: cifrar es
		// trabajo de CPU y no tiene sentido tener abierta una transacción —ni el
		// GUC de mantenimiento— mientras se hace.
		if rerr := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			rows, qerr := tx.Query(ctx,
				`SELECT id, signing_secret FROM `+t.nombre+`
				  WHERE signing_secret <> '' AND signing_secret NOT LIKE 'encv1:%'`)
			if qerr != nil {
				return qerr
			}
			defer rows.Close()
			for rows.Next() {
				var f fila
				if serr := rows.Scan(&f.id, &f.secret); serr != nil {
					return serr
				}
				pendientes = append(pendientes, f)
			}
			return rows.Err()
		}); rerr != nil {
			return webhooks, comandos, fmt.Errorf("store: leer %s: %w", t.nombre, rerr)
		}

		for _, f := range pendientes {
			sealed, eerr := s.cipher.Encrypt(f.secret)
			if eerr != nil {
				return webhooks, comandos, fmt.Errorf("store: cifrar %s/%s: %w", t.nombre, f.id, eerr)
			}
			if uerr := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
				// El NOT LIKE se repite en el UPDATE: si dos operadores lanzan el
				// sellado a la vez, el segundo no vuelve a cifrar lo ya cifrado
				// (cifrar dos veces dejaría el secreto irrecuperable).
				tag, xerr := tx.Exec(ctx,
					`UPDATE `+t.nombre+` SET signing_secret = $2
					  WHERE id = $1 AND signing_secret NOT LIKE 'encv1:%'`, f.id, sealed)
				if xerr != nil {
					return xerr
				}
				*t.n += int(tag.RowsAffected())
				return nil
			}); uerr != nil {
				return webhooks, comandos, fmt.Errorf("store: sellar %s/%s: %w", t.nombre, f.id, uerr)
			}
		}
	}
	return webhooks, comandos, nil
}
