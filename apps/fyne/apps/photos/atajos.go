// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # El teclado de Fotos en UNA tabla (`lib/atajos.ts`)
//
// De ella salen lo que hace cada tecla (`teclaSuelta`, `runaSuelta`, `teclaVisor`,
// `runaVisor`) y la hoja de «?» (`ui.MostrarHojaAtajos`). Así la hoja no puede prometer
// una tecla que no hace nada, ni una tecla quedarse sin explicar: los atajos del visor no
// se contaban en ningún sitio.

// accionAtajo es lo que hace una tecla: las mismas acciones que `AccionDeGaleria` y
// `AccionDelVisor` en la web.
type accionAtajo int

const (
	atajoNinguno accionAtajo = iota
	// La galería (la línea de tiempo y Favoritas).
	atajoSubir
	atajoSalirDeSeleccion
	// El visor.
	atajoAnterior
	atajoSiguiente
	atajoPrimera
	atajoUltima
	atajoPase
	atajoFicha
	atajoTamanyoReal
	atajoPapelera
	atajoCerrar
	// Las dos.
	atajoAyuda
)

// filaAtajo es una fila de la tabla. Las teclas son un `fyne.KeyName` («Left», «Home»)
// o, para lo que llega como runa, el carácter en minúscula («u», «?», « »): los nombres
// de tecla de Fyne de las letras van en mayúscula («I»), así que no se cruzan.
type filaAtajo struct {
	teclas []string
	accion accionAtajo
	clave  string // la descripción, en el catálogo
}

// atajosGaleria y atajosVisor son la tabla, en el orden de la hoja: las mismas filas que
// `GALERIA` y `VISOR` de la web.
var (
	atajosGaleria = []filaAtajo{
		// El atajo que pulsa el menú de la app de escritorio.
		{[]string{"u"}, atajoSubir, "gallery.add"},
		{[]string{string(fyne.KeyEscape)}, atajoSalirDeSeleccion, "gallery.cancelSelection"},
		{[]string{"?"}, atajoAyuda, "shortcuts.help"},
	}
	atajosVisor = []filaAtajo{
		{[]string{string(fyne.KeyLeft)}, atajoAnterior, "viewer.prev"},
		{[]string{string(fyne.KeyRight)}, atajoSiguiente, "viewer.next"},
		{[]string{string(fyne.KeyHome)}, atajoPrimera, "shortcuts.first"},
		{[]string{string(fyne.KeyEnd)}, atajoUltima, "shortcuts.last"},
		// El Espacio llega como runa, no como tecla.
		{[]string{" "}, atajoPase, "shortcuts.slideshow"},
		{[]string{"i"}, atajoFicha, "shortcuts.info"},
		{[]string{"z"}, atajoTamanyoReal, "shortcuts.zoom"},
		{[]string{string(fyne.KeyDelete), string(fyne.KeyBackspace)}, atajoPapelera, "viewer.delete"},
		{[]string{string(fyne.KeyEscape)}, atajoCerrar, "viewer.close"},
		{[]string{"?"}, atajoAyuda, "shortcuts.help"},
	}
)

// accionDe busca una tecla (un `fyne.KeyName` o el de `teclaDeRuna`) en una tabla.
func accionDe(tabla []filaAtajo, tecla string) accionAtajo {
	for _, f := range tabla {
		for _, k := range f.teclas {
			if k == tecla {
				return f.accion
			}
		}
	}
	return atajoNinguno
}

// teclaDeRuna es cómo se busca una runa en la tabla: en minúscula, porque con el bloqueo
// de mayúsculas puesto «i» llega como «I».
func teclaDeRuna(r rune) string { return string(unicode.ToLower(r)) }

// rotuloTecla es cómo se ESCRIBE una tecla en la hoja: símbolos, o la palabra en el
// idioma de quien lee (`rotuloDeTecla` en la web).
func rotuloTecla(v Vista, tecla string) string {
	switch fyne.KeyName(tecla) { //nolint:exhaustive // las demás teclas se escriben tal cual, abajo
	case fyne.KeyLeft:
		return "←"
	case fyne.KeyRight:
		return "→"
	case fyne.KeyBackspace:
		return "⌫"
	case fyne.KeyHome:
		return v.Traduce("shortcuts.keyHome")
	case fyne.KeyEnd:
		return v.Traduce("shortcuts.keyEnd")
	case fyne.KeyDelete:
		return v.Traduce("shortcuts.keyDelete")
	case fyne.KeyEscape:
		return v.Traduce("shortcuts.keyEsc")
	}
	if tecla == " " {
		return v.Traduce("shortcuts.keySpace")
	}
	r, _ := utf8.DecodeRuneInString(tecla)
	return string(unicode.ToUpper(r))
}

// grupoAtajos es un apartado de la hoja, ya traducido.
func grupoAtajos(v Vista, titulo string, filas []filaAtajo) ui.GrupoDeAtajos {
	g := ui.GrupoDeAtajos{Titulo: v.Traduce(titulo)}
	for _, f := range filas {
		teclas := make([]string, len(f.teclas))
		for i, k := range f.teclas {
			teclas[i] = rotuloTecla(v, k)
		}
		g.Atajos = append(g.Atajos, ui.AtajoDeTeclado{Teclas: teclas, Descripcion: v.Traduce(f.clave)})
	}
	return g
}

// gruposDeAtajos es la hoja (`gruposDeAtajos` en la web). En la galería lleva los dos
// grupos —el del visor es lo que se puede hacer al abrir una foto—; en el visor, solo el
// suyo y sin las filas que ESE visor no atiende: sin lista no hay saltos, y no todos tiran
// a la papelera.
func gruposDeAtajos(v Vista, enVisor, saltos, papelera bool) []ui.GrupoDeAtajos {
	visor := make([]filaAtajo, 0, len(atajosVisor))
	for _, f := range atajosVisor {
		if (!saltos && (f.accion == atajoPrimera || f.accion == atajoUltima)) || (!papelera && f.accion == atajoPapelera) {
			continue
		}
		visor = append(visor, f)
	}
	delVisor := grupoAtajos(v, "shortcuts.group.viewer", visor)
	if enVisor {
		return []ui.GrupoDeAtajos{delVisor}
	}
	return []ui.GrupoDeAtajos{grupoAtajos(v, "shortcuts.group.gallery", atajosGaleria), delVisor}
}

// verAtajos abre la hoja de «?» sobre lo que haya: la del visor si está abierto, y si no
// la de la galería.
func (a *App) verAtajos() {
	if a.w == nil {
		return
	}
	enVisor, saltos, papelera := false, true, true
	if e := a.vis; e != nil {
		_, conPosicion := posicionVisor(e.i, len(e.lista()), e.origen == desdeGaleria && !a.g.fin)
		_, delServidor := e.fotoDelServidor()
		enVisor, saltos, papelera = true, conPosicion, e.origen == desdeGaleria && delServidor
	}
	ui.MostrarHojaAtajos(a.w.Canvas(), a.v.Traduce("shortcuts.title"), a.v.Traduce("ui.close"),
		gruposDeAtajos(a.v, enVisor, saltos, papelera))
}
