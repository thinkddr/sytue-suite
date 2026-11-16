// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"errors"
	"testing"
)

func TestPrivateAppleGroupsCannotResolveToSharedWildcardOrAnotherBundle(t *testing.T) {
	const suite = "VHC5QW7BZ3.eu.sytue.suite"
	mail, err := grupoPrivadoDesdeBundle("eu.sytue.mail", suite)
	if err != nil {
		t.Fatal(err)
	}
	rooms, err := grupoPrivadoDesdeBundle("eu.sytue.rooms", suite)
	if err != nil {
		t.Fatal(err)
	}
	if mail == rooms || mail == suite || rooms == suite || mail == "" || rooms == "" {
		t.Fatal("distinct apps resolved to shared or wildcard keychain groups")
	}
	for _, bundle := range []string{"", "eu.sytue.suite", "eu..sytue.mail", "eu.sytue.*", "eu.sytue.mail/../../rooms", "eu.sytue.mail\x00", "eu.sytue.mail:scope", "eu.sytue.-mail", "eu.sytue.mail-", "eu.sytue.メール"} {
		if group, err := grupoPrivadoDesdeBundle(bundle, suite); group != "" || !errors.Is(err, ErrGrupoPrivadoApple) {
			t.Fatal("invalid identity gained a keychain group", bundle, group, err)
		}
	}
	// El equipo sale del grupo de la suite, que fija el perfil de firma: se deriva
	// igual con cualquier equipo válido, no sólo con el de esta compilación.
	const otroEquipo = "ABCDE12345.eu.sytue.suite"
	otro, err := grupoPrivadoDesdeBundle("eu.sytue.mail", otroEquipo)
	if err != nil || otro != "ABCDE12345.eu.sytue.mail" {
		t.Fatal("private group derivation depends on the build's team", otro, err)
	}
	if group, err := grupoPrivadoDesdeBundle("eu.sytue.suite", otroEquipo); group != "" || !errors.Is(err, ErrGrupoPrivadoApple) {
		t.Fatal("the suite group itself is not a private group", group, err)
	}
}
