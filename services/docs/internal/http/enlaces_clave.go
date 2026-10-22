// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/clave"
	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Contraseña opcional de los enlaces de compartir (docs/security.md#matriz-acl).
//
// Un enlace con contraseña no se abre solo con su token: la vista pública, su ficha y
// el canje exigen además la PRUEBA que da `POST /share/unlock` a quien acierta la
// contraseña. La contraseña solo viaja en esa petición (y al ponerla), nunca vuelve, y
// en la base hay solo su hash Argon2id.

// claveEnlacesStore es lo que esto necesita del store, fuera del contrato documental.
type claveEnlacesStore interface {
	ClaveDeEnlace(ctx context.Context, tokenHash string) (linkID, hash string, err error)
	EstablecerClaveEnlace(ctx context.Context, docID, linkID, passwordHash string) error
}

// cabeceraPrueba lleva la prueba en las lecturas sin sesión (`GET /share/{token}` y su
// vista); en el canje va en el cuerpo, con el token.
const cabeceraPrueba = "X-Share-Proof"

// limitesClave acota los intentos de contraseña ANTES de tocar Argon2id, por enlace y
// por IP. Por enlace es la defensa de verdad: quien martillea un enlace puede rotar de
// IP, pero no de enlace. Agotada la ráfaga, el enlace queda bloqueado para todos hasta
// que se recarga (un intento cada 30 s): es el bloqueo temporal.
//
// ponytail: en memoria, por réplica (N réplicas = N veces la ráfaga). Si hiciera falta
// un presupuesto compartido, `pg.RateLimiter` implementa la misma interfaz.
type limitesClave struct {
	porEnlace, porIP httpx.RateLimiter
	// argon acota los Argon2id simultáneos (64 MiB cada uno): una riada espera en
	// goroutines baratas y solo estos pagan la memoria.
	argon chan struct{}
}

func nuevosLimitesClave() *limitesClave {
	return &limitesClave{
		porEnlace: httpx.NewTokenBucket(1.0/30, 5),
		porIP:     httpx.NewTokenBucket(0.2, 20),
		argon:     make(chan struct{}, 8),
	}
}

// hashDeRelleno es un hash válido de una contraseña que nadie tiene. Contra él se
// verifica cuando el enlace no existe, ya no vale o no tiene contraseña, para que esa
// respuesta tarde lo mismo que una contraseña errónea de un enlace real.
var hashDeRelleno = sync.OnceValue(func() string {
	h, _ := clave.Hash("docs: enlace que no existe")
	return h
})

// exigirClave deja pasar si el enlace no pide contraseña o `prueba` es la suya.
// ErrShareExpired si el enlace no vale; ErrClaveDeEnlace si falta la prueba o no coincide.
// Sin almacén configurado no pasa nadie.
func (h *Handler) exigirClave(ctx context.Context, token, prueba string) error {
	if h.claveEnlaces == nil {
		return domain.ErrClaveDeEnlace
	}
	_, hash, err := h.claveEnlaces.ClaveDeEnlace(ctx, domain.HashToken(token))
	if err != nil {
		return err
	}
	if hash != "" && !domain.PruebaValida(prueba, hash) {
		return domain.ErrClaveDeEnlace
	}
	return nil
}

// errClaveErronea es LA respuesta de un desbloqueo fallido, sea cual sea el motivo:
// contraseña errónea, enlace inventado, caducado, revocado o sin contraseña.
var errClaveErronea = httpx.NewError(http.StatusForbidden, "share_password_wrong", "Wrong password.")

// unlockShare — POST /api/v1/share/unlock {"token","password"}, SIN sesión. Si la
// contraseña es la del enlace devuelve la prueba que abre su vista y su canje.
//
// El token va en el cuerpo y no en la ruta: la ruta se registra en el log de peticiones.
func (h *Handler) unlockShare(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Token == "" || h.claveEnlaces == nil || h.limitesClave == nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	tokenHash := domain.HashToken(body.Token)
	ip := clientip.TrustedProxiesFrom(r.Context()).ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
	for _, l := range []struct {
		lim   httpx.RateLimiter
		clave string
	}{{h.limitesClave.porEnlace, "enlace:" + tokenHash}, {h.limitesClave.porIP, "ip:" + ip}} {
		res, err := l.lim.Allow(r.Context(), l.clave)
		if err != nil || !res.Allowed {
			// El hash del token no es el token: identifica el enlace en el registro sin abrirlo.
			auditar(r, "enlace.clave.bloqueado", "enlace_hash", tokenHash[:16], "ip", ip)
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(res.RetryAfter.Seconds()))))
			httpx.Fail(w, r, httpx.ErrTooManyRequests)
			return
		}
	}
	select {
	case h.limitesClave.argon <- struct{}{}:
		defer func() { <-h.limitesClave.argon }()
	case <-time.After(2 * time.Second):
		w.Header().Set("Retry-After", "2")
		httpx.Fail(w, r, httpx.ErrTooManyRequests)
		return
	case <-r.Context().Done():
		return
	}

	linkID, hash, err := h.claveEnlaces.ClaveDeEnlace(r.Context(), tokenHash)
	conClave := err == nil && hash != ""
	if !conClave {
		hash = hashDeRelleno()
	}
	if !domain.VerificarClaveDeEnlace(body.Password, hash) || !conClave {
		auditar(r, "enlace.clave.fallo", "enlace", linkID, "enlace_hash", tokenHash[:16], "ip", ip)
		httpx.Fail(w, r, errClaveErronea)
		return
	}
	auditar(r, "enlace.clave.acierto", "enlace", linkID, "ip", ip)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"proof": domain.PruebaDeClave(hash)})
}

// setSharePassword — PUT /documents/{id}/shares/{linkId}/password {"password"}: pone,
// cambia o quita ("" = quitar) la contraseña de un enlace. Solo el propietario. Cambiarla
// invalida las pruebas ya dadas: quien tenía la vista abierta tiene que volver a entrar.
func (h *Handler) setSharePassword(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner); err != nil {
		fail(w, r, err)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if h.claveEnlaces == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	hash, err := domain.HashClaveDeEnlace(body.Password)
	if err != nil {
		fail(w, r, err)
		return
	}
	linkID := chi.URLParam(r, "linkId")
	if err := h.claveEnlaces.EstablecerClaveEnlace(r.Context(), docID(r), linkID, hash); err != nil {
		fail(w, r, err)
		return
	}
	accion := "enlace.clave.puesta"
	if hash == "" {
		accion = "enlace.clave.quitada"
	}
	auditar(r, accion, "documento", docID(r), "enlace", linkID)
	httpx.NoContent(w)
}
