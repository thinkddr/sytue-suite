// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"fmt"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// El grupo «Macros» de la pestaña «Vista» (`MacrosGroup.tsx`): grabar una secuencia de
// comandos de la cinta y reproducirla más tarde.
//
// Lo que se graba son COMANDOS, los que pasan por [Contexto.Mandar]; teclear texto no es un
// comando y no se graba, como en la web. Reproducir valida la macro ENTERA contra el
// registro común de comandos y el permiso de ahora (`office.ValidarMacro`, la misma regla
// que la web, sujeta por una tabla de casos compartida) y, si un solo paso falla, no
// despacha ninguno.
//
// ponytail: las macros viven en la memoria del proceso. En la web se guardan por usuario
// (`localStorage`, una clave por `sub`) y sin sesión no se guardan; en nativo esta pieza no
// conoce al usuario, y escribirlas a disco sin él sería la clave «anon» que la web acaba de
// quitar. Se persistirán cuando el producto pase el `sub` (hoy no llega al cromo).

// macro es una macro guardada: nombre y secuencia. Nunca vacía.
type macro struct {
	id     string
	nombre string
	tipo   string // el `DocKind` en que se grabó
	pasos  []office.Paso
}

// macros es el estado del grabador, común a todos los documentos del proceso: una macro es
// de la persona, no de un documento.
type macros struct {
	mu        sync.Mutex
	grabando  bool
	pasos     []office.Paso
	pasados   int // pasos que no cupieron al llegar a [office.MaxPasos]
	guardadas []macro
	n         int
}

var grabadora = &macros{}

// kindDe es el `DocKind` de la web de este tipo.
func (t Tipo) kindDe() string {
	switch t {
	case Hoja:
		return office.Hoja
	case Presentacion:
		return office.Presentacion
	case Documento:
	}
	return office.Documento
}

func (m *macros) enGrabacion() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.grabando
}

func (m *macros) empezar() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.grabando, m.pasos, m.pasados = true, nil, 0
}

// registrar es el gancho de [Contexto.Mandar]: fuera de grabación es un `if` que vuelve.
func (m *macros) registrar(t Tipo, cmd Comando, valor string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.grabando {
		return
	}
	if len(m.pasos) >= office.MaxPasos {
		m.pasados++
		return
	}
	m.pasos = append(m.pasos, office.Paso{Editor: t.kindDe(), Cmd: string(cmd), Valor: valor, ConValor: valor != ""})
}

// terminar cierra la grabación y devuelve la secuencia depurada —sin los comandos que no se
// reproducen, los que el registro no conoce y los de valor desmesurado— y cuántos pasos se
// dejaron fuera, para que quien grabó sepa que su macro no lleva todo lo que hizo.
func (m *macros) terminar() (pasos []office.Paso, descartados int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	crudos, pasados := m.pasos, m.pasados
	m.grabando, m.pasos, m.pasados = false, nil, 0
	for _, p := range crudos {
		if office.NoReproducibles[p.Cmd] {
			continue
		}
		if _, ok := office.BuscarComando(p.Cmd); !ok || len([]rune(p.Valor)) > office.MaxValor {
			continue
		}
		pasos = append(pasos, p)
	}
	return pasos, len(crudos) - len(pasos) + pasados
}

// guardar añade una macro con ese nombre; false si ya hay [office.MaxMacros].
func (m *macros) guardar(nombre, tipo string, pasos []office.Paso) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.guardadas) >= office.MaxMacros {
		return false
	}
	m.n++
	m.guardadas = append(m.guardadas, macro{id: "m" + strconv.Itoa(m.n), nombre: nombre, tipo: tipo, pasos: pasos})
	return true
}

func (m *macros) lista() []macro {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]macro(nil), m.guardadas...)
}

func (m *macros) borrar(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, x := range m.guardadas {
		if x.id == id {
			m.guardadas = append(m.guardadas[:i], m.guardadas[i+1:]...)
			return
		}
	}
}

// Reproducir despacha una macro sobre el lienzo de este contexto, SOLO si entera valida
// contra el registro y el permiso de ahora. Devuelve el veredicto: el motivo del primer
// paso que falla y su posición (desde 0), o [office.Valido]. No se graba a sí misma.
func (c *Contexto) reproducirMacro(m macro) (office.Rechazo, int) {
	pasos := make([]office.Paso, len(m.pasos))
	copy(pasos, m.pasos)
	if c.Lienzo == nil {
		return office.OtroTipo, 0
	}
	if r, i := office.ValidarMacro(pasos, office.Entorno{Kind: c.Tipo.kindDe(), SoloLectura: c.SoloLectura}); r != office.Valido {
		return r, i
	}
	for _, p := range pasos {
		c.Lienzo.Mandar(Comando(p.Cmd), p.Valor)
	}
	if c.Refrescar != nil {
		c.Refrescar()
	}
	return office.Valido, 0
}

// grupoMacros es el grupo «Macros» de «Vista»: grabar o detener, y el gestor.
func (c *Contexto) grupoMacros() fyne.CanvasObject {
	grabando := grabadora.enGrabacion()
	icono, rotulo := iconos.EdGrabar, "macros.record"
	if grabando {
		icono, rotulo = iconos.EdDetener, "macros.stop"
	}
	var grabar *BotonCinta
	grabar = NuevoBotonCinta(icono, c.Traducir(rotulo), func() { c.alternarGrabacion(grabar) })
	grabar.Activo = grabando
	// Grabar es una acción que edita (registro: `macroRecord`): se apaga en solo lectura.
	if def, ok := office.BuscarComando("macroRecord"); ok {
		grabar.Inerte = def.Bloqueado(c.SoloLectura)
	}
	var gestor *BotonCinta
	gestor = NuevoBotonCinta(iconos.EdMacro, c.Traducir("macros.manage"), func() { c.abrirGestorMacros(gestor) })
	return NuevoGrupo(c.Traducir("group.macros"), grabar, gestor)
}

// alternarGrabacion es `toggleRecord`: empezar, o detener y pedir nombre. Cancelar descarta.
func (c *Contexto) alternarGrabacion(desde fyne.CanvasObject) {
	if !grabadora.enGrabacion() {
		grabadora.empezar()
		c.refrescar()
		return
	}
	pasos, descartados := grabadora.terminar()
	c.refrescar()
	if len(pasos) == 0 {
		c.avisar(ui.AvisoInfo, c.Traducir("macros.empty.title"))
		return
	}
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(desde)
	if lienzo == nil {
		return
	}
	nombre := c.Traducir("macros.defaultName", i18n.Vars{"n": len(grabadora.lista()) + 1})
	ui.Pregunta(c.Traducir("macros.namePrompt"), nombre, c.Traducir("ui.cancel"), c.Traducir("ui.save"),
		c.Traducir("ui.required"), func(n string) {
			n = recortar(n, office.MaxNombre)
			if n == "" {
				return
			}
			if !grabadora.guardar(n, c.Tipo.kindDe(), pasos) {
				c.avisar(ui.AvisoAdvertencia, c.Traducir("macros.limit", i18n.Vars{"n": office.MaxMacros}))
				return
			}
			c.avisar(ui.AvisoExito, c.Traducir("macros.savedToast"))
			if descartados > 0 {
				c.avisar(ui.AvisoInfo, c.Traducir("macros.discarded", i18n.Vars{"count": descartados}))
			}
		}).Mostrar(lienzo)
}

// abrirGestorMacros es el menú de las macros guardadas: reproducir una y, al final, borrarla.
// Una macro grabada en otro tipo de documento sale apagada, como en la web.
func (c *Contexto) abrirGestorMacros(desde *BotonCinta) {
	guardadas := grabadora.lista()
	var elems []fyne.CanvasObject
	if len(guardadas) == 0 {
		elems = append(elems, ui.NuevoElementoMenu(c.Traducir("macros.none"), iconos.Ninguno, nil))
	}
	for _, m := range guardadas {
		texto := fmt.Sprintf("%s — %s", m.nombre, c.Traducir("macros.steps", i18n.Vars{"count": len(m.pasos)}))
		e := ui.NuevoElementoMenu(texto, iconos.Ninguno, func() { c.lanzarMacro(m) })
		e.Inerte = m.tipo != c.Tipo.kindDe() || grabadora.enGrabacion()
		elems = append(elems, e)
	}
	for _, m := range guardadas {
		elems = append(elems, ui.NuevoElementoMenu(c.Traducir("macros.delete")+": "+m.nombre, iconos.Ninguno, func() {
			c.Confirmar(desde, "macros.delete", "macros.deleteConfirm", true, func() { grabadora.borrar(m.id) },
				OpcionConfirmar{Titulo: "macros.deleteTitle", Vars: i18n.Vars{"name": m.nombre}})
		}))
	}
	menu := ui.NuevoMenu(elems...)
	menu.AlInicio = true
	menu.Rotulo = c.Traducir("macros.saved.label")
	menu.Mostrar(desde)
}

// lanzarMacro reproduce y, si no se puede, dice por qué: solo lectura, o el paso que falla.
func (c *Contexto) lanzarMacro(m macro) {
	r, paso := c.reproducirMacro(m)
	switch r {
	case office.Valido:
		return
	case office.SoloLectura:
		c.avisar(ui.AvisoAdvertencia, c.Traducir("macros.blocked.title")+" — "+c.Traducir("macros.blocked.readOnly"))
	default:
		c.avisar(ui.AvisoAdvertencia, c.Traducir("macros.blocked.title")+" — "+c.Traducir("macros.blocked.step", i18n.Vars{"n": paso + 1}))
	}
}

func (c *Contexto) avisar(tono ui.TonoAviso, titulo string) {
	if c.Notificar != nil {
		c.Notificar(tono, titulo)
	}
}

func recortar(s string, tope int) string {
	r := []rune(s)
	if len(r) > tope {
		r = r[:tope]
	}
	return string(r)
}
