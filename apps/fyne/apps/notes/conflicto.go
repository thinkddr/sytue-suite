// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

// Dos dispositivos editando la misma nota (ConflictoNota.tsx y client/conflicto.ts de la
// web). Cada guardado dice sobre qué versión se editó; si otro dispositivo ha guardado
// entretanto, el servicio responde 409 `note_edit_conflict` y no pisa lo suyo. Entonces
// se enseñan las dos versiones y se pregunta: quedarse con la de aquí, ver la otra (que
// pasa a ser la nota) o guardar las dos (la de aquí, como nota nueva). Si lo del otro no
// tocó nada de lo que se guarda —la movió en el lienzo—, no se pregunta.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// CodigoConflicto es el `code` del 409 de un guardado sobre una versión vieja.
const CodigoConflicto = "note_edit_conflict"

func esConflicto(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && e.Status == http.StatusConflict && e.Code() == CodigoConflicto
}

// conVersion es el cuerpo del guardado, editado sobre la versión `base` del servidor.
func conVersion(in api.NoteInput, base api.Note) api.NoteInput {
	if !base.UpdatedAt.IsZero() {
		v := base.UpdatedAt
		in.ExpectedUpdatedAt = &v
	}
	return in
}

// mismoContenido dice si dos versiones tienen lo mismo en todo lo que reescribe un
// guardado (`mismoContenido` de la web). Mover la nota cambia su `updated_at` y no es un
// conflicto que haya que preguntar. En una cifrada se compara el criptograma tal cual.
func mismoContenido(a, b api.Note) bool {
	return a.Kind == b.Kind && a.Title == b.Title && a.Content == b.Content &&
		a.ContentCipher == b.ContentCipher && a.Checked == b.Checked &&
		(a.CancelledAt == nil) == (b.CancelledAt == nil) && a.Pinned == b.Pinned &&
		a.Collapsed == b.Collapsed && a.Visibility == b.Visibility &&
		bytes.Equal(jsonCanonico(a.Data), jsonCanonico(b.Data))
}

// jsonCanonico vuelve a escribir un JSON con las claves en orden: `jsonb` devuelve los
// objetos en otro orden que el que se mandó.
func jsonCanonico(raw json.RawMessage) []byte {
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return raw
	}
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

// datosDeCopia son los datos de la versión de aquí para la nota nueva de «Guardar las
// dos»: sin la marca de captura (el servicio devolvería la nota que ya existe) ni los
// controles de tarea que solo pone el servicio.
func datosDeCopia(raw json.RawMessage) json.RawMessage {
	var d map[string]json.RawMessage
	if json.Unmarshal(raw, &d) != nil || d == nil {
		return nil
	}
	for _, k := range []string{"client_id", "repeated_into", "carried_from", "carry_count"} {
		delete(d, k)
	}
	out, _ := json.Marshal(d)
	return out
}

// versiones es `Versiones` de la web: la versión del servidor de cada nota guardada desde
// aquí, y una fila por nota para que sus guardados salgan de uno en uno. Sin esto, el
// segundo autoguardado salía antes de volver el primero, con la versión de antes, y
// chocaba consigo mismo.
type versiones struct {
	mu      sync.Mutex
	ultimas map[string]versionGuardada
	filas   map[string]*sync.Mutex
}

type versionGuardada struct {
	de   api.Note // la nota de pantalla sobre la que se guardó
	nota api.Note // lo que devolvió el servicio
}

// base es la última versión del servidor conocida para la nota que se ve (`vista`).
func (v *versiones) base(vista api.Note) api.Note {
	v.mu.Lock()
	defer v.mu.Unlock()
	if e, ok := v.ultimas[vista.ID]; ok && (e.de.UpdatedAt.Equal(vista.UpdatedAt) || e.nota.UpdatedAt.Equal(vista.UpdatedAt)) {
		return e.nota
	}
	return vista
}

func (v *versiones) guardada(vista, nueva api.Note) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.ultimas == nil {
		v.ultimas = map[string]versionGuardada{}
	}
	v.ultimas[vista.ID] = versionGuardada{de: vista, nota: nueva}
}

func (v *versiones) olvidar(id string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.ultimas, id)
}

// enFila espera a que acabe el guardado anterior de la nota y devuelve con qué soltar
// el turno.
func (v *versiones) enFila(id string) func() {
	v.mu.Lock()
	if v.filas == nil {
		v.filas = map[string]*sync.Mutex{}
	}
	m, ok := v.filas[id]
	if !ok {
		m = &sync.Mutex{}
		v.filas[id] = m
	}
	v.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// conflictoAbierto es un guardado que ha chocado, a la espera de decidir.
type conflictoAbierto struct {
	id, cuaderno string
	// vista es la nota de pantalla sobre la que se editó; in, lo de aquí listo para
	// mandar; mia, lo de aquí en claro; otra, la versión del servidor.
	vista  api.Note
	in     api.NoteInput
	mia    api.Note
	otra   api.Note
	dialog *ui.Dialogo
}

// guardarSobreVersionConfirmada es el PUT de `editarCon` con la versión sobre la que se editó. Si
// choca y lo del otro no tocó lo que se guarda, se guarda sobre la versión nueva; si lo
// tocó, devuelve la versión del otro en `otra` y ningún error. `acknowledge`, si no es
// nil, confirma lo guardado antes de darlo por hecho.
func (p *Pantalla) guardarSobreVersionConfirmada(ctx context.Context, n Nota, in api.NoteInput, acknowledge func(context.Context, api.Note) error) (actualizada api.Note, otra *api.Note, err error) {
	soltar := p.versiones.enFila(n.ID)
	defer soltar()
	base := p.versiones.base(n.Crudo)
	actualizar := func(entrada api.NoteInput) (api.Note, error) {
		if p.vida().work != nil && acknowledge != nil {
			return p.svc.UpdateNoteAcknowledged(ctx, n.ID, n.Cuaderno, entrada, acknowledge)
		}
		return p.svc.UpdateNote(ctx, n.ID, n.Cuaderno, entrada)
	}
	actualizada, err = actualizar(conVersion(in, base))
	if esConflicto(err) {
		leida, ok := p.leerOtra(ctx, n.Cuaderno, n.ID)
		switch {
		case !ok:
		case !mismoContenido(base, leida):
			return api.Note{}, &leida, nil
		default:
			actualizada, err = actualizar(conVersion(in, leida))
		}
	}
	if err == nil {
		p.versiones.guardada(n.Crudo, actualizada)
	}
	return actualizada, nil, err
}

// leerOtra es la versión del servidor de la nota.
func (p *Pantalla) leerOtra(ctx context.Context, cuaderno, id string) (api.Note, bool) {
	lista, err := p.svc.ListNotes(ctx, cuaderno)
	if err != nil {
		return api.Note{}, false
	}
	for _, x := range lista {
		if x.ID == id {
			return x, true
		}
	}
	return api.Note{}, false
}

// abrirConflicto enseña el diálogo, o pone al día el que ya está abierto para esa nota
// (los autoguardados que siguen chocando traen lo último escrito). En el hilo de la
// interfaz.
func (p *Pantalla) abrirConflicto(c *conflictoAbierto) {
	if a := p.conflicto; a != nil {
		if a.id == c.id {
			a.in, a.mia, a.otra = c.in, c.mia, c.otra
		}
		return
	}
	if p.win == nil {
		return
	}
	p.conflicto = c
	t, v := p.tr.T, p.variante()
	otra := p.deNota(c.otra) // abierta, si el cuaderno está cifrado y se tiene la clave
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(3)},
		ui.NuevoParrafo(t("conflict.body", nil), tema.TextoSM, ui.Normal, "text-muted", v),
		versionEnCaja(t("conflict.mine", nil), c.mia.Title, c.mia.Content, t("conflict.empty", nil), v),
		versionEnCaja(t("conflict.theirs", nil), otra.Titulo, otra.Contenido, t("conflict.empty", nil), v))
	var botones []*ui.Boton
	ocupado := func(si bool) {
		for _, b := range botones {
			if si {
				b.Disable()
			} else {
				b.Enable()
			}
		}
	}
	lasDos := ui.NuevoBoton(t("conflict.keepBoth", nil), func() { p.guardarLasDos(c) }).Con(ui.Fantasma, ui.TallaMD)
	laOtra := ui.NuevoBoton(t("conflict.seeTheirs", nil), func() { p.adoptarLaOtra(c) }).Con(ui.Secundario, ui.TallaMD)
	laMia := ui.NuevoBoton(t("conflict.keepMine", nil), func() {
		ocupado(true)
		go p.quedarmeConLaMia(c, func() { ocupado(false) })
	})
	botones = []*ui.Boton{lasDos, laOtra, laMia}
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, lasDos, laOtra, laMia)
	c.dialog = ui.NuevoDialogo(t("conflict.title", nil), cuerpo).Con(ui.DialogoMD, pie)
	// Cerrar no decide nada: lo escrito sigue en pantalla y el próximo guardado vuelve a
	// preguntar.
	c.dialog.AlCerrar = func() { p.cerrarConflicto(c) }
	p.mostrar(c.dialog)
}

// MaxExtractoConflicto es cuánto se enseña de cada versión: basta para reconocerla.
const MaxExtractoConflicto = 280

// extractoConflicto es el título y el texto plano de una versión, recortados.
func extractoConflicto(titulo, html, vacia string) string {
	partes := []string{}
	if s := strings.TrimSpace(titulo); s != "" {
		partes = append(partes, s)
	}
	if s := strings.Join(strings.Fields(htmltext.Text(html)), " "); s != "" {
		partes = append(partes, s)
	}
	texto := strings.Join(partes, " — ")
	if texto == "" {
		return vacia
	}
	if utf8.RuneCountInString(texto) > MaxExtractoConflicto {
		r := []rune(texto)
		return string(r[:MaxExtractoConflicto-1]) + "…"
	}
	return texto
}

// versionEnCaja es una versión del diálogo: `rounded-md border bg-bg-subtle p-3`, el
// rótulo en `sytue-meta font-semibold` y el extracto en `text-sm`.
func versionEnCaja(rotulo, titulo, html, vacia string, v tema.Variante) fyne.CanvasObject {
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD)
	g := tema.Espacio(3) + tema.Borde
	dentro := container.New(ui.Columna{Hueco: tema.Espacio(1)},
		ui.NuevoParrafo(rotulo, tema.TextoXS, ui.SemiNegrita, "text-subtle", v),
		ui.NuevoParrafo(extractoConflicto(titulo, html, vacia), tema.TextoSM, ui.Normal, "text", v))
	return container.New(&enCaja{caja: caja}, append(caja.Objetos(),
		container.New(ui.Margen{Izq: g, Der: g, Arr: g, Abj: g}, dentro))...)
}

// cerrarConflicto retira el diálogo. En el hilo de la interfaz.
func (p *Pantalla) cerrarConflicto(c *conflictoAbierto) {
	if p.conflicto == c {
		p.conflicto = nil
	}
	if c.dialog != nil {
		c.dialog.Cerrar()
	}
}

// quedarmeConLaMia guarda lo de aquí sobre la versión del otro. Fuera del hilo.
func (p *Pantalla) quedarmeConLaMia(c *conflictoAbierto, listo func()) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	soltar := p.versiones.enFila(c.id)
	hecha, err := p.svc.UpdateNote(ctx, c.id, c.cuaderno, conVersion(c.in, c.otra))
	if err == nil {
		p.versiones.guardada(c.vista, hecha)
	}
	soltar()
	var otra api.Note
	otraLeida := false
	if esConflicto(err) {
		// Un tercero ha vuelto a guardar mientras se decidía: se pregunta por la de ahora.
		otra, otraLeida = p.leerOtra(ctx, c.cuaderno, c.id)
	}
	p.enHilo(func() {
		listo()
		switch {
		case err == nil:
			if p.activo.ID == c.cuaderno {
				p.reemplazar(p.deNota(hecha))
				p.Repintar()
			}
			p.cerrarConflicto(c)
		case otraLeida:
			p.cerrarConflicto(c)
			c.otra, c.dialog = otra, nil
			p.abrirConflicto(c)
		default:
			p.fallo("note.saveFailed", err)
		}
	})
}

// adoptarLaOtra pone la versión del otro en la nota; lo de aquí se descarta, también lo
// que el editor tuviera abierto sin guardar. En el hilo de la interfaz.
func (p *Pantalla) adoptarLaOtra(c *conflictoAbierto) {
	if s := p.edicion; s != nil && s.nota == c.id {
		if s.reloj != nil {
			s.reloj.Stop()
		}
		s.html = s.guardado // que cerrar el editor no vuelva a mandar lo descartado
		p.cerrarEditor()
	}
	p.versiones.olvidar(c.id)
	if p.activo.ID == c.cuaderno {
		p.reemplazar(p.deNota(c.otra))
		p.Repintar()
	}
	p.cerrarConflicto(c)
}

// guardarLasDos deja la versión del otro en la nota y crea otra con la de aquí.
func (p *Pantalla) guardarLasDos(c *conflictoAbierto) {
	p.adoptarLaOtra(c)
	titulo := strings.TrimSpace(c.mia.Title)
	if titulo == "" {
		titulo = p.tr.T("note.untitled", nil)
	}
	tipo := c.mia.Kind
	if tipo != "todo" {
		tipo = "text"
	}
	in := api.NoteInput{
		Kind:    tipo,
		Title:   p.tr.T("conflict.copyTitle", i18n.Vars{"title": titulo}),
		Content: c.mia.Content,
		Data:    datosDeCopia(c.mia.Data),
	}
	p.iniciar(func() {
		if p.crear(in) == nil {
			p.enHilo(func() {
				p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, p.tr.T("conflict.keptBoth", nil), ""))
			})
		}
	})
}
