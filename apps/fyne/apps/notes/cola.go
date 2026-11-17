// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

// Captura sin conexión: la contrapartida de client/cola.ts de la web, con las mismas
// reglas. Lo capturado sin red va a una cola en el almacén local (`offline.Store`, por
// persona) con su `data.client_id`, y sale en cuanto vuelve la red; el servicio devuelve
// la nota que ya tenga con esa marca, así que reintentar nunca duplica. El borrador de
// la captura de cada cuaderno también se guarda: cerrar la app no lo pierde.

import (
	"crypto/rand"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// La cola y los borradores viven en apps/nucleo/notas: cerrar sesión, en cualquiera de
// las trece apps, los sube o los descarta. Aquí, los nombres de siempre.
const (
	espacioCola       = notas.EspacioCapturas
	espacioBorradores = notas.EspacioBorradores
	prefijoDia        = notas.PrefijoDia
	// cadaReintento es cada cuánto se prueba la cola mientras quede algo: el monitor solo
	// sabe que vuelve la red cuando una petición sale bien.
	cadaReintento = 30 * time.Second
)

type (
	capturaPendiente = notas.Captura
	envio            = notas.Envio
)

var (
	esFalloDeRed = notas.EsFalloDeRed
	pendientesDe = notas.Pendientes
	vaciarCola   = notas.Vaciar
	// capturaRapida la usan también los borradores que se suben al cerrar sesión.
	capturaRapida = notas.CapturaRapida
	// lineasDeCaptura parte la captura en las notas que nacen de ella (diario.go). Es la
	// misma con la que «Subir» parte un borrador al cerrar sesión: uno de varias líneas
	// sale como las notas que habría hecho Intro, y no como una con saltos.
	lineasDeCaptura = notas.LineasDeCaptura
)

// MaxLineasPegadas es lo más que nace de una vez de varias líneas, el del núcleo
// (`MAX_LINEAS_PEGADAS` de client/diario.ts).
const MaxLineasPegadas = notas.MaxLineasPegadas

// dueño es el `sub` de quien usa la app: el mismo en todas sus organizaciones, así que
// vale para comparar con OwnerSub pero no para indexar el almacén (claveAlmacen).
func (p *Pantalla) dueño() string {
	if work := p.vida().work; work != nil {
		return work.PrivateStorageScope().Subject
	}
	if p.Yo == nil {
		return ""
	}
	return p.Yo()
}

// claveAlmacen es persona y organización: de ella cuelgan la cola y los borradores.
func (p *Pantalla) claveAlmacen() string {
	if work := p.vida().work; work != nil {
		return work.Owner()
	}
	org := ""
	if p.Org != nil {
		org = p.Org()
	}
	return p.dueño() + "/" + org
}

// puedeCapturar es puedeEscribir sin exigir red cuando hay dónde encolar.
func (p *Pantalla) puedeCapturar() bool {
	return p.vigente() && !p.migrando && p.svc != nil && p.activo.ID != "" && !p.activo.Crudo.ReadOnly && (p.hayRed() || p.Almacen != nil)
}

// esProvisional dice si el cuaderno es un día que aún no existe en el servicio.
func esProvisional(id string) bool { return strings.HasPrefix(id, prefijoDia) }

// diaSinConexion es el cuaderno de un día sin red: el que ya se conozca o uno provisional.
func diaSinConexion(cuadernos []Cuaderno, dueño, fecha, titulo string) Cuaderno {
	for _, c := range cuadernos {
		if c.Crudo.DailyDate == fecha && c.Crudo.OwnerSub == dueño {
			return c
		}
	}
	return DeCuaderno(api.Notebook{ID: prefijoDia + fecha, Title: titulo, DailyDate: fecha, Layout: "list", Visibility: "personal", OwnerSub: dueño})
}

// encolar guarda la captura para el cuaderno abierto. En el hilo de la interfaz.
func (p *Pantalla) encolar(in api.NoteInput, c Cuaderno) error {
	if !p.vigente() {
		return offline.ErrWorkClosed
	}
	cp := capturaPendiente{ClientID: in.ClientID(), In: in, At: time.Now().UnixMilli()}
	if esProvisional(c.ID) {
		cp.Fecha, cp.Titulo = c.Crudo.DailyDate, c.Titulo
	} else {
		cp.Cuaderno = c.ID
	}
	ctx, done, err := p.admitir()
	if err != nil {
		return err
	}
	defer done()
	if err := p.escribirLocal(ctx, espacioCola, cp.ClientID, cp); err != nil {
		return err
	}
	p.cola = pendientesDe(p.Almacen, p.claveAlmacen())
	if p.hayRed() {
		p.iniciar(p.vaciar)
	}
	return nil
}

// pendientesAqui son las de la cola que van al cuaderno abierto.
func (p *Pantalla) pendientesAqui() []capturaPendiente {
	var out []capturaPendiente
	for _, c := range p.cola {
		if c.Cuaderno != "" && c.Cuaderno == p.activo.ID || c.Cuaderno == "" && c.Fecha != "" && c.Fecha == p.activo.Crudo.DailyDate {
			out = append(out, c)
		}
	}
	return out
}

// descartar quita de la cola una captura rechazada.
func (p *Pantalla) descartar(id string) {
	if !p.vigente() {
		return
	}
	ctx, done, err := p.admitir()
	if err != nil {
		return
	}
	defer done()
	if p.Almacen == nil {
		return
	}
	_ = p.escribirLocal(ctx, espacioCola, id, nil)
	p.cola = pendientesDe(p.Almacen, p.claveAlmacen())
	p.Repintar()
}

// vaciar envía la cola. Desde cualquier rutina; dos a la vez no se pisan.
func (p *Pantalla) vaciar() {
	if p.Almacen == nil || p.svc == nil || !p.vaciando.CompareAndSwap(false, true) {
		return
	}
	defer p.vaciando.Store(false)
	ctxEdiciones, cancelEdiciones := p.ctx()
	errEdiciones := p.vaciarEdiciones(ctxEdiciones)
	cancelEdiciones()
	if errors.Is(errEdiciones, api.ErrDatosCifradosNotas) {
		p.enHilo(func() {
			if !p.avisoBorradorBloqueado {
				p.avisoBorradorBloqueado = true
				p.fallo("note.saveFailed", api.ErrDatosCifradosNotas)
			}
		})
	}
	pend := pendientesDe(p.Almacen, p.claveAlmacen())
	if slices.ContainsFunc(pend, func(c capturaPendiente) bool { return c.Insegura }) {
		p.enHilo(func() {
			if !p.avisoCapturaBloqueada {
				p.avisoCapturaBloqueada = true
				p.fallo("note.saveFailed", api.ErrDatosCifradosNotas)
			}
		})
	}
	if !slices.ContainsFunc(pend, func(c capturaPendiente) bool { return !c.Rechazada }) {
		// Nada que enviar, pero lo que se ve puede ser de antes de cambiar de
		// organización: se pone al día en la siguiente ronda.
		p.enHilo(func() {
			if !slices.EqualFunc(p.cola, pend, func(a, b capturaPendiente) bool { return a.ClientID == b.ClientID && a.Rechazada == b.Rechazada }) {
				p.cola = pend
				p.Repintar()
			}
		})
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	tocados := vaciarCola(ctx, p.Almacen, p.claveAlmacen(), notas.EnvioDe(p.svc, time.Now().Format(time.DateOnly)))
	p.enHilo(func() {
		p.cola = pendientesDe(p.Almacen, p.claveAlmacen())
		if len(tocados) > 0 {
			switch {
			case esProvisional(p.activo.ID) && tocados[p.activo.ID]:
				p.abrirDia(p.activo.Crudo.DailyDate) // el día ya existe: el de verdad
			case tocados[p.activo.ID]:
				notaFuncion := p.cargarNotas
				notaArgumento0 := p.activo
				p.iniciar(func() { notaFuncion(notaArgumento0) })
			}
			p.iniciar(p.cargarCuadernos)
		}
		p.Repintar()
	})
}

// vigilarCola arranca la cola: lo que quedó de la última vez, al volver la red y cada
// cadaReintento mientras quede algo. Una vez por Pantalla.
func (p *Pantalla) vigilarCola() {
	if p.Almacen == nil || !p.vigente() {
		return
	}
	p.vigilando.Do(func() {
		if p.red != nil {
			p.vida().unsubscribe = p.red.Subscribe(func(e offline.State) {
				if e == offline.Online {
					p.iniciar(p.vaciar)
				}
				p.enHilo(p.Repintar)
			})
		}
		p.iniciar(func() {
			ctxTrabajo, _ := p.contextoTrabajo()
			reloj := time.NewTicker(cadaReintento)
			defer reloj.Stop()
			for {
				p.vaciar()
				select {
				case <-ctxTrabajo.Done():
					return
				case <-reloj.C:
				}
			}
		})
	})
}

// borradorGuardado es el texto de la captura de un cuaderno que quedó sin enviar.
func (p *Pantalla) borradorGuardado(id string) string {
	if t, ok := p.borradores[id]; ok || p.Almacen == nil {
		return t
	}
	var t string
	_, _ = p.Almacen.Read(p.claveAlmacen(), espacioBorradores, id, &t)
	return t
}

// guardarBorrador lo recuerda en memoria y en el almacén (vacío, lo borra).
func (p *Pantalla) guardarBorrador(id, texto string) { _ = p.guardarBorradorConError(id, texto) }

func (p *Pantalla) guardarBorradorConError(id, texto string) error {
	if !p.vigente() {
		return errors.Join(notas.ErrLocalAdmission, offline.ErrWorkClosed)
	}
	ctx, done, admissionErr := p.admitir()
	if admissionErr != nil {
		return errors.Join(notas.ErrLocalAdmission, admissionErr)
	}
	defer done()
	var err error
	if p.Almacen != nil && id != "" {
		var value any = texto
		if texto == "" {
			value = nil
		}
		err = p.escribirLocal(ctx, espacioBorradores, id, value)
		if errors.Is(err, notas.ErrLocalAdmission) {
			return err
		}
	}
	if p.borradores == nil {
		p.borradores = map[string]string{}
	}
	p.borradores[id] = texto
	if err != nil {
		slog.Warn("notes: no se ha podido guardar el borrador", "error", err)
	}
	return err
}

// nuevoClientID es la marca de una nota nueva.
func nuevoClientID() string { return rand.Text() }

// textoPendiente es lo que se escribió, sin HTML: el `textoPendiente` de la web.
func textoPendiente(c capturaPendiente) string {
	switch {
	case c.In.Title != "":
		return c.In.Title
	case c.In.ContentCipher != "":
		return ""
	}
	return htmltext.Text(c.In.Content)
}

// listaPendientes es la `ul.notes-pending` de NotebookDashboard.tsx: una línea por
// captura, `text-xs`, con su reloj (o el aviso si se rechazó) y, si se rechazó,
// «Descartar».
func (p *Pantalla) listaPendientes(v tema.Variante, pend []capturaPendiente) fyne.CanvasObject {
	t := p.tr.T
	filas := make([]ui.Bloque, 0, len(pend))
	for _, c := range pend {
		icono, tinta, rotulo := iconos.Reloj, "text-subtle", "capture.pending"
		if c.Rechazada {
			icono, tinta, rotulo = iconos.Aviso, "danger-text", "capture.rejected"
		}
		objs := []fyne.CanvasObject{
			Icono(icono, 14, tinta, v),
			lineaTok(textoPendiente(c), tema.TextoXS, ui.Normal, "text-muted", v),
			lineaTok(t(rotulo, nil), tema.TextoXS, ui.Normal, tinta, v),
		}
		if c.Rechazada {
			id := c.ClientID
			b := botonDiario(t("capture.discard", nil), ui.TallaXS, false, func() { p.descartar(id) })
			// Con varias rechazadas, «Descartar» a secas no dice cuál: el `aria-label` de la web.
			b.Rotulo = t("capture.discard", nil) + ": " + textoPendiente(c)
			objs = append(objs, b)
		}
		// `min-h-6 gap-2`, y el texto es lo que cede.
		fila := container.New(&filaElastica{hueco: tema.Espacio(2), elastico: 1}, objs...)
		filas = append(filas, ui.Bloque{Obj: container.New(ui.Margen{Izq: tema.Espacio(1), Der: tema.Espacio(1)}, container.New(&altoMinimo{alto: tema.Espacio(6)}, fila)), Margen: tema.Espacio(1)})
	}
	return ui.PilaBloques(filas...)
}

// altoMinimo centra su contenido en vertical con un alto mínimo (`min-h-6 items-center`).
type altoMinimo struct{ alto float32 }

func (a *altoMinimo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width, max(m.Height, a.alto))
}

func (a *altoMinimo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, (t.Height-m.Height)/2))
	objs[0].Resize(fyne.NewSize(t.Width, m.Height))
}
