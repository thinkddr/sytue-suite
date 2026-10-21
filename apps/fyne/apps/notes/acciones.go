// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: las escrituras de NotesApp.tsx —`crearNota`, `editarNota`, `moverNota`,
// `borrarNota`, `moverACuaderno`, `duplicarNota`, `guardarCuaderno`— y el menú «…» de
// NoteCard.tsx.
//
// # Cómo corren
//
// Cada escritura es una función que corre FUERA del hilo de la interfaz: lee el estado y
// pinta el cambio con `fyne.DoAndWait` —actualización OPTIMISTA, como la web: arrastrar
// una nota y verla volver medio segundo después es peor que no arrastrarla—, habla con
// el servicio, y deja el resultado con `enHilo`. Si el servicio dice que no, se vuelve a
// lo de antes y se avisa.
//
// Son funciones que corren en su rutina, y no manejadores del hilo de la interfaz, por
// el historial: la inversa de «crear» es «eliminar», y deshacer la llama desde la
// rutina que ya está hablando con el servicio.

// Parche es lo que cambia de una nota: el `ParcheDeNota` de la web. Un nil es «no lo
// toques».
type Parche struct {
	Titulo    *string
	Contenido *string
	Marcada   *bool
	// Cancelada cancela (true) o reactiva (false) una tarea. Cancelar la desmarca.
	Cancelada *bool
	Fijada    *bool
	Plegada   *bool
	Datos     json.RawMessage
	// Enlaces sustituye los `links[]` de la nota (nil: no se tocan). Al guardar el texto
	// van los enlaces a otras notas que hay en él, que es lo que alimenta «qué apunta
	// aquí» (`sincronizarEnlaces` de la web).
	Enlaces []api.NoteLink
}

// sobre devuelve la nota con el parche aplicado, sin tocar la de entrada.
func (pa Parche) sobre(n api.Note) api.Note {
	if pa.Titulo != nil {
		n.Title = *pa.Titulo
	}
	if pa.Contenido != nil {
		n.Content = *pa.Contenido
	}
	if pa.Marcada != nil {
		n.Checked = *pa.Marcada
		if n.Checked {
			n.CancelledAt = nil
		}
	}
	if pa.Cancelada != nil {
		n.CancelledAt = nil
		if *pa.Cancelada {
			ahora := time.Now()
			n.CancelledAt, n.Checked = &ahora, false
		}
	}
	if pa.Fijada != nil {
		n.Pinned = *pa.Fijada
	}
	if pa.Plegada != nil {
		n.Collapsed = *pa.Plegada
	}
	if pa.Datos != nil {
		n.Data = pa.Datos
	}
	if pa.Enlaces != nil {
		n.Links = pa.Enlaces
	}
	return n
}

// previo es el parche que devuelve la nota a como estaba: lo que se registra para
// deshacer una edición.
func previo(n api.Note) Parche {
	enlaces := n.Links
	if enlaces == nil {
		enlaces = []api.NoteLink{}
	}
	return Parche{
		Titulo: &n.Title, Contenido: &n.Content, Marcada: &n.Checked, Cancelada: ptr(n.CancelledAt != nil),
		Fijada: &n.Pinned, Plegada: &n.Collapsed, Datos: n.Data, Enlaces: enlaces,
	}
}

func ptr[T any](v T) *T { return &v }

// lanzar corre una escritura en su rutina. El error ya lo ha avisado ella misma
// (`fallo`); aquí solo se descarta, que es lo que el `void` hace en la web.
func lanzar(f func() error) { go func() { _ = f() }() }

// errSinRed es el fallo de crear sin intentarlo: no hay red, o el cuaderno es un día
// que aún no existe. Con almacén, la nota va a la cola.
var errSinRed = errors.New("notes: sin conexión")

// nota es la del cuaderno abierto con ese ID.
func (p *Pantalla) nota(id string) (Nota, bool) {
	for _, n := range p.notas {
		if n.ID == id {
			return n, true
		}
	}
	return Nota{}, false
}

// ponerNotas cambia las notas del cuaderno abierto y mueve la fase entre «vacío» y
// «éxito», que es lo único que una escritura puede cambiar: crear la primera nota no es
// volver a cargar.
func (p *Pantalla) ponerNotas(ns []Nota) {
	p.notas = ns
	switch {
	case p.lienzo.Fase == Vacia && len(ns) > 0:
		p.lienzo.Fase = Exito
	case p.lienzo.Fase == Exito && len(ns) == 0:
		p.lienzo.Fase = Vacia
	}
}

// reemplazar pone la versión nueva de una nota en su sitio.
func (p *Pantalla) reemplazar(n Nota) {
	ns := slices.Clone(p.notas)
	for i := range ns {
		if ns[i].ID == n.ID {
			ns[i] = n
		}
	}
	p.notas = ns
}

// quitar saca una nota de la lista, si está.
func (p *Pantalla) quitar(id string) {
	p.ponerNotas(slices.DeleteFunc(slices.Clone(p.notas), func(n Nota) bool { return n.ID == id }))
	if p.lienzo.Seleccionada == id {
		p.lienzo.Seleccionada = ""
	}
}

// añadir mete una nota al final si no estaba ya: deshacer un borrado la devuelve con el
// mismo ID, y dos «rehacer» seguidos no la pueden duplicar.
func (p *Pantalla) añadir(n Nota) {
	if _, ya := p.nota(n.ID); !ya {
		p.ponerNotas(append(slices.Clone(p.notas), n))
	}
}

// fallo avisa de una escritura que no ha salido. En el hilo de la interfaz.
func (p *Pantalla) fallo(clave string, err error) {
	p.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, p.tr.T(clave, nil), Motivo(err)))
}

// puedeEscribir son las condiciones de TODA escritura: servicio, red y cuaderno abierto.
func (p *Pantalla) puedeEscribir() bool {
	return !p.migrando && p.svc != nil && p.hayRed() && p.activo.ID != "" && !p.activo.Crudo.ReadOnly
}

// baseNueva completa una nota nueva como la web: en la siguiente columna y, en el lienzo
// libre, escalonada para que no se apile todo en la esquina superior izquierda. Lo que
// traiga `in` manda, que es el `...extras` de la web.
func baseNueva(in api.NoteInput, cuantas int) api.NoteInput {
	if in.Kind == "" {
		in.Kind = "text"
	}
	if in.Col == nil {
		in.Col = ptr(cuantas % NumColumnas)
	}
	paso := 24 + (cuantas%6)*28
	if in.X == nil {
		in.X = ptr(paso)
	}
	if in.Y == nil {
		in.Y = ptr(paso)
	}
	return in
}

// crear es el `crearNota` de la web: la nota nace en el cuaderno abierto, queda
// seleccionada y se puede deshacer.
func (p *Pantalla) crear(in api.NoteInput) error { return p.crearCon(in, nil) }

func (p *Pantalla) crearCon(in api.NoteInput, despues func()) error {
	return p.crearEn(in, "", despues)
}

func (p *Pantalla) crearEn(in api.NoteInput, cuaderno string, despues func()) error {
	var c Cuaderno
	var fallo error
	var epoch uint64
	var generacion string
	listo := false
	fyne.DoAndWait(func() {
		// `creando` corta el doble clic: dos «Nueva nota» seguidos eran dos notas.
		if p.bloqueado && p.puedeEscribir() {
			// Escribir en un cuaderno cerrado pide antes su contraseña, como la web.
			p.desbloquear()
			return
		}
		if !p.puedeCapturar() || p.creando || cuaderno != "" && p.activo.ID != cuaderno {
			return
		}
		p.creando, c, listo = true, p.activo, true
		epoch, generacion = p.cryptoEpoch, p.generacionCrypto()
		in = baseNueva(in, len(p.notas))
		if fallo = p.cifrar(&in); fallo != nil {
			p.creando, listo = false, false
		}
	})
	if !listo {
		return fallo
	}
	// La nota lleva SIEMPRE un `client_id` NUEVO, haya almacén o no: si la red cae con la
	// petición ya enviada, el reintento de la cola no la duplica (cola.go); y una copia
	// (duplicar) no hereda el del original, que el servicio deduplicaría devolviendo el
	// original.
	in = in.ConClientID(nuevoClientID())
	var nueva api.Note
	var err error
	if !p.hayRed() || esProvisional(c.ID) {
		err = errSinRed
	} else {
		ctx, cancelar := p.ctx()
		defer cancelar()
		nueva, err = p.svc.CreateNote(ctx, c.ID, in)
	}
	enHilo(func() {
		if !p.cryptoVigente(epoch, generacion) {
			return
		}
		p.creando = false
		if p.Almacen != nil && esFalloDeRed(err) {
			// Sin red: a la cola, y fuera del campo como si hubiera salido.
			if e := p.encolar(in, c); e != nil {
				p.fallo("note.saveFailed", e)
				return
			}
			if despues != nil {
				despues()
			}
			p.Repintar()
			return
		}
		if err != nil {
			p.fallo("note.saveFailed", err)
			return
		}
		if despues != nil {
			despues()
		}
		if p.activo.ID != c.ID {
			return // se ha cambiado de cuaderno mientras tanto: ya no se ve aquí
		}
		n := p.deNota(nueva)
		p.añadir(n)
		p.lienzo.Seleccionada = n.ID
		p.hist.Registrar(Accion{
			Etiqueta: p.tr.T("history.create", nil),
			Deshacer: func() error { return p.llamar(n.ID, p.svc.DeleteNote, func() { p.quitar(n.ID) }) },
			Rehacer:  func() error { return p.llamar(n.ID, p.svc.RestoreNote, func() { p.añadir(n) }) },
		}, "")
		p.Repintar()
	})
	return err
}

// llamar hace una llamada de un solo ID y, si sale, aplica el cambio en el hilo de la
// interfaz. Es lo que tienen en común las inversas de crear y de borrar.
func (p *Pantalla) llamar(id string, f func(context.Context, string) error, despues func()) error {
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := f(ctx, id); err != nil {
		return err
	}
	enHilo(func() { despues(); p.Repintar() })
	return nil
}

// editar es el `editarNota` de la web.
func (p *Pantalla) editar(id string, pa Parche, conHistorial bool) error {
	return p.editarCon(id, pa, conHistorial, "")
}

// editarCon es `editar` con la `fusion` del historial: los guardados de una misma sesión
// del editor son UN paso de deshacer, y deshacerlo devuelve la nota a como estaba antes
// de abrirla.
func (p *Pantalla) editarCon(id string, pa Parche, conHistorial bool, fusion string) error {
	var epoch uint64
	var generacion string
	var (
		n     Nota
		antes []Nota
		c     string
		listo bool
		in    api.NoteInput
		fallo error
	)
	fyne.DoAndWait(func() {
		var ok bool
		if n, ok = p.nota(id); !ok || !p.puedeEscribir() {
			return
		}
		in, fallo = p.entradaEditada(n, pa)
		if fallo != nil {
			return
		}
		antes, c, listo = p.notas, p.activo.ID, true
		epoch, generacion = p.cryptoEpoch, p.generacionCrypto()
		// Se pinta ya lo que se va a guardar, EN CLARO: el criptograma es para el viaje.
		vista := DeNota(pa.sobre(baseEnClaro(n)))
		vista.Crudo = n.Crudo
		p.reemplazar(vista)
		p.Repintar()
	})
	if fallo != nil {
		return fallo
	}
	if !listo {
		return nil
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	actualizada, err := p.svc.UpdateNote(ctx, id, n.Cuaderno, in)
	enHilo(func() {
		if p.activo.ID != c || !p.cryptoVigente(epoch, generacion) {
			return
		}
		if err != nil {
			p.notas = antes
			p.fallo("note.saveFailed", err)
			p.Repintar()
			return
		}
		p.reemplazar(p.deNota(actualizada))
		if conHistorial {
			vuelta := previo(baseEnClaro(n))
			p.hist.Registrar(Accion{
				Etiqueta: p.tr.T("history.edit", nil),
				Deshacer: func() error { return p.editar(id, vuelta, false) },
				Rehacer:  func() error { return p.editar(id, pa, false) },
			}, fusion)
		}
		p.Repintar()
	})
	// Una tarea que repite, recién hecha (o deshecha): el servicio ha creado (o retirado)
	// la siguiente y le ha mudado los recordatorios, sean de quien sean. Se traen las dos
	// cosas, como la web.
	if err == nil && actualizada.RepeatedInto != nil {
		p.releerNotas(ctx)
		p.cargarRecordatorios()
	}
	return err
}

// baseEnClaro es la nota de la que parte una escritura: la del servicio, con el título y
// el cuerpo EN CLARO si estaba cifrada y se ha abierto (en `Crudo` viajan vacíos).
func baseEnClaro(n Nota) api.Note {
	b := n.Crudo
	if b.ContentCipher != "" && !n.Cifrada {
		b.Title, b.Content, b.Data, b.ContentCipher = n.Titulo, n.Contenido, n.Datos, ""
	}
	return b
}

// entradaEditada es el cuerpo del PUT: la nota ENTERA con el parche (el PUT sustituye) y,
// en un cuaderno cifrado, el título y el cuerpo dentro del criptograma. Cerrado, solo se
// puede cambiar lo que no es texto (fijar, plegar, marcar), que viaja con el criptograma
// que ya tenía; tocar el texto sin la clave es `errBloqueado`.
func (p *Pantalla) entradaEditada(n Nota, pa Parche) (api.NoteInput, error) {
	if n.Cifrada {
		if pa.Titulo != nil || pa.Contenido != nil || pa.Datos != nil {
			return api.NoteInput{}, errBloqueado
		}
		in := pa.sobre(n.Crudo).Input()
		in.ExpectedCrypto = p.activo.Crudo.Crypto
		in.Cancelled = pa.Cancelada
		return in, nil
	}
	in := pa.sobre(baseEnClaro(n)).Input()
	// `Input()` no lleva la cancelación (nil = «como está»): solo viaja si se cambia.
	in.Cancelled = pa.Cancelada
	if err := p.cifrar(&in); err != nil {
		return api.NoteInput{}, err
	}
	return in, nil
}

// colocar es el `moverNota` de la web: cambia de sitio una nota DENTRO de su cuaderno
// —otra columna, otra posición, otro grupo—.
func (p *Pantalla) colocar(id string, pos api.NotePosition, conHistorial bool) error {
	var (
		n     Nota
		antes []Nota
		c     string
		listo bool
	)
	fyne.DoAndWait(func() {
		var ok bool
		if n, ok = p.nota(id); !ok || !p.puedeEscribir() {
			return
		}
		antes, c, listo = p.notas, p.activo.ID, true
		movida := n
		movida.X, movida.Y, movida.Col, movida.Posicion = pos.X, pos.Y, pos.Col, pos.Position
		movida.Grupo = ""
		if pos.GroupID != nil {
			movida.Grupo = *pos.GroupID
		}
		movida.Crudo = enPosicion(n.Crudo, pos)
		p.reemplazar(movida)
		p.Repintar()
	})
	if !listo {
		return nil
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	err := p.svc.MoveNote(ctx, id, pos)
	enHilo(func() {
		if p.activo.ID != c {
			return
		}
		if err != nil {
			p.notas = antes
			p.fallo("note.saveFailed", err)
			p.Repintar()
			return
		}
		if conHistorial {
			sitio := posicionDe(n.Crudo)
			p.hist.Registrar(Accion{
				Etiqueta: p.tr.T("history.move", nil),
				Deshacer: func() error { return p.colocar(id, sitio, false) },
				Rehacer:  func() error { return p.colocar(id, pos, false) },
			}, "")
			p.Repintar()
		}
	})
	return err
}

// posicionDe es dónde está una nota ahora, como punto de partida de un movimiento.
func posicionDe(n api.Note) api.NotePosition {
	return api.NotePosition{X: n.X, Y: n.Y, Col: n.Col, Position: n.Position, GroupID: n.GroupID}
}

func enPosicion(n api.Note, pos api.NotePosition) api.Note {
	n.X, n.Y, n.Col, n.Position, n.GroupID = pos.X, pos.Y, pos.Col, pos.Position, pos.GroupID
	return n
}

// borrar es el `borrarNota` de la web: a la papelera, y deshacer la saca de ella con el
// mismo ID y en el mismo sitio.
func (p *Pantalla) borrar(id string, conHistorial bool) error {
	var (
		n     Nota
		antes []Nota
		c     string
		listo bool
	)
	fyne.DoAndWait(func() {
		var ok bool
		if n, ok = p.nota(id); !ok || !p.puedeEscribir() {
			return
		}
		antes, c, listo = p.notas, p.activo.ID, true
		p.quitar(id)
		p.Repintar()
	})
	if !listo {
		return nil
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	err := p.svc.DeleteNote(ctx, id)
	enHilo(func() {
		if p.activo.ID != c {
			return
		}
		if err != nil {
			p.ponerNotas(antes)
			p.fallo("note.saveFailed", err)
			p.Repintar()
			return
		}
		if conHistorial {
			p.hist.Registrar(Accion{
				Etiqueta: p.tr.T("history.delete", nil),
				Deshacer: func() error { return p.llamar(id, p.svc.RestoreNote, func() { p.añadir(n) }) },
				Rehacer:  func() error { return p.borrar(id, false) },
			}, "")
			p.Repintar()
		}
	})
	return err
}

// familia son la nota y todo lo agrupado bajo ella, a cualquier nivel: lo que viaja
// junto al cambiar de cuaderno.
func familia(notas []Nota, raiz string) map[string]bool {
	f := map[string]bool{raiz: true}
	for cambio := true; cambio; {
		cambio = false
		for _, n := range notas {
			if n.Grupo != "" && f[n.Grupo] && !f[n.ID] {
				f[n.ID], cambio = true, true
			}
		}
	}
	return f
}

// moverACuaderno es el `moverACuaderno` de la web: la nota y sus agrupadas desaparecen
// de aquí en el acto y el servidor las coloca al final del destino.
func (p *Pantalla) moverACuaderno(id string, destino Cuaderno, conHistorial bool) error {
	var (
		origen Cuaderno
		antes  []Nota
		fam    map[string]bool
		listo  bool
	)
	fyne.DoAndWait(func() {
		if _, ok := p.nota(id); !ok || !p.puedeEscribir() {
			return
		}
		origen, antes, listo = p.activo, p.notas, true
		fam = familia(p.notas, id)
		p.ponerNotas(slices.DeleteFunc(slices.Clone(p.notas), func(n Nota) bool { return fam[n.ID] }))
		p.contar(origen.ID, -len(fam))
		p.contar(destino.ID, len(fam))
		p.Repintar()
	})
	if !listo {
		return nil
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	err := p.svc.MoveNoteToNotebook(ctx, id, destino.ID)
	enHilo(func() {
		if err != nil {
			if p.activo.ID == origen.ID {
				p.ponerNotas(antes)
			}
			p.contar(origen.ID, len(fam))
			p.contar(destino.ID, -len(fam))
			p.fallo("move.failed", err)
			p.Repintar()
			return
		}
		p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito,
			p.tr.T("move.done", i18n.Vars{"name": destino.Titulo}), ""))
		if conHistorial {
			p.hist.Registrar(Accion{
				Etiqueta: p.tr.T("history.move", nil),
				Deshacer: func() error {
					return p.llamar(id, func(ctx context.Context, id string) error {
						return p.svc.MoveNoteToNotebook(ctx, id, origen.ID)
					}, func() {
						if p.activo.ID != origen.ID {
							return
						}
						for _, n := range antes {
							if fam[n.ID] {
								p.añadir(n)
							}
						}
					})
				},
				Rehacer: func() error { return p.moverACuaderno(id, destino, false) },
			}, "")
		}
		p.Repintar()
	})
	return err
}

// contar suma al contador de un cuaderno del árbol. El del abierto se deriva de las
// notas cargadas (`Lateral.cuantas`); este es el de los demás.
func (p *Pantalla) contar(id string, delta int) {
	for i := range p.cuadernos {
		if p.cuadernos[i].ID == id {
			p.cuadernos[i].Notas = max(0, p.cuadernos[i].Notas+delta)
		}
	}
}

// duplicar es el `duplicarNota` de la web: una copia al lado, con sus datos y enlaces.
func (p *Pantalla) duplicar(n Nota) error {
	c := baseEnClaro(n)
	return p.crear(api.NoteInput{
		Kind: c.Kind, Title: c.Title, Content: c.Content, Data: c.Data, Links: c.Links,
		Col: ptr(c.Col), X: ptr(c.X + 20), Y: ptr(c.Y + 20),
	})
}

// nuevaDentro crea una nota agrupada bajo otra (`tree.newInside`).
func (p *Pantalla) nuevaDentro(padre Nota) error {
	return p.crear(api.NoteInput{Kind: "text", GroupID: ptr(padre.ID)})
}

// sacar saca una nota de su grupo sin moverla de sitio (`tree.unnest`).
func (p *Pantalla) sacar(n Nota) error {
	pos := posicionDe(n.Crudo)
	pos.GroupID = nil
	return p.colocar(n.ID, pos, true)
}

// menuNota es el «…» de una nota, con las entradas y el orden de NoteCard.tsx; con
// `enArbol`, el de la fila de NoteTree.tsx, que es el mismo con «Renombrar» en su clave
// del árbol.
//
// «Compartir en Rooms» va entre «Exportar» y «Qué apunta aquí», como en NoteCard.tsx y
// NoteTree.tsx, si hay con qué (suite.go).
func (p *Pantalla) menuNota(n Nota, enArbol bool) *ui.Menu {
	t := p.tr.T
	var el []fyne.CanvasObject
	entrada := func(clave string, f func()) *ui.ElementoMenu {
		e := ui.NuevoElementoMenu(t(clave, nil), iconos.Ninguno, f)
		el = append(el, e)
		return e
	}
	entrada("note.setIcon", func() { p.dialogoIcono(n) })
	entrada("tags.apply", func() { p.aplicarEtiquetas(n) })
	if p.hayRed() && p.svc != nil {
		entrada("reminder.set", func() { p.dialogoRecordatorio(n) })
	}
	fijar := "note.pin"
	if n.Fijada {
		fijar = "note.unpin"
	}
	entrada(fijar, func() { lanzar(func() error { return p.editar(n.ID, Parche{Fijada: ptr(!n.Fijada)}, true) }) })
	if enArbol {
		entrada("tree.rename", func() { p.renombrarEnArbol(n) })
	} else {
		entrada("note.rename", func() { p.renombrar(n) })
	}
	// Duplicar y crear dentro escriben TEXTO, y en un cuaderno cifrado y cerrado no
	// hay con qué cifrarlo: la web pide la contraseña, y aquí no se puede abrir.
	entrada("note.duplicate", func() { lanzar(func() error { return p.duplicar(n) }) }).Inerte = p.bloqueado
	entrada("tree.newInside", func() { lanzar(func() error { return p.nuevaDentro(n) }) }).Inerte = p.bloqueado
	if n.Grupo != "" {
		entrada("tree.unnest", func() { lanzar(func() error { return p.sacar(n) }) })
	}
	// Mover y exportar no se ofrecen en un cuaderno cifrado —la clave no viaja al
	// destino ni al fichero— ni, mover, si no hay otro cuaderno al que ir.
	if !p.activo.Cifrado && len(p.cuadernos) > 1 {
		entrada("note.moveTo", func() { p.dialogoMover(n) })
	}
	// «Añadir al calendario», como NoteCard.tsx: fuera de un cifrado, con escritura y
	// con Calendar a mano.
	if !p.activo.Cifrado && p.puedeEscribir() && p.Suite != nil && p.Suite.Calendar != nil {
		entrada("event.create", func() { p.crearEvento(n) })
	}
	if !p.activo.Cifrado {
		entrada("note.export", func() { p.dialogoExportarNota(n) })
	}
	if n.Tipo == "todo" {
		if !n.Terminada() && !p.activo.Cifrado {
			entrada("task.postpone", func() { p.dialogoPosponer(n.Crudo) })
		}
		cancelar := "task.cancel"
		if n.Cancelada {
			cancelar = "task.reopen"
		}
		entrada(cancelar, func() {
			lanzar(func() error { return p.editar(n.ID, Parche{Cancelada: ptr(!n.Cancelada)}, true) })
		})
	}
	if p.puedeCompartirNota(n) {
		entrada("sharePeople.open", func() { p.compartirNota(n) })
	}
	if p.ofreceCompartir() {
		entrada("share.toRoom", func() { p.compartirEnSala(n) })
	}
	entrada("backlinks.title", func() { p.dialogoRetroenlaces(n) })
	if !enArbol {
		entrada("note.attach", func() { p.adjuntar(n) })
	}
	el = append(el, ui.NuevoSeparadorMenu())
	entrada("note.delete", func() { lanzar(func() error { return p.borrar(n.ID, true) }) }).Peligro = true

	m := ui.NuevoMenu(el...)
	m.Rotulo = t("common.more", nil)
	return m
}

// renombrar abre el campo del título en la tarjeta: Intro o salir guardan, Escape
// descarta. Un título vacío SÍ vale aquí —es quitarlo—, como en la web.
func (p *Pantalla) renombrar(n Nota) {
	c := NuevoCampoEnLinea(n.Titulo, p.tr.T("note.titlePlaceholder", nil), PYCampoTarjeta, "surface", TopeTitulo)
	cerrar := func() {
		p.lienzo.Renombrando, p.lienzo.CampoTitulo = "", nil
		p.Repintar()
	}
	c.AlConfirmar = func(s string) {
		nuevo := strings.TrimSpace(s)
		cerrar()
		if nuevo != n.Titulo {
			lanzar(func() error { return p.editar(n.ID, Parche{Titulo: &nuevo}, true) })
		}
	}
	c.AlCancelar = cerrar
	p.lienzo.Renombrando, p.lienzo.CampoTitulo = n.ID, c
	p.Repintar()
	p.enfocar(c.Editor())
}

// renombrarEnArbol es lo mismo en la fila del árbol, donde el campo trae el rótulo que
// se ve —«Nueva nota» si no tiene— todo marcado, y un título en blanco NO se guarda: la
// fila volvería a decir «Nueva nota» y parecería que se ha perdido.
func (p *Pantalla) renombrarEnArbol(n Nota) {
	actual := n.Titulo
	if actual == "" {
		actual = p.tr.T("note.new", nil)
	}
	c := NuevoCampoEnLinea(actual, "", PYCampoArbol, "bg", 0)
	cerrar := func() {
		p.arbol.Renombrando, p.arbol.Campo = "", nil
		p.Repintar()
	}
	c.AlConfirmar = func(s string) {
		nuevo := strings.TrimSpace(s)
		cerrar()
		if nuevo != "" && nuevo != actual {
			lanzar(func() error { return p.editar(n.ID, Parche{Titulo: &nuevo}, true) })
		}
	}
	c.AlCancelar = cerrar
	p.arbol.Renombrando, p.arbol.Campo = n.ID, c
	p.Repintar()
	p.enfocar(c.Editor())
	c.SeleccionarTodo()
}

// enfocar da el foco a algo de la ventana, si la hay.
func (p *Pantalla) enfocar(f fyne.Focusable) {
	if p.win != nil {
		p.win.Canvas().Focus(f)
	}
}

// exportar baja el cuaderno —o una nota, con `nota`— en un formato.
//
// En la web es navegar a la URL: el navegador deja el fichero en Descargas. Aquí es lo
// mismo con `ficheros`, que sabe cuál es la carpeta de descargas de cada sistema y no
// pisa lo que ya haya. Ver el PDF lo abre con la aplicación del sistema, que es el
// proceso aparte que en la web es el visor del navegador.
func (p *Pantalla) exportar(cuaderno, nota, formato string, ver bool) {
	if p.svc == nil || cuaderno == "" {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	cuerpo, nombre, err := p.svc.ExportNotebook(ctx, cuaderno, nota, formato)
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("error.loadFailed", nil), Motivo(err))
		return
	}
	defer func() { _ = cuerpo.Close() }()
	if !ver {
		p.descargar(nombre, cuerpo)
		return
	}
	// Carpeta NUEVA y solo de quien la crea: con un nombre fijo en el /tmp compartido,
	// otra cuenta del equipo la preparaba antes y leía lo exportado (NAT-13).
	dir, err := os.MkdirTemp("", "sytue-notepads-")
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("error.loadFailed", nil), "")
		return
	}
	ruta, err := ficheros.Guardar(dir, nombre, cuerpo)
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("error.loadFailed", nil), "")
		return
	}
	// ponytail: en móvil `ficheros.Abrir` no está (ErrNoDisponible) y el PDF se queda en
	// la carpeta temporal; toca el visor del sistema de cada móvil.
	_ = ficheros.Abrir(ruta)
}

// descargar deja un fichero en la carpeta de descargas, sin pisar nada, y dice cómo ha
// quedado: es lo que el navegador hace con una descarga.
func (p *Pantalla) descargar(nombre string, cuerpo io.Reader) {
	carpeta, err := ficheros.Carpeta()
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("error.loadFailed", nil), "")
		return
	}
	ruta, err := ficheros.Guardar(carpeta, nombre, cuerpo)
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("error.loadFailed", nil), "")
		return
	}
	p.avisar(ui.AvisoExito, filepath.Base(ruta), "")
}

// guardarCuaderno es el `guardarCuaderno` de la web: crea (`id` vacío) o cambia, vuelve
// a cargar el árbol y abre el cuaderno guardado.
func (p *Pantalla) guardarCuaderno(id string, in api.NotebookInput) error {
	if p.svc == nil || !p.hayRed() {
		return nil
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	var (
		nb  api.Notebook
		err error
	)
	if id == "" {
		nb, err = p.svc.CreateNotebook(ctx, in)
	} else {
		nb, err = p.svc.UpdateNotebook(ctx, id, in)
	}
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("note.saveFailed", nil), Motivo(err))
		return err
	}
	c := DeCuaderno(nb)
	enHilo(func() {
		if c.ID == p.activo.ID {
			// El MISMO cuaderno: se cambian sus ajustes sin volver a cargar sus notas,
			// que es lo que hace la web (su efecto de carga mira `activo.id`).
			p.activo = c
			p.Repintar()
			return
		}
		p.abrirCuaderno(c)
	})
	p.cargarCuadernos()
	return nil
}

// borrarCuaderno manda el cuaderno y su rama a la papelera, y abre el primero que quede.
func (p *Pantalla) borrarCuaderno(c Cuaderno) {
	if p.svc == nil || !p.hayRed() {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := p.svc.DeleteNotebook(ctx, c.ID); err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("note.saveFailed", nil), Motivo(err))
		return
	}
	p.olvidarClave(c.ID) // su clave recordada ya no abre nada
	enHilo(func() {
		if p.activo.ID == c.ID {
			p.activo, p.notas, p.lienzo.Seleccionada = Cuaderno{}, nil, ""
		}
	})
	p.cargarCuadernos()
}

// deshacer y rehacer son los botones de la barra y Ctrl+Z / Ctrl+Mayús+Z.
func (p *Pantalla) deshacer() { p.historia(p.hist.Deshacer, "history.undone") }
func (p *Pantalla) rehacer()  { p.historia(p.hist.Rehacer, "history.redone") }

func (p *Pantalla) historia(f func() (string, error), hecho string) {
	etiqueta, err := f()
	enHilo(func() {
		switch {
		case err != nil:
			p.fallo("note.saveFailed", err)
		case etiqueta != "":
			p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito,
				p.tr.T(hecho, i18n.Vars{"action": etiqueta}), ""))
		}
		p.Repintar()
	})
}
