// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La burbuja de formato (la de RichText.tsx): encima de lo seleccionado, con lo que se
// hace con una selección —negrita, cursiva, «Más formato» (subrayado, tachado, código,
// resaltado, quitar formato) y enlace— y, en su propio grupo, copiar, cortar y pegar. La
// barra de arriba sigue ahí; la burbuja es para no tener que subir a buscarla. Como el
// menú de «/», es un `flotante` y no una capa: sus botones no se llevan el foco.

func (e *EditorNota) armarBurbuja() *flotante {
	t := e.tr.T
	atajo := func(rotulo, teclas string) string {
		return t("editor.shortcut", i18n.Vars{"label": rotulo, "keys": teclas})
	}
	boton := func(glifo string, icono iconos.Icono, rotulo, pista string, f func()) fyne.CanvasObject {
		return e.botonDeOrden(glifo, icono, rotulo, f).conPista(pista)
	}
	enlace := nuevoBotonBarra(EnlaceD, iconos.Ninguno, t("editor.linkTool", nil), nil)
	enlace.AlPulsar = func() { llamar1(e.AlEnlazar, fyne.CanvasObject(enlace)) }
	// Como la barra flotante de un editor de páginas: negrita, cursiva, el resto del
	// formato en «Más formato» y el enlace; así cabe en un teléfono a 44 por botón.
	mas := e.botonDeOrden(FormatoD, iconos.Ninguno, t("editor.moreFormatting", nil), nil)
	mas.AlPulsar = func() { e.menuFormato(mas) }
	formato := make([]fyne.CanvasObject, 0, 8)
	formato = append(formato,
		boton(NegritaD, iconos.Ninguno, t("editor.bold", nil), atajo(t("editor.bold", nil), ui.AtajoDelSistema("Ctrl+B")), func() { e.Orden("b", "") }),
		boton(CursivaD, iconos.Ninguno, t("editor.italic", nil), atajo(t("editor.italic", nil), ui.AtajoDelSistema("Ctrl+I")), func() { e.Orden("i", "") }),
		mas.conPista(t("editor.moreFormatting", nil)),
		enlace.conPista(atajo(t("editor.linkTool", nil), ui.AtajoDelSistema("Ctrl+K"))),
	)
	// Copiar, cortar y pegar, en su grupo: con el dedo no hay Ctrl+C, y el menú del
	// sistema de un `contenteditable` no existe aquí. Van por el MISMO camino que sus
	// atajos (`zonaEdicion.TypedShortcut`), así que pegar pasa por el Markdown y por el
	// `Atajo` de quien aloja el editor, como Ctrl/⌘+V.
	portapapeles := func() fyne.Clipboard { return fyne.CurrentApp().Clipboard() }
	edicion := []fyne.CanvasObject{
		boton(CopiarD, iconos.Ninguno, t("editor.copy", nil), atajo(t("editor.copy", nil), ui.AtajoDelSistema("Ctrl+C")),
			func() { e.zona.TypedShortcut(&fyne.ShortcutCopy{Clipboard: portapapeles()}) }),
		boton(TijerasD, iconos.Ninguno, t("editor.cut", nil), atajo(t("editor.cut", nil), ui.AtajoDelSistema("Ctrl+X")),
			func() { e.zona.TypedShortcut(&fyne.ShortcutCut{Clipboard: portapapeles()}) }),
		boton(PegarD, iconos.Ninguno, t("editor.paste", nil), atajo(t("editor.paste", nil), ui.AtajoDelSistema("Ctrl+V")),
			func() { e.zona.TypedShortcut(&fyne.ShortcutPaste{Clipboard: portapapeles()}) }),
	}
	fila := append(append(formato, nuevoSeparadorEd()), edicion...)
	f := nuevoFlotante(container.New(ui.Fila{Hueco: HuecoBarraEd}, fila...), tema.Espacio(1))
	f.rotulo = t("editor.selectionTools", nil)
	return f
}

// conBurbuja dice si la burbuja se ve: con algo seleccionado en el cuerpo enfocado, y sin
// el menú de «/» encima.
func (e *EditorNota) conBurbuja() bool {
	return e.haySeleccion() && e.zona.enfocado && e.menu == nil
}

// colocarFlotantes pone el menú de «/» bajo su barra y la burbuja encima de lo
// seleccionado (o debajo, si arriba no cabe), en el sistema del editor. `alto` es el
// techo de abajo (el del portal); cero, sin techo.
func (e *EditorNota) colocarFlotantes(zona fyne.Position, ancho, alto float32) {
	m := e.zona.maquetado()
	en := func(p Pos) (x, arriba, abajo float32, ok bool) {
		i := e.lineaDe(m, p)
		if i < 0 {
			return 0, 0, 0, false
		}
		l := m.Lineas[i]
		return zona.X + PXZonaEd + l.XDe(p.O), zona.Y + PYZonaEd + l.Y, zona.Y + PYZonaEd + l.Y + l.Alto, true
	}
	if e.menu != nil && e.menu.panel != nil {
		if x, arriba, abajo, ok := en(e.menu.inicio); ok {
			// Debajo de la barra; encima si debajo no cabe, como Popover.tsx.
			y, h := abajo+4, e.menu.panel.MinSize().Height
			if alto > 0 && y+h > alto && arriba-h-4 >= 0 {
				y = arriba - h - 4
			}
			colocarFlotante(e.menu.panel, x, y, ancho)
		}
	}
	if e.conBurbuja() {
		a, b := e.seleccion()
		x, arriba, _, ok := en(a)
		if !ok {
			return
		}
		h := e.burbuja.MinSize().Height
		y := arriba - h - 6
		if y < 0 {
			if _, _, abajo, ok := en(b); ok {
				y = abajo + 6
			}
		}
		colocarFlotante(e.burbuja, x-e.burbuja.MinSize().Width/2, y, ancho)
	}
}

// flotantes son los paneles que se ven ahora, para los `Objects()` del editor: van los
// últimos, encima de todo lo demás.
func (e *EditorNota) flotantes() []fyne.CanvasObject {
	var out []fyne.CanvasObject
	if e.menu != nil && e.menu.panel != nil {
		out = append(out, e.menu.panel)
	}
	if e.conBurbuja() {
		out = append(out, e.burbuja)
	}
	return out
}
