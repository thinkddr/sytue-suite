// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

// ConnectWhatsAppConsent accepts only public identifiers and an optional Meta PIN.
func (ih *intHandlers) ConnectWhatsAppConsent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TeamID        uuid.UUID `json:"team_id"`
		InstanceID    string    `json:"instance_id"`
		PhoneNumberID string    `json:"phone_number_id"`
		PIN           string    `json:"pin"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	account, err := ih.svc.ConnectWhatsAppConsent(r.Context(), actor, req.TeamID, req.InstanceID, req.PhoneNumberID, req.PIN)
	if errors.Is(err, service.ErrRoomsIntegrationUnavailable) {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("WhatsApp connection could not be completed. Try again."))
		return
	}
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"account": account})
}
