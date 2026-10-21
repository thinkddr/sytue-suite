// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// IntegrationDestinations lists writable destinations for an organization administrator.
func (ih *intHandlers) IntegrationDestinations(w http.ResponseWriter, r *http.Request) {
	items, err := ih.svc.IntegrationDestinations(r.Context(), r.URL.Query().Get("instance"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateIntegrationMeeting creates and shares a Meet link in an administered channel.
func (ih *intHandlers) CreateIntegrationMeeting(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InstanceID string    `json:"instance_id"`
		ChannelID  uuid.UUID `json:"channel_id"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, name := caller(r)
	meetingURL, err := ih.svc.CreateIntegrationMeeting(r.Context(), req.InstanceID, req.ChannelID, actor, name)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]string{"url": meetingURL})
}
