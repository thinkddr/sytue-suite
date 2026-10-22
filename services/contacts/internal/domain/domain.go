// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define los tipos y reglas de negocio de Sytue Contacts,
// independientes de transporte (HTTP/CardDAV) y de almacenamiento (PostgreSQL).
//
// Clean-room: modelo propio sobre los estandares abiertos vCard 4.0 (RFC 6350),
// CardDAV (RFC 6352) y sincronizacion WebDAV (RFC 6578). Implementa la semantica
// desde las especificaciones publicas; no deriva de codigo de terceros AGPL.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Errores de dominio. La capa HTTP/CardDAV los traduce a su error de transporte.
var (
	ErrNotFound           = errors.New("contacts: recurso no encontrado")
	ErrConflict           = errors.New("contacts: conflicto de unicidad")
	ErrInvalidName        = errors.New("contacts: nombre invalido")
	ErrInvalidContact     = errors.New("contacts: contacto invalido")
	ErrInvalidVCard       = errors.New("contacts: vCard invalida")
	ErrInvalidCSV         = errors.New("contacts: CSV no reconocido")
	ErrInvalidGroup       = errors.New("contacts: grupo invalido")
	ErrPreconditionFailed = errors.New("contacts: precondicion ETag fallida")
	ErrDefaultImmutable   = errors.New("contacts: la libreta por defecto no se puede borrar")
)

// Kind es el KIND de una vCard (RFC 6350 6.1.4): naturaleza del objeto.
type Kind string

const (
	KindIndividual  Kind = "individual"
	KindGroup       Kind = "group"
	KindOrg         Kind = "org"
	KindLocation    Kind = "location"
	KindApplication Kind = "application"
	KindDevice      Kind = "device"
)

// ValidKind valida un KIND soportado.
func ValidKind(k Kind) bool {
	switch k {
	case KindIndividual, KindGroup, KindOrg, KindLocation, KindApplication, KindDevice:
		return true
	default:
		return false
	}
}

// AddressBook es una coleccion CardDAV (RFC 6352) propiedad de un usuario.
type AddressBook struct {
	ID          uuid.UUID `json:"id"`
	Tenant      string    `json:"-"`
	OwnerSub    string    `json:"owner_sub"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description,omitempty"`
	Color       string    `json:"color"`
	IsDefault   bool      `json:"is_default"`
	CTag        uuid.UUID `json:"ctag"`
	SyncToken   int64     `json:"sync_token"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CardObject es el recurso CardDAV (RFC 6352): la vCard serializada de un
// contacto (o grupo), mas los metadatos de sincronizacion.
type CardObject struct {
	ID            uuid.UUID `json:"id"`
	Tenant        string    `json:"-"`
	AddressBookID uuid.UUID `json:"address_book_id"`
	UID           string    `json:"uid"`
	ResourceName  string    `json:"resource_name"`
	Kind          Kind      `json:"kind"`
	Data          string    `json:"-"`
	ETag          uuid.UUID `json:"etag"`
	SizeBytes     int       `json:"size_bytes"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// HTTPETag devuelve el ETag en formato fuerte para cabeceras HTTP/CardDAV.
func (c CardObject) HTTPETag() string { return `"` + c.ETag.String() + `"` }

// EmailType es la etiqueta TYPE de un EMAIL (RFC 6350 6.4.2 / RFC 2426 vCard).
type EmailType string

const (
	EmailHome  EmailType = "home"
	EmailWork  EmailType = "work"
	EmailOther EmailType = "other"
)

// Email es un EMAIL de un contacto.
type Email struct {
	Email     string    `json:"email"`
	Type      EmailType `json:"type"`
	IsPrimary bool      `json:"is_primary"`
}

// PhoneType es la etiqueta TYPE de un TEL (RFC 6350 6.4.1).
type PhoneType string

const (
	PhoneCell  PhoneType = "cell"
	PhoneHome  PhoneType = "home"
	PhoneWork  PhoneType = "work"
	PhoneFax   PhoneType = "fax"
	PhoneOther PhoneType = "other"
)

// Phone es un TEL de un contacto.
type Phone struct {
	Phone     string    `json:"phone"`
	Type      PhoneType `json:"type"`
	IsPrimary bool      `json:"is_primary"`
}

// AddressType es la etiqueta TYPE de un ADR (RFC 6350 6.3.1).
type AddressType string

const (
	AddressHome  AddressType = "home"
	AddressWork  AddressType = "work"
	AddressOther AddressType = "other"
)

// Address es una direccion postal (ADR) de un contacto (RFC 6350 6.3.1). Los
// campos siguen los componentes estructurados de ADR: apartado de correos,
// direccion extendida, calle, localidad (ciudad), region (provincia/estado),
// codigo postal y pais.
type Address struct {
	POBox      string      `json:"po_box,omitempty"`
	Extended   string      `json:"extended,omitempty"`
	Street     string      `json:"street,omitempty"`
	Locality   string      `json:"locality,omitempty"`
	Region     string      `json:"region,omitempty"`
	PostalCode string      `json:"postal_code,omitempty"`
	Country    string      `json:"country,omitempty"`
	Type       AddressType `json:"type"`
	IsPrimary  bool        `json:"is_primary"`
}

// IsZero indica si la direccion no tiene ningun componente postal (solo etiqueta
// o principal): se usa para descartar filas vacias antes de serializar.
func (a Address) IsZero() bool {
	return a.POBox == "" && a.Extended == "" && a.Street == "" && a.Locality == "" &&
		a.Region == "" && a.PostalCode == "" && a.Country == ""
}

// Contact es la proyeccion normalizada de una vCard de tipo individual/org.
type Contact struct {
	ID              uuid.UUID  `json:"id"`
	Tenant          string     `json:"-"`
	AddressBookID   uuid.UUID  `json:"address_book_id"`
	ObjectID        uuid.UUID  `json:"object_id"`
	UID             string     `json:"uid"`
	FormattedName   string     `json:"formatted_name"`
	FamilyName      string     `json:"family_name,omitempty"`
	GivenName       string     `json:"given_name,omitempty"`
	AdditionalName  string     `json:"additional_name,omitempty"`
	HonorificPrefix string     `json:"honorific_prefix,omitempty"`
	HonorificSuffix string     `json:"honorific_suffix,omitempty"`
	Organization    string     `json:"organization,omitempty"`
	Title           string     `json:"title,omitempty"`
	Nickname        string     `json:"nickname,omitempty"`
	Note            string     `json:"note,omitempty"`
	Birthday        *time.Time `json:"birthday,omitempty"`
	PhotoURL        string     `json:"photo_url,omitempty"`
	HasPhoto        bool       `json:"has_photo"` // PHOTO embebida: la que sirve GET /contacts/photo (migración 0005)
	Emails          []Email    `json:"emails,omitempty"`
	Phones          []Phone    `json:"phones,omitempty"`
	Addresses       []Address  `json:"addresses,omitempty"`
	ETag            uuid.UUID  `json:"etag"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// DisplayName devuelve el mejor nombre legible del contacto: FN si existe, o el
// compuesto de given/family, o la organizacion como ultimo recurso.
func (c Contact) DisplayName() string {
	if fn := strings.TrimSpace(c.FormattedName); fn != "" {
		return fn
	}
	composed := strings.TrimSpace(strings.TrimSpace(c.GivenName) + " " + strings.TrimSpace(c.FamilyName))
	if composed != "" {
		return composed
	}
	return strings.TrimSpace(c.Organization)
}

// PrimaryEmail devuelve el correo marcado como principal, o el primero, o "".
func (c Contact) PrimaryEmail() string {
	for _, e := range c.Emails {
		if e.IsPrimary {
			return e.Email
		}
	}
	if len(c.Emails) > 0 {
		return c.Emails[0].Email
	}
	return ""
}

// Group es una vCard con KIND=group (RFC 6350 6.1.4) cuyos miembros son los UID
// de los contactos referenciados (propiedad MEMBER, RFC 6350 6.6.5).
type Group struct {
	ID            uuid.UUID `json:"id"`
	Tenant        string    `json:"-"`
	AddressBookID uuid.UUID `json:"address_book_id"`
	ObjectID      uuid.UUID `json:"object_id"`
	UID           string    `json:"uid"`
	Name          string    `json:"name"`
	Members       []string  `json:"members"` // UID de los contactos miembros
	ETag          uuid.UUID `json:"etag"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ValidName valida el nombre de coleccion (segmento de ruta CardDAV): no vacio,
// sin separadores ni control, longitud razonable.
func ValidName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return false
	}
	if name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return false
	}
	for _, r := range name {
		if r < 0x20 {
			return false
		}
	}
	return true
}

// ResourceName devuelve el nombre de recurso CardDAV canonico de un UID
// (<uid>.vcf), saneado para usarse como segmento de ruta.
func ResourceName(uid string) string {
	uid = strings.TrimSpace(uid)
	uid = strings.NewReplacer("/", "_", "\\", "_", "\x00", "").Replace(uid)
	if uid == "" {
		uid = uuid.NewString()
	}
	if strings.HasSuffix(strings.ToLower(uid), ".vcf") {
		return uid
	}
	return uid + ".vcf"
}

// NewUID genera un UID estable para una vCard nueva (RFC 6350 6.7.6).
func NewUID() string { return uuid.NewString() }
