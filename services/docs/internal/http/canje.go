// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// Canjear un enlace y abrir un documento de OTRA organización (ADR 0052).
//
// Es el diseño del 0040 para las conversaciones, aplicado a un documento: el
// documento vive en la organización ANFITRIONA, con sus filas y su RLS; quien viene
// de fuera es una fila de `doc_permissions` con `externo`, y las rutas de ese
// documento pasan a la anfitriona SIN los grupos ni la organización de su token.

// canjeStore es lo que esto necesita del store, fuera del contrato documental.
type canjeStore interface {
	EnlaceParaCanjear(ctx context.Context, tokenHash string) (*store.EnlaceCanjeable, error)
	Canjear(ctx context.Context, e *store.EnlaceCanjeable, sub string, externo bool) (bool, error)
	Anfitrion(ctx context.Context, docID, sub string) (string, error)
	CanjeadosDe(ctx context.Context, docID, linkID string) ([]string, error)
	DeFuera(ctx context.Context, sub string, f domain.ListFilter) ([]domain.Document, error)
}

// redeemShare — POST /api/v1/share/redeem {"token"}, con sesión. El enlace de editor
// o de comentarista pasa a ser un permiso de quien llama; uno de lector no se canjea
// (para leer ya está `/s/{token}`, sin sesión).
//
// El token va en el CUERPO y no en la ruta: la ruta se registra en cada línea del
// log de peticiones, y este token abre un documento para editar desde fuera.
func (h *Handler) redeemShare(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok || h.canje == nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var body struct {
		Token string `json:"token"`
		// Proof: la prueba de la contraseña, si el enlace la pide (enlaces_clave.go).
		Proof string `json:"proof"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Token == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	// Canjear un enlace con contraseña sin haberla dado sería saltársela con una sesión.
	if err := h.exigirClave(r.Context(), body.Token, body.Proof); err != nil {
		fail(w, r, err)
		return
	}
	e, err := h.canje.EnlaceParaCanjear(r.Context(), domain.HashToken(body.Token))
	if err != nil {
		fail(w, r, err)
		return
	}
	if !e.Role.AtLeast(domain.RoleCommenter) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("This link is read-only."))
		return
	}
	t, err := tenancy.New(e.Tenant)
	if err != nil {
		fail(w, r, err)
		return
	}
	ctx := tenancy.Into(r.Context(), t)
	externo := subj.Tenant != e.Tenant
	if externo {
		subj = domain.Subject{Sub: subj.Sub} // ni sus grupos ni su organización
	}
	// Si ya puede lo mismo o más (su dueño, su organización, un permiso directo), no
	// se escribe nada: el enlace no añade.
	_, rol, err := h.svc.Authorize(ctx, e.DocumentID, subj, domain.RoleViewer)
	if err != nil && !errors.Is(err, domain.ErrForbidden) {
		fail(w, r, err)
		return
	}
	if err == nil && rol.AtLeast(e.Role) {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"document_id": e.DocumentID, "role": rol})
		return
	}
	escrito, err := h.canje.Canjear(ctx, e, subj.Sub, externo)
	if err != nil {
		fail(w, r, err)
		return
	}
	if !escrito {
		if rol == "" {
			// El dueño le quitó el acceso que le dio ESTE enlace (bloqueado).
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The owner removed your access through this link."))
			return
		}
		// Un permiso DIRECTO más bajo no se sustituye (revocar el enlace se lo
		// llevaría): se contesta con el que tiene de verdad.
		httpx.JSON(w, r, http.StatusOK, map[string]any{"document_id": e.DocumentID, "role": rol})
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"document_id": e.DocumentID, "role": e.Role})
}

type claveExterno struct{}

// esExterno dice si la petición opera en la organización anfitriona de un documento.
func esExterno(ctx context.Context) bool {
	v, _ := ctx.Value(claveExterno{}).(bool)
	return v
}

// enAnfitrion es el middleware de `/documents/{id}`: si el documento no está en la
// organización del token y quien llama es externo en la de su dueño, la petición
// sigue en ESA, y `subjectFrom` le quita los grupos. Si no, nada cambia: el RLS de su
// organización decide, como siempre.
//
// Que un documento es de la organización del token se recuerda unos minutos
// (`locales`): un documento no cambia de organización, y sin esto cada petición a un
// documento pagaba una lectura más. Lo que se recuerda es solo ese «sí».
func (h *Handler) enAnfitrion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := authn.FromContext(r.Context())
		id := chi.URLParam(r, "id")
		if h.canje == nil || !ok || id == "" || c.Subject == "" {
			next.ServeHTTP(w, r)
			return
		}
		if h.locales.es(c.Tenant, id) {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := h.store.GetDocument(r.Context(), id); !errors.Is(err, domain.ErrNotFound) {
			if err == nil {
				h.locales.recordar(c.Tenant, id)
			}
			next.ServeHTTP(w, r) // es de su organización (o el error lo dirá la ruta)
			return
		}
		anfitriona, err := h.canje.Anfitrion(r.Context(), id, c.Subject)
		if err != nil || anfitriona == "" || anfitriona == c.Tenant {
			next.ServeHTTP(w, r)
			return
		}
		t, err := tenancy.New(anfitriona)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(tenancy.Into(r.Context(), t), claveExterno{}, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// locales recuerda qué documentos son de qué organización (ver enAnfitrion).
//
// ponytail: un mapa con caducidad que se vacía entero al llegar al tope; si alguna vez
// se viera en el perfil, un LRU de verdad.
type locales struct {
	mu sync.Mutex
	m  map[string]time.Time
}

const (
	localesTope = 10000
	localesVida = 5 * time.Minute
)

func (l *locales) es(tenant, id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	hasta, ok := l.m[tenant+"/"+id]
	return ok && time.Now().Before(hasta)
}

func (l *locales) recordar(tenant, id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil || len(l.m) >= localesTope {
		l.m = make(map[string]time.Time)
	}
	l.m[tenant+"/"+id] = time.Now().Add(localesVida)
}
