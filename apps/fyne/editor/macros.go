// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

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
// Las macros guardadas son de la PERSONA, como en la web (`localStorage`, una clave por
// `sub`): van al almacén por persona de la sesión ([Personal]), con el mismo JSON que la
// web, y quien entra con otra cuenta no ve las de la anterior. Sin nadie dentro no se
// guarda nada —la clave «anon» que la web quitó mezclaba a las personas— y la macro vive
// lo que dure el proceso. Lo leído se vuelve a depurar contra el registro: lo guardado
// pudo escribirlo otra versión.

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
	de        string // la persona de quien son las guardadas; "" sin nadie dentro
}

var grabadora = &macros{}

// AlmacenPersonal es donde se guardan las macros; lo cumple `sesion.PorPersona`, que pone
// la cuenta (emisor, persona y organización) en la clave y no guarda nada sin nadie dentro.
type AlmacenPersonal interface {
	Leer(clave string) string
	Poner(clave, valor string)
	Persona() string // la cuenta de quien ha entrado (cambia con la organización), o ""
}

// Personal es el almacén de la persona que ha entrado, si la app lo da (lo pone el `main`
// de cada editor con la sesión). Sin él, las macros viven lo que dure el proceso.
var Personal AlmacenPersonal

// ClaveMacros es el prefijo de la clave de la web (`sytue:docs:macros:<sub>`): la persona
// la pone el almacén.
const ClaveMacros = "sytue:docs:macros"

// sincronizar, con el candado tomado, carga las guardadas de quien está dentro si no son
// las suyas: al entrar, al cambiar de cuenta o al salir (sin nadie, ninguna).
func (m *macros) sincronizar() {
	persona := ""
	if Personal != nil {
		persona = Personal.Persona()
	}
	if persona == m.de {
		return
	}
	m.de, m.guardadas = persona, nil
	if persona != "" {
		m.guardadas = leerMacros(Personal.Leer(ClaveMacros))
	}
}

// persistir, con el candado tomado, guarda las de quien está dentro. Sin nadie, nada.
func (m *macros) persistir() {
	if Personal == nil || m.de == "" {
		return
	}
	if len(m.guardadas) == 0 {
		Personal.Poner(ClaveMacros, "")
		return
	}
	Personal.Poner(ClaveMacros, escribirMacros(m.guardadas))
}

// pasoJSON y macroJSON son la forma de `MacroStep` y `Macro` de la web.
type pasoJSON struct {
	Editor string  `json:"editor"`
	Cmd    string  `json:"cmd"`
	Valor  *string `json:"value,omitempty"`
}

type macroJSON struct {
	ID     string     `json:"id"`
	Nombre string     `json:"name"`
	Pasos  []pasoJSON `json:"steps"`
}

// macroLeida es [macroJSON] al leer: cada paso por separado, para que uno mal formado
// tire solo ese paso y no la macro entera, como `coerceStep`.
type macroLeida struct {
	ID     string            `json:"id"`
	Nombre string            `json:"name"`
	Pasos  []json.RawMessage `json:"steps"`
}

// pasoLeido es [pasoJSON] al leer: un `value` que no es texto se ignora, como en la web,
// en vez de tirar el paso.
type pasoLeido struct {
	Editor string          `json:"editor"`
	Cmd    string          `json:"cmd"`
	Valor  json.RawMessage `json:"value"`
}

func esKind(k string) bool {
	return k == office.Documento || k == office.Hoja || k == office.Presentacion
}

// leerMacros es `parseMacros`: tolerante, nunca falla y deja fuera lo mal formado, lo que
// el registro no conoce o no se reproduce y lo que pasa de los techos. Una macro sin un
// paso válido no se carga.
func leerMacros(crudo string) []macro {
	var datos []json.RawMessage
	if crudo == "" || json.Unmarshal([]byte(crudo), &datos) != nil {
		return nil
	}
	var out []macro
	for _, d := range datos {
		if len(out) >= office.MaxMacros {
			break
		}
		var mj macroLeida
		if json.Unmarshal(d, &mj) != nil || mj.ID == "" || mj.Pasos == nil {
			continue
		}
		var pasos []office.Paso
		for _, crudoPaso := range mj.Pasos[:min(len(mj.Pasos), office.MaxPasos)] {
			var p pasoLeido
			if json.Unmarshal(crudoPaso, &p) != nil || !esKind(p.Editor) || office.NoReproducibles[p.Cmd] {
				continue
			}
			if _, ok := office.BuscarComando(p.Cmd); !ok {
				continue
			}
			paso := office.Paso{Editor: p.Editor, Cmd: p.Cmd}
			var valor string
			if len(p.Valor) > 0 && json.Unmarshal(p.Valor, &valor) == nil {
				if len([]rune(valor)) > office.MaxValor {
					continue
				}
				paso.Valor, paso.ConValor = valor, true
			}
			pasos = append(pasos, paso)
		}
		if len(pasos) == 0 {
			continue
		}
		out = append(out, macro{id: mj.ID, nombre: recortar(mj.Nombre, office.MaxNombre), tipo: pasos[0].Editor, pasos: pasos})
	}
	return out
}

// escribirMacros es `serializeMacros`.
func escribirMacros(ms []macro) string {
	out := make([]macroJSON, 0, len(ms))
	for _, m := range ms {
		mj := macroJSON{ID: m.id, Nombre: m.nombre, Pasos: make([]pasoJSON, 0, len(m.pasos))}
		for _, p := range m.pasos {
			pj := pasoJSON{Editor: p.Editor, Cmd: p.Cmd}
			if p.ConValor {
				v := p.Valor
				pj.Valor = &v
			}
			mj.Pasos = append(mj.Pasos, pj)
		}
		out = append(out, mj)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}

// idMacro es un id nuevo, al azar como el `crypto.randomUUID` de la web: un contador
// chocaría con los ids que ya trae lo guardado.
func idMacro() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "m" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

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
	m.sincronizar()
	if len(m.guardadas) >= office.MaxMacros {
		return false
	}
	m.guardadas = append(m.guardadas, macro{id: idMacro(), nombre: nombre, tipo: tipo, pasos: pasos})
	m.persistir()
	return true
}

func (m *macros) lista() []macro {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sincronizar()
	return append([]macro(nil), m.guardadas...)
}

func (m *macros) borrar(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sincronizar()
	for i, x := range m.guardadas {
		if x.id == id {
			m.guardadas = append(m.guardadas[:i], m.guardadas[i+1:]...)
			m.persistir()
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
