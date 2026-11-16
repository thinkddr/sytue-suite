// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import card "git.sytue.eu/sytue/one/apps/nucleo/contacts/card"

const paramOmitirAno = "X-APPLE-OMIT-YEAR"

func NormalizeBirthdayNoYear(v string) (string, error) { return card.NormalizeBirthdayNoYear(v) }
