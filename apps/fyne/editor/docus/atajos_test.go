// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

// atajoEn pulsa la combinación sobre el folio con todo el texto elegido, como quien
// selecciona y luego pulsa el atajo, y devuelve el HTML que queda.
func atajoEn(t *testing.T, html string, tecla fyne.KeyName, mods fyne.KeyModifier) string {
	t.Helper()
	f := folioCon(t, html)
	f.pag.TypedShortcut(&fyne.ShortcutSelectAll{})
	f.pag.TypedShortcut(&desktop.CustomShortcut{KeyName: tecla, Modifier: fyne.KeyModifierShortcutDefault | mods})
	return f.Serializar()
}

// TestLosAtajosDeGoogleDocsSalenDelRegistro: cada atajo de formato del registro común
// hace en el folio lo que hace su botón. VISTO EN ROJO con el `switch` a mano de antes,
// que solo sabía negrita, cursiva, subrayado y tachado.
func TestLosAtajosDeGoogleDocsSalenDelRegistro(t *testing.T) {
	sh, alt := fyne.KeyModifierShift, fyne.KeyModifierAlt
	casos := []struct {
		nombre string
		tecla  fyne.KeyName
		mods   fyne.KeyModifier
		html   string
		quiere string
	}{
		{"negrita", fyne.KeyB, 0, "<p>Hola</p>", "<b>Hola</b>"},
		{"centrar", fyne.KeyE, sh, "<p>Hola</p>", "text-align: center"},
		{"a la derecha", fyne.KeyR, sh, "<p>Hola</p>", "text-align: right"},
		{"justificar", fyne.KeyJ, sh, "<p>Hola</p>", "text-align: justify"},
		{"numerada", fyne.Key7, sh, "<p>Hola</p>", "<ol>"},
		{"viñetas", fyne.Key8, sh, "<p>Hola</p>", "<ul>"},
		{"título 1", fyne.Key1, alt, "<p>Hola</p>", "<h1>Hola</h1>"},
		{"título 3", fyne.Key3, alt, "<p>Hola</p>", "<h3>Hola</h3>"},
		{"texto normal", fyne.Key0, alt, "<h2>Hola</h2>", "<p>Hola</p>"},
		{"superíndice", fyne.KeyPeriod, 0, "<p>Hola</p>", "<sup>Hola</sup>"},
		{"subíndice", fyne.KeyComma, 0, "<p>Hola</p>", "<sub>Hola</sub>"},
		{"quitar formato", fyne.KeyBackslash, 0, "<p><b>Hola</b></p>", "<p>Hola</p>"},
	}
	for _, c := range casos {
		if got := atajoEn(t, c.html, c.tecla, c.mods); !strings.Contains(got, c.quiere) {
			t.Errorf("%s: %s no lleva %q", c.nombre, got, c.quiere)
		}
	}
	// La sangría: Ctrl+] la pone y Ctrl+[ la quita.
	f := folioCon(t, "<p>Hola</p>")
	f.pag.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyRightBracket, Modifier: fyne.KeyModifierShortcutDefault})
	con := f.Serializar()
	f.pag.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyLeftBracket, Modifier: fyne.KeyModifierShortcutDefault})
	if con == "<p>Hola</p>" || f.Serializar() != "<p>Hola</p>" {
		t.Errorf("sangría: con Ctrl+] %s y con Ctrl+[ %s", con, f.Serializar())
	}
}

// TestCtrlKAbreElPanelDelEnlace: el enlace no se manda vacío, abre su panel; y con lo
// elegido dentro, lo enlaza. En solo lectura no abre nada.
func TestCtrlKAbreElPanelDelEnlace(t *testing.T) {
	f := folioCon(t, "<p>Hola</p>")
	w := test.NewWindow(f)
	t.Cleanup(w.Close)
	f.pag.TypedShortcut(&fyne.ShortcutSelectAll{})
	f.pag.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyK, Modifier: fyne.KeyModifierShortcutDefault})
	if f.enlaceAbierto == nil || !f.enlaceAbierto.Abierto() {
		t.Fatal("Ctrl+K no abre el panel del enlace")
	}
	if f.Serializar() != "<p>Hola</p>" {
		t.Errorf("abrir el panel no cambia el documento: %s", f.Serializar())
	}
}
