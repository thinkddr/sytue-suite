// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store es la capa de persistencia de Sytue Mail sobre PostgreSQL (pgx).
// Metadatos (cuentas, buzones, sobres de mensaje, cola de salida) en Postgres
// —única fuente de verdad—; los cuerpos MIME crudos y adjuntos viven en S3.
//
// Aislamiento multi-tenant por RLS: todas las operaciones de negocio se ejecutan
// dentro de pg.InTenantTx, que fija `app.tenant` LOCAL a la transacción. Las
// tareas cross-tenant legítimas (resolución de dirección del MX entrante en
// LookupAddress y drenaje de la cola de salida) corren en pg.InMaintenanceTx, que
// fija `app.maintenance='on'` para activar la política PERMISSIVE de mantenimiento
// SIN requerir un rol BYPASSRLS (que anularía el aislamiento también para JMAP).
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

// Store agrupa el acceso a datos de Sytue Mail.
type Store struct {
	pool *pg.Pool
}

// New crea el Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// Healthz delega en el pool (para sondas /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// Pool expone el pool subyacente (para outbox de eventos / cola).
func (s *Store) Pool() *pg.Pool { return s.pool }

// ---------------------------------------------------------------------------
// Cuentas y aprovisionamiento
// ---------------------------------------------------------------------------

// EnsureAccount localiza o crea la cuenta del subject en el tenant del contexto,
// provisionando los buzones de sistema en la misma transacción. Idempotente.
func (s *Store) EnsureAccount(ctx context.Context, subject, address, displayName string, quotaBytes int64) (domain.Account, error) {
	return s.EnsureAccountConCuota(ctx, subject, address, displayName, quotaBytes, false)
}

// EnsureAccountConCuota es EnsureAccount con la cuota dictada por el plan.
//
// `fijar` falso: `quotaBytes` es el defecto del servicio y solo vale para una
// cuenta NUEVA. Verdadero: la cuota viene del token de Sytue ID (`mailbox_bytes`,
// cuentas personales) y manda sobre la fila: al pasar a 1 TB o al caducar el
// pago, el buzón se ajusta en la siguiente petición. Solo escribe si CAMBIA, para
// no perder la garantía de «leer no escribe» que este método defiende arriba.
func (s *Store) EnsureAccountConCuota(ctx context.Context, subject, address, displayName string, quotaBytes int64, fijar bool) (domain.Account, error) {
	var acc domain.Account
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		t := tenantOf(ctx)
		// Leer ANTES de escribir. Este método es el preámbulo de toda petición JMAP
		// (44 handlers lo llaman), y el upsert de abajo reescribía la fila en CADA
		// una: `updated_at = now()` garantiza que la actualización es real, así que
		// dejaba una tupla muerta, su WAL y un bloqueo sobre la fila de la cuenta
		// que serializaba las peticiones concurrentes del mismo buzón. Si la cuenta
		// ya existe y el token no trae nada distinto, no hay NADA que escribir.
		//
		// Los buzones de sistema tampoco hace falta reprovisionarlos: se crean en
		// ESTA misma transacción que la fila de la cuenta (o las dos o ninguna), y
		// DeleteMailbox se niega a borrar un buzón con rol (ErrSpecialMailbox). Que
		// la cuenta exista implica que sus seis buzones existen, de modo que los
		// seis INSERT ... ON CONFLICT DO NOTHING por petición eran trabajo muerto.
		row := tx.QueryRow(ctx, `
			SELECT id, tenant, subject, address, display_name, quota_bytes, used_bytes, created_at, updated_at
			FROM accounts WHERE tenant = $1 AND subject = $2`, t, subject)
		// Comparación EXACTA, la misma que haría el upsert: un claim vacío no pisa
		// lo que hay (COALESCE/NULLIF) y uno distinto —aunque solo cambie de caja—
		// sí debe escribir, porque el upsert lo asignaría tal cual.
		switch got, gerr := scanAccount(row); {
		case gerr == nil && fijar && quotaBytes > 0 && got.QuotaBytes != quotaBytes &&
			(address == "" || address == got.Address) &&
			(displayName == "" || displayName == got.DisplayName):
			// Solo cambia la cuota: una escritura mínima y explícita, sin pasar por
			// el upsert de abajo, que reescribiría dirección y nombre.
			row = tx.QueryRow(ctx, `
				UPDATE accounts SET quota_bytes = $2, updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1
				RETURNING id, tenant, subject, address, display_name, quota_bytes, used_bytes, created_at, updated_at`,
				got.ID, quotaBytes)
			var e error
			acc, e = scanAccount(row)
			return e
		case gerr == nil && (address == "" || address == got.Address) &&
			(displayName == "" || displayName == got.DisplayName):
			acc = got
			return nil
		case gerr != nil && !errors.Is(gerr, pgx.ErrNoRows):
			return fmt.Errorf("store: leer cuenta: %w", gerr)
		}
		// La dirección del token MANDA: quien la asigna es Sytue ID y puede cambiarla
		// (un cambio de dominio de la organización, un renombrado). Conservar la
		// primera para siempre dejaba la cuenta anclada a una dirección que ya no
		// existe: el correo a la nueva no encontraba destinatario y los alias del
		// dominio nuevo se rechazaban por «dominio no servido».
		// Auto-sanador: un token sin claims de perfil (address/display_name vacíos)
		// nunca pisa con vacío lo que ya había.
		row = tx.QueryRow(ctx, `
			INSERT INTO accounts (tenant, subject, address, display_name, quota_bytes)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (tenant, subject) DO UPDATE SET
				address      = COALESCE(NULLIF(EXCLUDED.address, ''), accounts.address),
				display_name = COALESCE(NULLIF(EXCLUDED.display_name, ''), accounts.display_name),
				updated_at   = now()
			RETURNING id, tenant, subject, address, display_name, quota_bytes, used_bytes, created_at, updated_at`,
			t, subject, address, displayName, quotaBytes)
		var err error
		acc, err = scanAccount(row)
		if err != nil {
			return fmt.Errorf("store: ensure account: %w", err)
		}
		return ensureDefaultMailboxes(ctx, tx, t, acc.ID)
	})
	return acc, err
}

func ensureDefaultMailboxes(ctx context.Context, tx pgx.Tx, tenant string, accountID uuid.UUID) error {
	for _, mb := range domain.DefaultMailboxes {
		_, err := tx.Exec(ctx, `
			INSERT INTO mailboxes (tenant, account_id, name, role)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (account_id, role) WHERE role <> '' DO NOTHING`,
			tenant, accountID, mb.Name, string(mb.Role))
		if err != nil {
			return fmt.Errorf("store: provisionar buzón %q: %w", mb.Name, err)
		}
	}
	return nil
}

// AccountBySubject devuelve la cuenta del subject en el tenant del contexto.
func (s *Store) AccountBySubject(ctx context.Context, subject string) (domain.Account, error) {
	var acc domain.Account
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Defensa en profundidad: filtrar por tenant en la consulta ADEMÁS de RLS.
		// El índice único es (tenant, subject), así que el subject no es único global;
		// el predicado explícito garantiza el aislamiento aunque RLS no aplicara (p. ej.
		// un rol con BYPASSRLS) y evita resolver la cuenta de otro tenant.
		row := tx.QueryRow(ctx, `
			SELECT id, tenant, subject, address, display_name, quota_bytes, used_bytes, created_at, updated_at
			FROM accounts WHERE tenant = $1 AND subject = $2`, tenantOf(ctx), subject)
		var err error
		acc, err = scanAccount(row)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, domain.ErrNotFound
	}
	return acc, err
}

// ResolvedAddress es el resultado de resolver una dirección entrante: la cuenta
// destino y su tenant. Usado por el MX (transacción de mantenimiento) antes de
// fijar app.tenant.
type ResolvedAddress struct {
	Tenant    string
	AccountID uuid.UUID
	Subject   string
	Address   string
}

// LookupAddress resuelve una dirección (principal o alias) a su cuenta/tenant.
// Se ejecuta SIN contexto de tenant (el MX aún no lo conoce): corre en una
// transacción de MANTENIMIENTO (app.maintenance='on') que activa la política
// PERMISSIVE cross-tenant, en lugar de depender de un rol BYPASSRLS. Devuelve
// ErrNoSuchUser si no existe.
func (s *Store) LookupAddress(ctx context.Context, address string) (ResolvedAddress, error) {
	var r ResolvedAddress
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT a.tenant, a.id AS account_id, a.subject, u.addr FROM (
				SELECT tenant, id AS account_id, lower(address) AS addr FROM accounts
				UNION ALL
				SELECT tenant, account_id, lower(address) AS addr FROM aliases
			) u
			JOIN accounts a ON a.tenant = u.tenant AND a.id = u.account_id
			WHERE u.addr = lower($1) LIMIT 1`, address)
		return row.Scan(&r.Tenant, &r.AccountID, &r.Subject, &r.Address)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedAddress{}, domain.ErrNoSuchUser
		}
		return ResolvedAddress{}, fmt.Errorf("store: lookup address: %w", err)
	}
	return r, nil
}

// ---------------------------------------------------------------------------
// Buzones
// ---------------------------------------------------------------------------

// ListMailboxes devuelve los buzones de una cuenta con contadores (total/no leído).
func (s *Store) ListMailboxes(ctx context.Context, accountID uuid.UUID) ([]domain.Mailbox, error) {
	var out []domain.Mailbox
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT m.id, m.tenant, m.account_id, m.parent_id, m.name, m.role,
			       m.uid_validity, m.uid_next, m.created_at, m.updated_at,
			       COUNT(msg.id) FILTER (WHERE msg.deleted_at IS NULL) AS total,
			       COUNT(msg.id) FILTER (WHERE msg.deleted_at IS NULL AND NOT (msg.flags @> ARRAY['\Seen'])) AS unread
			FROM mailboxes m
			LEFT JOIN messages msg ON msg.tenant = m.tenant AND msg.mailbox_id = m.id
			WHERE m.tenant = current_setting('app.tenant', true) AND m.account_id = $1
			GROUP BY m.id
			ORDER BY CASE m.role WHEN 'inbox' THEN 0 ELSE 1 END, m.name`, accountID)
		if err != nil {
			return fmt.Errorf("store: list mailboxes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			mb, err := scanMailbox(rows)
			if err != nil {
				return err
			}
			out = append(out, mb)
		}
		return rows.Err()
	})
	return out, err
}

// MailboxByRole devuelve el buzón con el rol dado de una cuenta (p. ej. Inbox).
func (s *Store) MailboxByRole(ctx context.Context, accountID uuid.UUID, role domain.SpecialUse) (domain.Mailbox, error) {
	var mb domain.Mailbox
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant, account_id, parent_id, name, role, uid_validity, uid_next, created_at, updated_at
			FROM mailboxes WHERE tenant = current_setting('app.tenant', true) AND account_id = $1 AND role = $2`, accountID, string(role))
		var err error
		mb, err = scanMailboxRow(row)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Mailbox{}, domain.ErrNotFound
	}
	return mb, err
}

// MailboxByID devuelve un buzón CONCRETO de una cuenta. El filtro por cuenta no
// es decorativo: es lo que impide que un identificador ajeno acabe recibiendo
// correo importado.
func (s *Store) MailboxByID(ctx context.Context, accountID, mailboxID uuid.UUID) (domain.Mailbox, error) {
	var mb domain.Mailbox
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant, account_id, parent_id, name, role, uid_validity, uid_next, created_at, updated_at
			FROM mailboxes WHERE tenant = current_setting('app.tenant', true) AND account_id = $1 AND id = $2`, accountID, mailboxID)
		var err error
		mb, err = scanMailboxRow(row)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Mailbox{}, domain.ErrNotFound
	}
	return mb, err
}

// CreateMailbox crea un buzón de usuario (sin rol especial).
func (s *Store) CreateMailbox(ctx context.Context, accountID uuid.UUID, parentID *uuid.UUID, name string) (domain.Mailbox, error) {
	if !domain.ValidMailboxName(name) {
		return domain.Mailbox{}, domain.ErrInvalidAddress
	}
	var mb domain.Mailbox
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO mailboxes (tenant, account_id, parent_id, name)
			VALUES ($1, $2, $3, $4)
			RETURNING id, tenant, account_id, parent_id, name, role, uid_validity, uid_next, created_at, updated_at`,
			tenantOf(ctx), accountID, parentID, name)
		var err error
		mb, err = scanMailboxRow(row)
		return err
	})
	if isUniqueViolation(err) {
		return domain.Mailbox{}, domain.ErrMailboxExists
	}
	return mb, err
}

// ---------------------------------------------------------------------------
// Mensajes
// ---------------------------------------------------------------------------

// InsertMessage inserta un sobre de mensaje en un buzón, asignando el siguiente
// UID IMAP de forma atómica (incrementa mailboxes.uid_next bajo bloqueo de fila).
// El cuerpo MIME crudo ya debe estar en S3 bajo m.BlobKey.
func (s *Store) InsertMessage(ctx context.Context, m *domain.Message) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return insertMessageTx(ctx, tx, tenantOf(ctx), m)
	})
}

// insertMessageTx es el núcleo de InsertMessage sobre una transacción existente:
// reserva el UID IMAP, resuelve el hilo, inserta el sobre y acumula el uso. Lo
// reutiliza la liberación de diferidos (PromoteDueHeld) para crear la copia en
// Sent DENTRO de la misma transacción de mantenimiento (exactly-once). `tenant`
// debe estar fijado en la sesión (app.tenant) para que el INSERT pase su RLS.
func insertMessageTx(ctx context.Context, tx pgx.Tx, tenant string, m *domain.Message) error {
	// Reservar UID y bloquear el buzón para serializar inserciones concurrentes.
	var uid uint32
	if err := tx.QueryRow(ctx, `
		UPDATE mailboxes SET uid_next = uid_next + 1, updated_at = now()
		WHERE tenant = current_setting('app.tenant', true) AND id = $1 RETURNING uid_next - 1`, m.MailboxID).Scan(&uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("store: reservar UID: %w", err)
	}
	m.UID = uid

	// Threading: agrupar por In-Reply-To/Message-ID dentro de la cuenta.
	threadID, err := resolveThread(ctx, tx, tenant, m)
	if err != nil {
		return err
	}
	m.ThreadID = threadID

	// Normalizar slices de direcciones/flags: las columnas text[] son NOT NULL
	// DEFAULT '{}', pero un []string nil de Go se envía como SQL NULL y viola la
	// restricción. Coaccionar a slice vacío preserva la semántica "sin destinatarios".
	normAddrs(m)
	row := tx.QueryRow(ctx, `
		INSERT INTO messages (
			tenant, account_id, mailbox_id, thread_id, uid, rfc822_msg_id, in_reply_to,
			subject, from_addr, from_name, to_addrs, cc_addrs, bcc_addrs, reply_to,
			snippet, blob_key, size_bytes, flags, has_attach,
			spf_result, dkim_result, dmarc_result, received_at, sent_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
		RETURNING id, received_at`,
		tenant, m.AccountID, m.MailboxID, m.ThreadID, m.UID, m.MessageID, m.InReplyTo,
		m.Subject, m.FromAddr, m.FromName, m.To, m.Cc, m.Bcc, m.ReplyTo,
		m.Snippet, m.BlobKey, m.SizeBytes, m.Flags, m.HasAttach,
		string(m.SPFResult), string(m.DKIMResult), string(m.DMARCResult), nz(m.ReceivedAt), nzPtr(m.SentAt))
	if err := row.Scan(&m.ID, &m.ReceivedAt); err != nil {
		return fmt.Errorf("store: insert message: %w", err)
	}

	// Acumular uso de almacenamiento en la cuenta. El uso es el tamaño LÓGICO del
	// mensaje: lo que la persona ve como ocupado no depende de que el mismo
	// correo lo tenga también otra organización ni de cuánto comprima.
	if _, err := tx.Exec(ctx, `
		UPDATE accounts SET used_bytes = used_bytes + $2, updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1`,
		m.AccountID, m.SizeBytes); err != nil {
		return fmt.Errorf("store: actualizar uso: %w", err)
	}
	return sumarReferencia(ctx, tx, m.BlobKey)
}

func resolveThread(ctx context.Context, tx pgx.Tx, tenant string, m *domain.Message) (uuid.UUID, error) {
	if m.InReplyTo != "" {
		var tid uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT thread_id FROM messages
			WHERE tenant = current_setting('app.tenant', true) AND account_id = $1 AND rfc822_msg_id = $2 AND thread_id IS NOT NULL
			ORDER BY received_at DESC LIMIT 1`, m.AccountID, m.InReplyTo).Scan(&tid)
		if err == nil {
			return tid, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("store: resolver hilo: %w", err)
		}
	}
	var tid uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO threads (tenant, account_id, subject) VALUES ($1, $2, $3) RETURNING id`,
		tenant, m.AccountID, m.Subject).Scan(&tid); err != nil {
		return uuid.Nil, fmt.Errorf("store: crear hilo: %w", err)
	}
	return tid, nil
}

// ListMessages pagina los sobres de un buzón (más recientes primero) por
// KEYSET: before/beforeID son el (received_at, id) del último mensaje de la
// página anterior; con before en cero devuelve la primera página. Sin OFFSET:
// el coste por página es constante aunque el buzón tenga millones de mensajes
// (el índice messages_mailbox_idx ya ordena por received_at DESC; id desempata).
func (s *Store) ListMessages(ctx context.Context, mailboxID uuid.UUID, limit int, before time.Time, beforeID uuid.UUID) ([]domain.Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := selectMessageColsConHilo + `
		WHERE m.tenant = current_setting('app.tenant', true) AND mailbox_id = $1 AND deleted_at IS NULL
		ORDER BY received_at DESC, id DESC LIMIT $2`
	args := []any{mailboxID, limit}
	if !before.IsZero() {
		// received_at <= $3 es redundante con la comparación de fila, pero es lo
		// que permite al planner acotar el arranque del index scan en el cursor:
		// una row comparison solo actúa de index qual si TODAS sus columnas están
		// en el índice, y messages_mailbox_idx no incluye id. Sin el predicado
		// escalar degradaría a filtro post-scan (coste O(posición), como OFFSET).
		// La comparación de fila queda para descartar los empates exactos de
		// received_at con id >= cursor.
		q = selectMessageColsConHilo + `
			WHERE m.tenant = current_setting('app.tenant', true) AND mailbox_id = $1 AND deleted_at IS NULL
			  AND received_at <= $3 AND (received_at, id) < ($3, $4)
			ORDER BY received_at DESC, id DESC LIMIT $2`
		args = append(args, before, beforeID)
	}
	var out []domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("store: list messages: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			msg, err := scanMessageConHilo(rows)
			if err != nil {
				return err
			}
			out = append(out, msg)
		}
		return rows.Err()
	})
	return out, err
}

// ListThreadMessages devuelve TODOS los mensajes vivos de un hilo, ATRAVESANDO
// carpetas: la copia en Enviados de tu propia respuesta, lo ya archivado y lo que
// sigue en Recibidos salen juntos. Es la diferencia entre una vista de
// conversación y una lista plegada por carpeta —ListMessages filtra por
// mailbox_id, así que un hilo abierto desde Recibidos nunca enseñaba lo que tú
// contestaste—.
//
// Owner-scoped por account_id (además de RLS por tenant): un thread_id ajeno no
// casa ninguna fila y sale vacío, sin distinguir «no es tuyo» de «no existe»
// (anti-enumeración). Orden reciente→antiguo, el mismo que ListMessages, para
// que el cliente pliegue las dos listas sin reordenar.
//
// El índice messages_thread_idx (tenant, thread_id) WHERE deleted_at IS NULL ya
// resuelve el filtro; el orden es un sort sobre las filas del hilo.
//
// ponytail: sin paginación. Un hilo son decenas de mensajes, no millones; el
// tope duro evita que un hilo patológico se lleve la memoria por delante. Si
// hiciera falta, keyset como ListMessages.
func (s *Store) ListThreadMessages(ctx context.Context, accountID, threadID uuid.UUID, limit int) ([]domain.Message, error) {
	if limit <= 0 || limit > maxThreadMessages {
		limit = maxThreadMessages
	}
	var out []domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, selectMessageCols+`
			  AND account_id = $1 AND thread_id = $2 AND deleted_at IS NULL
			ORDER BY received_at DESC, id DESC LIMIT $3`, accountID, threadID, limit)
		if err != nil {
			return fmt.Errorf("store: list thread messages: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			msg, err := scanMessage(rows)
			if err != nil {
				return err
			}
			out = append(out, msg)
		}
		return rows.Err()
	})
	return out, err
}

// maxThreadMessages acota el hilo que se devuelve de una vez (ver ListThreadMessages).
const maxThreadMessages = 200

// GetMessage devuelve un sobre por id.
func (s *Store) GetMessage(ctx context.Context, id uuid.UUID) (domain.Message, error) {
	var msg domain.Message
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, selectMessageCols+` AND id = $1 AND deleted_at IS NULL`, id)
		var err error
		msg, err = scanMessageRow(row)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Message{}, domain.ErrNotFound
	}
	return msg, err
}

// SetFlags añade (add=true) o quita (add=false) un flag/keyword de un mensaje.
// Owner-scoped por account_id (como SetFlagsBatch): un id ajeno del mismo tenant
// no casa y devuelve ErrNotFound (anti-enumeración), además de RLS por tenant.
// Toca updated_at para que el delta incremental (Changes) lo reporte.
func (s *Store) SetFlags(ctx context.Context, accountID, id uuid.UUID, flag string, add bool) error {
	q := `UPDATE messages SET flags = array_append(array_remove(flags, $2), $2), updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $3 AND deleted_at IS NULL`
	if !add {
		q = `UPDATE messages SET flags = array_remove(flags, $2), updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $3 AND deleted_at IS NULL`
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, q, id, flag, accountID)
		if err != nil {
			return fmt.Errorf("store: set flags: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// MoveMessage mueve un mensaje a otro buzón, reasignando su UID en el destino.
// Owner-scoped por account_id (como MoveMessages): tanto el mensaje como el buzón
// destino deben ser de la cuenta; un id ajeno devuelve ErrNotFound.
func (s *Store) MoveMessage(ctx context.Context, accountID, id, destMailbox uuid.UUID) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var uid uint32
		if err := tx.QueryRow(ctx, `
			UPDATE mailboxes SET uid_next = uid_next + 1 WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $2 RETURNING uid_next - 1`,
			destMailbox, accountID).Scan(&uid); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("store: reservar UID destino: %w", err)
		}
		tag, err := tx.Exec(ctx, `
			UPDATE messages SET mailbox_id = $2, uid = $3, updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $4 AND deleted_at IS NULL`,
			id, destMailbox, uid, accountID)
		if err != nil {
			return fmt.Errorf("store: mover mensaje: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// DeleteMessage hace soft-delete y descuenta el uso de la cuenta. Owner-scoped
// por account_id: un id ajeno del mismo tenant devuelve ErrNotFound. El blob S3
// se recolecta de forma diferida por un job de GC (no en la ruta de borrado).
func (s *Store) DeleteMessage(ctx context.Context, accountID, id uuid.UUID) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var size int64
		err := tx.QueryRow(ctx, `
			UPDATE messages SET deleted_at = now(), updated_at = now()
			WHERE tenant = current_setting('app.tenant', true) AND id = $1 AND account_id = $2 AND deleted_at IS NULL
			RETURNING size_bytes`, id, accountID).Scan(&size)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("store: borrar mensaje: %w", err)
		}
		_, err = tx.Exec(ctx, `
			UPDATE accounts SET used_bytes = GREATEST(used_bytes - $2, 0), updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1`,
			accountID, size)
		return err
	})
}

// LA ORGANIZACIÓN VA EN EL SQL. Todas las sentencias de este store que tocan una
// tabla con `tenant` lo comparan con `current_setting('app.tenant', true)`, que es
// lo que fija pg.InTenantTx (y el worker, fila a fila, con pg.SetTenant). Las
// políticas de fila dicen lo mismo, pero no se evalúan para un superusuario ni para
// un rol con BYPASSRLS; el predicado sí, con cualquier rol. Sin `app.tenant` fijado
// no casa ninguna fila: falla cerrado. Lo vigila TestElSQLLlevaLaOrganizacion.
//
// selectMessageCols trae ya ese predicado: quien lo usa AÑADE condiciones con AND.
const selectMessageCols = `
	SELECT id, tenant, account_id, mailbox_id, thread_id, uid, rfc822_msg_id, in_reply_to,
	       subject, from_addr, from_name, to_addrs, cc_addrs, bcc_addrs, reply_to,
	       snippet, blob_key, size_bytes, flags, has_attach,
	       spf_result, dkim_result, dmarc_result, received_at, sent_at
	FROM messages
	WHERE tenant = current_setting('app.tenant', true)`

// selectMessageColsConHilo añade el recuento del hilo ENTERO de la cuenta. Es una
// subconsulta correlacionada y no un JOIN con GROUP BY porque así se resuelve por el
// índice parcial `messages_thread_idx (tenant, thread_id) WHERE deleted_at IS NULL`:
// una búsqueda por fila de la página (50 como mucho), no un agregado de la carpeta
// entera. Sigue siendo UNA consulta: lo que se evita es el N+1 de red, que es el caro.
const selectMessageColsConHilo = `
	SELECT m.id, m.tenant, m.account_id, m.mailbox_id, m.thread_id, m.uid, m.rfc822_msg_id, m.in_reply_to,
	       m.subject, m.from_addr, m.from_name, m.to_addrs, m.cc_addrs, m.bcc_addrs, m.reply_to,
	       m.snippet, m.blob_key, m.size_bytes, m.flags, m.has_attach,
	       m.spf_result, m.dkim_result, m.dmarc_result, m.received_at, m.sent_at,
	       (SELECT count(*) FROM messages t
	         WHERE t.tenant = m.tenant AND t.thread_id = m.thread_id
	           AND t.account_id = m.account_id AND t.deleted_at IS NULL) AS thread_count
	FROM messages m`

// normAddrs coacciona los slices de direcciones y flags de un mensaje a slice
// vacío (no nil), de modo que pgx los serialice como '{}' y no como SQL NULL en
// las columnas text[] NOT NULL (to_addrs/cc_addrs/bcc_addrs/reply_to/flags).
func normAddrs(m *domain.Message) {
	if m.To == nil {
		m.To = []string{}
	}
	if m.Cc == nil {
		m.Cc = []string{}
	}
	if m.Bcc == nil {
		m.Bcc = []string{}
	}
	if m.ReplyTo == nil {
		m.ReplyTo = []string{}
	}
	if m.Flags == nil {
		m.Flags = []string{}
	}
}

func nz(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}

func nzPtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// Dominios son los dominios de correo dados de alta en Sytue Mail: los de todas las
// direcciones (principales y alias) de todas las organizaciones, que son justo los que
// el MX acepta como propios (ver LookupAddress). Calendar los pide para saber quién es
// de la suite y a quién hay que mandarle la invitación por correo.
//
// Cruza organizaciones, así que va en la transacción de mantenimiento, igual que
// LookupAddress. Solo sale el dominio, nunca una dirección.
func (s *Store) Dominios(ctx context.Context) ([]string, error) {
	var out []string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT lower(split_part(address, '@', 2)) AS d FROM (
				SELECT address FROM accounts
				UNION ALL
				SELECT address FROM aliases
			) u
			WHERE split_part(address, '@', 2) <> ''
			ORDER BY d`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("store: dominios: %w", err)
	}
	return out, nil
}
