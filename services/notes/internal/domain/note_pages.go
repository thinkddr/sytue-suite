// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "errors"

// The SDK bounds JSON responses at 32 MiB. A page must fit that existing bound.
const MaxNotePageBytes = 32 << 20

var (
	ErrNotePageTooLarge       = errors.New("notes: one note exceeds the response size limit")
	ErrNotePaginationRequired = errors.New("notes: this notebook requires paginated reading")
)
