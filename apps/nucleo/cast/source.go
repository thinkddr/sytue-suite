// SPDX-License-Identifier: Apache-2.0
package cast

import (
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type HTTPSource struct{ records.Source }

func NewHTTPSource(client *api.Cast) (*HTTPSource, error) {
	if client == nil {
		return nil, ErrSource
	}
	source, err := records.NewAppSource(client)
	if err != nil {
		return nil, err
	}
	return &HTTPSource{Source: source}, nil
}
