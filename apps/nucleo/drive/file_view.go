// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Migration retains exact SQL JSON, whose label snapshot is flattened. New
// application records may use the API's nested label. Neither form may silently
// drop an external-sharing restriction or override a contradictory snapshot.
func fileView(raw []byte, id string) (api.File, error) {
	var view struct {
		api.File
		LabelID      string `json:"label_id"`
		LabelName    string `json:"label_name"`
		LabelBlocked bool   `json:"label_block_external_share"`
	}
	if json.Unmarshal(raw, &view) != nil || view.ID != id {
		return api.File{}, ErrSource
	}
	if view.LabelID != "" || view.LabelName != "" || view.LabelBlocked {
		if view.LabelID == "" || view.LabelName == "" {
			return api.File{}, ErrSource
		}
		expected := api.DriveLabel{Name: view.LabelName, BlockExternalShare: view.LabelBlocked}
		if view.Label != nil && *view.Label != expected {
			return api.File{}, ErrSource
		}
		view.Label = &expected
	}
	return view.File, nil
}
