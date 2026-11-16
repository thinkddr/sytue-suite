// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package model contains Mail domain values shared by the service and private clients.
package model

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidFilter = errors.New("mail: regla de filtrado inválida")

// Filter is the complete service rule. Tenant and AccountID remain owner scoped,
// and never enter its public JSON representation.
type Filter struct {
	ID           uuid.UUID  `json:"id"`
	Tenant       string     `json:"-"`
	AccountID    uuid.UUID  `json:"-"`
	Name         string     `json:"name"`
	Enabled      bool       `json:"enabled"`
	Priority     int        `json:"priority"`
	MatchFrom    string     `json:"match_from,omitempty"`
	MatchSubject string     `json:"match_subject,omitempty"`
	MoveTo       *uuid.UUID `json:"move_to,omitempty"`
	MarkRead     bool       `json:"mark_read"`
	MarkFlagged  bool       `json:"mark_flagged"`
}

func (f Filter) HasCondition() bool {
	return strings.TrimSpace(f.MatchFrom) != "" || strings.TrimSpace(f.MatchSubject) != ""
}

func (f Filter) HasAction() bool {
	return f.MoveTo != nil || f.MarkRead || f.MarkFlagged
}

func (f Filter) Validate() error {
	if !f.HasCondition() || !f.HasAction() || len(f.Name) > 255 {
		return ErrInvalidFilter
	}
	return nil
}

// Vacation retains the full server value, including its update timestamp.
// Saving this value alone does not execute or authorize an automatic reply.
type Vacation struct {
	// PrivateRevision is a native optimistic concurrency token, never server JSON.
	PrivateRevision string     `json:"-"`
	Enabled         bool       `json:"enabled"`
	Subject         string     `json:"subject"`
	Body            string     `json:"body"`
	StartsAt        *time.Time `json:"starts_at,omitempty"`
	EndsAt          *time.Time `json:"ends_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at,omitempty"`
}

func (v Vacation) ActiveAt(t time.Time) bool {
	return v.Enabled && (v.StartsAt == nil || !t.Before(*v.StartsAt)) && (v.EndsAt == nil || !t.After(*v.EndsAt))
}
