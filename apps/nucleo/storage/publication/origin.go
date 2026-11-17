// SPDX-License-Identifier: Apache-2.0
package publication

import (
	"encoding/json"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// OriginEvent is sent only by the authenticated homeserver module. Plaintext
// events carry routing only; their body must never be copied to this API.
type OriginEvent struct {
	Version   int               `json:"v"`
	Reference storage.Reference `json:"reference"`
	Sender    string            `json:"sender"`
	Type      string            `json:"type"`
	Content   json.RawMessage   `json:"content,omitempty"`
}

func (e OriginEvent) Valid() bool {
	local, server, err := id.UserID(e.Sender).Parse()
	return e.Version == 1 && e.Reference.Valid() && err == nil && local != "" && server != "" &&
		((e.Type == "m.room.encrypted" && len(e.Content) > 0 && len(e.Content) <= 65536) || (e.Type == "m.room.message" && len(e.Content) == 0))
}

// OriginCoverageKind is required on every decision, so a mixed rollout cannot
// accept an old backend response that did not track registered resource rooms.
const OriginCoverageKind = "storage-resource-managed-v3"

type OriginDecision struct {
	Coverage string `json:"coverage"`
	Version  int    `json:"v"`
	Managed  bool   `json:"managed"`
	Allow    bool   `json:"allow"`
}
