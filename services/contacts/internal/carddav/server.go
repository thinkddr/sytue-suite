// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package carddav

import (
	"errors"
	"net/http"

	"github.com/emersion/go-webdav/carddav"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

// maxVCardBytes es el tamaño máximo de un recurso vCard (CARDDAV:max-resource-size).
// 1 MiB: holgado para una tarjeta con FOTO embebida (una imagen de hasta 256 KiB
// crece ~33% en base64 —~342 KiB— más el resto de propiedades), de modo que editar
// un contacto con foto vía CardDAV PUT no rebota por tamaño.
const maxVCardBytes int64 = 1024 * 1024

// Handler construye el http.Handler de CardDAV (RFC 6352) sobre el Backend de
// Sytue Contacts. El prefijo debe coincidir con la ruta de montaje (/dav). Debe
// componerse DESPUÉS de RequireAuth (necesita los claims en el contexto).
// El cliente de cache puede ser nil: entonces el backend consulta PostgreSQL como
// siempre. Se pasa por parámetro en vez de un setter porque el Backend vive dentro
// del Handler y no queda accesible después de construirlo.
func Handler(st *store.Store, c *cache.Client) http.Handler {
	b := NewBackend(st)
	b.UsarCache(c)
	return &carddav.Handler{Backend: b, Prefix: basePath}
}

// WithTenant garantiza que el contexto de las peticiones CardDAV transporta un
// tenant válido; sin él, RLS no puede aislar y se rechaza con 403. Debe
// componerse DESPUÉS de RequireAuth.
func WithTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := tenancy.From(r.Context()); err != nil {
			log.From(r.Context()).Warn("carddav: petición sin tenant")
			http.Error(w, "tenant requerido", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// supportedAddressData declara los tipos/versión de vCard soportados
// (CARDDAV:supported-address-data, RFC 6352 6.2.2): vCard 4.0 (y 3.0 en lectura).
func supportedAddressData() []carddav.AddressDataType {
	return []carddav.AddressDataType{
		{ContentType: "text/vcard", Version: "4.0"},
		{ContentType: "text/vcard", Version: "3.0"},
	}
}

// putConditions extrae de las opciones de PUT las precondiciones HTTP: el ETag
// de If-Match (operación condicional sobre recurso existente) y si se exige que
// el recurso esté ausente (If-None-Match: *), conforme a RFC 6352 6.3.2.
func putConditions(opts *carddav.PutAddressObjectOptions) (ifMatch *uuid.UUID, requireAbsent bool) {
	if opts == nil {
		return nil, false
	}
	if opts.IfNoneMatch.IsWildcard() {
		requireAbsent = true
	}
	if tag, terr := opts.IfMatch.ETag(); terr == nil {
		if id, err := uuid.Parse(unquoteETag(tag)); err == nil {
			ifMatch = &id
		}
	}
	return ifMatch, requireAbsent
}

// unquoteETag retira las comillas (y el prefijo débil W/) de un ETag HTTP.
func unquoteETag(tag string) string {
	if len(tag) >= 2 && tag[0] == 'W' && tag[1] == '/' {
		tag = tag[2:]
	}
	if len(tag) >= 2 && tag[0] == '"' && tag[len(tag)-1] == '"' {
		tag = tag[1 : len(tag)-1]
	}
	return tag
}

// isErr es un atajo para errors.Is.
func isErr(err, target error) bool { return errors.Is(err, target) }
