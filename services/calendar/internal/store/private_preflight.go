// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"net/mail"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

var (
	ErrPrivateStorageCollaboration = errors.New("calendar: encrypted collaboration migration is required")
	ErrPrivateStorageResources     = errors.New("calendar: encrypted resource calendars are required")
	ErrPrivateStorageSubscriptions = errors.New("calendar: private subscription refresh is required")
	ErrPrivateStorageReminders     = errors.New("calendar: private reminder delivery is required")
	ErrPrivateStorageOutbox        = errors.New("calendar: private outbox delivery is required")
	ErrPrivateStorageAmbiguous     = errors.New("calendar: a retained outbox row has no unambiguous owner")
	ErrPrivateStorageIngress       = errors.New("calendar: a verified device ingress bridge is required")
)

type privateCalendarIdentity struct {
	scope     storage.Scope
	addresses []string
}

// Identity routes come exclusively from the verified token in context. A
// migration body supplies an epoch, never subjects, tenants, aliases or readiness.
func verifiedCalendarIdentity(ctx context.Context, owner string) (privateCalendarIdentity, error) {
	tenant, err := tenancy.From(ctx)
	claims, ok := authn.FromContext(ctx)
	scope := storage.Scope{Tenant: tenant.Slug, Subject: owner, Product: "calendar"}
	if err != nil || !ok || claims.Delegado || claims.Subject != owner || claims.Tenant != tenant.Slug || !scope.Valid() {
		return privateCalendarIdentity{}, pg.ErrPrivateStorageScope
	}
	var addresses []string
	for _, value := range append([]string{claims.Email, claims.Mailbox}, claims.Mailboxes...) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		address, err := mail.ParseAddress(value)
		if err != nil || address.Address != value || len(value) > 1024 {
			return privateCalendarIdentity{}, pg.ErrPrivateStorageScope
		}
		addresses = append(addresses, value)
	}
	slices.Sort(addresses)
	addresses = slices.Compact(addresses)
	if len(addresses) == 0 {
		return privateCalendarIdentity{}, ErrPrivateStorageIngress
	}
	return privateCalendarIdentity{scope: scope, addresses: addresses}, nil
}

// Compatibility records unsupported continuing behavior separately from the
// complete retained inventory. A blocker is never permission to omit its rows.
type PrivateStorageCompatibility struct {
	Collaboration   bool `json:"collaboration"`
	Resources       bool `json:"resources"`
	Subscriptions   bool `json:"subscriptions"`
	Reminders       bool `json:"reminders"`
	Outbox          bool `json:"outbox"`
	AmbiguousOutbox bool `json:"ambiguous_outbox"`
}

func (c PrivateStorageCompatibility) Err() error {
	return errors.Join(
		privateCalendarBlocker(c.Collaboration, ErrPrivateStorageCollaboration),
		privateCalendarBlocker(c.Resources, ErrPrivateStorageResources),
		privateCalendarBlocker(c.Subscriptions, ErrPrivateStorageSubscriptions),
		privateCalendarBlocker(c.Reminders, ErrPrivateStorageReminders),
		privateCalendarBlocker(c.Outbox, ErrPrivateStorageOutbox),
		privateCalendarBlocker(c.AmbiguousOutbox, ErrPrivateStorageAmbiguous),
	)
}

func privateCalendarBlocker(present bool, err error) error {
	if present {
		return err
	}
	return nil
}

func privateCalendarPreflight(ctx context.Context, tx pgx.Tx, identity privateCalendarIdentity) (PrivateStorageCompatibility, error) {
	// Only this internal compatibility transaction uses maintenance visibility.
	// It returns booleans, never another tenant's Calendar rows or email content.
	// Replacing a master SET NULLs historical invitations. No stable actor is
	// retained there or in invitation.sent outbox payloads; current organizer
	// emails or a currently unique UID cannot prove the historical owner. Any
	// local invitation without a same-tenant host therefore blocks the tenant's
	// personal captures until its ownership/recipient migration is resolved.
	if err := pg.SetMaintenance(ctx, tx); err != nil {
		return PrivateStorageCompatibility{}, err
	}
	var c PrivateStorageCompatibility
	err := tx.QueryRow(ctx, calendarPrivatePreflightSQL, identity.scope.Tenant, identity.scope.Subject, identity.addresses).Scan(&c.Collaboration, &c.Resources, &c.Subscriptions, &c.Reminders, &c.Outbox, &c.AmbiguousOutbox)
	return c, err
}

func (s *Store) PrivateStorageCompatibility(ctx context.Context, owner string) (PrivateStorageCompatibility, error) {
	identity, err := verifiedCalendarIdentity(ctx, owner)
	if err != nil {
		return PrivateStorageCompatibility{}, err
	}
	var result PrivateStorageCompatibility
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = privateCalendarPreflight(ctx, tx, identity)
		return err
	})
	return result, err
}

const calendarPrivatePreflightSQL = calendarPrivateOwnedRowsSQL + `
SELECT (
 EXISTS(SELECT 1 FROM calendar_shares s WHERE (s.tenant=$1 AND s.calendar_id IN(SELECT id FROM owned_calendars)) OR s.grantee_sub=$2 OR lower(btrim(s.grantee_email))=ANY($3))
 OR EXISTS(SELECT 1 FROM calendar_peticiones_edicion p WHERE (p.tenant=$1 AND p.calendar_id IN(SELECT id FROM owned_calendars)) OR p.sujeto=$2)
 OR EXISTS(SELECT 1 FROM attendees a WHERE (a.tenant=$1 AND a.event_id IN(SELECT id FROM owned_events)) OR lower(btrim(a.email))=ANY($3))
 OR EXISTS(SELECT 1 FROM invitations i WHERE (i.tenant=$1 AND i.event_id IN(SELECT id FROM owned_events)) OR i.recipient_sub=$2 OR lower(btrim(i.recipient))=ANY($3)
  OR (i.tenant=$1 AND NOT EXISTS(SELECT 1 FROM events linked WHERE linked.tenant=i.tenant AND linked.id=i.event_id)))
 OR EXISTS(SELECT 1 FROM calendar_reminders r WHERE r.tenant=$1 AND r.calendar_id IN(SELECT id FROM owned_calendars) AND r.owner_sub<>$2)
 OR EXISTS(SELECT 1 FROM owned_outbox o WHERE coalesce(o.payload->>'owner_sub','') NOT IN('',$2))
),(
 EXISTS(SELECT 1 FROM managed_resources)
 OR EXISTS(SELECT 1 FROM calendar_resources r WHERE r.tenant=$1 AND r.active AND r.deleted_at IS NULL)
),EXISTS(SELECT 1 FROM calendar_subscriptions s WHERE s.tenant=$1 AND s.calendar_id IN(SELECT id FROM owned_calendars)),
 EXISTS(SELECT 1 FROM calendar_reminders r WHERE r.tenant=$1 AND (r.calendar_id IN(SELECT id FROM owned_calendars) OR r.owner_sub=$2) AND r.fired_at IS NULL AND r.dismissed_at IS NULL),
 EXISTS(SELECT 1 FROM owned_outbox o WHERE o.processed_at IS NULL),
 EXISTS(SELECT 1 FROM events_outbox o WHERE o.tenant=$1 AND coalesce(o.payload->>'owner_sub','')=''
  AND NOT EXISTS(SELECT 1 FROM calendars c WHERE c.tenant=o.tenant AND c.id::text=o.payload->>'calendar_id')
  AND NOT EXISTS(SELECT 1 FROM events e WHERE e.tenant=o.tenant AND e.id::text=o.payload->>'event_id')
  AND NOT EXISTS(SELECT 1 FROM invitations i WHERE i.tenant=o.tenant AND i.id::text=o.payload->>'invitation_id')
  AND NOT EXISTS(SELECT 1 FROM calendar_resources r WHERE r.tenant=o.tenant AND r.id::text=o.payload->>'resource_id'))`
