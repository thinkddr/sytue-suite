// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package tenancy modela el contexto de tenant (organización) que atraviesa
// todas las superficies de Sytue One.
//
// Modelo (docs/CONVENTIONS.md §9): organización ⇄ grupo en Sytue ID
// `org/<slug>` ⇄ namespace Kapsule `tenant-<slug>` ⇄ filas por tenant en
// PostgreSQL (RLS) ⇄ prefijos S3 dedicados. Este paquete es el origen único
// del identificador de tenant que el resto de paquetes (pg/RLS, objectstore,
// authn) consume.
package tenancy

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrNoTenant indica que no hay tenant en el contexto.
var ErrNoTenant = errors.New("tenancy: no hay tenant en el contexto")

// slugRe valida slugs de tenant: minúsculas, dígitos y guiones; sin guiones al
// borde. Coincide con `org/<slug>` de Sytue ID y `tenant-<slug>` de Kapsule.
var slugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// Tenant identifica una organización dentro de la suite.
type Tenant struct {
	// Slug es el identificador canónico (p. ej. "acme").
	Slug string
}

// ValidSlug indica si un slug de tenant es sintácticamente válido.
func ValidSlug(slug string) bool {
	return slugRe.MatchString(slug)
}

// New crea un Tenant validando el slug.
func New(slug string) (Tenant, error) {
	if !ValidSlug(slug) {
		return Tenant{}, fmt.Errorf("tenancy: slug de tenant inválido: %q", slug)
	}
	return Tenant{Slug: slug}, nil
}

// Group devuelve el nombre del grupo en Sytue ID (`org/<slug>`).
func (t Tenant) Group() string { return "org/" + t.Slug }

// Namespace devuelve el namespace Kapsule (`tenant-<slug>`).
func (t Tenant) Namespace() string { return "tenant-" + t.Slug }

// StoragePrefix devuelve el prefijo S3 dedicado (`tenants/<slug>/`).
func (t Tenant) StoragePrefix() string { return "tenants/" + t.Slug + "/" }

// PersonalSlugPrefix distingue a las cuentas personales —una persona, cuya
// organización es su correo— del resto: Sytue ID acuña sus slugs con él y no
// deja que ninguna organización empiece así. Vive aquí, y no solo en Sytue ID,
// porque es lo que permite a objectstore mandar sus blobs a otro almacén sin
// preguntar a nadie.
const PersonalSlugPrefix = "p-"

// Personal indica si el tenant es una cuenta personal.
func (t Tenant) Personal() bool { return strings.HasPrefix(t.Slug, PersonalSlugPrefix) }

type ctxKey struct{}

// Into devuelve un contexto que transporta el tenant dado.
func Into(ctx context.Context, t Tenant) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

// From extrae el tenant del contexto.
func From(ctx context.Context) (Tenant, error) {
	if ctx != nil {
		if t, ok := ctx.Value(ctxKey{}).(Tenant); ok && t.Slug != "" {
			return t, nil
		}
	}
	return Tenant{}, ErrNoTenant
}

// MustFrom extrae el tenant o entra en pánico (uso interno tras RequireAuth).
func MustFrom(ctx context.Context) Tenant {
	t, err := From(ctx)
	if err != nil {
		panic(err)
	}
	return t
}
