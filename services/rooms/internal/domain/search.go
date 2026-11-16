// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package domain

import roomsearch "git.sytue.eu/sytue/one/apps/nucleo/rooms/search"

type SearchQuery = roomsearch.Query

func ParseSearchQuery(raw string) SearchQuery { return roomsearch.Parse(raw) }
