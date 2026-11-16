// SPDX-License-Identifier: Apache-2.0
// Package model validates retained Calendar rows as a complete graph while
// preserving their exact JSON, including iCalendar and auxiliary state.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
	calendarical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const MaxRowBytes = 8 << 20

var ErrSnapshot = errors.New("calendar: invalid or incomplete private snapshot")

// Row retains original columns without passing them through a UI projection.
type Row struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	JSON []byte `json:"json"`
}

// Snapshot owns independent copies of every row in a validated source graph.
type Snapshot struct {
	scope storage.Scope
	rows  map[string]Row
}

type relation struct {
	ID           string          `json:"id"`
	Tenant       string          `json:"tenant"`
	Owner        string          `json:"owner_sub"`
	Calendar     string          `json:"calendar_id"`
	Object       string          `json:"object_id"`
	Event        string          `json:"event_id"`
	UID          string          `json:"uid"`
	Subject      string          `json:"sujeto"`
	RecipientSub string          `json:"recipient_sub"`
	Resource     string          `json:"resource_id"`
	CreatedBy    string          `json:"created_by"`
	Managers     []string        `json:"managers"`
	Payload      json.RawMessage `json:"payload"`
	DeletedAt    *time.Time      `json:"deleted_at"`
	kind         string
}

func rowKey(kind, id string) string { return kind + "\x00" + id }

func SourceKind(kind string) bool {
	switch kind {
	case "calendar", "object", "event", "attendee", "invitation", "reminder", "subscription", "share", "request", "resource", "outbox":
		return true
	default:
		return false
	}
}

func canonicalUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

// RequestID routes a composite request key without putting a subject or slash
// in a URL. Canonical UUID length and the separator keep tuple boundaries exact;
// the original, unnormalized subject remains inside the retained encrypted JSON.
func RequestID(calendar, subject string) (string, error) {
	if !canonicalUUID(calendar) || subject == "" || !utf8.ValidString(subject) {
		return "", ErrSnapshot
	}
	sum := sha256.Sum256([]byte(calendar + ":" + subject))
	return hex.EncodeToString(sum[:]), nil
}

func decodeRow(scope storage.Scope, row Row) (relation, error) {
	var out relation
	if !scope.Valid() || scope.Product != "calendar" || !SourceKind(row.Kind) || len(row.JSON) == 0 || len(row.JSON) > MaxRowBytes || json.Unmarshal(row.JSON, &out) != nil || out.Tenant != scope.Tenant {
		return out, ErrSnapshot
	}
	out.kind = row.Kind
	if row.Kind == "request" {
		expected, err := RequestID(out.Calendar, out.Subject)
		if err != nil || row.ID != expected {
			return out, ErrSnapshot
		}
		out.ID = row.ID
	} else if !canonicalUUID(row.ID) || row.ID != out.ID {
		return out, ErrSnapshot
	}
	for _, id := range []string{out.Calendar, out.Object, out.Event, out.Resource} {
		if id != "" && !canonicalUUID(id) {
			return out, ErrSnapshot
		}
	}
	if err := validateContents(scope, row, out); err != nil {
		return out, err
	}
	return out, nil
}

func validateContents(scope storage.Scope, row Row, rel relation) error {
	switch row.Kind {
	case "calendar":
		return validateCalendarRow(row, rel)
	case "object":
		return validateObjectRow(row, rel)
	case "event":
		return validateEventRow(row, rel)
	case "attendee":
		return validateAttendeeRow(row, rel)
	case "invitation":
		return validateInvitationRow(row, rel)
	case "reminder":
		return validateReminderRow(rel)
	case "subscription":
		return validateSubscriptionRow(row, rel)
	case "share":
		return validateShareRow(row, rel)
	case "request":
		return validateRequestRow(rel)
	case "resource":
		return validateResourceRow(scope, row, rel)
	case "outbox":
		return validateOutboxRow(rel)
	default:
		return ErrSnapshot
	}
}

func validateCalendarRow(row Row, rel relation) error {
	var v event.Calendar
	if json.Unmarshal(row.JSON, &v) != nil || rel.Owner == "" || !event.ValidName(v.Name) || v.SyncToken < 1 || v.CTag == uuid.Nil {
		return ErrSnapshot
	}
	return nil
}

func validateObjectRow(row Row, rel relation) error {
	var v struct {
		event.CalendarObject
		Data string `json:"data"`
	}
	if json.Unmarshal(row.JSON, &v) != nil || rel.Calendar == "" || rel.UID == "" || v.ResourceName == "" || v.ETag == uuid.Nil || v.SizeBytes != len(v.Data) || v.Data == "" || (v.Component != "VEVENT" && v.Component != "VTODO" && v.Component != "VJOURNAL") {
		return ErrSnapshot
	}
	if v.Component == "VEVENT" {
		parsed, err := calendarical.DecodeCalendarObject(v.Data, rel.Calendar)
		if err != nil || parsed.UID != rel.UID {
			return ErrSnapshot
		}
	}
	return nil
}

func validateEventRow(row Row, rel relation) error {
	var v event.Event
	if json.Unmarshal(row.JSON, &v) != nil || rel.Calendar == "" || rel.Object == "" || rel.UID == "" || v.DTEnd.Before(v.DTStart) || !event.ValidStatus(v.Status) || !event.ValidClassification(v.Classification) || !event.ValidTransparency(v.Transparency) {
		return ErrSnapshot
	}
	return nil
}

func validateAttendeeRow(row Row, rel relation) error {
	var v event.Attendee
	if json.Unmarshal(row.JSON, &v) != nil || rel.Event == "" || v.Email == "" || !event.ValidPartStat(v.PartStat) {
		return ErrSnapshot
	}
	return nil
}

func validateInvitationRow(row Row, rel relation) error {
	var v event.Invitation
	if json.Unmarshal(row.JSON, &v) != nil || rel.UID == "" || v.Recipient == "" || !event.ValidMethod(v.Method) {
		return ErrSnapshot
	}
	return nil
}

func validateReminderRow(rel relation) error {
	if rel.Calendar == "" || rel.Object == "" || rel.Event == "" || rel.UID == "" || rel.Owner == "" {
		return ErrSnapshot
	}
	return nil
}

func validateSubscriptionRow(row Row, rel relation) error {
	var v event.Subscription
	if json.Unmarshal(row.JSON, &v) != nil || rel.Calendar == "" || !event.ValidSubscriptionURL(v.URL) || v.RefreshInterval <= 0 {
		return ErrSnapshot
	}
	return nil
}

func validateShareRow(row Row, rel relation) error {
	var v event.CalendarShare
	if json.Unmarshal(row.JSON, &v) != nil || rel.Calendar == "" || !event.ValidSharePermission(v.Permission) || (v.GranteeSub == "" && v.GranteeEmail == "") {
		return ErrSnapshot
	}
	return nil
}

func validateRequestRow(rel relation) error {
	if rel.Calendar == "" || rel.Subject == "" {
		return ErrSnapshot
	}
	return nil
}

func validateResourceRow(scope storage.Scope, row Row, rel relation) error {
	var v event.Resource
	if json.Unmarshal(row.JSON, &v) != nil || rel.Calendar == "" || !event.ValidResourceKind(v.Kind) || v.Name == "" || v.Capacity < 0 || (v.CreatedBy != scope.Subject && !slices.Contains(v.Managers, scope.Subject)) {
		return ErrSnapshot
	}
	return nil
}

func validateOutboxRow(rel relation) error {
	var payload map[string]json.RawMessage
	if json.Unmarshal(rel.Payload, &payload) != nil || payload == nil {
		return ErrSnapshot
	}
	return nil
}

// ValidateRow checks per-row scope, identity and table constraints. NewSnapshot
// additionally checks ownership and every reference against the full inventory.
func ValidateRow(scope storage.Scope, row Row) error { _, err := decodeRow(scope, row); return err }

func NewSnapshot(scope storage.Scope, rows []Row) (*Snapshot, error) {
	if !scope.Valid() || scope.Product != "calendar" {
		return nil, ErrSnapshot
	}
	snapshot := &Snapshot{scope: scope, rows: make(map[string]Row, len(rows))}
	all := make(map[string]relation, len(rows))
	for _, row := range rows {
		rel, err := decodeRow(scope, row)
		if err != nil {
			return nil, err
		}
		key := rowKey(row.Kind, row.ID)
		if _, found := all[key]; found {
			return nil, ErrSnapshot
		}
		row.JSON = slices.Clone(row.JSON)
		snapshot.rows[key] = row
		all[key] = rel
	}
	for _, rel := range all {
		if err := validateRelation(scope, rel, all); err != nil {
			return nil, err
		}
	}
	if err := snapshot.validateUnique(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func validateRelation(scope storage.Scope, rel relation, all map[string]relation) error {
	calendar := func(id string) (relation, bool) { v, ok := all[rowKey("calendar", id)]; return v, ok }
	switch rel.kind {
	case "calendar":
		if rel.Owner != scope.Subject {
			id := strings.TrimPrefix(rel.Owner, "resource:")
			resource, ok := all[rowKey("resource", id)]
			if !ok || resource.Calendar != rel.ID || rel.Owner != "resource:"+resource.ID {
				return ErrSnapshot
			}
		}
	case "object", "subscription", "share", "request":
		if _, ok := calendar(rel.Calendar); !ok {
			return ErrSnapshot
		}
	case "event":
		object, ok := all[rowKey("object", rel.Object)]
		if !ok || object.Calendar != rel.Calendar || object.UID != rel.UID {
			return ErrSnapshot
		}
	case "attendee":
		if _, ok := all[rowKey("event", rel.Event)]; !ok {
			return ErrSnapshot
		}
	default:
		return validateAuxRelation(scope, rel, all)
	}
	return nil
}

func validateAuxRelation(scope storage.Scope, rel relation, all map[string]relation) error {
	calendar := func(id string) (relation, bool) { v, ok := all[rowKey("calendar", id)]; return v, ok }
	switch rel.kind {
	case "invitation":
		value, own := all[rowKey("event", rel.Event)]
		if own && value.UID != rel.UID {
			return ErrSnapshot
		}
		if !own && rel.RecipientSub != scope.Subject {
			return ErrSnapshot
		}
	case "reminder":
		value, ok := all[rowKey("event", rel.Event)]
		owner, found := calendar(rel.Calendar)
		if !ok || !found || value.Calendar != rel.Calendar || value.Object != rel.Object || value.UID != rel.UID || rel.Owner != owner.Owner {
			return ErrSnapshot
		}
	case "resource":
		owner, ok := calendar(rel.Calendar)
		if !ok || owner.Owner != "resource:"+rel.ID {
			return ErrSnapshot
		}
	case "outbox":
		return validateOutboxRoute(scope, rel, all)
	default:
		return ErrSnapshot
	}
	return nil
}

func validateOutboxRoute(scope storage.Scope, rel relation, all map[string]relation) error {
	var route struct {
		Owner      string `json:"owner_sub"`
		Calendar   string `json:"calendar_id"`
		Event      string `json:"event_id"`
		Invitation string `json:"invitation_id"`
		Resource   string `json:"resource_id"`
	}
	if json.Unmarshal(rel.Payload, &route) != nil || (route.Owner != "" && route.Owner != scope.Subject) {
		return ErrSnapshot
	}
	own := route.Owner == scope.Subject
	for kind, id := range map[string]string{"calendar": route.Calendar, "event": route.Event, "invitation": route.Invitation, "resource": route.Resource} {
		if id != "" {
			if _, ok := all[rowKey(kind, id)]; ok {
				own = true
			} else {
				return ErrSnapshot
			}
		}
	}
	if !own {
		return ErrSnapshot
	}
	return nil
}

func (s *Snapshot) Scope() storage.Scope { return s.scope }

// Rows returns deterministic, independent copies, never references to the source.
func (s *Snapshot) Rows() []Row {
	rows := make([]Row, 0, len(s.rows))
	for _, row := range s.rows {
		row.JSON = slices.Clone(row.JSON)
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b Row) int { return strings.Compare(rowKey(a.Kind, a.ID), rowKey(b.Kind, b.ID)) })
	return rows
}

func (s *Snapshot) validateUnique() error {
	seen := map[string]bool{}
	for _, row := range s.rows {
		var v struct {
			Owner        string     `json:"owner_sub"`
			Name         string     `json:"name"`
			Calendar     string     `json:"calendar_id"`
			Object       string     `json:"object_id"`
			Event        string     `json:"event_id"`
			UID          string     `json:"uid"`
			Resource     string     `json:"resource_name"`
			Email        string     `json:"email"`
			GranteeEmail string     `json:"grantee_email"`
			GranteeSub   string     `json:"grantee_sub"`
			Default      bool       `json:"is_default"`
			DeletedAt    *time.Time `json:"deleted_at"`
			RecurrenceID *time.Time `json:"recurrence_id"`
		}
		if json.Unmarshal(row.JSON, &v) != nil {
			return ErrSnapshot
		}
		if v.DeletedAt != nil {
			continue
		}
		keys := []string{}
		switch row.Kind {
		case "calendar":
			keys = append(keys, "calendar-name\x00"+v.Owner+"\x00"+strings.ToLower(v.Name))
			if v.Default {
				keys = append(keys, "calendar-default\x00"+v.Owner)
			}
		case "object":
			keys = append(keys, "object-uid\x00"+v.Calendar+"\x00"+v.UID, "object-resource\x00"+v.Calendar+"\x00"+v.Resource)
		case "event":
			instance := "master"
			if v.RecurrenceID != nil {
				instance = v.RecurrenceID.UTC().Format(time.RFC3339Nano)
			}
			keys = append(keys, "event-instance\x00"+v.Object+"\x00"+instance)
		case "attendee":
			keys = append(keys, "attendee\x00"+v.Event+"\x00"+strings.ToLower(v.Email))
		case "subscription":
			keys = append(keys, "subscription\x00"+v.Calendar)
		case "share":
			keys = append(keys, "share\x00"+v.Calendar+"\x00"+v.GranteeEmail+"\x00"+v.GranteeSub)
		case "resource":
			keys = append(keys, "resource\x00"+v.Calendar)
		}
		for _, key := range keys {
			if seen[key] {
				return ErrSnapshot
			}
			seen[key] = true
		}
	}
	return nil
}
