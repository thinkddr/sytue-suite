// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// legiblePorCuaderno es la cláusula de LEER la nota `n` (con su cuaderno `nb`) por el
// cuaderno —dueño, organización, público, lector o editor—, salvo que la nota sea
// privada de otra persona. Es la regla de `ListNotes`.
func legiblePorCuaderno(p string) string {
	return `((nb.owner_sub = ` + p + ` OR nb.visibility IN ('organization','public')
	          OR EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id = nb.id AND l.sujeto = ` + p + `)
	          OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = ` + p + `))
	         AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = ` + p + `))`
}

// legiblePorCompartida es la de tenerla porque se la compartieron a `p` (0010), con edición si
// `editar`. Lo que compartió quien ya no escribe en ese cuaderno deja de valer, como en
// «Compartido conmigo» (NotasCompartidas) y al escribir (UpdateNote).
func legiblePorCompartida(p string, editar bool) string {
	cond := ""
	if editar {
		cond = " AND c.puede_editar"
	}
	return `(EXISTS (SELECT 1 FROM note_personas c WHERE c.note_id = n.id AND c.sujeto = ` + p + cond + `)
	         AND (nb.owner_sub = n.owner_sub OR nb.visibility = 'organization'
	              OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = n.owner_sub)))`
}

// notaLegible es poder LEER la nota: por el cuaderno o porque se la compartieron.
func notaLegible(p string) string {
	return "(" + legiblePorCuaderno(p) + " OR " + legiblePorCompartida(p, false) + ")"
}

// notaEscribible es poder ESCRIBIRLA, la regla de UpdateNote: por el cuaderno (dueño,
// organización o editora, y no privada de otra) o porque se la compartieron con edición.
func notaEscribible(p string) string {
	return "(" + escribiblePorCuaderno(p) + " OR " + legiblePorCompartida(p, true) + ")"
}

func escribiblePorCuaderno(p string) string {
	return `((nb.owner_sub = ` + p + ` OR nb.visibility = 'organization'
	           OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = ` + p + `))
	          AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = ` + p + `))`
}

const reminderCols = "r.note_id, n.notebook_id, r.due_at, r.all_day, r.rrule, r.after_days, r.tzid, r.dtstart"

// camposReminder son los destinos de Scan en el orden de reminderCols.
func camposReminder(r *domain.Reminder) []any {
	return []any{&r.NoteID, &r.NotebookID, &r.DueAt, &r.AllDay, &r.RRule, &r.AfterDays, &r.TZID, &r.DTStart}
}

func scanReminder(row pgx.Row) (domain.Reminder, error) {
	var r domain.Reminder
	err := row.Scan(camposReminder(&r)...)
	return r, err
}

// ListReminders son los recordatorios pendientes de `sub`, por vencimiento. Con tope:
// es lo que pinta el indicador de cada fila, y nadie tiene quinientos pendientes.
//
// También los de una TAREA que repite y ya sonó sin hacerse: siguen siendo suyos hasta
// que se complete (RepetirAlCompletar los muda), y sin ellos la fila perdía su «repite»
// y no había dónde cambiarlo.
func (s *Store) ListReminders(ctx context.Context, sub string) ([]domain.Reminder, error) {
	t := tenancy.MustFrom(ctx)
	out := []domain.Reminder{}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+reminderCols+`
			  FROM note_reminders r
			  JOIN notes n ON n.id = r.note_id AND n.deleted_at IS NULL
			 WHERE r.tenant = $1 AND r.owner_sub = $2
			   AND (r.fired_at IS NULL
			        OR (n.kind = 'todo' AND NOT n.checked AND (r.rrule <> '' OR r.after_days > 0)))
			 ORDER BY r.due_at
			 LIMIT 500`, t.Slug, sub)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanReminder(rows)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// SetReminder pone (o cambia) el recordatorio de `sub` en una nota que puede LEER: un
// recordatorio no escribe en la nota, así que leerla basta. Volver a ponerlo lo rearma.
// Su DTSTART es el vencimiento, salvo que se guarde otra vez tal cual: abrir y aceptar
// un «cada mes del 31» en febrero no lo vuelve «cada 28».
func (s *Store) SetReminder(ctx context.Context, noteID uuid.UUID, sub string, in domain.ReminderInput) (domain.Reminder, error) {
	t := tenancy.MustFrom(ctx)
	var r domain.Reminder
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `
			WITH n AS (
			    SELECT n.id, n.notebook_id
			      FROM notes n
			      JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
			     WHERE n.tenant = $1 AND n.id = $2 AND n.deleted_at IS NULL AND `+notaLegible("$3")+`
			)
			INSERT INTO note_reminders AS r (note_id, tenant, owner_sub, due_at, all_day, rrule, after_days, tzid, dtstart)
			SELECT n.id, $1, $3, $4, $5, $6, $7, $8, $4 FROM n
			ON CONFLICT (note_id, owner_sub) DO UPDATE
			   SET due_at = EXCLUDED.due_at, all_day = EXCLUDED.all_day, rrule = EXCLUDED.rrule,
			       after_days = EXCLUDED.after_days, tzid = EXCLUDED.tzid, fired_at = NULL,
			       dtstart = CASE WHEN r.due_at = EXCLUDED.due_at AND r.rrule = EXCLUDED.rrule AND r.tzid = EXCLUDED.tzid
			                      THEN r.dtstart ELSE EXCLUDED.dtstart END
			RETURNING r.note_id, (SELECT notebook_id FROM n), r.due_at, r.all_day, r.rrule, r.after_days, r.tzid, r.dtstart`,
			t.Slug, noteID, sub, in.DueAt, in.AllDay, in.RRule, in.AfterDays, in.TZID).
			Scan(camposReminder(&r)...)
		if isNoRows(e) {
			return domain.ErrNotFound
		}
		if checkViolation(e) {
			return domain.ErrInvalidInput
		}
		return e
	})
	return r, err
}

// DeleteReminder quita el recordatorio de `sub` en una nota. Quitar uno que no hay no
// es un error: el resultado es el mismo.
func (s *Store) DeleteReminder(ctx context.Context, noteID uuid.UUID, sub string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM note_reminders WHERE tenant = $1 AND note_id = $2 AND owner_sub = $3`,
			t.Slug, noteID, sub)
		return err
	})
}

// RepetirAlCompletar es lo que pasa al marcar hecha una TAREA que repite. Se llama en
// el paso de sin hacer a hecha, después de guardarla. Devuelve la siguiente, o nil.
//
//   - Si algún recordatorio que repite es de quien puede ESCRIBIR la nota, nace la
//     siguiente —una copia sin marcar en el mismo sitio, compartida con las mismas
//     personas— y todos los que repiten se mudan a ella con su próximo vencimiento.
//     Los de la hecha se quedan apagados (no borrados) y la hecha apunta a la copia en
//     `data.repeated_into`: es lo que deja deshacerlo (DeshacerRepeticion).
//   - Si solo son de quien la LEE, se reprograman ahí mismo: el recordatorio de quien
//     solo lee no puede escribir una tarea nueva en un cuaderno ajeno.
//
// Es idempotente: la nota se bloquea y la que ya apunta a su copia no crea otra.
//
// ponytail: la copia no lleva enlaces ni estados de etiqueta; son de la que se hizo.
func (s *Store) RepetirAlCompletar(ctx context.Context, noteID uuid.UUID, ahora time.Time) (*domain.Note, error) {
	t := tenancy.MustFrom(ctx)
	var nueva *domain.Note
	err := s.inWriteTx(ctx, func(tx pgx.Tx) error {
		var ya bool
		e := tx.QueryRow(ctx, `
			SELECT COALESCE(data ? 'repeated_into', false) FROM notes
			 WHERE tenant = $1 AND id = $2 AND kind = 'todo' AND checked AND deleted_at IS NULL
			   FOR UPDATE`, t.Slug, noteID).Scan(&ya)
		if isNoRows(e) || ya {
			return nil
		}
		if e != nil {
			return e
		}
		van, escribe, err := mudanzasDe(ctx, tx, t.Slug, noteID, ahora)
		if err != nil || len(van) == 0 {
			return err
		}
		if !escribe {
			for _, m := range van {
				if _, err := tx.Exec(ctx, `
					UPDATE note_reminders SET due_at = $3, fired_at = NULL
					 WHERE tenant = $1 AND note_id = $2 AND owner_sub = $4`,
					t.Slug, noteID, m.r.DueAt, m.duena); err != nil {
					return err
				}
			}
			return nil
		}
		n, err := crearSiguiente(ctx, tx, t.Slug, noteID, van, ahora)
		if err == nil {
			nueva = &n
		}
		return err
	})
	return nueva, err
}

// mudanza es un recordatorio que repite, ya con su próximo vencimiento, y si su dueña
// puede escribir la nota.
type mudanza struct {
	duena   string
	escribe bool
	r       domain.Reminder
}

// mudanzasDe bloquea los recordatorios que repiten de la nota y calcula su siguiente
// vencimiento; los agotados no salen. `escribe`: alguno es de quien puede escribirla.
func mudanzasDe(ctx context.Context, tx pgx.Tx, slug string, noteID uuid.UUID, ahora time.Time) ([]mudanza, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.owner_sub, `+notaEscribible("r.owner_sub")+`, `+reminderCols+`
		  FROM note_reminders r
		  JOIN notes n ON n.id = r.note_id
		  JOIN notebooks nb ON nb.id = n.notebook_id
		 WHERE r.tenant = $1 AND r.note_id = $2 AND (r.rrule <> '' OR r.after_days > 0)
		   FOR UPDATE OF r`, slug, noteID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var van []mudanza
	escribe := false
	for rows.Next() {
		var m mudanza
		if err := rows.Scan(append([]any{&m.duena, &m.escribe}, camposReminder(&m.r)...)...); err != nil {
			return nil, false, err
		}
		if sig, ok := domain.Siguiente(m.r, ahora); ok {
			m.r.DueAt = sig
			van = append(van, m)
			escribe = escribe || m.escribe
		}
	}
	return van, escribe, rows.Err()
}

// crearSiguiente crea la copia sin marcar de la tarea `noteID`, compartida con las
// mismas personas, le muda los recordatorios `van` y deja la hecha apuntando a ella.
func crearSiguiente(ctx context.Context, tx pgx.Tx, slug string, noteID uuid.UUID, van []mudanza, ahora time.Time) (domain.Note, error) {
	n, err := scanNote(tx.QueryRow(ctx, `
		INSERT INTO notes (tenant, notebook_id, owner_sub, kind, title, content, content_cipher,
		                   data, x, y, col, position, width, group_id, visibility, indexable)
		SELECT tenant, notebook_id, owner_sub, kind, title, content, content_cipher,
		       data - 'carried_from' - 'repeated_into', x, y, col, position, width, group_id, visibility, indexable
		  FROM notes WHERE tenant = $1 AND id = $2
		RETURNING `+noteColsBase, slug, noteID), slug)
	if err != nil {
		return n, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO note_personas (note_id, tenant, sujeto, puede_editar, concedido_por)
		SELECT $3, tenant, sujeto, puede_editar, concedido_por FROM note_personas WHERE tenant = $1 AND note_id = $2`,
		slug, noteID, n.ID); err != nil {
		return n, err
	}
	for _, m := range van {
		if _, err := tx.Exec(ctx, `
			INSERT INTO note_reminders (note_id, tenant, owner_sub, due_at, all_day, rrule, after_days, tzid, dtstart)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			n.ID, slug, m.duena, m.r.DueAt, m.r.AllDay, m.r.RRule, m.r.AfterDays, m.r.TZID, m.r.DTStart); err != nil {
			return n, err
		}
	}
	// Los que se mudaron se quedan apagados en la hecha, para deshacer; los agotados se van.
	_, err = tx.Exec(ctx, `
		WITH mudados AS (SELECT owner_sub FROM note_reminders WHERE note_id = $3),
		     apagados AS (
		         UPDATE note_reminders SET fired_at = $4
		          WHERE tenant = $1 AND note_id = $2 AND owner_sub IN (SELECT owner_sub FROM mudados)),
		     agotados AS (
		         DELETE FROM note_reminders
		          WHERE tenant = $1 AND note_id = $2 AND (rrule <> '' OR after_days > 0)
		            AND owner_sub NOT IN (SELECT owner_sub FROM mudados))
		UPDATE notes SET data = COALESCE(data, '{}'::jsonb) || jsonb_build_object('repeated_into', $3::text)
		 WHERE tenant = $1 AND id = $2`, slug, noteID, n.ID, ahora)
	return n, err
}

// DeshacerRepeticion es lo contrario, al desmarcar una tarea que creó su siguiente: si
// esa copia sigue intacta (nadie la ha tocado desde que nació), va a la papelera y los
// recordatorios vuelven a la original con el vencimiento que tenían. Si ya la tocaron,
// la copia se queda y la original suelta sus recordatorios mudados, para no tener dos.
// Devuelve la copia retirada, o nil.
func (s *Store) DeshacerRepeticion(ctx context.Context, noteID uuid.UUID) (*uuid.UUID, error) {
	t := tenancy.MustFrom(ctx)
	var retirada *uuid.UUID
	err := s.inWriteTx(ctx, func(tx pgx.Tx) error {
		var copia *uuid.UUID
		e := tx.QueryRow(ctx, `
			SELECT (data ->> 'repeated_into')::uuid FROM notes
			 WHERE tenant = $1 AND id = $2 AND NOT checked AND deleted_at IS NULL
			   FOR UPDATE`, t.Slug, noteID).Scan(&copia)
		if isNoRows(e) || (e == nil && copia == nil) {
			return nil
		}
		if e != nil {
			return e
		}
		if _, err := tx.Exec(ctx, `UPDATE notes SET data = data - 'repeated_into' WHERE tenant = $1 AND id = $2`,
			t.Slug, noteID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE notes SET deleted_at = now(), updated_at = now()
			 WHERE tenant = $1 AND id = $2 AND deleted_at IS NULL AND NOT checked AND updated_at = created_at`,
			t.Slug, *copia)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			_, err := tx.Exec(ctx, `
				DELETE FROM note_reminders WHERE tenant = $1 AND note_id = $2 AND (rrule <> '' OR after_days > 0)`,
				t.Slug, noteID)
			return err
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM note_reminders
			 WHERE tenant = $1 AND note_id = $3
			   AND owner_sub IN (SELECT owner_sub FROM note_reminders WHERE note_id = $2)`,
			t.Slug, noteID, *copia); err != nil {
			return err
		}
		// Vuelven armados, salvo los ya vencidos: esos ya sonaron y no se repiten ahora.
		if _, err := tx.Exec(ctx, `
			UPDATE note_reminders SET fired_at = CASE WHEN due_at <= now() THEN now() END
			 WHERE tenant = $1 AND note_id = $2 AND (rrule <> '' OR after_days > 0)`,
			t.Slug, noteID); err != nil {
			return err
		}
		retirada = copia
		return nil
	})
	return retirada, err
}

// RecordatorioVencido es lo que el worker necesita para avisar. Title va VACÍO en un
// cuaderno cifrado: el aviso viaja por el Hub en claro y no puede llevarlo.
type RecordatorioVencido struct {
	Tenant, OwnerSub   string
	NoteID, NotebookID uuid.UUID
	Title              string
	DueAt              time.Time
	// SoloCompartida: la lee porque se la compartieron, no por su cuaderno, que no ve;
	// el enlace del aviso la abre en «Compartido conmigo».
	SoloCompartida bool
}

// ReclamarRecordatorios reclama hasta `limite` recordatorios vencidos de TODAS las
// organizaciones y llama a `avisar` con los que tocan. Devuelve cuántos reclamó.
//
// # Cross-tenant, con la política estrecha
//
// Corre bajo `InMaintenanceTx` y la política `_mantenimiento` (0005 y 0014). `FOR
// UPDATE SKIP LOCKED` reparte la cola entre réplicas sin que dos avisen del mismo.
//
// Antes de avisar se mira, CON EL TENANT DE LA FILA fijado, que su dueña aún pueda
// leer la nota: las tablas de compartir no tienen política de mantenimiento, así que
// sin `app.tenant` «me la compartieron» saldría falso. Lo que no toca avisar —la nota
// en la papelera, una que ya no se lee, una tarea ya hecha— se da por avisado en
// silencio para que no vuelva cada minuto.
//
// Una nota que no es tarea y repite por calendario se reprograma aquí; las tareas que
// repiten se reprograman al completarse (RepetirAlCompletar), no al sonar. Salvo en una
// tarea YA hecha: ahí solo quedan los de quien la lee, que no la completa, y siguen
// sonando y reprogramándose como los de una nota.
func (s *Store) ReclamarRecordatorios(ctx context.Context, ahora time.Time, limite int, avisar func(RecordatorioVencido)) (int, error) {
	n := 0
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT r.tenant, r.owner_sub, `+reminderCols+`,
			       CASE WHEN nb.encrypted THEN '' ELSE n.title END,
			       n.kind = 'todo', n.checked, n.deleted_at IS NULL AND nb.deleted_at IS NULL
			  FROM note_reminders r
			  JOIN notes n ON n.id = r.note_id
			  JOIN notebooks nb ON nb.id = n.notebook_id
			 WHERE r.fired_at IS NULL AND r.due_at <= $1
			 ORDER BY r.due_at
			 LIMIT $2
			   FOR UPDATE OF r SKIP LOCKED`, ahora, limite)
		if err != nil {
			return err
		}
		var filas []vencido
		for rows.Next() {
			var f vencido
			dest := append([]any{&f.v.Tenant, &f.v.OwnerSub}, camposReminder(&f.r)...)
			if err := rows.Scan(append(dest, &f.v.Title, &f.tarea, &f.hecha, &f.viva)...); err != nil {
				rows.Close()
				return err
			}
			f.v.NoteID, f.v.NotebookID, f.v.DueAt = f.r.NoteID, f.r.NotebookID, f.r.DueAt
			filas = append(filas, f)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		n = len(filas)
		for i := range filas {
			f := &filas[i]
			toca, err := resolverVencido(ctx, tx, f, ahora)
			if err != nil {
				return err
			}
			if toca {
				avisar(f.v)
			}
		}
		return nil
	})
	return n, err
}

// vencido es una fila reclamada: el aviso, el recordatorio y el estado de su nota.
type vencido struct {
	v                  RecordatorioVencido
	r                  domain.Reminder
	tarea, hecha, viva bool
}

// resolverVencido decide si `f` avisa y lo deja reprogramado (una nota que repite por
// calendario, o una tarea hecha que repite) o dado por avisado. Corre en la
// transacción de ReclamarRecordatorios.
func resolverVencido(ctx context.Context, tx pgx.Tx, f *vencido, ahora time.Time) (bool, error) {
	toca := f.viva && (!f.tarea || !f.hecha || f.r.Repite())
	if toca {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant', $1, true)`, f.v.Tenant); err != nil {
			return false, err
		}
		var cuaderno, compartida bool
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(bool_or(`+legiblePorCuaderno("$2")+`), false), COALESCE(bool_or(`+legiblePorCompartida("$2", false)+`), false)
			  FROM notes n JOIN notebooks nb ON nb.id = n.notebook_id
			 WHERE n.id = $1`,
			f.r.NoteID, f.v.OwnerSub).Scan(&cuaderno, &compartida); err != nil {
			return false, err
		}
		toca, f.v.SoloCompartida = cuaderno || compartida, !cuaderno && compartida
	}
	sig, repite := time.Time{}, false
	if toca && ((!f.tarea && f.r.RRule != "") || (f.tarea && f.hecha)) {
		sig, repite = domain.Siguiente(f.r, ahora)
	}
	var err error
	if repite {
		_, err = tx.Exec(ctx, `UPDATE note_reminders SET due_at = $3 WHERE note_id = $1 AND owner_sub = $2`,
			f.r.NoteID, f.v.OwnerSub, sig)
	} else {
		_, err = tx.Exec(ctx, `UPDATE note_reminders SET fired_at = $3 WHERE note_id = $1 AND owner_sub = $2`,
			f.r.NoteID, f.v.OwnerSub, ahora)
	}
	return toca, err
}
