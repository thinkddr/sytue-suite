// SPDX-License-Identifier: Apache-2.0
// Package domain retains the server contract over the shared device-side calendar model.
package domain

import (
	"time"

	event "git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
)

type (
	Component           = event.Component
	EventStatus         = event.Status
	Classification      = event.Classification
	Transparency        = event.Transparency
	Calendar            = event.Calendar
	CalendarObject      = event.CalendarObject
	EventInput          = event.Input
	Event               = event.Event
	AttendeeRole        = event.AttendeeRole
	ParticipationStatus = event.ParticipationStatus
	Attendee            = event.Attendee
	SchedulingMethod    = event.SchedulingMethod
	InvitationStatus    = event.InvitationStatus
	Invitation          = event.Invitation
	FreeBusyType        = event.FreeBusyType
	FreeBusyPeriod      = event.FreeBusyPeriod
	Instance            = event.Instance
	Frequency           = event.Frequency
	RRule               = event.RRule
	ResourceKind        = event.ResourceKind
	Resource            = event.Resource
	FreeSlot            = event.FreeSlot
	WorkingHours        = event.WorkingHours
	SharePermission     = event.SharePermission
	CalendarShare       = event.CalendarShare
	Subscription        = event.Subscription
)

const (
	CompEvent              = event.CompEvent
	StatusConfirmed        = event.StatusConfirmed
	StatusTentative        = event.StatusTentative
	StatusCancelled        = event.StatusCancelled
	ClassPublic            = event.ClassPublic
	ClassPrivate           = event.ClassPrivate
	ClassConfidential      = event.ClassConfidential
	TranspOpaque           = event.TranspOpaque
	TranspTransparent      = event.TranspTransparent
	RoleChair              = event.RoleChair
	RoleRequired           = event.RoleRequired
	RoleOptional           = event.RoleOptional
	RoleNonPart            = event.RoleNonPart
	PartNeedsAction        = event.PartNeedsAction
	PartAccepted           = event.PartAccepted
	PartDeclined           = event.PartDeclined
	PartTentative          = event.PartTentative
	PartDelegated          = event.PartDelegated
	MethodRequest          = event.MethodRequest
	MethodReply            = event.MethodReply
	MethodCancel           = event.MethodCancel
	MethodAdd              = event.MethodAdd
	MethodRefresh          = event.MethodRefresh
	MethodCounter          = event.MethodCounter
	MethodDeclineCounter   = event.MethodDeclineCounter
	InvPending             = event.InvPending
	InvAccepted            = event.InvAccepted
	InvDeclined            = event.InvDeclined
	InvTentative           = event.InvTentative
	InvDelivered           = event.InvDelivered
	InvCancelled           = event.InvCancelled
	FBBusy                 = event.FBBusy
	FBBusyTentative        = event.FBBusyTentative
	FBBusyUnavailable      = event.FBBusyUnavailable
	FBFree                 = event.FBFree
	FreqDaily              = event.FreqDaily
	FreqWeekly             = event.FreqWeekly
	FreqMonthly            = event.FreqMonthly
	FreqYearly             = event.FreqYearly
	ResourceRoom           = event.ResourceRoom
	ResourceEquipment      = event.ResourceEquipment
	PermReader             = event.PermReader
	PermWriter             = event.PermWriter
	PermManage             = event.PermManage
	MinRefreshInterval     = event.MinRefreshInterval
	MaxRefreshInterval     = event.MaxRefreshInterval
	DefaultRefreshInterval = event.DefaultRefreshInterval
)

var (
	ErrNotFound            = event.ErrNotFound
	ErrConflict            = event.ErrConflict
	ErrInvalidName         = event.ErrInvalidName
	ErrInvalidEvent        = event.ErrInvalidEvent
	ErrInvalidRange        = event.ErrInvalidRange
	ErrInvalidRRULE        = event.ErrInvalidRRULE
	ErrPreconditionFailed  = event.ErrPreconditionFailed
	ErrDefaultImmutable    = event.ErrDefaultImmutable
	ErrDemasiadosInvitados = event.ErrDemasiadosInvitados
	ErrCupoInvitaciones    = event.ErrCupoInvitaciones
	ErrInvalidResource     = event.ErrInvalidResource
	ErrResourceInactive    = event.ErrResourceInactive
	ErrNotManager          = event.ErrNotManager
	ErrInvalidShare        = event.ErrInvalidShare
	ErrShareSelf           = event.ErrShareSelf
	ErrReadOnly            = event.ErrReadOnly
	ErrInvalidURL          = event.ErrInvalidURL
)

func ValidStatus(s EventStatus) bool            { return event.ValidStatus(s) }
func ValidClassification(c Classification) bool { return event.ValidClassification(c) }
func ValidTransparency(t Transparency) bool     { return event.ValidTransparency(t) }
func SanitizeColor(v string) string             { return event.SanitizeColor(v) }
func SanitizeConferenceURL(v string) string     { return event.SanitizeConferenceURL(v) }
func ValidPartStat(p ParticipationStatus) bool  { return event.ValidPartStat(p) }
func ValidMethod(m SchedulingMethod) bool       { return event.ValidMethod(m) }
func ValidName(name string) bool                { return event.ValidName(name) }
func ExpandEvents(events []Event, windowStart, windowEnd time.Time) []Instance {
	return event.ExpandEvents(events, windowStart, windowEnd)
}

func FreeBusy(instances []Instance, windowStart, windowEnd time.Time) []FreeBusyPeriod {
	return event.FreeBusy(instances, windowStart, windowEnd)
}
func ParseRRule(s string) (RRule, error)    { return event.ParseRRule(s) }
func ValidResourceKind(k ResourceKind) bool { return event.ValidResourceKind(k) }
func ValidResource(kind ResourceKind, name string, capacity int, email string) bool {
	return event.ValidResource(kind, name, capacity, email)
}

func ResourceConflict(existing []Event, candidate Event, from, to time.Time) (FreeBusyPeriod, bool) {
	return event.ResourceConflict(existing, candidate, from, to)
}

func SuggestFreeSlots(busy []FreeBusyPeriod, from, to time.Time, loc *time.Location, wh WorkingHours, dur time.Duration, limit int) []FreeSlot {
	return event.SuggestFreeSlots(busy, from, to, loc, wh, dur, limit)
}
func ValidSharePermission(p SharePermission) bool        { return event.ValidSharePermission(p) }
func CanWrite(p SharePermission) bool                    { return event.CanWrite(p) }
func NormalizeEmail(s string) string                     { return event.NormalizeEmail(s) }
func ValidEmail(s string) bool                           { return event.ValidEmail(s) }
func ClampRefreshInterval(d time.Duration) time.Duration { return event.ClampRefreshInterval(d) }
func ValidSubscriptionURL(raw string) bool               { return event.ValidSubscriptionURL(raw) }

func LoadLocationCached(tzid string) *time.Location { return event.LoadLocationCached(tzid) }

func EventLocation(tzid string, allDay bool) *time.Location { return event.Location(tzid, allDay) }

func UnirICS(nombre string, objetos []string) string { return event.UnirICS(nombre, objetos) }
