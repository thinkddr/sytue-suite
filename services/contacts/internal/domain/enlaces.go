// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import card "git.sytue.eu/sytue/one/apps/nucleo/contacts/card"

func NormalizeURL(v string) (string, error)        { return card.NormalizeURL(v) }
func NormalizeURLType(t string) string             { return card.NormalizeURLType(t) }
func NormalizeRelated(name string) (string, error) { return card.NormalizeRelated(name) }
func NormalizeRelatedType(t string) string         { return card.NormalizeRelatedType(t) }
