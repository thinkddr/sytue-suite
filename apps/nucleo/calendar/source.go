// SPDX-License-Identifier: Apache-2.0
// Package calendar uses the shared records engine for private Calendar data.
package calendar

import (
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// NewSource preserves the original authenticated API solely for the immutable
// retained source. The shared adapter owns delegation and typed-nil validation.
func NewSource(client *api.Calendars) (records.Source, error) { return records.NewAppSource(client) }
