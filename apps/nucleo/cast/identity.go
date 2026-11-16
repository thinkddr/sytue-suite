// SPDX-License-Identifier: Apache-2.0
package cast

import "git.sytue.eu/sytue/one/apps/nucleo/storage/records"

// NewHistoricalSource uses the same video/media graph checks as the private
// repository, without exposing historical CRUD or plaintext fallback to a view.
func NewHistoricalSource(cfg Config) (*records.HistoricalSource, error) {
	validator := &Repository{scope: cfg.Scope}
	return records.NewHistoricalSource(validator.recordsConfiguration(cfg))
}

func (r *Repository) IdentityCutover(cfg records.IdentityCutoverConfig) (*records.IdentityCutover, error) {
	if r == nil || r.engine == nil || cfg.Scope != r.scope || cfg.Isolated != nil {
		return nil, ErrSource
	}
	cfg.Isolated = r.engine
	return records.NewIdentityCutover(cfg)
}
