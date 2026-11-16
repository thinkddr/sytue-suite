// SPDX-License-Identifier: Apache-2.0
package erp

import (
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// NewSource preserves the original authenticated API solely for the immutable
// retained source. The shared adapter owns delegation and typed-nil validation.
func NewSource(client *api.ERP) (records.Source, error) {
	return records.NewAppSource(client)
}

// NewHistoricalSource uses ERP's validation while exposing only the common
// read/prepare lifecycle. Historical CRUD is never mounted.
func NewHistoricalSource(cfg Config) (*records.HistoricalSource, error) {
	configuration, err := recordsConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	return records.NewHistoricalSource(configuration)
}
