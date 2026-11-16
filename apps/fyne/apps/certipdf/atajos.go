// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// Atajos engancha al lienzo de la ventana los atajos que la web escucha en `document`:
// los que cualquiera trae aprendidos de un visor de escritorio.
//
//   - Ctrl+F abre Buscar (y el cajón en estrecho): el buscador de un visor solo encuentra
//     lo dibujado, y aquí solo lo está lo que se ha visto.
//   - Ctrl +/= y Ctrl − cambian el zoom en 0,15; Ctrl 0 vuelve a «al ancho».
//   - Escape suelta la herramienta ANTES que cerrar el cajón: quien acaba de marcar algo
//     sin querer quiere volver a la mano, no perder la vista.
//   - Av Pág / Re Pág pasan de hoja, Inicio y Fin van a la primera y a la última.
//   - Ctrl+C copia el texto seleccionado con la mano, que en la web es el del navegador.
//   - Ctrl+P imprime. En la web es el del navegador, que imprime la página; aquí va al
//     diálogo de impresión del sistema con el PDF tal como está, que es lo que se quiere
//     imprimir de un visor.
//   - «?» abre la hoja que los enseña todos (`gruposDeAtajos`): los atajos que nadie
//     enseña no los usa nadie.
//
// Las teclas sueltas solo llegan aquí sin nada enfocado, que es el `enCampoDeTexto` de la
// web: dentro de un campo, Inicio va al principio de la línea.
func (p *Pantalla) Atajos(c fyne.Canvas) {
	p.atajos = map[fyne.KeyName]func(){}
	atajo := func(k fyne.KeyName, f func()) {
		p.atajos[k] = f
		c.AddShortcut(&desktop.CustomShortcut{KeyName: k, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) { f() })
	}
	atajo(fyne.KeyF, func() { p.abrirPanel(PanelBuscar) })
	atajo(fyne.KeyP, p.imprimir)
	for _, k := range []fyne.KeyName{fyne.KeyPlus, fyne.KeyEqual} {
		atajo(k, func() { p.zoomConTeclado(pasoZoom) })
	}
	atajo(fyne.KeyMinus, func() { p.zoomConTeclado(-pasoZoom) })
	atajo(fyne.Key0, func() {
		if p.Contenido != nil {
			p.PonerAjuste(AjusteAncho)
		}
	})
	// Ctrl+C copia lo seleccionado con la mano. Con un campo enfocado lo copia el campo:
	// Fyne le da el atajo a él antes que al lienzo.
	c.AddShortcut(&fyne.ShortcutCopy{}, func(fyne.Shortcut) { p.copiarSeleccion() })
	c.SetOnTypedKey(p.Tecla)
	c.SetOnTypedRune(func(r rune) { p.Runa(c, r) })
	ui.AlAtras(c, p.atras)
}

// Runa atiende los caracteres sueltos sin foco; de momento, solo «?». Con un diálogo o
// un menú delante el teclado es de la capa, que es el `haySuperposicionAbierta` de la web.
func (p *Pantalla) Runa(c fyne.Canvas, r rune) {
	if r != '?' || c == nil || c.Overlays().Top() != nil {
		return
	}
	ui.MostrarHojaAtajos(c, p.t("pdf.shortcuts.title"), p.t("ui.close"), p.gruposDeAtajos())
}

// filaDeAtajo es una fila de la hoja de «?» y lo que la engancha: los Ctrl+tecla que
// `Atajos` deja en `p.atajos`. Sin ninguno es de `Tecla` (las sueltas) o Ctrl+C, que va
// por `fyne.ShortcutCopy`.
type filaDeAtajo struct {
	teclas      func(t func(string) string) []string
	descripcion string
	ctrl        []fyne.KeyName
}

// conCtrl es la tecla de una fila de Ctrl+algo, ya escrita como en el sistema («⌘F»):
// quien la llama pasa por `ui.AtajoDelSistema` en la MISMA línea, que es lo que vigila
// TestLasPistasDeAtajoPasanPorAtajoDelSistema.
func conCtrl(atajo string) func(func(string) string) []string {
	return func(func(string) string) []string { return []string{atajo} }
}

// nombres son teclas con nombre propio, que van por el catálogo («AvPág», «Inicio»).
func nombres(claves ...string) func(func(string) string) []string {
	return func(t func(string) string) []string {
		out := make([]string, len(claves))
		for i, k := range claves {
			out[i] = t(k)
		}
		return out
	}
}

// gruposDelVisor es la hoja: las mismas filas, en el mismo orden, que `GRUPOS_VISOR` de
// la web (`pdf/atajos.ts`).
var gruposDelVisor = []struct {
	titulo string
	filas  []filaDeAtajo
}{
	{"pdf.shortcuts.group.move", []filaDeAtajo{
		{nombres("slides.present.key.pageDown"), "pdf.shortcuts.nextPage", nil},
		{nombres("slides.present.key.pageUp"), "pdf.shortcuts.prevPage", nil},
		{nombres("slides.present.key.home", "slides.present.key.end"), "pdf.shortcuts.ends", nil},
		{func(func(string) string) []string { return []string{"↑", "↓", "←", "→"} }, "pdf.shortcuts.scroll", nil},
		{nombres("slides.present.key.space"), "pdf.shortcuts.screen", nil},
	}},
	{"pdf.shortcuts.group.view", []filaDeAtajo{
		{conCtrl(ui.AtajoDelSistema("Ctrl+F")), "pdf.shortcuts.search", []fyne.KeyName{fyne.KeyF}},
		{conCtrl(ui.AtajoDelSistema("Ctrl++")), "pdf.zoom.in", []fyne.KeyName{fyne.KeyPlus, fyne.KeyEqual}},
		{conCtrl(ui.AtajoDelSistema("Ctrl+−")), "pdf.zoom.out", []fyne.KeyName{fyne.KeyMinus}},
		{conCtrl(ui.AtajoDelSistema("Ctrl+0")), "pdf.zoom.width", []fyne.KeyName{fyne.Key0}},
	}},
	{"pdf.shortcuts.group.actions", []filaDeAtajo{
		{conCtrl(ui.AtajoDelSistema("Ctrl+C")), "pdf.shortcuts.copy", nil},
		{conCtrl(ui.AtajoDelSistema("Ctrl+P")), "pdf.menu.print", []fyne.KeyName{fyne.KeyP}},
		{nombres("slides.present.key.esc"), "pdf.shortcuts.escape", nil},
		{func(func(string) string) []string { return []string{"?"} }, "pdf.shortcuts.help", nil},
	}},
}

// gruposDeAtajos son los grupos de la hoja, ya traducidos.
func (p *Pantalla) gruposDeAtajos() []ui.GrupoDeAtajos {
	out := make([]ui.GrupoDeAtajos, 0, len(gruposDelVisor))
	for _, g := range gruposDelVisor {
		grupo := ui.GrupoDeAtajos{Titulo: p.t(g.titulo)}
		for _, f := range g.filas {
			grupo.Atajos = append(grupo.Atajos, ui.AtajoDeTeclado{Teclas: f.teclas(p.t), Descripcion: p.t(f.descripcion)})
		}
		out = append(out, grupo)
	}
	return out
}

// copiarSeleccion deja en el portapapeles el texto seleccionado, si sigue a la vista.
func (p *Pantalla) copiarSeleccion() {
	for _, h := range p.hojas {
		if len(h.seleccion) > 0 && p.copiable != "" {
			fyne.CurrentApp().Clipboard().SetContent(p.copiable)
			return
		}
	}
}

// imprimirPDF es quien imprime; los tests lo cambian por un espía.
var imprimirPDF = ficheros.ImprimirDatos

// imprimir manda al sistema el PDF abierto, con sus cambios ya aplicados. En el móvil no
// hay cadena de impresión que pedir y no pasa nada.
//
// ponytail: en Android (PrintManager) e iOS (UIPrintInteractionController) no se imprime;
// el primero pide una clase de Java que la APK de Fyne no lleva.
//
// Lo que el documento pide que no se imprima no se imprime tampoco con Ctrl+P: el botón
// apagado con su motivo no sirve de nada si el atajo lo salta.
func (p *Pantalla) imprimir() {
	if p.doc == nil || p.Contenido == nil || !puedeImprimir(p.Contenido.Permisos) || sinImpresion() {
		return
	}
	f, _ := p.doc.paraElMotor()
	go func() {
		if err := imprimirPDF(f.Nombre, f.Datos); err != nil {
			slog.Info("certipdf: no se ha podido imprimir", "error", err)
		}
	}()
}

// puedeImprimir es el `puedeAnotar` de `Barra.tsx` con el permiso de imprimir: abierto con
// la contraseña del dueño no hay restricción ninguna.
func puedeImprimir(pm Permisos) bool { return pm.Imprimir || !pm.SoloConClaveDeDueno }

// sinImpresion dice si aquí no hay cadena de impresión (el móvil); las pruebas lo cambian.
var sinImpresion = ui.EsTactil

func (p *Pantalla) zoomConTeclado(d float64) {
	if p.Contenido != nil {
		p.CambiarZoom(d)
	}
}

// abrirPanel abre un panel lateral y, en estrecho, el cajón que lo lleva.
func (p *Pantalla) abrirPanel(a PanelActivo) {
	if p.panel == nil || p.Contenido == nil {
		return
	}
	p.panel.Abrir(a)
	p.PanelAbierto = true
	p.Recomponer()
}

// Lo que corre el documento una flecha: el `scrollbarPixelsPerLine` de Chromium.
const pasoFlecha float32 = 40

// desplazarConTeclado es lo que hace el `overflow-auto` de la web con el teclado: las
// flechas corren 40 px y Espacio (con Mayúsculas, hacia arriba) el 87,5 % de lo que se
// ve, que es la página de Chromium. Arriba y abajo se animan con el salto de página.
// ponytail: izquierda y derecha, de golpe; Chromium las anima igual.
func (p *Pantalla) desplazarConTeclado(k fyne.KeyName) {
	d := p.desplazar
	if d == nil || p.rejilla == nil {
		return
	}
	paso := pasoFlecha
	switch k {
	case fyne.KeyLeft, fyne.KeyRight:
		if k == fyne.KeyLeft {
			paso = -paso
		}
		x := min(max(d.Offset.X+paso, 0), max(p.rejilla.Size().Width-d.Size().Width, 0))
		d.ScrollToOffset(fyne.NewPos(x, d.Offset.Y))
		return
	case fyne.KeySpace:
		paso = 0.875 * d.Size().Height
		if mayusculas() {
			paso = -paso
		}
	case fyne.KeyUp:
		paso = -paso
	default:
	}
	y := min(max(d.Offset.Y+paso, 0), max(p.rejilla.Size().Height-d.Size().Height, 0))
	p.desplazarA(y)
}

// mayusculas dice si Mayúsculas está pulsada; en el móvil, nunca.
func mayusculas() bool {
	dr, ok := fyne.CurrentApp().Driver().(desktop.Driver)
	return ok && dr.CurrentKeyModifiers()&fyne.KeyModifierShift != 0
}

// Tecla atiende las teclas sueltas sin foco: Escape, pasar de hoja y los extremos.
func (p *Pantalla) Tecla(e *fyne.KeyEvent) {
	if e == nil {
		return
	}
	if e.Name == fyne.KeyEscape {
		if p.barra.Herramienta != Mano {
			p.barra.Herramienta = Mano
		} else {
			p.PanelAbierto = false
		}
		p.Recomponer()
		return
	}
	if p.Contenido == nil {
		return
	}
	switch e.Name {
	case fyne.KeyDown, fyne.KeyUp, fyne.KeySpace, fyne.KeyLeft, fyne.KeyRight:
		p.desplazarConTeclado(e.Name)
	case fyne.KeyPageDown:
		p.IrA(p.PaginaActual + 1)
	case fyne.KeyPageUp:
		p.IrA(p.PaginaActual - 1)
	case fyne.KeyHome:
		p.IrA(1)
	case fyne.KeyEnd:
		p.IrA(p.Contenido.Total)
	default:
	}
}
