// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compat

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// Los tags JSON de este fichero son el contrato LITERAL de Microsoft Graph v1.0
// (contact, businessPhones, @odata.nextLink…): el cliente que apunta a Sytue cree
// hablar con Microsoft, así que renombrarlos a snake_case rompería la emulación.

// GraphEmailAddress es el par dirección/nombre que Graph usa en los correos de un
// contacto.
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0.
type GraphEmailAddress struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

// GraphContact es el recurso `contact` de Graph con el subconjunto de campos que
// Sytue modela.
//
// Graph solo tiene tres huecos para teléfonos (businessPhones, homePhones,
// mobilePhone): el primer móvil va a mobilePhone y el resto de etiquetas que Graph
// no distingue (fax, otros, móviles adicionales) viajan en businessPhones — se
// prefiere una etiqueta aproximada a PERDER el número por el camino.
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0.
type GraphContact struct {
	ID                   string              `json:"id"`
	DisplayName          string              `json:"displayName,omitempty"`
	GivenName            string              `json:"givenName,omitempty"`
	Surname              string              `json:"surname,omitempty"`
	MiddleName           string              `json:"middleName,omitempty"`
	NickName             string              `json:"nickName,omitempty"`
	Title                string              `json:"title,omitempty"`
	CompanyName          string              `json:"companyName,omitempty"`
	JobTitle             string              `json:"jobTitle,omitempty"`
	PersonalNotes        string              `json:"personalNotes,omitempty"`
	Birthday             string              `json:"birthday,omitempty"`
	EmailAddresses       []GraphEmailAddress `json:"emailAddresses,omitempty"`
	BusinessPhones       []string            `json:"businessPhones,omitempty"`
	HomePhones           []string            `json:"homePhones,omitempty"`
	MobilePhone          string              `json:"mobilePhone,omitempty"`
	CreatedDateTime      string              `json:"createdDateTime,omitempty"`
	LastModifiedDateTime string              `json:"lastModifiedDateTime,omitempty"`
}

// GraphCollection es una colección OData con su enlace a la página siguiente.
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0 (OData).
type GraphCollection[T any] struct {
	Value    []T    `json:"value"`
	NextLink string `json:"@odata.nextLink,omitempty"`
}

// NewGraphCollection arma la colección (value nunca nulo: las librerías iteran el
// array sin comprobarlo).
func NewGraphCollection[T any](items []T, nextLink string) GraphCollection[T] {
	if items == nil {
		items = []T{}
	}
	return GraphCollection[T]{Value: items, NextLink: nextLink}
}

// NextLink construye el `@odata.nextLink` ABSOLUTO que Graph exige: el cliente lo
// vuelve a pedir tal cual. skip<=0 ⇒ "" (no hay más páginas).
func NextLink(baseURL, path string, top, skip int) string {
	if skip <= 0 {
		return ""
	}
	v := url.Values{}
	if top > 0 {
		v.Set("$top", strconv.Itoa(top))
	}
	v.Set("$skip", strconv.Itoa(skip))
	return strings.TrimRight(baseURL, "/") + path + "?" + v.Encode()
}

// GraphFromContact traduce un contacto de Sytue al recurso contact de Graph.
func GraphFromContact(c domain.Contact) GraphContact {
	g := GraphContact{
		ID:                   c.ID.String(),
		DisplayName:          c.DisplayName(),
		GivenName:            c.GivenName,
		Surname:              c.FamilyName,
		MiddleName:           c.AdditionalName,
		NickName:             c.Nickname,
		Title:                c.HonorificPrefix,
		CompanyName:          c.Organization,
		JobTitle:             c.Title,
		PersonalNotes:        c.Note,
		CreatedDateTime:      rfc3339(c.CreatedAt),
		LastModifiedDateTime: rfc3339(c.UpdatedAt),
	}
	if c.Birthday != nil {
		g.Birthday = rfc3339(*c.Birthday)
	}
	name := c.DisplayName()
	for _, e := range c.Emails {
		g.EmailAddresses = append(g.EmailAddresses, GraphEmailAddress{Address: e.Email, Name: name})
	}
	for _, p := range c.Phones {
		switch {
		case p.Type == domain.PhoneCell && g.MobilePhone == "":
			g.MobilePhone = p.Phone
		case p.Type == domain.PhoneHome:
			g.HomePhones = append(g.HomePhones, p.Phone)
		default:
			g.BusinessPhones = append(g.BusinessPhones, p.Phone)
		}
	}
	return g
}

// ---------------------------------------------------------------------------
// Errores en el dialecto de Graph
// ---------------------------------------------------------------------------

// GraphError es el sobre de error de Microsoft Graph.
type GraphError struct {
	Error GraphErrorBody `json:"error"`
}

// GraphErrorBody es el cuerpo del sobre: `code` es un símbolo, no un número.
type GraphErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewGraphError traduce un estado HTTP + mensaje al sobre de Graph.
func NewGraphError(status int, message string) GraphError {
	return GraphError{Error: GraphErrorBody{Code: graphCode(status), Message: message}}
}

// graphCode mapea el estado HTTP al código simbólico de Graph.
func graphCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalidRequest"
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return "accessDenied"
	case http.StatusNotFound:
		return "itemNotFound"
	case http.StatusConflict:
		return "nameAlreadyExists"
	case http.StatusPreconditionFailed:
		return "preconditionFailed"
	case http.StatusTooManyRequests:
		return "activityLimitReached"
	case http.StatusNotImplemented:
		return "notImplemented"
	default:
		return "generalException"
	}
}
