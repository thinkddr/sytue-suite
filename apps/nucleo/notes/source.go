// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// HTTPSource is the explicit legacy export port. The private facade denies
// these routes; use the original authenticated API only during migration.
type HTTPSource struct{ records.Source }

func NewHTTPSource(client *api.Notes) (*HTTPSource, error) {
	if client == nil {
		return nil, ErrSource
	}
	source, err := records.NewAppSource(client)
	if err != nil {
		return nil, err
	}
	return &HTTPSource{Source: source}, nil
}
