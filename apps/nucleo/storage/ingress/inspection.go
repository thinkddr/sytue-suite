// SPDX-License-Identifier: Apache-2.0
package ingress

import "git.sytue.eu/sytue/one/apps/nucleo/storage"

type InspectRequest struct {
	Epoch string `json:"epoch"`
	ID    string `json:"id"`
}

type HandoffBodyRequest struct {
	Control ControlProof `json:"control"`
	Task    Task         `json:"task"`
}

func (r HandoffBodyRequest) Validate(scope storage.Scope) error {
	p := r.Control
	if p.Validate(scope, false) != nil || p.Request.Kind != "task.handoff" || p.Request.Handoff == nil || p.Request.Handoff.Phase != "begin" || r.Task.Validate(scope, p.Request.Epoch) != nil || r.Task.ID != p.Request.Handoff.TaskID || r.Task.Cipher.SHA256 != p.Request.Handoff.CipherSHA256 {
		return ErrContract
	}
	return nil
}
