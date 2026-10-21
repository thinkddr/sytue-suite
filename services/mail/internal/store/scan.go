// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// tenantOf devuelve el slug del tenant del contexto (RequireAuth/InTenantTx ya
// garantizan su presencia; en su ausencia devuelve "" y RLS no casará nada).
func tenantOf(ctx context.Context) string {
	if t, err := tenancy.From(ctx); err == nil {
		return t.Slug
	}
	return ""
}

// u32 convierte contadores/UIDs IMAP persistidos como BIGINT a uint32 acotando
// al rango del protocolo (RFC 9051); los valores se generan de contadores
// uint32, así que el clamp solo protege frente a datos corruptos (gosec G115).
func u32(v int64) uint32 {
	if v < 0 {
		return 0
	}
	if v > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(v)
}

// scanner abstrae pgx.Row y pgx.Rows para reusar los helpers de escaneo.
type scanner interface {
	Scan(dest ...any) error
}

func scanAccount(s scanner) (domain.Account, error) {
	var a domain.Account
	err := s.Scan(&a.ID, &a.Tenant, &a.Subject, &a.Address, &a.DisplayName,
		&a.QuotaBytes, &a.UsedBytes, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

func scanMailboxRow(s scanner) (domain.Mailbox, error) {
	var m domain.Mailbox
	var role string
	var uidValidity, uidNext int64
	err := s.Scan(&m.ID, &m.Tenant, &m.AccountID, &m.ParentID, &m.Name, &role,
		&uidValidity, &uidNext, &m.CreatedAt, &m.UpdatedAt)
	m.Role = domain.SpecialUse(role)
	m.UIDValidity = u32(uidValidity)
	m.UIDNext = u32(uidNext)
	return m, err
}

// scanMailbox escanea la fila extendida de ListMailboxes (con contadores).
func scanMailbox(s scanner) (domain.Mailbox, error) {
	var m domain.Mailbox
	var role string
	var uidValidity, uidNext int64
	err := s.Scan(&m.ID, &m.Tenant, &m.AccountID, &m.ParentID, &m.Name, &role,
		&uidValidity, &uidNext, &m.CreatedAt, &m.UpdatedAt, &m.TotalCount, &m.UnreadCount)
	m.Role = domain.SpecialUse(role)
	m.UIDValidity = u32(uidValidity)
	m.UIDNext = u32(uidNext)
	return m, err
}

// scanMessageRow escanea las columnas de `selectMessageCols` en ese orden. `extra`
// recibe los destinos de las columnas que la consulta añade DESPUÉS (hoy solo el
// recuento de hilo del listado de bandeja): un único punto de escaneo, para que
// ampliar la SELECT no deje dos escáneres desincronizados.
func scanMessageRow(s scanner, extra ...any) (domain.Message, error) {
	var m domain.Message
	var uid int64
	var spf, dkim, dmarc string
	var sentAt *time.Time // sent_at es NULLable
	dest := append([]any{
		&m.ID, &m.Tenant, &m.AccountID, &m.MailboxID, &m.ThreadID, &uid,
		&m.MessageID, &m.InReplyTo, &m.Subject, &m.FromAddr, &m.FromName,
		&m.To, &m.Cc, &m.Bcc, &m.ReplyTo, &m.Snippet, &m.BlobKey, &m.SizeBytes,
		&m.Flags, &m.HasAttach, &spf, &dkim, &dmarc, &m.ReceivedAt, &sentAt,
	}, extra...)
	if err := s.Scan(dest...); err != nil {
		return m, err
	}
	m.UID = u32(uid)
	if sentAt != nil {
		m.SentAt = *sentAt
	}
	m.SPFResult = domain.AuthResult(spf)
	m.DKIMResult = domain.AuthResult(dkim)
	m.DMARCResult = domain.AuthResult(dmarc)
	return m, nil
}

func scanMessage(rows pgx.Rows) (domain.Message, error) { return scanMessageRow(rows) }

// scanMessageConHilo escanea además el recuento del hilo entero (listado de bandeja).
func scanMessageConHilo(rows pgx.Rows) (domain.Message, error) {
	var n int
	m, err := scanMessageRow(rows, &n)
	m.ThreadCount = n
	return m, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
