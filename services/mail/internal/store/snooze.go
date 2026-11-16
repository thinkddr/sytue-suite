// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Posponer y silenciar conversaciones (migraciones 0036 y 0037).
//
// Posponer MUEVE los mensajes del hilo que están en Recibidos al buzón de rol
// `snoozed` y apunta en mail_snoozes cuándo vuelven. Volver es el movimiento
// contrario, y lo hacen tres caminos: el trabajador al vencer la fecha
// (DespertarPospuestos), la persona («Quitar posposición», UnsnoozeThread) y una
// respuesta nueva al hilo (InsertMessageEntrante), que lo trae de vuelta como en
// Gmail: si alguien contesta, ya no es «más tarde».
//
// Silenciar marca el hilo (threads.muted) y archiva lo que tenga en Recibidos; la
// entrega manda al Archivo las respuestas nuevas de un hilo silenciado.
//
// Todo es owner-scoped por account_id además de RLS por tenant: un hilo ajeno no
// casa y sale ErrNotFound, sin distinguir «no es tuyo» de «no existe».

// buzonDeRolTx devuelve el buzón de un rol de la cuenta; ErrNotFound si no lo tiene.
func buzonDeRolTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, role domain.SpecialUse) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM mailboxes
		WHERE tenant = current_setting('app.tenant', true) AND account_id = $1 AND role = $2`,
		accountID, string(role)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, domain.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("store: buzón %q: %w", role, err)
	}
	return id, nil
}

// nombresDePospuestos son los nombres que se prueban, por orden, al crear el buzón
// de lo pospuesto. Los nombres son únicos entre hermanos sin distinguir mayúsculas,
// y alguien puede tener ya una carpeta propia «Snoozed»: no se adopta (sus mensajes
// no están pospuestos, y despertarlos los mandaría a Recibidos), se usa otro nombre.
// Los clientes lo rotulan por su rol, así que el nombre casi nunca se ve.
var nombresDePospuestos = []string{"Snoozed", "Snoozed (Sytue)", "Snoozed (Sytue) 2"}

// buzonPospuestosTx devuelve el buzón de rol `snoozed` de la cuenta, creándolo la
// primera vez. Concurrencia: el índice único por (cuenta, rol) hace que de dos altas
// a la vez gane una; la otra no inserta nada y lo lee después.
func buzonPospuestosTx(ctx context.Context, tx pgx.Tx, tenant string, accountID uuid.UUID) (uuid.UUID, error) {
	if id, err := buzonDeRolTx(ctx, tx, accountID, domain.RoleSnoozed); !errors.Is(err, domain.ErrNotFound) {
		return id, err
	}
	for _, nombre := range nombresDePospuestos {
		// Sin objetivo en ON CONFLICT: cubre tanto el rol ya creado por otra
		// transacción como un nombre ocupado por una carpeta propia.
		if _, err := tx.Exec(ctx, `
			INSERT INTO mailboxes (tenant, account_id, name, role)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`,
			tenant, accountID, nombre, string(domain.RoleSnoozed)); err != nil {
			return uuid.Nil, fmt.Errorf("store: crear buzón de pospuestos: %w", err)
		}
		if id, err := buzonDeRolTx(ctx, tx, accountID, domain.RoleSnoozed); !errors.Is(err, domain.ErrNotFound) {
			return id, err
		}
	}
	return uuid.Nil, fmt.Errorf("store: crear buzón de pospuestos: nombres ocupados")
}

// moverHiloTx mueve los mensajes vivos del hilo que están en `desde` a `hasta`,
// con UIDs consecutivos en el destino (como MoveMessages, en un solo statement).
// Devuelve los ids movidos, el más reciente primero.
func moverHiloTx(ctx context.Context, tx pgx.Tx, accountID, threadID, desde, hasta uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `
		WITH moving AS (
			SELECT id, row_number() OVER (ORDER BY received_at, id) - 1 AS rn
			FROM messages
			WHERE tenant = current_setting('app.tenant', true) AND account_id = $1
			  AND thread_id = $2 AND mailbox_id = $3 AND deleted_at IS NULL
		),
		reserve AS (
			UPDATE mailboxes
			SET uid_next = uid_next + (SELECT count(*) FROM moving), updated_at = now()
			WHERE tenant = current_setting('app.tenant', true) AND id = $4 AND account_id = $1
			  AND (SELECT count(*) FROM moving) > 0
			RETURNING uid_next - (SELECT count(*) FROM moving) AS base
		)
		UPDATE messages m
		SET mailbox_id = $4,
		    uid = (SELECT base FROM reserve) + moving.rn,
		    updated_at = now()
		FROM moving
		WHERE m.tenant = current_setting('app.tenant', true) AND m.id = moving.id
		RETURNING m.id, m.received_at`, accountID, threadID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("store: mover hilo: %w", err)
	}
	defer rows.Close()
	type movido struct {
		id uuid.UUID
		at time.Time
	}
	var todos []movido
	for rows.Next() {
		var m movido
		if err := rows.Scan(&m.id, &m.at); err != nil {
			return nil, err
		}
		todos = append(todos, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// RETURNING no garantiza orden: el más reciente primero, a mano.
	out := make([]uuid.UUID, 0, len(todos))
	for len(todos) > 0 {
		j := 0
		for i := range todos {
			if todos[i].at.After(todos[j].at) {
				j = i
			}
		}
		out = append(out, todos[j].id)
		todos = append(todos[:j], todos[j+1:]...)
	}
	return out, nil
}

// hiloDeLaCuentaTx confirma que el hilo es de la cuenta (y de la organización).
func hiloDeLaCuentaTx(ctx context.Context, tx pgx.Tx, accountID, threadID uuid.UUID) error {
	var ok bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM threads
		WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $2)`,
		threadID, accountID).Scan(&ok); err != nil {
		return fmt.Errorf("store: hilo: %w", err)
	}
	if !ok {
		return domain.ErrNotFound
	}
	return nil
}

// despertarHiloTx devuelve a Recibidos lo que el hilo tenga en el buzón de
// pospuestos y borra su posposición. `habia` dice si estaba pospuesto.
//
// Con `vencio` (la fecha del posponer, cuando la devolución es porque venció), el
// más reciente de lo devuelto vuelve SIN LEER y con unsnoozed_at = vencio, así que
// sube arriba de Recibidos a la hora de vuelta, como en Gmail: es lo que hace que se
// vea al volver, que para eso se pospuso. Con `vencio` en cero (lo trae la persona,
// o una respuesta nueva que ya va arriba por sí sola) vuelve a su sitio cronológico.
func despertarHiloTx(ctx context.Context, tx pgx.Tx, accountID, threadID uuid.UUID, vencio time.Time) (habia bool, err error) {
	tag, err := tx.Exec(ctx, `
		DELETE FROM mail_snoozes
		WHERE tenant = current_setting('app.tenant', true) AND account_id = $1 AND thread_id = $2`,
		accountID, threadID)
	if err != nil {
		return false, fmt.Errorf("store: quitar posposición: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	pospuestos, err := buzonDeRolTx(ctx, tx, accountID, domain.RoleSnoozed)
	if errors.Is(err, domain.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	entrada, err := buzonDeRolTx(ctx, tx, accountID, domain.RoleInbox)
	if err != nil {
		return true, err
	}
	movidos, err := moverHiloTx(ctx, tx, accountID, threadID, pospuestos, entrada)
	if err != nil {
		return true, err
	}
	if !vencio.IsZero() && len(movidos) > 0 {
		// Solo el más reciente: es el que representa la conversación en la lista
		// (la fila del hilo), y lo que quede del hilo más abajo se pliega en ella.
		if _, err := tx.Exec(ctx, `
			UPDATE messages SET flags = array_remove(flags, $2), unsnoozed_at = $3, updated_at = now()
			WHERE tenant = current_setting('app.tenant', true) AND id = $1`,
			movidos[0], domain.FlagSeen, vencio.UTC()); err != nil {
			return true, fmt.Errorf("store: marcar la vuelta: %w", err)
		}
	}
	return true, nil
}

// SnoozeThread pospone el hilo hasta `until`: lo que tenga en Recibidos pasa al
// buzón de pospuestos. Posponer otra vez un hilo ya pospuesto solo cambia la fecha.
// ErrInvalidSnooze si la fecha no vale; ErrNothingToSnooze si no hay nada en
// Recibidos ni estaba pospuesto; ErrNotFound si el hilo no es de la cuenta.
func (s *Store) SnoozeThread(ctx context.Context, accountID, threadID uuid.UUID, until time.Time) (domain.Snooze, error) {
	if err := domain.ValidarSnooze(until, time.Now()); err != nil {
		return domain.Snooze{}, err
	}
	out := domain.Snooze{ThreadID: threadID}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := hiloDeLaCuentaTx(ctx, tx, accountID, threadID); err != nil {
			return err
		}
		entrada, err := buzonDeRolTx(ctx, tx, accountID, domain.RoleInbox)
		if err != nil {
			return err
		}
		pospuestos, err := buzonPospuestosTx(ctx, tx, tenantOf(ctx), accountID)
		if err != nil {
			return err
		}
		movidos, err := moverHiloTx(ctx, tx, accountID, threadID, entrada, pospuestos)
		if err != nil {
			return err
		}
		if len(movidos) == 0 {
			// Nada que sacar de Recibidos: solo vale si ya estaba pospuesto (cambiar
			// la fecha desde la carpeta de pospuestos).
			var ya bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS(SELECT 1 FROM mail_snoozes
				WHERE tenant = current_setting('app.tenant', true) AND account_id = $1 AND thread_id = $2)`,
				accountID, threadID).Scan(&ya); err != nil {
				return fmt.Errorf("store: posponer: %w", err)
			}
			if !ya {
				return domain.ErrNothingToSnooze
			}
		}
		// Lo que se pospone otra vez deja de contar como «vuelto»: si se trae a mano
		// antes de tiempo, vuelve a su sitio cronológico y no al de la vuelta anterior.
		if _, err := tx.Exec(ctx, `
			UPDATE messages SET unsnoozed_at = NULL, updated_at = now()
			WHERE tenant = current_setting('app.tenant', true) AND account_id = $1
			  AND thread_id = $2 AND mailbox_id = $3 AND unsnoozed_at IS NOT NULL`,
			accountID, threadID, pospuestos); err != nil {
			return fmt.Errorf("store: posponer: %w", err)
		}
		return tx.QueryRow(ctx, `
			INSERT INTO mail_snoozes (thread_id, tenant, account_id, until)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (thread_id) DO UPDATE SET until = EXCLUDED.until, updated_at = now()
			RETURNING until`,
			threadID, tenantOf(ctx), accountID, until.UTC()).Scan(&out.Until)
	})
	if err != nil {
		return domain.Snooze{}, err
	}
	return out, nil
}

// UnsnoozeThread devuelve ya a Recibidos un hilo pospuesto. No lo marca sin leer ni
// lo sube arriba: quien lo trae a mano lo está mirando, y el «Deshacer» de posponer
// (que también pasa por aquí) debe dejarlo exactamente donde estaba. ErrNotFound si no estaba pospuesto o no es
// de la cuenta.
func (s *Store) UnsnoozeThread(ctx context.Context, accountID, threadID uuid.UUID) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		habia, err := despertarHiloTx(ctx, tx, accountID, threadID, time.Time{})
		if err != nil {
			return err
		}
		if !habia {
			return domain.ErrNotFound
		}
		return nil
	})
}

// ListSnoozes devuelve las posposiciones de la cuenta, la que vence antes primero.
func (s *Store) ListSnoozes(ctx context.Context, accountID uuid.UUID) ([]domain.Snooze, error) {
	var out []domain.Snooze
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT thread_id, until FROM mail_snoozes
			WHERE tenant = current_setting('app.tenant', true) AND account_id = $1
			ORDER BY until, thread_id`, accountID)
		if err != nil {
			return fmt.Errorf("store: list snoozes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var sn domain.Snooze
			if err := rows.Scan(&sn.ThreadID, &sn.Until); err != nil {
				return err
			}
			out = append(out, sn)
		}
		return rows.Err()
	})
	return out, err
}

// ThreadState dice si el hilo está silenciado y hasta cuándo está pospuesto.
// ErrNotFound si no es de la cuenta.
//
// Solo cuenta como pospuesto si aún le queda algo en el buzón de pospuestos: quien
// archiva o tira a la papelera un hilo desde «Pospuestos» ya lo ha sacado de la
// espera, y la franja «Pospuesto hasta…» mentiría sobre un hilo archivado hasta que
// el trabajador borrara la fila en su fecha (sin mover nada: no queda qué).
func (s *Store) ThreadState(ctx context.Context, accountID, threadID uuid.UUID) (domain.ThreadState, error) {
	var st domain.ThreadState
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT t.muted,
			       (SELECT s.until FROM mail_snoozes s
			         WHERE s.tenant = t.tenant AND s.thread_id = t.id
			           AND EXISTS (SELECT 1 FROM messages m
			                       JOIN mailboxes b ON b.id = m.mailbox_id AND b.tenant = m.tenant
			                       WHERE m.tenant = t.tenant AND m.thread_id = t.id
			                         AND m.account_id = t.account_id AND m.deleted_at IS NULL
			                         AND b.role = 'snoozed'))
			FROM threads t
			WHERE t.tenant = current_setting('app.tenant', true) AND t.id = $1 AND t.account_id = $2`,
			threadID, accountID).Scan(&st.Muted, &st.SnoozedUntil)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	})
	return st, err
}

// MuteThread silencia (o deja de silenciar) el hilo. Al silenciar, lo que tenga en
// Recibidos se archiva: silenciar es «no quiero ver más esto», y dejarlo en la
// Entrada obligaría a archivarlo además. Devuelve cuántos mensajes archivó.
// ErrNotFound si el hilo no es de la cuenta.
func (s *Store) MuteThread(ctx context.Context, accountID, threadID uuid.UUID, muted bool) (int, error) {
	var archivados int
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE threads SET muted = $3, updated_at = now()
			WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $2`,
			threadID, accountID, muted)
		if err != nil {
			return fmt.Errorf("store: silenciar hilo: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		if !muted {
			return nil
		}
		entrada, err := buzonDeRolTx(ctx, tx, accountID, domain.RoleInbox)
		if err != nil {
			return err
		}
		archivo, err := buzonDeRolTx(ctx, tx, accountID, domain.RoleArchive)
		if errors.Is(err, domain.ErrNotFound) {
			return nil // sin Archivo, el hilo queda silenciado donde está
		}
		if err != nil {
			return err
		}
		movidos, err := moverHiloTx(ctx, tx, accountID, threadID, entrada, archivo)
		archivados = len(movidos)
		return err
	})
	return archivados, err
}

// InsertMessageEntrante es InsertMessage para la ENTREGA de un correo recibido:
// con el hilo ya resuelto, si iba a Recibidos y el hilo está silenciado lo manda al
// Archivo (silenciado=true: quien llama no debe hacerlo sonar), y si el hilo estaba
// pospuesto lo devuelve a la Entrada junto con la respuesta nueva. Todo en la misma
// transacción que la inserción.
func (s *Store) InsertMessageEntrante(ctx context.Context, m *domain.Message) (silenciado bool, err error) {
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		silenciado, err = insertarEntranteTx(ctx, tx, tenantOf(ctx), m)
		return err
	})
	return silenciado, err
}

// insertarEntranteTx es el núcleo de InsertMessageEntrante sobre una transacción
// existente: la entrega SMTP lo llama dentro de la admisión (InsertEntrante), que ya
// tiene abierta la transacción del cerco de almacenamiento y la reserva del blob.
func insertarEntranteTx(ctx context.Context, tx pgx.Tx, tenant string, m *domain.Message) (silenciado bool, err error) {
	threadID, err := resolveThread(ctx, tx, tenant, m)
	if err != nil {
		return false, err
	}
	m.ThreadID = threadID
	entrada, err := buzonDeRolTx(ctx, tx, m.AccountID, domain.RoleInbox)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return false, err
	}
	if err == nil && m.MailboxID == entrada {
		var muted bool
		if err := tx.QueryRow(ctx, `
			SELECT muted FROM threads
			WHERE tenant = current_setting('app.tenant', true) AND id = $1`, threadID).Scan(&muted); err != nil {
			return false, fmt.Errorf("store: estado del hilo: %w", err)
		}
		if muted {
			archivo, err := buzonDeRolTx(ctx, tx, m.AccountID, domain.RoleArchive)
			if err == nil {
				m.MailboxID = archivo
				silenciado = true
			} else if !errors.Is(err, domain.ErrNotFound) {
				return false, err
			}
		} else if _, err := despertarHiloTx(ctx, tx, m.AccountID, threadID, time.Time{}); err != nil {
			return false, err
		}
	}
	return silenciado, insertarEnHiloTx(ctx, tx, tenant, m)
}

// Despertado es una cuenta a la que el trabajador acaba de devolver un hilo
// pospuesto: lo justo para mandar la señal de cambios del widget de Mail.
type Despertado struct {
	Tenant  string
	Subject string
}

// DespertarPospuestos devuelve a Recibidos los hilos cuya posposición venció
// (until <= now), sin leer y arriba (a la hora de `until`). Lo llama el trabajador de la cola, en todas las
// organizaciones (app.maintenance, como PromoteDueHeld): FOR UPDATE SKIP LOCKED
// reparte entre réplicas sin despertar dos veces el mismo hilo, y app.tenant se fija
// por fila antes de tocar sus mensajes. Todo en una transacción: si falla, nada
// cambia y el siguiente barrido lo reintenta.
func (s *Store) DespertarPospuestos(ctx context.Context, now time.Time, limit int) ([]Despertado, error) {
	if limit <= 0 {
		limit = 50
	}
	type vencido struct {
		threadID, accountID uuid.UUID
		tenant, subject     string
		until               time.Time
	}
	var out []Despertado
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT s.thread_id, s.account_id, s.tenant, a.subject, s.until
			FROM mail_snoozes s
			JOIN accounts a ON a.id = s.account_id AND a.tenant = s.tenant
			WHERE s.until <= $1
			ORDER BY s.until
			FOR UPDATE OF s SKIP LOCKED
			LIMIT $2`, now.UTC(), limit)
		if err != nil {
			return fmt.Errorf("store: reclamar pospuestos: %w", err)
		}
		var due []vencido
		for rows.Next() {
			var v vencido
			if err := rows.Scan(&v.threadID, &v.accountID, &v.tenant, &v.subject, &v.until); err != nil {
				rows.Close()
				return err
			}
			due = append(due, v)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, v := range due {
			if err := pg.SetTenant(ctx, tx, v.tenant); err != nil {
				return err
			}
			// La hora de vuelta es la del posponer, no la del barrido: la que se eligió,
			// aunque el trabajador pase unos segundos después.
			if _, err := despertarHiloTx(ctx, tx, v.accountID, v.threadID, v.until); err != nil {
				return err
			}
			out = append(out, Despertado{Tenant: v.tenant, Subject: v.subject})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
