// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// NewSource retains a separate original HTTP client solely for the immutable
// migration source. Ordinary private address book methods use WithPrivateData.
func NewSource(client *api.Contacts) (records.Source, error) {
	if client == nil {
		return nil, ErrSnapshot
	}
	return records.NewAppSource(client)
}
