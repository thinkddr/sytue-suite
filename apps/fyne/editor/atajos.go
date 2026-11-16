// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// modificadorDelRegistro es como el registro común de comandos escribe la tecla de
// atajos del sistema en sus claves («Ctrl+Shift+L»), también en un Mac. Es una clave
// de búsqueda, no un texto que se enseñe: lo que ve la persona pasa por
// ui.AtajoDelSistema (TestLasPistasDeAtajoPasanPorAtajoDelSistema).
const modificadorDelRegistro = "Ctrl"

// TeclasDelAtajo escribe una combinación de Fyne como las del registro común
// («Ctrl+Shift+L», «Ctrl+Alt+1», «Ctrl+]»), con «Ctrl» por la tecla de atajos del sistema
// (⌘ en un Mac, como en la web). Sin esa tecla, o con un modificador que el registro no
// conoce, devuelve "": no es un atajo de comando.
func TeclasDelAtajo(s *desktop.CustomShortcut) string {
	if s == nil {
		return ""
	}
	ctrl := fyne.KeyModifierShortcutDefault
	resto := s.Modifier &^ (ctrl | fyne.KeyModifierShift | fyne.KeyModifierAlt)
	if s.Modifier&ctrl == 0 || resto != 0 {
		return ""
	}
	alt := s.Modifier&fyne.KeyModifierAlt != 0
	// En Windows AltGr llega como Ctrl+Alt: AltGr+2 es la «@» de un teclado español, y
	// casarla con «Ctrl+Alt+2» pondría un título cada vez que se escribe una arroba. Allí
	// la web tampoco la casa (`key` ya es la «@»), así que aquí se deja pasar.
	if alt && runtime.GOOS == "windows" && ctrl == fyne.KeyModifierControl {
		return ""
	}
	teclas := modificadorDelRegistro + "+"
	if s.Modifier&fyne.KeyModifierShift != 0 {
		teclas += "Shift+"
	}
	if alt {
		teclas += "Alt+"
	}
	return teclas + string(s.KeyName)
}

// ComandoDelAtajo es el comando del registro común que dispara la combinación en el texto
// editable (el folio de Docus y la caja de Diapos, que es el mismo folio), con su valor, o
// false. Es el mismo manejador que el de la web (`comandoDelAtajo(e, 'document')`): un
// atajo nuevo se añade al registro y funciona en las dos sin tocar ningún lienzo.
func ComandoDelAtajo(s *desktop.CustomShortcut) (Comando, string, bool) {
	teclas := TeclasDelAtajo(s)
	if teclas == "" {
		return "", "", false
	}
	id, valor, ok := office.ComandoDelAtajo(teclas, office.Documento)
	return Comando(id), valor, ok
}

// TextoAtajo es `textoAtajo` de la cinta web: una combinación del registro («Ctrl+Shift+V»)
// como la lee quien tiene delante el teclado. En Linux y Windows, «Ctrl» y «Shift» por el
// catálogo («Ctrl+Mayús+V»); en el Mac, los símbolos sin «+» («⌘⇧V»).
func TextoAtajo(teclas string, t func(string) string) string {
	return TextoAtajoPara(runtime.GOOS, teclas, t)
}

// TextoAtajoPara es TextoAtajo con el sistema como argumento, para probarlo en Linux.
func TextoAtajoPara(goos, teclas string, t func(string) string) string {
	if goos == "darwin" || goos == "ios" {
		return strings.NewReplacer("Ctrl+Y", "⌘⇧Z", "Ctrl+", "⌘", "Ctrl ", "⌘", "Mayús+", "⇧", "Shift+", "⇧", "Alt+", "⌥").
			Replace(teclas)
	}
	if strings.HasPrefix(teclas, "Ctrl+") {
		teclas = t("shortcut.ctrl") + "+" + strings.TrimPrefix(teclas, "Ctrl+")
	}
	return strings.Replace(teclas, "Shift+", t("shortcut.shift")+"+", 1)
}
