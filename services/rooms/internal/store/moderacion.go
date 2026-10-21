// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Denuncias y bloqueos de Rooms (ADR 0073, migración 0038).
//
// DENUNCIA: de la organización que ALOJA la conversación. La escribe quien puede
// leer el canal, y la ven y la resuelven quienes ya moderaban ese canal antes de
// que esto existiera —los administradores del equipo que lo aloja— y los
// administradores de la organización. Nadie ve una denuncia contra sí mismo, y
// quién denunció no sale de aquí en ninguna respuesta.
//
// BLOQUEO: de la PERSONA, entre organizaciones. Por eso todo lo de `user_blocks`
// va por la política de mantenimiento, y SIEMPRE por un `sub` que viene del token
// verificado o de la propia base, nunca del cuerpo de la petición.
package store

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// reportsPerDay es el tope de denuncias de una persona en 24 h, por organización.
// Con la deduplicación, es lo que impide usar la denuncia para acosar o para
// inundar la cola. Se cuenta en la base y no en memoria: vale con varias réplicas.
const reportsPerDay = 20

// reportsPageSize acota la cola.
// ponytail: sin paginar; con 200 abiertas el problema ya no es de paginación.
const reportsPageSize = 200

// enMantenimientoDe abre una transacción de mantenimiento con el tenant del
// contexto fijado: ve las filas de la organización Y los bloqueos de todas. Es el
// mismo par que usa el worker de programados (ClaimDueScheduled).
func (s *Store) enMantenimientoDe(ctx context.Context, fn func(tx pgx.Tx, tenant string) error) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, t.Slug); err != nil {
			return err
		}
		return fn(tx, t.Slug)
	})
}

// ---------------------------------------------------------------------------
// Bloqueos
// ---------------------------------------------------------------------------

// bloqueoEnDirectoTx devuelve ErrBlocked si el canal es un mensaje directo de DOS
// personas y entre ellas hay un bloqueo, en cualquiera de los dos sentidos: quien
// está bloqueado no escribe, y quien bloqueó tampoco hasta que desbloquee. Un
// grupo o un canal no se cierran por un bloqueo entre dos de sus miembros: ahí el
// bloqueo quita los avisos y el cliente pliega los mensajes.
//
// La transacción tiene que VER los bloqueos de todas las organizaciones
// (enMantenimientoDe, o la del worker de programados).
func bloqueoEnDirectoTx(ctx context.Context, tx pgx.Tx, tenant string, channelID uuid.UUID, author string) error {
	var bloqueado bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS (
		   SELECT 1
		     FROM channels c
		     JOIN channel_members otro
		       ON otro.tenant = c.tenant AND otro.channel_id = c.id AND otro.subject <> $3
		     JOIN user_blocks b
		       ON (b.subject = otro.subject AND b.blocked_sub = $3)
		       OR (b.subject = $3 AND b.blocked_sub = otro.subject)
		    WHERE c.tenant = $1 AND c.id = $2 AND c.kind = 'direct'
		      AND (SELECT count(*) FROM channel_members m
		            WHERE m.tenant = c.tenant AND m.channel_id = c.id) = 2)`,
		tenant, channelID, author).Scan(&bloqueado)
	if err != nil {
		return err
	}
	if bloqueado {
		return domain.ErrBlocked
	}
	return nil
}

// quienesBloquean devuelve, de los candidatos, los que han bloqueado al autor. Es
// lo que quita un aviso (push, APNs, Hub) antes de mandarlo.
func (s *Store) quienesBloquean(ctx context.Context, author string, candidatos []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(candidatos) == 0 {
		return out, nil
	}
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT subject FROM user_blocks WHERE blocked_sub = $1 AND subject = ANY($2)`,
			author, candidatos)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sub string
			if err := rows.Scan(&sub); err != nil {
				return err
			}
			out[sub] = true
		}
		return rows.Err()
	})
	return out, err
}

// sinQuienesBloquean quita de una lista de destinatarios de un aviso a quienes han
// bloqueado al autor. Si la consulta falla NO se avisa a nadie: un aviso de menos
// se recupera abriendo la app; uno de alguien bloqueado, no.
func sinQuienesBloquean[T any](ctx context.Context, s *Store, author string, xs []T, sub func(T) string) ([]T, error) {
	subs := make([]string, len(xs))
	for i, x := range xs {
		subs[i] = sub(x)
	}
	bloquean, err := s.quienesBloquean(ctx, author, subs)
	if err != nil {
		return nil, err
	}
	if len(bloquean) == 0 {
		return xs, nil
	}
	out := xs[:0]
	for _, x := range xs {
		if !bloquean[sub(x)] {
			out = append(out, x)
		}
	}
	return out, nil
}

// personaDelCanalTx devuelve el nombre de alguien con quien `actor` comparte el
// canal: un miembro, o el autor de algún mensaje (quien ya se fue sigue teniendo
// sus mensajes a la vista). ErrNotFound si no es ni lo uno ni lo otro, que es lo
// que impide bloquear o denunciar a un `sub` cualquiera.
func personaDelCanalTx(ctx context.Context, tx pgx.Tx, tenant string, channelID uuid.UUID, subject string) (string, error) {
	var name string
	err := tx.QueryRow(ctx,
		`SELECT name FROM (
		   SELECT name, 0 AS orden FROM channel_members
		    WHERE tenant = $1 AND channel_id = $2 AND subject = $3
		   UNION ALL
		   (SELECT author_name, 1 FROM messages
		     WHERE tenant = $1 AND channel_id = $2 AND author_sub = $3
		     ORDER BY created_at DESC LIMIT 1)
		 ) p ORDER BY orden LIMIT 1`,
		tenant, channelID, subject).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return name, err
}

// BlockMember bloquea a alguien con quien `actor` comparte el canal. Idempotente.
func (s *Store) BlockMember(ctx context.Context, channelID uuid.UUID, actor, subject string) (domain.Block, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" || subject == actor || domain.IsBotAuthor(subject) {
		return domain.Block{}, domain.ErrInvalidInput
	}
	var b domain.Block
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, tenant string) error {
		ch, err := getChannelTx(ctx, tx, tenant, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, tenant, ch, actor); err != nil {
			return err
		}
		name, err := personaDelCanalTx(ctx, tx, tenant, channelID, subject)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx,
			`INSERT INTO user_blocks (tenant, subject, blocked_sub, blocked_name)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (subject, blocked_sub) DO UPDATE SET blocked_name = EXCLUDED.blocked_name
			 RETURNING blocked_sub, blocked_name, created_at`,
			tenant, actor, subject, name).Scan(&b.Subject, &b.Name, &b.CreatedAt)
		if err != nil {
			return err
		}
		return s.reconcileMatrixBlockPairTx(ctx, tx, actor, subject)
	})
	return b, err
}

// ListBlocks devuelve a quién tiene bloqueado `actor`, lo bloqueara en la
// organización que lo bloqueara.
func (s *Store) ListBlocks(ctx context.Context, actor string) ([]domain.Block, error) {
	out := []domain.Block{}
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT blocked_sub, blocked_name, created_at FROM user_blocks
			  WHERE subject = $1 ORDER BY created_at DESC`, actor)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b domain.Block
			if err := rows.Scan(&b.Subject, &b.Name, &b.CreatedAt); err != nil {
				return err
			}
			out = append(out, b)
		}
		return rows.Err()
	})
	return out, err
}

// Unblock quita el bloqueo de `actor` sobre `subject`. Idempotente: quitar uno que
// no existe no es un error (ni dice si existía).
func (s *Store) Unblock(ctx context.Context, actor, subject string) error {
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`DELETE FROM user_blocks WHERE subject = $1 AND blocked_sub = $2`, actor, subject)
		if err != nil {
			return err
		}
		return s.reconcileMatrixBlockPairTx(ctx, tx, actor, subject)
	})
}

// ---------------------------------------------------------------------------
// Denuncias
// ---------------------------------------------------------------------------

// NewReport es una denuncia por registrar. Con MessageID se denuncia el mensaje
// (y a su autor); sin él, a la persona TargetSub dentro del canal ChannelID.
type NewReport struct {
	Reporter  string
	MessageID *uuid.UUID
	ChannelID uuid.UUID
	TargetSub string
	Reason    string
	Note      string
}

// CreatedReport es lo que el servicio necesita tras registrar una denuncia.
type CreatedReport struct {
	ID uuid.UUID
	// Nueva es false si ya existía (deduplicación): no se avisa otra vez.
	Nueva       bool
	ChannelName string
	// Moderadores son los administradores del equipo que aloja el canal, sin quien
	// denuncia ni el denunciado: a quienes se avisa.
	Moderadores []string
}

// CreateReport registra una denuncia. Verjas, todas en la misma transacción:
// quien denuncia puede leer el canal; lo denunciado está en ESE canal; no es uno
// mismo; y no ha pasado del tope diario.
//
//nolint:gocognit // una transacción con sus verjas en orden; trocearla las separaría de lo que protegen.
func (s *Store) CreateReport(ctx context.Context, rec NewReport) (CreatedReport, error) {
	t := tenancy.MustFrom(ctx)
	note := strings.TrimSpace(rec.Note)
	if !domain.ValidReportReason(rec.Reason) || utf8.RuneCountInString(note) > domain.ReportNoteMaxRunes {
		return CreatedReport{}, domain.ErrInvalidInput
	}
	var out CreatedReport
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		channelID, target, targetName := rec.ChannelID, strings.TrimSpace(rec.TargetSub), ""
		if rec.MessageID != nil {
			err := tx.QueryRow(ctx,
				`SELECT channel_id, author_sub, author_name FROM messages
				  WHERE tenant = $1 AND id = $2 AND deleted_at IS NULL`,
				t.Slug, *rec.MessageID).Scan(&channelID, &target, &targetName)
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			if err != nil {
				return err
			}
		}
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, rec.Reporter); err != nil {
			return err
		}
		if target == "" || target == rec.Reporter {
			return domain.ErrInvalidInput
		}
		if rec.MessageID == nil {
			if targetName, err = personaDelCanalTx(ctx, tx, t.Slug, channelID, target); err != nil {
				return err
			}
		}
		out.ChannelName = ch.Name

		var hoy int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM message_reports
			  WHERE tenant = $1 AND reporter_sub = $2 AND created_at > now() - interval '24 hours'`,
			t.Slug, rec.Reporter).Scan(&hoy); err != nil {
			return err
		}

		err = tx.QueryRow(ctx,
			`INSERT INTO message_reports (tenant, reporter_sub, target_sub, target_name, channel_id, message_id, reason, note)
			 SELECT $1, $2, $3, $4, $5::uuid, $6::uuid, $7, $8 WHERE $9::int < $10::int
			 ON CONFLICT DO NOTHING RETURNING id`,
			t.Slug, rec.Reporter, target, targetName, channelID, rec.MessageID, rec.Reason, note,
			hoy, reportsPerDay).Scan(&out.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			// O ya la había denunciado (se devuelve la suya, sin avisar otra vez) o ha
			// llegado al tope.
			err = tx.QueryRow(ctx,
				`SELECT id FROM message_reports
				  WHERE tenant = $1 AND reporter_sub = $2
				    AND ((message_id IS NOT NULL AND message_id = $3::uuid)
				      OR ($3::uuid IS NULL AND message_id IS NULL AND channel_id = $4
				          AND target_sub = $5 AND status = 'open'))
				  LIMIT 1`,
				t.Slug, rec.Reporter, rec.MessageID, channelID, target).Scan(&out.ID)
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrTooManyLookups
			}
			return err
		}
		if err != nil {
			return err
		}
		out.Nueva = true

		rows, err := tx.Query(ctx,
			`SELECT subject FROM team_members
			  WHERE tenant = $1 AND team_id = $2 AND role = 'admin' AND subject NOT IN ($3, $4)`,
			t.Slug, ch.TeamID, rec.Reporter, target)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sub string
			if err := rows.Scan(&sub); err != nil {
				return err
			}
			out.Moderadores = append(out.Moderadores, sub)
		}
		return rows.Err()
	})
	return out, err
}

// reportSelect lee una denuncia como la ve quien modera. La VERJA va en la propia
// consulta y la comparten la cola y la resolución, para que no puedan divergir:
// $2 es quien mira y $3 si es administrador de la organización. Ve la denuncia
// quien administra la organización o el equipo que aloja el canal, y nunca el
// denunciado.
const reportSelect = `
	SELECT r.id, r.channel_id, c.name, c.kind, r.target_sub, r.target_name, r.message_id,
	       COALESCE(m.body, ''), m.created_at, r.reason, r.note, r.status,
	       COALESCE(r.resolved_by, ''), r.resolved_at, r.created_at, r.resolved_from
	  FROM message_reports r
	  JOIN channels c ON c.tenant = r.tenant AND c.id = r.channel_id
	  LEFT JOIN messages m ON m.tenant = r.tenant AND m.id = r.message_id AND m.deleted_at IS NULL
	 WHERE r.tenant = $1 AND r.target_sub <> $2
	   AND ($3 OR EXISTS (SELECT 1 FROM team_members tm
	                       WHERE tm.tenant = r.tenant AND tm.team_id = c.team_id
	                         AND tm.subject = $2 AND tm.role = 'admin'))`

func scanReport(row pgx.Row) (domain.Report, error) {
	var r domain.Report
	var kind string
	err := row.Scan(&r.ID, &r.ChannelID, &r.ChannelName, &kind, &r.TargetSub, &r.TargetName, &r.MessageID,
		&r.MessageBody, &r.MessageAt, &r.Reason, &r.Note, &r.Status, &r.ResolvedBy, &r.ResolvedAt, &r.CreatedAt, &r.ResolvedFrom)
	r.ChannelKind = domain.ChannelKind(kind)
	return r, err
}

// ListReports devuelve la cola de denuncias que `actor` puede moderar. `abiertas`
// la limita a las pendientes; si no, salen también las resueltas (la traza).
func (s *Store) ListReports(ctx context.Context, actor string, orgAdmin, abiertas bool) ([]domain.Report, error) {
	t := tenancy.MustFrom(ctx)
	out := []domain.Report{}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			reportSelect+` AND ($4 = false OR r.status = 'open') ORDER BY r.created_at DESC LIMIT $5`,
			t.Slug, actor, orgAdmin, abiertas, reportsPageSize)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanReport(rows)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// ResolveReport cierra una denuncia con una acción: descartarla, borrar el mensaje
// o expulsar del canal al denunciado. Una denuncia se resuelve UNA vez: la segunda
// es ErrConflict. Quien no puede moderarla recibe ErrNotFound, lo mismo que si no
// existiera.
//
// Borrar y expulsar NO exigen pertenecer al canal —es justo lo que distingue
// moderar de participar—, y por eso van aquí, dentro de la verja de la denuncia,
// y no por DeleteMessage / RemoveChannelMember.
func (s *Store) ResolveReport(ctx context.Context, id uuid.UUID, actor string, orgAdmin bool, action string) (domain.Report, error) {
	return s.resolveReport(ctx, id, actor, orgAdmin, action, "org")
}

// resolveReport es ResolveReport diciendo desde qué cola: "org" o "platform".
//
//nolint:gocognit // moderation report resolution transaction evaluates actor permissions, resolution state, and audit logs
func (s *Store) resolveReport(ctx context.Context, id uuid.UUID, actor string, orgAdmin bool, action, desde string) (domain.Report, error) {
	t := tenancy.MustFrom(ctx)
	var r domain.Report
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		var err error
		r, err = scanReport(tx.QueryRow(ctx, reportSelect+` AND r.id = $4 FOR UPDATE OF r`, t.Slug, actor, orgAdmin, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if r.Status != domain.ReportOpen {
			return domain.ErrConflict
		}
		switch action {
		case domain.ReportDismissed:
		case domain.ReportMessageDeleted:
			if r.MessageID == nil {
				return domain.ErrInvalidInput
			}
			if err := s.redactMatrixMessageTx(ctx, tx, t.Slug, *r.MessageID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			// Ya borrado por su autor o por otra denuncia: se cierra igual.
			if err := borrarMensajeTx(ctx, tx, t.Slug, *r.MessageID); err != nil && !errors.Is(err, domain.ErrNotFound) {
				return err
			}
		case domain.ReportMemberRemoved:
			// De un mensaje directo no se expulsa a nadie: no quedaría conversación.
			if r.ChannelKind == domain.ChannelDirect {
				return domain.ErrInvalidInput
			}
			if err := lockMatrixChannelTx(ctx, tx, t.Slug, r.ChannelID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx,
				`DELETE FROM channel_members WHERE tenant = $1 AND channel_id = $2 AND subject = $3`,
				t.Slug, r.ChannelID, r.TargetSub); err != nil {
				return err
			}
			if err := s.revokeMatrixTx(ctx, tx, t.Slug, r.ChannelID); err != nil {
				return err
			}
		default:
			return domain.ErrInvalidInput
		}
		return tx.QueryRow(ctx,
			`UPDATE message_reports SET status = $3, resolved_by = $4, resolved_at = now(), resolved_from = $5
			  WHERE tenant = $1 AND id = $2 AND status = 'open'
			 RETURNING status, resolved_by, resolved_at, resolved_from`,
			t.Slug, id, action, actor, desde).Scan(&r.Status, &r.ResolvedBy, &r.ResolvedAt, &r.ResolvedFrom)
	})
	return r, err
}
