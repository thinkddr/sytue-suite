// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compat

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// Los tags JSON de este fichero son el contrato LITERAL de la Google People API v1
// (resourceName, emailAddresses, formattedName…): el cliente que apunta a Sytue
// cree hablar con Google, así que renombrarlos a snake_case rompería la emulación.

// FieldMetadata marca el valor principal de un campo repetido (People pone
// `primary` en los metadatos, no en el valor).
//
//nolint:tagliatelle // Contrato literal de Google People API v1.
type FieldMetadata struct {
	Primary bool `json:"primary,omitempty"`
}

// PersonName es el bloque `names[]` de una persona.
//
//nolint:tagliatelle // Contrato literal de Google People API v1.
type PersonName struct {
	Metadata        *FieldMetadata `json:"metadata,omitempty"`
	DisplayName     string         `json:"displayName,omitempty"`
	FamilyName      string         `json:"familyName,omitempty"`
	GivenName       string         `json:"givenName,omitempty"`
	MiddleName      string         `json:"middleName,omitempty"`
	HonorificPrefix string         `json:"honorificPrefix,omitempty"`
	HonorificSuffix string         `json:"honorificSuffix,omitempty"`
}

// PersonEmail es una entrada de `emailAddresses[]`.
//
//nolint:tagliatelle // Contrato literal de Google People API v1.
type PersonEmail struct {
	Metadata *FieldMetadata `json:"metadata,omitempty"`
	Value    string         `json:"value"`
	Type     string         `json:"type,omitempty"`
}

// PersonPhone es una entrada de `phoneNumbers[]`.
//
//nolint:tagliatelle // Contrato literal de Google People API v1.
type PersonPhone struct {
	Metadata *FieldMetadata `json:"metadata,omitempty"`
	Value    string         `json:"value"`
	Type     string         `json:"type,omitempty"`
}

// PersonOrganization es una entrada de `organizations[]` (empresa y puesto).
//
//nolint:tagliatelle // Contrato literal de Google People API v1.
type PersonOrganization struct {
	Metadata *FieldMetadata `json:"metadata,omitempty"`
	Name     string         `json:"name,omitempty"`
	Title    string         `json:"title,omitempty"`
}

// Person es el recurso `person` de la People API con el subconjunto de campos que
// Sytue modela y las librerías leen de verdad.
//
//nolint:tagliatelle // Contrato literal de Google People API v1.
type Person struct {
	ResourceName   string               `json:"resourceName"`
	ETag           string               `json:"etag,omitempty"`
	Names          []PersonName         `json:"names,omitempty"`
	EmailAddresses []PersonEmail        `json:"emailAddresses,omitempty"`
	PhoneNumbers   []PersonPhone        `json:"phoneNumbers,omitempty"`
	Organizations  []PersonOrganization `json:"organizations,omitempty"`
}

// Connections es la respuesta de people.connections.list.
//
//nolint:tagliatelle // Contrato literal de Google People API v1.
type Connections struct {
	Connections   []Person `json:"connections"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
}

// ResourceName compone el nombre de recurso de People a partir del id del contacto
// (el MISMO id que sirve la API nativa).
func ResourceName(id string) string { return "people/" + id }

// SplitResourceName extrae el id de un `people/{id}`; acepta también el id pelado,
// que es como llega en la ruta.
func SplitResourceName(name string) string {
	return strings.TrimPrefix(strings.TrimSpace(name), "people/")
}

// PersonFromContact traduce un contacto de Sytue al recurso person. Los bloques
// vacíos se OMITEN (un contacto sin organización no lleva `organizations`, en vez
// de un objeto vacío que el cliente pintaría como empresa sin nombre).
func PersonFromContact(c domain.Contact) Person {
	p := Person{
		ResourceName: ResourceName(c.ID.String()),
		ETag:         c.ETag.String(),
	}
	if name := personName(c); name != nil {
		p.Names = []PersonName{*name}
	}
	p.EmailAddresses = personEmails(c.Emails)
	p.PhoneNumbers = personPhones(c.Phones)
	if org := strings.TrimSpace(c.Organization); org != "" || strings.TrimSpace(c.Title) != "" {
		p.Organizations = []PersonOrganization{{
			Metadata: &FieldMetadata{Primary: true},
			Name:     org,
			Title:    strings.TrimSpace(c.Title),
		}}
	}
	return p
}

// personName arma el bloque names[] (nil si el contacto no tiene nombre alguno).
func personName(c domain.Contact) *PersonName {
	n := PersonName{
		Metadata:        &FieldMetadata{Primary: true},
		DisplayName:     c.DisplayName(),
		FamilyName:      c.FamilyName,
		GivenName:       c.GivenName,
		MiddleName:      c.AdditionalName,
		HonorificPrefix: c.HonorificPrefix,
		HonorificSuffix: c.HonorificSuffix,
	}
	if n.DisplayName == "" && n.FamilyName == "" && n.GivenName == "" {
		return nil
	}
	return &n
}

// personEmails traduce los EMAIL de la vCard a emailAddresses[].
func personEmails(emails []domain.Email) []PersonEmail {
	if len(emails) == 0 {
		return nil
	}
	out := make([]PersonEmail, 0, len(emails))
	for _, e := range emails {
		out = append(out, PersonEmail{
			Metadata: &FieldMetadata{Primary: e.IsPrimary},
			Value:    e.Email,
			Type:     string(e.Type),
		})
	}
	return out
}

// personPhones traduce los TEL de la vCard a phoneNumbers[]. `cell` es `mobile` en
// el vocabulario de People; el resto de etiquetas coincide.
func personPhones(phones []domain.Phone) []PersonPhone {
	if len(phones) == 0 {
		return nil
	}
	out := make([]PersonPhone, 0, len(phones))
	for _, p := range phones {
		t := string(p.Type)
		if p.Type == domain.PhoneCell {
			t = "mobile"
		}
		out = append(out, PersonPhone{
			Metadata: &FieldMetadata{Primary: p.IsPrimary},
			Value:    p.Phone,
			Type:     t,
		})
	}
	return out
}

// PersonWritableFields son los campos de People que la emulación sabe escribir: los
// mismos que sabe leer. Los demás (direcciones, cumpleaños, biografía…) se rechazan
// por su nombre en vez de guardarse a medias.
var PersonWritableFields = []string{"names", "emailAddresses", "phoneNumbers", "organizations"}

// ErrUnsupportedPersonField es un campo de updatePersonFields que no se escribe.
var ErrUnsupportedPersonField = errors.New("campo de persona no soportado")

// ApplyTo copia sobre el contacto los campos pedidos de la persona, dejando el resto
// como estaba (updateContact). En createContact se piden todos. El nombre legible
// sale de displayName o, si falta, de nombre y apellidos; sin ninguno es inválido.
func (p Person) ApplyTo(c domain.Contact, fields []string) (domain.Contact, error) {
	for _, f := range fields {
		switch f {
		case "names":
			c.FormattedName, c.GivenName, c.FamilyName = "", "", ""
			c.AdditionalName, c.HonorificPrefix, c.HonorificSuffix = "", "", ""
			if len(p.Names) > 0 {
				n := p.Names[0]
				c.GivenName, c.FamilyName = strings.TrimSpace(n.GivenName), strings.TrimSpace(n.FamilyName)
				c.AdditionalName = strings.TrimSpace(n.MiddleName)
				c.HonorificPrefix, c.HonorificSuffix = strings.TrimSpace(n.HonorificPrefix), strings.TrimSpace(n.HonorificSuffix)
				c.FormattedName = strings.TrimSpace(n.DisplayName)
			}
		case "emailAddresses":
			c.Emails = nil
			for _, e := range p.EmailAddresses {
				if v := strings.TrimSpace(e.Value); v != "" {
					c.Emails = append(c.Emails, domain.Email{Email: v, Type: emailTypeFromPeople(e.Type), IsPrimary: e.Metadata != nil && e.Metadata.Primary})
				}
			}
		case "phoneNumbers":
			c.Phones = nil
			for _, t := range p.PhoneNumbers {
				if v := strings.TrimSpace(t.Value); v != "" {
					c.Phones = append(c.Phones, domain.Phone{Phone: v, Type: phoneTypeFromPeople(t.Type), IsPrimary: t.Metadata != nil && t.Metadata.Primary})
				}
			}
		case "organizations":
			c.Organization, c.Title = "", ""
			if len(p.Organizations) > 0 {
				c.Organization = strings.TrimSpace(p.Organizations[0].Name)
				c.Title = strings.TrimSpace(p.Organizations[0].Title)
			}
		default:
			return domain.Contact{}, fmt.Errorf("%w: %s", ErrUnsupportedPersonField, f)
		}
	}
	if c.FormattedName == "" {
		c.FormattedName = strings.TrimSpace(c.GivenName + " " + c.FamilyName)
	}
	if c.FormattedName == "" {
		c.FormattedName = c.Organization
	}
	if c.FormattedName == "" {
		return domain.Contact{}, domain.ErrInvalidContact
	}
	return c, nil
}

// emailTypeFromPeople: People dice home/work/other (o una etiqueta libre).
func emailTypeFromPeople(t string) domain.EmailType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "work":
		return domain.EmailWork
	case "other":
		return domain.EmailOther
	}
	return domain.EmailHome
}

// phoneTypeFromPeople: `mobile` es `cell` en la vCard; los faxes de People son fax.
func phoneTypeFromPeople(t string) domain.PhoneType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "mobile", "workmobile":
		return domain.PhoneCell
	case "home":
		return domain.PhoneHome
	case "work", "main":
		return domain.PhoneWork
	case "homefax", "workfax", "otherfax", "fax":
		return domain.PhoneFax
	}
	return domain.PhoneOther
}

// NewConnections arma la respuesta del listado (connections nunca nulo: las
// librerías iteran el array sin comprobarlo).
func NewConnections(people []Person, nextPageToken string) Connections {
	if people == nil {
		people = []Person{}
	}
	return Connections{Connections: people, NextPageToken: nextPageToken}
}

// ---------------------------------------------------------------------------
// Errores en el dialecto de Google
// ---------------------------------------------------------------------------

// GoogleError es el sobre de error de las APIs de Google.
type GoogleError struct {
	Error GoogleErrorBody `json:"error"`
}

// GoogleErrorBody es el cuerpo del sobre de error.
type GoogleErrorBody struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Errors  []GoogleErrorItem `json:"errors"`
}

// GoogleErrorItem es el detalle por error que leen los clientes para decidir
// reintentos (reason).
type GoogleErrorItem struct {
	Domain  string `json:"domain"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// NewGoogleError traduce un estado HTTP + mensaje al sobre de Google.
func NewGoogleError(status int, message string) GoogleError {
	return GoogleError{Error: GoogleErrorBody{
		Code:    status,
		Message: message,
		Errors: []GoogleErrorItem{{
			Domain:  "global",
			Reason:  googleReason(status),
			Message: message,
		}},
	}}
}

// googleReason mapea el estado HTTP al `reason` que usan las librerías de Google.
func googleReason(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "badRequest"
	case http.StatusUnauthorized:
		return "authError"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "notFound"
	case http.StatusConflict:
		return "duplicate"
	case http.StatusPreconditionFailed:
		return "conditionNotMet"
	case http.StatusTooManyRequests:
		return "rateLimitExceeded"
	case http.StatusNotImplemented:
		return "notImplemented"
	default:
		return "backendError"
	}
}
