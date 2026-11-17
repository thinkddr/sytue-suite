// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// ReadPrivateStorageInventory validates the complete owned Calendar graph in
// the caller's tenant transaction. Capture calls it while holding the shared
// private-storage fence exclusively; it does not freeze or activate anything.
// Shared relationships remain data here, never implicit authorization to turn a
// personal repository on. Their migration compatibility is a separate preflight.
func ReadPrivateStorageInventory(ctx context.Context, tx pgx.Tx, scope storage.Scope) (*model.Snapshot, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || tx == nil || !scope.Valid() || scope.Product != "calendar" || tenant.Slug != scope.Tenant {
		return nil, pg.ErrPrivateStorageScope
	}
	rows, err := tx.Query(ctx, calendarPrivateInventorySQL, scope.Tenant, scope.Subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []model.Row{}
	for rows.Next() {
		var value model.Row
		if err := rows.Scan(&value.Kind, &value.ID, &value.JSON); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return model.NewSnapshot(scope, values)
}

const calendarPrivateOwnedRowsSQL = `WITH managed_resources AS MATERIALIZED (
 SELECT r.* FROM calendar_resources r WHERE r.tenant=$1 AND (r.created_by=$2 OR $2=ANY(r.managers))
), owned_calendars AS MATERIALIZED (
 SELECT c.* FROM calendars c WHERE c.tenant=$1 AND (c.owner_sub=$2 OR c.id IN(SELECT calendar_id FROM managed_resources))
), owned_objects AS MATERIALIZED (
 SELECT o.* FROM calendar_objects o JOIN owned_calendars c ON c.tenant=o.tenant AND c.id=o.calendar_id
), owned_events AS MATERIALIZED (
 SELECT e.* FROM events e JOIN owned_calendars c ON c.tenant=e.tenant AND c.id=e.calendar_id
), owned_invitations AS MATERIALIZED (
 SELECT i.* FROM invitations i WHERE i.tenant=$1 AND (i.recipient_sub=$2 OR i.event_id IN(SELECT id FROM owned_events))
), owned_outbox AS MATERIALIZED (
 SELECT o.* FROM events_outbox o WHERE o.tenant=$1 AND (
  o.payload->>'owner_sub'=$2
  OR o.payload->>'calendar_id' IN(SELECT id::text FROM owned_calendars)
  OR o.payload->>'event_id' IN(SELECT id::text FROM owned_events)
  OR o.payload->>'invitation_id' IN(SELECT id::text FROM owned_invitations)
  OR o.payload->>'resource_id' IN(SELECT id::text FROM managed_resources)
 )
)`

const calendarPrivateInventorySQL = calendarPrivateOwnedRowsSQL + `, source(kind,id,payload) AS (
 SELECT 'calendar',c.id::text,convert_to(to_jsonb(c)::text,'UTF8') FROM owned_calendars c
 UNION ALL SELECT 'object',o.id::text,convert_to(to_jsonb(o)::text,'UTF8') FROM owned_objects o
 UNION ALL SELECT 'event',e.id::text,convert_to(to_jsonb(e)::text,'UTF8') FROM owned_events e
 UNION ALL SELECT 'attendee',a.id::text,convert_to(to_jsonb(a)::text,'UTF8') FROM attendees a JOIN owned_events e ON e.tenant=a.tenant AND e.id=a.event_id
 UNION ALL SELECT 'invitation',i.id::text,convert_to(to_jsonb(i)::text,'UTF8') FROM owned_invitations i
 UNION ALL SELECT 'reminder',r.id::text,convert_to(to_jsonb(r)::text,'UTF8') FROM calendar_reminders r JOIN owned_calendars c ON c.tenant=r.tenant AND c.id=r.calendar_id
 UNION ALL SELECT 'subscription',s.id::text,convert_to(to_jsonb(s)::text,'UTF8') FROM calendar_subscriptions s JOIN owned_calendars c ON c.tenant=s.tenant AND c.id=s.calendar_id
 UNION ALL SELECT 'share',s.id::text,convert_to(to_jsonb(s)::text,'UTF8') FROM calendar_shares s JOIN owned_calendars c ON c.tenant=s.tenant AND c.id=s.calendar_id
 UNION ALL SELECT 'request',encode(digest(convert_to(p.calendar_id::text||':'||p.sujeto,'UTF8'),'sha256'),'hex'),convert_to(to_jsonb(p)::text,'UTF8') FROM calendar_peticiones_edicion p JOIN owned_calendars c ON c.tenant=p.tenant AND c.id=p.calendar_id
 UNION ALL SELECT 'resource',r.id::text,convert_to(to_jsonb(r)::text,'UTF8') FROM managed_resources r
 UNION ALL SELECT 'outbox',o.id::text,convert_to(to_jsonb(o)::text,'UTF8') FROM owned_outbox o
)
SELECT kind,id,payload FROM source ORDER BY kind COLLATE "C",id COLLATE "C"`
