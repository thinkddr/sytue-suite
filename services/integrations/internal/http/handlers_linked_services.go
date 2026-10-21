// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
	"git.sytue.eu/sytue/one/services/integrations/internal/linkedservices"
	"git.sytue.eu/sytue/one/services/integrations/internal/mailbox"
)

const (
	mailServicesKey = "_mail_services"
	mailScopesKey   = "_mail_service_scopes"
)

func mailConsentReturn(section string) bool {
	switch section {
	case "", "mail", "native-mail", "native-calendar", "native-contacts", "native-notes", "external-calendar", "external-contacts", "external-notes":
		return true
	default:
		return false
	}
}

func mailConsentService(section string) string {
	for _, service := range []string{linkedservices.Calendar, linkedservices.Contacts, linkedservices.Notes} {
		if section == "native-"+service || section == "external-"+service {
			return service
		}
	}
	return linkedservices.Mail
}

func connectedMailServices(cfg map[string]string) []string {
	if _, present := cfg[mailServicesKey]; !present {
		return []string{linkedservices.Mail}
	}
	services := []string{}
	_ = json.Unmarshal([]byte(cfg[mailServicesKey]), &services)
	return services
}

func parseMailGrant(raw json.RawMessage) ([]string, error) {
	scopes, err := consent.ParseScopes(raw)
	if err != nil {
		return nil, mailbox.ErrAuth
	}
	return scopes, nil
}

func (h *Handlers) linkedServicesRoutes(r chi.Router) {
	r.Get("/linked-services/{instance}/{service}/collections", h.LinkedCollections)
	r.Get("/linked-services/{instance}/calendar/events", h.LinkedEvents)
	r.Get("/linked-services/{instance}/calendar/event", h.LinkedEvent)
	r.Post("/linked-services/{instance}/calendar/save", h.LinkedSaveEvent)
	r.Get("/linked-services/{instance}/contacts/items", h.LinkedContacts)
	r.Get("/linked-services/{instance}/contacts/item", h.LinkedContact)
	r.Post("/linked-services/{instance}/contacts/save", h.LinkedSaveContact)
	r.Get("/linked-services/{instance}/notes/items", h.LinkedNotes)
	r.Get("/linked-services/{instance}/notes/item", h.LinkedNote)
	r.Post("/linked-services/{instance}/notes/write", h.LinkedWriteNote)
}

// A refresh may narrow a grant, never activate a previously unselected service.
// Its patch participates in the existing credential CAS, so a reconnect cannot
// be overwritten by an older token refresh.
func narrowedMailServices(provider string, cfg map[string]string, raw json.RawMessage) (map[string]string, error) {
	patch := map[string]string{}
	if len(raw) == 0 {
		return patch, nil
	}
	granted, err := parseMailGrant(raw)
	if err != nil {
		return nil, err
	}
	var requested []string
	if cfg[mailScopesKey] == "" {
		return patch, nil
	}
	if err := json.Unmarshal([]byte(cfg[mailScopesKey]), &requested); err != nil {
		return nil, mailbox.ErrAuth
	}
	services := linkedservices.Granted(provider, requested, granted)
	services = slices.DeleteFunc(services, func(service string) bool { return !slices.Contains(connectedMailServices(cfg), service) })
	encoded, _ := json.Marshal(services)
	patch[mailServicesKey] = string(encoded)
	return patch, nil
}

func applyMailServicesGrant(key string, cfg map[string]string, raw json.RawMessage, sealedScopes, legacyScopes []string, origin ...string) error {
	granted, err := parseMailGrant(raw)
	if err != nil {
		return err
	}
	if len(sealedScopes) == 0 {
		sealedScopes = legacyScopes
	}
	services := linkedservices.Granted(externalMailProvider(key), sealedScopes, granted)
	required := linkedservices.Mail
	if len(origin) > 0 {
		required = origin[0]
	}
	if !slices.Contains(services, required) {
		return mailbox.ErrAuth
	}
	encoded, _ := json.Marshal(services)
	cfg[mailServicesKey] = string(encoded)
	encoded, _ = json.Marshal(sealedScopes)
	cfg[mailScopesKey] = string(encoded)
	return nil
}

func (h *Handlers) linkedClient(r *http.Request, service string, expected ...string) (*linkedservices.Client, error) {
	connected, err := h.mailAccount(r, expected...)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(connectedMailServices(connected.conn.Config), service) {
		return nil, httpx.ErrForbidden.WithCode("linked_service_not_granted").WithMessage("Connect the account again and select this service.")
	}
	return linkedservices.NewClient(connected.account.Provider, h.egressRuntime(connected.conn, log.From(r.Context())).HTTP)
}

func failLinkedService(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, linkedservices.ErrInvalid):
		httpx.Fail(w, r, httpx.ErrBadRequest)
	case errors.Is(err, linkedservices.ErrAuth):
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("external_auth_failed").WithMessage("Connect this account again and check its permissions."))
	case errors.Is(err, linkedservices.ErrNotFound):
		httpx.Fail(w, r, httpx.ErrNotFound)
	case errors.Is(err, linkedservices.ErrConflict):
		httpx.Fail(w, r, httpx.ErrConflict.WithCode("external_resource_changed").WithMessage("This item changed at the provider. Refresh it before saving."))
	case errors.Is(err, linkedservices.ErrReadOnly):
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("external_read_only").WithMessage("This item must be edited in the provider's app."))
	case errors.Is(err, linkedservices.ErrWriteUnknown):
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "external_write_unknown", "The provider did not confirm the change. Refresh and check before trying again."))
	case errors.Is(err, linkedservices.ErrUnavailable):
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("external_unavailable"))
	default:
		failExternalMail(w, r, err)
	}
}

func (h *Handlers) LinkedCollections(w http.ResponseWriter, r *http.Request) {
	service := chi.URLParam(r, "service")
	if service != linkedservices.Calendar && service != linkedservices.Contacts && service != linkedservices.Notes {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	client, err := h.linkedClient(r, service)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	result, err := client.Collections(r.Context(), service)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) LinkedEvents(w http.ResponseWriter, r *http.Request) {
	client, err := h.linkedClient(r, linkedservices.Calendar)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	q := r.URL.Query()
	from, _ := time.Parse(time.RFC3339, q.Get("from"))
	to, _ := time.Parse(time.RFC3339, q.Get("to"))
	result, err := client.Events(r.Context(), q.Get("collection"), from, to, q.Get("cursor"))
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) LinkedEvent(w http.ResponseWriter, r *http.Request) {
	client, err := h.linkedClient(r, linkedservices.Calendar)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	q := r.URL.Query()
	result, err := client.Event(r.Context(), q.Get("collection"), q.Get("id"))
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) LinkedSaveEvent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedAddress string                    `json:"expected_address"`
		Collection      string                    `json:"collection"`
		ID              string                    `json:"id"`
		ETag            string                    `json:"etag"`
		Patch           linkedservices.EventPatch `json:"patch"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.ExpectedAddress == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	client, err := h.linkedClient(r, linkedservices.Calendar, req.ExpectedAddress)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	result, err := client.SaveEvent(r.Context(), req.Collection, req.ID, req.ETag, req.Patch)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) LinkedContacts(w http.ResponseWriter, r *http.Request) {
	client, err := h.linkedClient(r, linkedservices.Contacts)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	q := r.URL.Query()
	result, err := client.Contacts(r.Context(), q.Get("collection"), q.Get("cursor"))
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) LinkedContact(w http.ResponseWriter, r *http.Request) {
	client, err := h.linkedClient(r, linkedservices.Contacts)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	q := r.URL.Query()
	result, err := client.Contact(r.Context(), q.Get("collection"), q.Get("id"))
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) LinkedSaveContact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedAddress string                      `json:"expected_address"`
		Collection      string                      `json:"collection"`
		ID              string                      `json:"id"`
		ETag            string                      `json:"etag"`
		Patch           linkedservices.ContactPatch `json:"patch"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.ExpectedAddress == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	client, err := h.linkedClient(r, linkedservices.Contacts, req.ExpectedAddress)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	result, err := client.SaveContact(r.Context(), req.Collection, req.ID, req.ETag, req.Patch)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) LinkedNotes(w http.ResponseWriter, r *http.Request) {
	client, err := h.linkedClient(r, linkedservices.Notes)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	q := r.URL.Query()
	result, err := client.Notes(r.Context(), q.Get("collection"), q.Get("cursor"))
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) LinkedNote(w http.ResponseWriter, r *http.Request) {
	client, err := h.linkedClient(r, linkedservices.Notes)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	q := r.URL.Query()
	result, err := client.Note(r.Context(), q.Get("collection"), q.Get("id"))
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) LinkedWriteNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedAddress string `json:"expected_address"`
		Collection      string `json:"collection"`
		ID              string `json:"id"`
		Title           string `json:"title"`
		Text            string `json:"text"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.ExpectedAddress == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	client, err := h.linkedClient(r, linkedservices.Notes, req.ExpectedAddress)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	if req.ID != "" {
		if err = client.AppendNote(r.Context(), req.Collection, req.ID, req.Text); err != nil {
			failLinkedService(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	result, err := client.CreateNote(r.Context(), req.Collection, req.Title, req.Text)
	if err != nil {
		failLinkedService(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, result)
}
