// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # El teclado de Drive en UNA tabla (`lib/atajos.ts`)
//
// De ella salen lo que hace cada tecla (`Atajo`, `AtajoTecla`, `AtajoMarcarTodo`) y la
// hoja de «?» (`ui.MostrarHojaAtajos`). Así la hoja no puede prometer una tecla que no
// hace nada, ni una tecla quedarse sin explicar. Son los de cualquier explorador de
// archivos —Supr, F2, Ctrl+A, Esc, que es lo que trae la mano de Finder y del Explorador
// de Windows— más los de las apps de la suite (/, ?). Antes solo existían «u» y «n», y
// nadie sabía que estaban.

// accionAtajo es lo que hace una tecla: las mismas acciones que `AccionAtajo` en la web.
type accionAtajo int

const (
	atajoNinguno accionAtajo = iota
	atajoSubir
	atajoNuevaCarpeta
	atajoPapelera
	atajoRenombrar
	atajoMarcarTodo
	atajoQuitarSeleccion
	atajoBuscar
	atajoCambiarVista
	atajoAyuda
)

// teclaMarcarTodo es la fila de ⌘/Ctrl+A («Ctrl+a» en la web). No llega ni como runa
// ni como tecla suelta, sino como atajo del lienzo: se nombra como él
// (`fyne.ShortcutSelectAll`), y la hoja la escribe con `ui.AtajoDelSistema`.
const teclaMarcarTodo = "SelectAll"

// filaAtajo es una fila de la tabla. Las teclas son un `fyne.KeyName` («Delete», «F2»),
// el carácter en minúscula de lo que llega como runa («u», «?») o `teclaMarcarTodo`: los
// nombres de tecla de Fyne de las letras van en mayúscula («U»), así que no se cruzan.
type filaAtajo struct {
	teclas []string
	accion accionAtajo
	clave  string // la descripción, en el catálogo
	// conRed: necesita red. Sin ella Drive se queda en solo lectura —la caché de la
	// última carpeta, sin botones de escritura ni casillas—, y la tecla tampoco puede
	// hacer lo que ellos no hacen.
	conRed bool
}

// grupoTabla es un apartado de la hoja, con su rótulo en el catálogo.
type grupoTabla struct {
	titulo string
	filas  []filaAtajo
}

// tablaAtajos es la tabla, en el orden de la hoja: las mismas filas que `GRUPOS` de la
// web, que vigila `TestLaTablaDeAtajosEsLaDeLaWeb`.
var tablaAtajos = []grupoTabla{
	{"shortcuts.group.actions", []filaAtajo{
		{[]string{"u"}, atajoSubir, "shortcuts.upload", true},
		{[]string{"n"}, atajoNuevaCarpeta, "shortcuts.newFolder", true},
		// Retroceso es la tecla «delete» de un teclado de Mac, que no tiene Supr.
		{[]string{string(fyne.KeyDelete), string(fyne.KeyBackspace)}, atajoPapelera, "shortcuts.trash", true},
		{[]string{string(fyne.KeyF2)}, atajoRenombrar, "shortcuts.rename", true},
	}},
	{"shortcuts.group.selection", []filaAtajo{
		{[]string{teclaMarcarTodo}, atajoMarcarTodo, "shortcuts.selectAll", true},
		{[]string{string(fyne.KeyEscape)}, atajoQuitarSeleccion, "shortcuts.clearSelection", false},
	}},
	{"shortcuts.group.view", []filaAtajo{
		{[]string{"/"}, atajoBuscar, "shortcuts.search", false},
		{[]string{"v"}, atajoCambiarVista, "shortcuts.toggleView", false},
		{[]string{"?"}, atajoAyuda, "shortcuts.help", false},
	}},
}

// filaDe busca una tecla en la tabla.
func filaDe(tecla string) (filaAtajo, bool) {
	for _, g := range tablaAtajos {
		for _, f := range g.filas {
			for _, k := range f.teclas {
				if k == tecla {
					return f, true
				}
			}
		}
	}
	return filaAtajo{}, false
}

// teclaDeRuna es cómo se busca una runa en la tabla: en minúscula, porque con el bloqueo
// de mayúsculas puesto «u» llega como «U» (`teclaDeEvento` en la web).
func teclaDeRuna(r rune) string {
	if r >= 'A' && r <= 'Z' {
		r += 'a' - 'A'
	}
	return string(r)
}

// EngancharAtajos pone el teclado de Drive en el lienzo de la ventana: las letras por
// la ranura de runas, Supr, F2 y Esc por la de teclas sueltas, y ⌘/Ctrl+A como atajo.
//
// Las tres ranuras solo reciben lo que NO se queda un control enfocado —escribir una «n»
// en el buscador no crea una carpeta, y Ctrl+A en él sigue marcando su texto—, y con un
// diálogo o un menú encima ninguna hace nada: el teclado es de la capa. Es la
// `esAtajoDeApp` de la web.
//
// `runas` y `teclas` dan la pantalla cuando la haya. Son dos porque las letras de
// escritura («u», «n») traen la pantalla aunque haga falta abrirla, como el menú de la
// app; las demás teclas, no: un Escape o un Supr con Drive bloqueado no deben pedir que
// se desbloquee.
func EngancharAtajos(c fyne.Canvas, runas, teclas func(func(*Pantalla))) {
	libre := func() bool { return c.Overlays().Top() == nil }
	c.SetOnTypedRune(func(r rune) {
		if libre() {
			runas(func(p *Pantalla) { p.Atajo(r) })
		}
	})
	// Lo que hubiera en la ranura sigue detrás: las capas la envuelven al abrirse para
	// que Escape las cierre (`ui.asegurarTeclas`).
	previa := c.OnTypedKey()
	c.SetOnTypedKey(func(k *fyne.KeyEvent) {
		suya := false
		if libre() {
			teclas(func(p *Pantalla) { suya = p.AtajoTecla(k) })
		}
		if !suya && previa != nil {
			previa(k)
		}
	})
	// ⌘ y Ctrl cuentan igual, como en la web: el driver da `ShortcutSelectAll` con la
	// tecla de atajos del sistema (Ctrl, o ⌘ en el Mac) y un atajo propio con la otra.
	marcar := func(fyne.Shortcut) {
		if libre() {
			teclas(func(p *Pantalla) { p.AtajoMarcarTodo() })
		}
	}
	c.AddShortcut(&fyne.ShortcutSelectAll{}, marcar)
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyA, Modifier: fyne.KeyModifierControl}, marcar)
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyA, Modifier: fyne.KeyModifierSuper}, marcar)
}

// Atajo atiende una letra suelta (fuera de un campo y sin nada superpuesto: eso lo
// comprueba quien llama). Devuelve si la tecla era suya.
func (p *Pantalla) Atajo(r rune) bool { return p.atajo(teclaDeRuna(r)) }

// AtajoTecla atiende una tecla suelta con nombre: Supr, Retroceso, F2 y Esc.
func (p *Pantalla) AtajoTecla(k *fyne.KeyEvent) bool {
	if k == nil {
		return false
	}
	return p.atajo(string(k.Name))
}

// AtajoMarcarTodo es ⌘/Ctrl+A.
func (p *Pantalla) AtajoMarcarTodo() bool { return p.atajo(teclaMarcarTodo) }

// atajo hace lo que dice la fila de la tecla. Solo en «Mi unidad», que es la pantalla
// que en la web escucha el teclado (`FileBrowser`); sin red, solo las que no escriben.
// Devuelve si la tecla hizo algo: una que no aplica ahora (F2 sin un único elemento
// marcado, Esc sin selección) sigue su camino, que puede ser de otro.
func (p *Pantalla) atajo(tecla string) bool {
	f, ok := filaDe(tecla)
	if !ok || p.seccion != MiUnidad || (f.conRed && p.explor.SinRed()) {
		return false
	}
	e := p.explor
	switch f.accion {
	case atajoSubir:
		p.elegirYSubir()
	case atajoNuevaCarpeta:
		p.pedirCarpeta()
	case atajoPapelera:
		return p.papeleraPorTecla()
	case atajoRenombrar:
		sel := p.seleccionados()
		if len(sel) != 1 {
			return false
		}
		p.pedirRenombrar(sel[0])
	case atajoMarcarTodo:
		return p.marcarTodoPorTecla()
	case atajoQuitarSeleccion:
		// Sin nada marcado Escape no es de Drive: se le deja a quien venga detrás.
		if len(p.seleccionados()) == 0 {
			return false
		}
		e.limpiarSeleccion()
	case atajoBuscar:
		if p.win == nil || e.buscador == nil {
			return false
		}
		p.win.Canvas().Focus(e.buscador)
	case atajoCambiarVista:
		if e.AlCambiarVista == nil {
			return false
		}
		e.AlCambiarVista(!e.Rejilla)
	case atajoAyuda:
		p.verAtajos()
	default:
		return false
	}
	return true
}

// papeleraPorTecla es Supr: lo marcado, a la papelera.
func (p *Pantalla) papeleraPorTecla() bool {
	sel := p.seleccionados()
	if len(sel) == 0 || p.explor.Ocupado {
		return false
	}
	// Uno solo va directo, como «Eliminar» en su menú, con «Deshacer» en el aviso;
	// varios pasan por la confirmación del botón de la barra, porque una tecla que
	// tira cincuenta archivos sin preguntar es un accidente esperando.
	if len(sel) > 1 {
		p.papeleraLote()
		return true
	}
	// Mantener la tecla la repite, y cada repetición encontraba el mismo elemento aún
	// marcado: otra petición a la papelera y otro aviso de error. Se suelta ya; el
	// aviso trae «Deshacer» si hacía falta.
	p.explor.limpiarSeleccion()
	go p.mandarAPapelera(sel)
	return true
}

// marcarTodoPorTecla es ⌘/Ctrl+A: todo lo cargado, siempre. A diferencia de la casilla de
// la cabecera, que con todo marcado lo desmarca, pulsar Ctrl+A dos veces no deja nada
// marcado en ningún explorador de archivos.
func (p *Pantalla) marcarTodoPorTecla() bool {
	e := p.explor
	if len(e.Elementos) == 0 {
		return false
	}
	for _, el := range e.Elementos {
		e.Seleccion[el.ID] = true
	}
	p.ancla = -1
	p.Repintar()
	return true
}

// esMac: en un Mac no hay Supr y la tecla de los atajos es ⌘.
func esMac() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "ios" }

// rotuloTecla es cómo se ESCRIBE una tecla de la tabla en la hoja (`rotulo` en la web).
func (p *Pantalla) rotuloTecla(tecla string) string {
	switch tecla {
	case string(fyne.KeyDelete):
		return p.tr.T("shortcuts.key.delete", nil)
	case string(fyne.KeyBackspace):
		return "⌫"
	case string(fyne.KeyEscape):
		return "Esc"
	case teclaMarcarTodo:
		return ui.AtajoDelSistema("Ctrl+A")
	}
	return tecla
}

// gruposDeAtajos es la hoja (`gruposDeAtajos` en la web), en el idioma de la pantalla y
// con las teclas de este sistema. En un Mac se enseña solo ⌫; en el resto, solo Supr:
// Retroceso también funciona, pero es la tecla que nadie espera que tire nada.
func (p *Pantalla) gruposDeAtajos(mac bool) []ui.GrupoDeAtajos {
	grupos := make([]ui.GrupoDeAtajos, 0, len(tablaAtajos))
	for _, g := range tablaAtajos {
		grupo := ui.GrupoDeAtajos{Titulo: p.tr.T(g.titulo, nil)}
		for _, f := range g.filas {
			var teclas []string
			for _, k := range f.teclas {
				if (k == string(fyne.KeyDelete) && mac) || (k == string(fyne.KeyBackspace) && !mac) {
					continue
				}
				teclas = append(teclas, p.rotuloTecla(k))
			}
			grupo.Atajos = append(grupo.Atajos, ui.AtajoDeTeclado{Teclas: teclas, Descripcion: p.tr.T(f.clave, nil)})
		}
		grupos = append(grupos, grupo)
	}
	return grupos
}

// verAtajos abre la hoja de «?» sobre la ventana.
func (p *Pantalla) verAtajos() {
	if p.win == nil {
		return
	}
	ui.MostrarHojaAtajos(p.win.Canvas(), p.tr.T("shortcuts.title", nil), p.tr.T("ui.close", nil), p.gruposDeAtajos(esMac()))
}
