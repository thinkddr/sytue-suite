// SPDX-License-Identifier: Apache-2.0
// Package domain keeps the backend contract on the same device-side card model.
package domain

import card "git.sytue.eu/sytue/one/apps/nucleo/contacts/card"

type (
	Kind        = card.Kind
	AddressBook = card.AddressBook
	CardObject  = card.Object
	EmailType   = card.EmailType
	Email       = card.Email
	PhoneType   = card.PhoneType
	Phone       = card.Phone
	AddressType = card.AddressType
	Address     = card.Address
	URL         = card.URL
	Related     = card.Related
	Contact     = card.Contact
	Group       = card.Group
)

const (
	KindIndividual  = card.KindIndividual
	KindGroup       = card.KindGroup
	KindOrg         = card.KindOrg
	KindLocation    = card.KindLocation
	KindApplication = card.KindApplication
	KindDevice      = card.KindDevice
	EmailHome       = card.EmailHome
	EmailWork       = card.EmailWork
	EmailOther      = card.EmailOther
	PhoneCell       = card.PhoneCell
	PhoneHome       = card.PhoneHome
	PhoneWork       = card.PhoneWork
	PhoneFax        = card.PhoneFax
	PhoneOther      = card.PhoneOther
	AddressHome     = card.AddressHome
	AddressWork     = card.AddressWork
	AddressOther    = card.AddressOther
	URLOther        = card.URLOther
)

var (
	ErrNotFound           = card.ErrNotFound
	ErrConflict           = card.ErrConflict
	ErrInvalidName        = card.ErrInvalidName
	ErrInvalidContact     = card.ErrInvalidContact
	ErrInvalidVCard       = card.ErrInvalidVCard
	ErrInvalidCSV         = card.ErrInvalidCSV
	ErrCSVTooManyContacts = card.ErrCSVTooManyContacts
	ErrInvalidGroup       = card.ErrInvalidGroup
	ErrPreconditionFailed = card.ErrPreconditionFailed
	ErrDefaultImmutable   = card.ErrDefaultImmutable
	TiposRelacion         = card.TiposRelacion
)

func ValidKind(k Kind) bool          { return card.ValidKind(k) }
func ValidName(name string) bool     { return card.ValidName(name) }
func ResourceName(uid string) string { return card.ResourceName(uid) }
func NewUID() string                 { return card.NewUID() }
