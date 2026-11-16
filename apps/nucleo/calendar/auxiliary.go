// SPDX-License-Identifier: Apache-2.0
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

var (
	ErrPrivateIngress       = errors.New("calendar: verified encrypted device ingress is unavailable")
	ErrPrivateSubscriptions = errors.New("calendar: device subscription fetch and scheduling are unavailable")
)

func (r *Repository) Invitations(ctx context.Context, pending bool) ([]api.Invitation, error) {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	result := []api.Invitation{}
	for _, row := range s.Rows() {
		if row.Kind != "invitation" || s.Deleted(row) {
			continue
		}
		var value struct {
			api.Invitation
			RecipientSub string `json:"recipient_sub"`
		}
		if json.Unmarshal(row.JSON, &value) != nil {
			return nil, model.ErrSnapshot
		}
		// An address in a retained row is never proof of its current assignee.
		if value.RecipientSub != r.scope.Subject || pending && value.Status != "pending" {
			continue
		}
		v := value.Invitation
		v.Organizer = strings.TrimPrefix(v.Organizer, "mailto:")
		if v.EventID != "" && v.Organization == "" {
			if ev, err := findEvent(s, v.EventID); err == nil {
				v.Summary = ev.Summary
				v.Start = ev.DTStart
			}
		}
		result = append(result, v)
	}
	slices.SortFunc(result, func(a, b api.Invitation) int {
		if n := b.CreatedAt.Compare(a.CreatedAt); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	return result, nil
}

func (r *Repository) Reminders(ctx context.Context) ([]api.Reminder, error) {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	result := []api.Reminder{}
	for _, row := range s.Rows() {
		if row.Kind != "reminder" || s.Deleted(row) {
			continue
		}
		var value struct {
			api.Reminder
			Owner     string     `json:"owner_sub"`
			Dismissed *time.Time `json:"dismissed_at"`
		}
		if json.Unmarshal(row.JSON, &value) != nil {
			return nil, model.ErrSnapshot
		}
		if value.Owner != r.scope.Subject || value.Dismissed != nil || value.TriggerAt.After(now) || !value.TriggerAt.After(now.Add(-24*time.Hour)) {
			continue
		}
		if _, err := findEvent(s, value.EventID); err != nil {
			continue
		}
		result = append(result, value.Reminder)
	}
	slices.SortFunc(result, func(a, b api.Reminder) int {
		if n := b.TriggerAt.Compare(a.TriggerAt); n != 0 {
			return n
		}
		return strings.Compare(b.ID, a.ID)
	})
	if len(result) > 50 {
		result = result[:50]
	}
	return result, nil
}

func (r *Repository) DismissReminder(ctx context.Context, id string) error {
	_, err := r.mutate(ctx, "dismiss_reminder", id, func(s *model.Snapshot) (model.Aggregate, error) {
		row, ok := s.Row("reminder", id)
		if !ok {
			return model.Aggregate{}, missing()
		}
		var rel struct {
			Owner     string     `json:"owner_sub"`
			Object    string     `json:"object_id"`
			Dismissed *time.Time `json:"dismissed_at"`
		}
		if json.Unmarshal(row.JSON, &rel) != nil {
			return model.Aggregate{}, model.ErrSnapshot
		}
		if rel.Owner != r.scope.Subject {
			return model.Aggregate{}, missing()
		}
		a, err := s.Edit("object_state", rel.Object)
		if err != nil {
			return a, err
		}
		for i, row := range a.Rows {
			if row.Kind == "reminder" && row.ID == id {
				if rel.Dismissed != nil {
					return a, nil
				}
				a.Rows[i], err = patch(row, map[string]any{"dismissed_at": time.Now().UTC()})
				return a, err
			}
		}
		return a, missing()
	})
	return err
}

func (r *Repository) Shares(ctx context.Context, calendar string) ([]api.CalendarShare, error) {
	s, err := r.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := ownCalendar(s, calendar, false); err != nil {
		return nil, err
	}
	result := []api.CalendarShare{}
	for _, row := range s.Rows() {
		if row.Kind != "share" || s.Deleted(row) {
			continue
		}
		var value struct {
			api.CalendarShare
			Calendar string `json:"calendar_id"`
		}
		if json.Unmarshal(row.JSON, &value) != nil {
			return nil, model.ErrSnapshot
		}
		if value.Calendar == calendar {
			result = append(result, value.CalendarShare)
		}
	}
	return result, nil
}

// These operations require capabilities beyond a personal encrypted catalog.
// They deliberately fail before publishing any revision. There is no legacy
// HTTP fallback, readiness boolean, local-only success or second inbox here.
func (*Repository) RespondInvitation(context.Context, string, string) (api.Invitation, error) {
	return api.Invitation{}, ErrPrivateIngress
}

func (*Repository) SharedWithMe(context.Context) ([]api.Calendar, error) {
	return nil, records.ErrShared
}
func (*Repository) SetPartStat(context.Context, string, string) error   { return ErrPrivateIngress }
func (*Repository) Share(context.Context, string, string, string) error { return records.ErrShared }
func (*Repository) RevokeShare(context.Context, string, string) error   { return records.ErrShared }
func (*Repository) Subscribe(context.Context, api.SubscriptionInput) (api.Calendar, error) {
	return api.Calendar{}, ErrPrivateSubscriptions
}

func (*Repository) RefreshSubscription(context.Context, string) error { return ErrPrivateSubscriptions }

func (*Repository) Resources(context.Context, time.Time, time.Time) ([]api.Resource, error) {
	return nil, records.ErrShared
}

func (*Repository) CreateResource(context.Context, api.ResourceInput) (api.Resource, error) {
	return api.Resource{}, records.ErrShared
}

func (*Repository) UpdateResource(context.Context, string, api.ResourceInput) (api.Resource, error) {
	return api.Resource{}, records.ErrShared
}
func (*Repository) DeleteResource(context.Context, string) error { return records.ErrShared }
func (*Repository) BookResource(context.Context, string, api.Booking) (string, error) {
	return "", records.ErrShared
}
