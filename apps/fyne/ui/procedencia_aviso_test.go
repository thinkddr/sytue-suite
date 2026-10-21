// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La campana de Fyne marca lo reconstruido como anterior al registro de avisos, como
// la web (procedenciaAviso), y lo de otra organización con su nombre.
func TestProcedenciaAviso(t *testing.T) {
	tr := traductorPrueba(t)
	marca := tr.T("notifications.historic", nil)
	if marca == "notifications.historic" {
		t.Fatal("la marca no está en el catálogo común de Fyne (go generate ./i18n en apps/nucleo)")
	}
	for n, quiero := range map[*api.HubNotification]string{
		{}:                                       "",
		{Historic: true}:                         marca + " · ",
		{Organization: "globex", Historic: true}: "globex · " + marca + " · ",
		{Organization: "globex"}:                 "globex · ",
		{Organization: "globex", OrganizationName: "Globex Ingeniería S.L."}: "Globex Ingeniería S.L. · ",
	} {
		if got := ProcedenciaAviso(tr, *n); got != quiero {
			t.Errorf("%+v: %q, quiero %q", *n, got, quiero)
		}
	}
}
