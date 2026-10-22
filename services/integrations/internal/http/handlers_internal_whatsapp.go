// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func (h *Handlers) whatsAppAccount(ctx context.Context, instance, phone string) (connector.Instance, whatsAppNumber, error) {
	if !whatsAppIdentifier.MatchString(phone) {
		return connector.Instance{}, whatsAppNumber{}, httpx.ErrBadRequest.WithMessage("Choose a WhatsApp Business number.")
	}
	conn, _, err := h.oauthConnection(ctx, instance, func(key string) bool { return key == tmplWhatsAppBusiness })
	if err != nil {
		return connector.Instance{}, whatsAppNumber{}, err
	}
	numbers, err := h.whatsAppNumbers(ctx, conn)
	if err != nil {
		return connector.Instance{}, whatsAppNumber{}, err
	}
	for _, number := range numbers {
		if number.ID == phone {
			return conn, number, nil
		}
	}
	return connector.Instance{}, whatsAppNumber{}, httpx.ErrNotFound
}

// InternalWhatsAppAccount entrega credenciales solo a Rooms por la puerta interna.
// El BFF nunca accede a esta ruta; Rooms las guarda con su vault existente.
func (h *Handlers) InternalWhatsAppAccount(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctx, err := whatsAppTenant(r.Context(), q.Get("tenant"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	conn, number, err := h.whatsAppAccount(ctx, q.Get("instance"), q.Get("phone_number_id")) //nolint:contextcheck // ctx derives from r.Context()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if conn.Secrets[secretWAAppSecret] == "" {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("Connect WhatsApp Business again."))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"phone_number_id": number.ID, "display_phone": number.DisplayPhone, "verified_name": number.VerifiedName, "access_token": conn.Secrets[store.SecretKeyBearer], "app_secret": conn.Secrets[secretWAAppSecret], "registration_required": number.RegistrationRequired})
}

type whatsAppActivation struct {
	Tenant   string `json:"tenant"`
	Instance string `json:"instance"`
	Phone    string `json:"phone_number_id"`
	Callback string `json:"callback_url"`
	Verify   string `json:"verify_token"`
	PIN      string `json:"pin"`
}

// InternalWhatsAppActivate registra y suscribe antes de que Rooms active su cuenta.
func (h *Handlers) InternalWhatsAppActivate(w http.ResponseWriter, r *http.Request) {
	var req whatsAppActivation
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !validWhatsAppCallback(h.roomsPublicURL, req.Callback, req.Verify) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid WhatsApp callback."))
		return
	}
	ctx, err := whatsAppTenant(r.Context(), req.Tenant)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	conn, number, err := h.whatsAppAccount(ctx, req.Instance, req.Phone) //nolint:contextcheck // ctx derives from r.Context()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	_, base, available := h.whatsAppPlatform(ctx) //nolint:contextcheck // ctx derives from r.Context()
	if !available {
		httpx.Fail(w, r, whatsAppNotConfigured())
		return
	}
	if err := h.activateWhatsAppNumber(ctx, conn, number, base, req); err != nil { //nolint:contextcheck // ctx derives from r.Context()
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validWhatsAppCallback(origin, callback, verify string) bool {
	base, err := url.Parse(origin)
	if err != nil {
		return false
	}
	u, err := url.Parse(callback)
	if err != nil || u.Scheme != "https" || base.Scheme != "https" || u.Host != base.Host || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(callback) > 200 || len(verify) < 32 || len(verify) > 200 {
		return false
	}
	return u.Path == "/webhooks/whatsapp/"+verify && !strings.ContainsAny(verify, "/%?#\\\r\n")
}

func (h *Handlers) activateWhatsAppNumber(ctx context.Context, conn connector.Instance, number whatsAppNumber, base string, req whatsAppActivation) error {
	if number.RegistrationRequired {
		if len(req.PIN) != 6 || strings.Trim(req.PIN, "0123456789") != "" {
			return httpx.ErrBadRequest.WithMessage("Enter the six-digit two-step verification PIN for this number.")
		}
		body := map[string]string{"messaging_product": "whatsapp", "pin": req.PIN}
		if err := whatsAppMutation(ctx, h.egressRuntime(conn, log.From(ctx)), base+"/"+number.ID+"/register", body); err != nil {
			return err
		}
		// Registro completado: si falla después la suscripción, el reintento no
		// vuelve a registrar el número ni pierde el avance ya hecho en Meta.
		var registered []string
		_ = json.Unmarshal([]byte(conn.Secrets[secretWARegistered]), &registered)
		if !slices.Contains(registered, number.ID) {
			registered = append(registered, number.ID)
		}
		encoded, _ := json.Marshal(registered)
		progressCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		err := h.store.MergeSecrets(progressCtx, conn.ID, map[string]string{secretWARegistered: string(encoded)})
		cancel()
		if err != nil {
			return toAPIError(err)
		}
	}
	return configureWhatsAppWebhooks(ctx, h.egressRuntime(conn, log.From(ctx)), base, number, req.Callback, req.Verify)
}

func configureWhatsAppWebhooks(ctx context.Context, rt connector.Runtime, base string, number whatsAppNumber, callback, verify string) error {
	if err := whatsAppMutation(ctx, rt, base+"/"+number.waba+"/subscribed_apps", map[string]any{}); err != nil {
		return err
	}
	return whatsAppMutation(ctx, rt, base+"/"+number.ID, map[string]any{"webhook_configuration": map[string]string{"override_callback_uri": callback, "verify_token": verify}})
}

func whatsAppMutation(ctx context.Context, rt connector.Runtime, target string, body any) error {
	var out struct {
		Success bool `json:"success"`
	}
	if err := roomsProviderJSON(ctx, rt, http.MethodPost, target, body, &out); err != nil {
		return err
	}
	if !out.Success {
		return httpx.NewError(http.StatusBadGateway, "whatsapp_activation_rejected", "Meta did not activate this number. Try connecting it again.")
	}
	return nil
}
