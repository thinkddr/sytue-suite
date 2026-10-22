// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// Superficie de OPERADOR DE PLATAFORMA. Vive FUERA de /api/v1 y, por tanto, fuera
// del middleware de JWT: no se autentica con una sesión de la suite sino con la
// credencial del despliegue (SYTUE_INTEGRATIONS_OPERATOR_TOKEN), que ningún
// usuario —tampoco un owner— tiene.
//
// Es deliberado y es la mitad que falta del diseño del canje: en Sytue ID,
// `may_impersonate` no lo activa ninguna API (mig 0025) y el client delegable de
// cada org lo aprovisiona `id delegated-client <org>` con acceso a la base de
// datos. Aquí se INSTALA ese secreto, sellado con la KEK. Si un admin de
// organización pudiera hacerlo desde la Console, el robo de una sola sesión de
// admin daría impersonación silenciosa de toda la organización — justo lo que este
// diseño existe para impedir.
//
// Sin token de operador configurado, la ruta ni siquiera se monta: no hay endpoint
// que sondear ni contra el que probar credenciales.

// delegatedClientReq instala (o rota) las credenciales del client delegable de una
// organización. El secreto es el que imprimió `id delegated-client <org>`.
type delegatedClientReq struct {
	Org          string `json:"org"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// PutDelegatedClient — PUT /operator/delegated-client — guarda las credenciales
// con las que la pasarela canjeará tokens acotados a cada usuario de esa org.
//
// El tenant se toma del CUERPO (no hay JWT del que derivarlo) y se fija en el
// contexto, así que la escritura sigue pasando por RLS con esa organización: una
// fila de otra org es imposible de escribir aunque el operador se equivoque de
// nombre en la consulta.
func (h *Handlers) PutDelegatedClient(w http.ResponseWriter, r *http.Request) {
	var req delegatedClientReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	org := strings.TrimSpace(req.Org)
	clientID := strings.TrimSpace(req.ClientID)
	secret := strings.TrimSpace(req.ClientSecret)
	if org == "" || clientID == "" || secret == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"Missing details: org, client_id and client_secret are required.",
		))
		return
	}

	ctx := tenancy.Into(r.Context(), tenancy.Tenant{Slug: org})
	if err := h.store.SaveDelegatedClient(ctx, store.Delegate{ClientID: clientID, Secret: secret}); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// El secreto NUNCA vuelve, ni siquiera al operador que lo acaba de instalar.
	httpx.JSON(w, r, http.StatusOK, map[string]any{"org": org, "client_id": clientID, "status": "saved"})
}

// requireOperator autentica al operador de plataforma por Bearer, en tiempo
// CONSTANTE sobre el hash de ambos lados (así ni la longitud del token filtra).
func requireOperator(token string) func(http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const prefix = "Bearer "
			raw := r.Header.Get("Authorization")
			if len(raw) <= len(prefix) || !strings.EqualFold(raw[:len(prefix)], prefix) {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			got := sha256.Sum256([]byte(strings.TrimSpace(raw[len(prefix):])))
			if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
