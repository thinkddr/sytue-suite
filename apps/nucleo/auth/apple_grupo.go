// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"errors"
	"regexp"
	"strings"
)

var ErrGrupoPrivadoApple = errors.New("auth: private Apple keychain group is unavailable")

// Ambiguous legacy material must be claimed and verified by its owner before
// a reader can use it or a writer can create a replacement over an existing DB.
var ErrSecretoLegadoCompartido = errors.New("auth: legacy private material requires scoped migration")

// The native bundle identifier comes from CoreFoundation, never application
// preferences, an OAuth callback or a caller-supplied storage consumer.
var (
	appleTeamPattern   = regexp.MustCompile(`^[A-Z0-9]{10}$`)
	appleBundlePattern = regexp.MustCompile(`^[A-Za-z0-9]+(?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9]+(?:[A-Za-z0-9-]*[A-Za-z0-9])?){2,}$`)
)

func grupoPrivadoDesdeBundle(bundle, suite string) (string, error) {
	team, _, ok := strings.Cut(suite, ".")
	if !ok || !appleTeamPattern.MatchString(team) || len(bundle) > 255 || !appleBundlePattern.MatchString(bundle) {
		return "", ErrGrupoPrivadoApple
	}
	own := team + "." + bundle
	if own == suite {
		return "", ErrGrupoPrivadoApple
	}
	return own, nil
}
