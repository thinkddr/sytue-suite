// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package merge

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// LO QUE OTROS SERVICIOS GUARDAN DE UNA PERSONA Y NO DE SU ORGANIZACIÓN.
//
// Cuando alguien borra su cuenta (ADR 0054), lo que es de la organización se
// queda en la organización: su correo, sus archivos de «Mi unidad», sus
// eventos, sus mensajes en los canales. Lo que se borra es lo que solo tiene
// sentido para esa persona: sus avisos, sus preferencias, los dispositivos a los
// que se le manda push, sus borradores y guardados de Rooms, sus favoritos.
//
// Va por el mismo camino que la fusión (ver el encabezado del paquete): las
// bases viven en la misma instancia y se abren con las credenciales de Sytue
// ID, así que no hacen falta endpoints internos nuevos ni un secreto por
// servicio para borrar unas filas. Y va dentro de `InMergeTx` por la misma
// razón: con FORCE ROW LEVEL SECURITY, un DELETE sin `app.merge='on'` borraría
// cero filas sin decir nada.
//
// La lista es CERRADA y cada tabla tiene su columna `subject`. Una tabla nueva
// con datos personales se añade aquí; el test de integración comprueba que
// cada una existe con esa columna y con su política de fusión, para que un
// renombrado no convierta el borrado en un no-op silencioso. La tabla no tiene
// por qué llevar `tenant`: un bloqueo de Cast es de la persona, no de una
// organización.
var DatosDeLaPersona = []struct {
	Servicio string
	Tablas   []string
}{
	{"hub", []string{"notifications", "notification_prefs", "presence"}},
	{"rooms", []string{
		"push_apns", "push_subscriptions", "user_notify_prefs",
		"message_drafts", "message_saves", "read_state", "thread_follows", "channel_categories",
		// A quién ha bloqueado (ADR 0073). Sus DENUNCIAS no: son de la organización.
		"user_blocks",
	}},
	// A quién ha bloqueado en Cast (ADR 0074; `subject` es columna generada sobre
	// `blocker_sub`). Sus denuncias no: son de la organización del contenido, y se
	// purgan con su plazo.
	{"cast", []string{"person_blocks"}},
	{"drive", []string{"favorites"}},
	// A quién ha bloqueado para las peticiones de firma de fuera (ADR 0059).
	{"docs", []string{"pdf_firma_bloqueos"}},
}

// BorrarPersona borra de UNA base las filas personales de `subject`, en una sola
// transacción. Idempotente: repetirlo tras un éxito borra cero filas.
func BorrarPersona(ctx context.Context, pool *pg.Pool, tablas []string, subject string, revokers ...callaccess.Revoker) (int, error) {
	if subject == "" {
		return 0, fmt.Errorf("merge: borrar persona sin subject")
	}
	total := 0
	err := pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		for _, t := range tablas {
			tag, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE subject = $1",
				pgx.Identifier{t}.Sanitize()), subject)
			if err != nil {
				return fmt.Errorf("merge: borrar persona en %s: %w", t, err)
			}
			total += int(tag.RowsAffected())
		}
		return callaccess.Flush(ctx, tx, "", firstCallRevoker(revokers))
	})
	return total, err
}

func firstCallRevoker(revokers []callaccess.Revoker) callaccess.Revoker {
	if len(revokers) > 0 {
		return revokers[0]
	}
	return nil
}
