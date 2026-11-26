// SPDX-License-Identifier: Apache-2.0
package notes

import "git.sytue.eu/sytue/one/apps/nucleo/storage/records"

// NewHistoricalSource has the same domain guards as New, but exposes no CRUD
// or view of the historical identity to an application presenter.
func NewHistoricalSource(cfg Config) (*records.HistoricalSource, error) {
	configuration, err := recordsConfiguration(cfg, false)
	if err != nil {
		return nil, err
	}
	return records.NewHistoricalSource(configuration)
}

// IdentityCutover binds the common transfer to this exact private repository.
func (r *Repository) IdentityCutover(cfg records.IdentityCutoverConfig) (*records.IdentityCutover, error) {
	if r == nil || r.engine == nil || cfg.Scope != r.cfg.Scope || cfg.Isolated != nil {
		return nil, ErrSource
	}
	cfg.Isolated = r.engine
	return records.NewIdentityCutover(cfg)
}
