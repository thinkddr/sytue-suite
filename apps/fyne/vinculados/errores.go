// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vinculados

import (
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func (p *Panel) fallar(err error, escritura bool) {
	key := "error.connection"
	if escritura {
		p.escrituraIncierta = true
		key = "linkedServices.writeUnknown"
	}
	var remote *api.Error
	if errors.As(err, &remote) {
		switch remote.Code() {
		case "external_resource_changed":
			p.escrituraIncierta = true
			key = "linkedServices.changed"
		case "external_reconnect_required":
			p.reconectarNecesario = true
			key = "linkedServices.notGranted"
		}
	}
	p.fallo = p.t(key)
}
