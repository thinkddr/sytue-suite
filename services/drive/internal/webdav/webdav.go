// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package webdav expone el almacén de Sytue Drive vía WebDAV (RFC 4918) usando
// la librería permisiva golang.org/x/net/webdav (BSD-3-Clause). Implementa un
// webdav.FileSystem propio respaldado por el store (metadatos en PostgreSQL) y
// el objectstore (blobs en S3), respetando el aislamiento por tenant del JWT.
//
// Clean-room: implementación original sobre el estándar WebDAV; sin código AGPL.
package webdav

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/webdav"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// Handler construye el http.Handler de WebDAV sobre el FileSystem de Drive.
// El prefijo (StripPrefix) debe coincidir con la ruta de montaje (p. ej. /dav).
func Handler(prefix string, svc *service.Service) http.Handler {
	return manejador(prefix, NewFileSystem(svc))
}

func manejador(prefix string, fs *FileSystem) http.Handler {
	bloqueos := &bloqueosPorOrganizacion{de: map[string]webdav.LockSystem{}}
	logger := func(r *http.Request, err error) {
		if err != nil {
			log.From(r.Context()).Warn("webdav",
				"method", r.Method, "path", logsafe.Text(r.URL.Path), "error", err)
		}
	}
	// Un Handler por petición: es una estructura de cuatro campos, y es la forma de que
	// los bloqueos sean los de la organización de quien pide (la interfaz LockSystem no
	// recibe el contexto).
	return comoDescarga(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metodoDeVerdad(r)
		acotarBloqueo(r)
		ctx := normalizarCabecerasCliente(r.Context(), r, prefix)
		r = r.WithContext(ctx)
		(&webdav.Handler{
			Prefix:     prefix,
			FileSystem: fs,
			LockSystem: bloqueos.organizacion(ctx),
			Logger:     logger,
		}).ServeHTTP(w, r)
	}))
}

type claveOrigenMove struct{}

func conOrigenMove(ctx context.Context, src string) context.Context {
	return context.WithValue(ctx, claveOrigenMove{}, src)
}

func origenMoveDe(ctx context.Context) string {
	v, _ := ctx.Value(claveOrigenMove{}).(string)
	return v
}

// normalizarCabecerasCliente adapta cabeceras RFC 4918 válidas que envían clientes
// reales (macOS Finder, Windows WebClient, Microsoft Office) a lo que espera
// golang.org/x/net/webdav:
//  1. En MOVE sin cabecera Overwrite, el valor por defecto según RFC 4918 §9.9.3 y
//     §10.6 es "T" (golang.org/x/net/webdav exige explícitamente Overwrite == "T").
//  2. En la cabecera If con etiquetas de recurso de solo ruta `</dav/...>` (RFC 4918
//     §10.4.2), antepone el esquema y r.Host para que confirmLocks no las descarte
//     por tener u.Host vacío.
func normalizarCabecerasCliente(ctx context.Context, r *http.Request, prefix string) context.Context {
	if ifHdr := r.Header.Get("If"); ifHdr != "" && r.Host != "" {
		r.Header.Set("If", normalizarIfEtiquetado(ifHdr, r.Host))
	}
	if r.Method == "MOVE" {
		if r.Header.Get("Overwrite") == "" {
			r.Header.Set("Overwrite", "T")
		}
		src := strings.TrimPrefix(r.URL.Path, prefix)
		ctx = conOrigenMove(ctx, src)
	}
	return ctx
}

func normalizarIfEtiquetado(hdr, host string) string {
	var b strings.Builder
	rest := hdr
	for {
		idx := strings.Index(rest, "</")
		if idx < 0 {
			b.WriteString(rest)
			break
		}
		// Solo es un resource-tag si no va precedido inmediatamente de '(' o 'Not' dentro de una lista.
		prefix := strings.TrimSpace(rest[:idx])
		esTag := prefix == "" || strings.HasSuffix(prefix, ")")
		if !esTag {
			b.WriteString(rest[:idx+2])
			rest = rest[idx+2:]
			continue
		}
		end := strings.IndexByte(rest[idx:], '>')
		if end < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:idx])
		b.WriteString("<http://")
		b.WriteString(host)
		b.WriteString(rest[idx+1 : idx+end+1])
		rest = rest[idx+end+1:]
	}
	return b.String()
}

// metodosOcultos son los de WebDAV que un cliente puede mandar como POST con
// `X-HTTP-Method-Override`: el HttpURLConnection de Android (el proveedor de documentos
// de Drive) solo deja usar los de HTTP/1.1. Solo estos: GET, PUT o DELETE no hacen falta
// y un POST cualquiera sigue siendo un POST.
var metodosOcultos = map[string]bool{"PROPFIND": true, "PROPPATCH": true, "MKCOL": true, "MOVE": true, "COPY": true}

// metodoDeVerdad pone el método que dice `X-HTTP-Method-Override` en un POST.
func metodoDeVerdad(r *http.Request) {
	if m := strings.ToUpper(r.Header.Get("X-HTTP-Method-Override")); r.Method == http.MethodPost && metodosOcultos[m] {
		r.Method = m
	}
}

// duracionMaximaDeBloqueo acota un LOCK. Sin cabecera Timeout la biblioteca lo da por
// infinito, y en memoria no caducaría nunca. Los clientes de escritorio lo renuevan.
const duracionMaximaDeBloqueo = time.Hour

// bloqueosPorOrganizacion separa los bloqueos de cada organización (FILE-05). Las rutas
// de WebDAV son las mismas cadenas en todas ellas —la organización sale del token—, y
// con un solo sistema de bloqueos un LOCK de «/» en una organización dejaba sin escribir
// a todas las demás (423).
//
// ponytail: un mapa que no olvida organizaciones; crece con las que usan WebDAV en esta
// réplica, que se reinicia en cada despliegue.
type bloqueosPorOrganizacion struct {
	mu sync.Mutex
	de map[string]webdav.LockSystem
}

func (b *bloqueosPorOrganizacion) organizacion(ctx context.Context) webdav.LockSystem {
	// Sin organización no se llega aquí (WithTenant va delante); si se llegara, comparte
	// el espacio vacío, que es lo que había antes.
	org := ""
	if t, err := tenancy.From(ctx); err == nil {
		org = t.Slug
	}
	if sub := store.OwnerFrom(ctx); sub != "" {
		org += "\x00" + sub
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	ls, ok := b.de[org]
	if !ok {
		ls = webdav.NewMemLS()
		b.de[org] = ls
	}
	return ls
}

// acotarBloqueo pide como mucho duracionMaximaDeBloqueo en un LOCK (nuevo o renovación).
// Se hace en la cabecera y no en el LockSystem para que la respuesta diga la duración
// de verdad y el cliente sepa que tiene que renovarlo.
func acotarBloqueo(r *http.Request) {
	if r.Method != "LOCK" {
		return
	}
	maximo := int(duracionMaximaDeBloqueo / time.Second)
	primero, _, _ := strings.Cut(r.Header.Get("Timeout"), ",")
	if s, ok := strings.CutPrefix(strings.TrimSpace(primero), "Second-"); ok {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= maximo {
			return
		}
	}
	r.Header.Set("Timeout", "Second-"+strconv.Itoa(maximo))
}

// comoDescarga hace que el contenido de un fichero salga por WebDAV igual que por
// el resto de Drive: como descarga, nunca como página.
//
// La biblioteca sirve GET con `http.ServeContent`, que elige el `Content-Type` por
// la extensión y no pone `Content-Disposition`: un `.html` o un `.svg` subidos
// saldrían en línea desde el origen de Drive, que es el de la aplicación. Hoy hace
// falta un Bearer para llegar aquí y un navegador no lo manda al navegar, pero eso
// es una propiedad de quien llama, no de esta respuesta. `attachment` y una CSP
// `sandbox` valen aunque mañana WebDAV acepte otra forma de autenticarse.
//
// Solo GET y HEAD: PROPFIND y compañía devuelven XML del protocolo, no contenido.
func comoDescarga(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			h := w.Header()
			h.Set("Content-Disposition", "attachment")
			h.Set("Content-Security-Policy", "sandbox")
			h.Set("X-Content-Type-Options", "nosniff")
		}
		next.ServeHTTP(w, r)
	})
}

// WithTenant garantiza que el contexto de las peticiones WebDAV transporta el
// tenant derivado de los claims (RequireAuth ya lo inyecta, pero este envoltorio
// lo reafirma y rechaza peticiones sin tenant válido, que WebDAV no podría
// aislar). Debe componerse DESPUÉS de RequireAuth.
func WithTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := tenancy.From(r.Context()); err != nil {
			// Sin tenant en el token: WebDAV no puede aislar; 403.
			http.Error(w, "tenant requerido", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ctxWithActor enriquece el contexto con el subject para auditoría de escrituras.
func ctxWithActor(ctx context.Context) context.Context {
	if c, ok := authn.FromContext(ctx); ok {
		return log.With(ctx, "actor", c.Subject)
	}
	return ctx
}
