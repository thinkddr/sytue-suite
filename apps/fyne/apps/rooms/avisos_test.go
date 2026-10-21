// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// «Ajustes de notificación» trae lo guardado —sin franja, «No molestar» apagado—, añade
// palabras en minúsculas y sin repetir (Intro o la coma), enciende la franja con sus
// horas por defecto y guarda; un fallo se queda en el diálogo con su aviso.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el `ToLower` de Anadir, «Deploy» entra
// como otra palabra junto a «deploy».
func TestLosAjustesDeNotificacion(t *testing.T) {
	s := servicioConDos()
	s.prefs = api.RoomsNotifyPrefs{Keywords: []string{"deploy"}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.AbrirAjustesAviso()
	a := sa.ajustesAviso
	if a == nil || a.Cargando || a.NoMolestar || len(a.Palabras) != 1 {
		t.Fatalf("el diálogo abre con %+v", a)
	}
	a.caja.PonerTexto("Deploy")
	a.caja.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	a.caja.PonerTexto("urgente,")
	if len(a.Palabras) != 2 || a.Palabras[1] != "urgente" || a.caja.Texto() != "" {
		t.Errorf("las palabras quedan %v y la caja %q", a.Palabras, a.caja.Texto())
	}
	a.NoMolestar = true
	a.pintar()
	if a.desde.Texto() != "22:00" || a.hast.Texto() != "07:00" {
		t.Errorf("la franja nace de %q a %q", a.desde.Texto(), a.hast.Texto())
	}
	a.hast.PonerTexto("8")
	s.errAccion = errors.New("caído")
	a.pedirGuardar()
	if !a.Fallo || a.Guardando || !s.pidio("PutNotifyPrefs(deploy+urgente,22:00,08:00)") {
		t.Errorf("con el servicio caído: fallo %v, guardando %v, %v", a.Fallo, a.Guardando, s.llamadas)
	}
	s.errAccion = nil
	a.NoMolestar = false
	a.pedirGuardar()
	if sa.prefsAviso.DNDStart != 0 || sa.prefsAviso.DNDEnd != 0 || len(sa.prefsAviso.Keywords) != 2 {
		t.Errorf("sin franja se guarda %+v", sa.prefsAviso)
	}
}
