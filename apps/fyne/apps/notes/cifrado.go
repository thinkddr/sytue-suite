// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: los cuadernos CIFRADOS —PassphraseDialog.tsx y lo que NotesApp.tsx hace
// con la clave—. El cifrado es el de `apps/nucleo/api` (notes_cifrado.go), el mismo que
// el del navegador: un cuaderno cifrado allí se abre aquí.
//
// LA CLAVE vive solo en memoria y solo mientras el cuaderno está abierto: al cambiar de
// cuaderno, al bloquearlo o al cerrar la app se olvida, y con ella todo lo descifrado.

// MinimoContrasena es el `MINIMO` de PassphraseDialog.tsx.
const MinimoContrasena = 10

// deNota convierte una nota del servicio y, si es de un cuaderno cifrado y hay clave, la
// descifra. Una que no abra con esta clave se queda cerrada: no es un error de la pantalla.
func (p *Pantalla) deNota(n api.Note) Nota {
	if !p.activo.Cifrado {
		return DeNota(n)
	}
	vista := n
	if p.clave == nil {
		vista.Title, vista.Content, vista.Data = "", "", nil
		o := DeNota(vista)
		o.Cifrada = true
		o.Crudo = n
		return o
	}
	claro, _, err := api.AbrirContenidoNota(p.clave, n)
	if err != nil {
		vista.Title, vista.Content, vista.Data = "", "", nil
		o := DeNota(vista)
		o.Cifrada = true
		o.Crudo = n
		return o
	}
	vista.Title, vista.Content, vista.ContentCipher, vista.Data = claro.Title, claro.Content, "", claro.Data
	o := DeNota(vista)
	o.Crudo = n
	return o
}

func (p *Pantalla) deNotas(ns []api.Note) []Nota {
	out := make([]Nota, len(ns))
	for i, n := range ns {
		out[i] = p.deNota(n)
	}
	return out
}

// errBloqueado es una escritura de texto en un cuaderno cifrado sin su clave.
var errBloqueado = errors.New("notes: el cuaderno cifrado está cerrado")

// cifrar prepara el cuerpo de una nota para un cuaderno cifrado: título y contenido
// viajan DENTRO del criptograma y las dos columnas van vacías (`conCifrado` de la web).
// En un cuaderno sin cifrar no toca nada.
func (p *Pantalla) cifrar(in *api.NoteInput) error {
	if !p.activo.Cifrado {
		return nil
	}
	if p.clave == nil {
		return errBloqueado
	}
	if err := api.CifrarContenidoNota(p.clave, in); err != nil {
		return err
	}
	in.ExpectedCrypto = bytes.Clone(p.activo.Crudo.Crypto)
	return nil
}

// desbloquear abre el cuaderno cifrado activo: con la clave recordada si su dueño lo
// pidió y lo verifica el sistema (recordar.go), y si no con su contraseña, que vale
// siempre.
func (p *Pantalla) desbloquear() {
	c, ok := api.CryptoDe(p.activo.Crudo.Crypto)
	if !ok {
		return
	}
	id, epoch, generacion := p.activo.ID, p.cryptoEpoch, p.generacionCrypto()
	vigente := func() bool { return p.activo.ID == id && p.cryptoEpoch == epoch && p.generacionCrypto() == generacion }
	if p.Recordar == nil {
		p.pedirContrasena(c, id, false)
		return
	}
	go func() {
		if clave, recordada := p.conRecordada(c, id); clave != nil {
			enHilo(func() {
				if vigente() {
					p.abrirCon(id, clave)
				}
			})
		} else {
			enHilo(func() {
				if vigente() {
					p.pedirContrasena(c, id, recordada)
				}
			})
		}
	}()
}

// pedirContrasena es el diálogo de la contraseña. Con Recordar, lleva además la casilla
// de recordarla en este equipo, marcada si ya lo estaba: desmarcarla la olvida.
func (p *Pantalla) pedirContrasena(c api.NotesCrypto, id string, recordada bool) {
	epoch, generacion := p.cryptoEpoch, p.generacionCrypto()
	vigente := func() bool { return p.activo.ID == id && p.cryptoEpoch == epoch && p.generacionCrypto() == generacion }
	var extra fyne.CanvasObject
	// La casilla se pulsa en la rutina de la interfaz y se lee en la de la derivación.
	var recordar atomic.Bool
	recordar.Store(recordada)
	if p.Recordar != nil {
		casilla := ui.NuevaCasilla(p.tr.T("shell.lock.rememberNotebook", i18n.Vars{"metodo": p.Recordar.Metodo}), recordar.Store)
		casilla.Marcada = recordada
		extra = casilla
	}
	p.dialogoClave(false, extra, func(pass string) error {
		clave, err := api.AbrirCifradoNotas(c, pass)
		if err != nil {
			return err
		}
		seguir := false
		fyne.DoAndWait(func() { seguir = vigente() })
		if !seguir {
			clear(clave)
			return context.Canceled
		}
		if p.Recordar != nil {
			p.recordarClave(id, clave, recordar.Load())
		}
		enHilo(func() {
			if vigente() {
				p.abrirCon(id, clave)
			} else {
				clear(clave)
			}
		})
		return nil
	})
}

// abrirCon descifra las notas del cuaderno `id` con su clave, si sigue siendo el abierto:
// la verificación va en su rutina, y entretanto se ha podido cambiar de cuaderno.
func (p *Pantalla) abrirCon(id string, clave []byte) {
	if p.activo.ID != id {
		return
	}
	propia := bytes.Clone(clave)
	p.olvidarClaveActiva()
	p.clave, p.bloqueado = propia, false
	crudas := make([]api.Note, len(p.notas))
	for i, n := range p.notas {
		crudas[i] = n.Crudo
	}
	p.notas = p.deNotas(crudas)
	// La nota vinculada de un enlace esperaba a que el cuaderno se abriera: la web la
	// crea en cuanto `bloqueado` pasa a falso.
	p.migrarContenido()
	if !p.migrando {
		p.aplicarEntrante()
	}
	p.Repintar()
}

// olvidarClaveActiva limpia la copia que posee la pantalla, exclusivamente en el
// hilo de la interfaz. Las operaciones pendientes poseen sus propias copias y las
// limpian al terminar; nunca se modifica el slice entregado por el recordador.
func (p *Pantalla) olvidarClaveActiva() {
	clear(p.clave)
	p.clave = nil
}

// bloquear olvida la clave y vuelve a cerrar lo descifrado.
func (p *Pantalla) bloquear() {
	p.cancelarMigracion()
	p.cerrarEditor()
	p.creando = false
	p.olvidarClaveActiva()
	p.bloqueado = p.activo.Cifrado
	crudas := make([]api.Note, len(p.notas))
	for i, n := range p.notas {
		crudas[i] = n.Crudo
	}
	p.notas = p.deNotas(crudas)
	p.Repintar()
}

// dialogoClave es PassphraseDialog.tsx: `crear` pide la contraseña dos veces (y al menos
// diez caracteres) para cifrar; si no, una para abrir. `probar` deriva la clave —medio
// segundo de PBKDF2, en su rutina— y devuelve `api.ErrCifrado` si no es la buena.
//
// `extra` va debajo de la contraseña, si no es nil: la casilla de recordarla.
func (p *Pantalla) dialogoClave(crear bool, extra fyne.CanvasObject, probar func(pass string) error) {
	if p.win == nil {
		return
	}
	t := p.tr.T
	v := p.variante()
	titulo, pista, accion := t("crypto.unlock", nil), t("crypto.unlockHint", nil), t("crypto.unlock", nil)
	if crear {
		titulo, pista, accion = t("crypto.title", nil), t("crypto.hint", nil), t("crypto.enable", nil)
	}
	pass := ui.NuevaEntrada("")
	pass.Contrasena(true)
	repite := ui.NuevaEntrada("")
	repite.Contrasena(true)
	ok := ui.NuevoBoton(accion, nil)
	ok.Inerte = true
	cancelar := ui.NuevoBoton(t("common.cancel", nil), nil).Con(ui.Fantasma, ui.TallaMD)
	// Una `Columna` y no un `Stack`: pasa el ancho a la pista al medir (`ui.AltoA`) y la
	// pista parte de línea al ancho de verdad. A 352 fijos se cortaba en el teléfono.
	cuerpo := container.New(ui.Columna{})
	var d *ui.Dialogo
	pintar := func(err string) {
		piezas := []fyne.CanvasObject{
			ui.NuevoParrafo(pista, tema.TextoBase, ui.Normal, "text-muted", v),
			ui.GrupoCampo(t("crypto.passphrase", nil), false, pass, "", err),
		}
		if crear {
			piezas = append(piezas, ui.GrupoCampo(t("crypto.passphraseRepeat", nil), false, repite, "", ""))
		}
		if extra != nil {
			piezas = append(piezas, extra)
		}
		cuerpo.Objects = []fyne.CanvasObject{container.New(ui.Columna{Hueco: 3 * 0.25 * tema.Rem}, piezas...)}
		cuerpo.Refresh()
		if d != nil {
			d.Refresh()
		}
	}
	pass.AlCambiar(func(s string) { ok.Inerte = s == ""; ok.Refresh() })
	enviar := func() {
		if ok.Inerte || ok.Cargando {
			return
		}
		if crear && len([]rune(pass.Texto())) < MinimoContrasena {
			pintar(t("crypto.tooShort", nil))
			return
		}
		if crear && pass.Texto() != repite.Texto() {
			pintar(t("crypto.mismatch", nil))
			return
		}
		ok.Cargando = true
		ok.Refresh()
		secreto := pass.Texto()
		go func() {
			err := probar(secreto)
			enHilo(func() {
				ok.Cargando = false
				ok.Refresh()
				if err != nil {
					pintar(t("crypto.wrong", nil))
					return
				}
				d.Cerrar()
			})
		}()
	}
	ok.AlPulsar = enviar
	intro := func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			enviar()
			return true
		}
		return false
	}
	pass.AlTeclear, repite.AlTeclear = intro, intro
	pintar("")
	// Los botones en el pie del diálogo, como en la web: a todo el ancho en el teléfono.
	d = ui.NuevoDialogo(titulo, cuerpo).Con(ui.DialogoSM, container.New(ui.Fila{Hueco: HuecoFilaDialogo}, cancelar, ok))
	d.Foco = pass
	cancelar.AlPulsar = d.Cerrar
	p.mostrar(d)
}

// cifrarCuaderno es encender el interruptor de cifrado en los ajustes: la contraseña dos
// veces y los parámetros nuevos, que se guardan con el cuaderno.
func (p *Pantalla) cifrarCuaderno(listo func(crypto json.RawMessage)) {
	p.dialogoClave(true, nil, func(pass string) error {
		c, _, err := api.NuevoCifradoNotas(pass)
		if err != nil {
			return err
		}
		crudo, err := json.Marshal(c)
		if err != nil {
			return err
		}
		// La clave NO se queda: como en la web, el cuaderno recién cifrado se abre
		// cerrado y se desbloquea con la contraseña que se acaba de poner.
		enHilo(func() { listo(crudo) })
		return nil
	})
}

// cancelarMigracion invalida el trabajo pendiente incluso si se vuelve al mismo
// cuaderno o a la misma cuenta. Siempre se llama desde el hilo de la interfaz.
func (p *Pantalla) cancelarMigracion() {
	p.cryptoEpoch++
	if p.migracionCancel != nil {
		p.migracionCancel()
		p.migracionCancel = nil
	}
	p.migrando = false
	p.migracionFallida = false
}

func (p *Pantalla) generacionCrypto() string {
	if p.GeneracionSesion != nil {
		return p.GeneracionSesion()
	}
	return p.claveAlmacen()
}

// cryptoVigente sólo se consulta en el hilo de la interfaz.
func (p *Pantalla) cryptoVigente(epoch uint64, generacion string) bool {
	return p.cryptoEpoch == epoch && p.generacionCrypto() == generacion
}

// ReiniciarSesion descarta también el claro y las respuestas pendientes al cambiar
// la identidad adoptada por Suite, antes de cargar la cuenta nueva.
func (p *Pantalla) ReiniciarSesion() {
	p.cancelarMigracion()
	p.olvidarClaveActiva()
	p.bloqueado = false
	p.creando = false
	p.activo = Cuaderno{}
	p.notas = nil
	p.cuadernos = nil
	p.vista = VistaCuaderno
	p.tareas = estadoTareas{}
	p.notasDeOtros = nil
	p.campoCaptura = nil
	p.borradores = nil
	p.hist = Historial{}
	p.edicion = nil
	p.lienzo.Editor = nil
	p.Repintar()
}

func (p *Pantalla) necesitaMigracion() bool {
	for _, n := range p.notas {
		_, m, err := api.AbrirContenidoNota(p.clave, n.Crudo)
		if m || err != nil {
			return true
		}
	}
	return false
}

func (p *Pantalla) avisoMigracion() fyne.CanvasObject {
	if !p.migrando {
		return nil
	}
	key := "crypto.migrating"
	if p.migracionFallida {
		key = "crypto.migrationFailed"
	}
	return ui.NuevoAviso(ui.AvisoInfo, p.tr.T(key, nil), "")
}

type migracionNotas struct {
	pantalla             *Pantalla
	epoch                uint64
	cuaderno, generacion string
	crypto               json.RawMessage
	clave                []byte
	contexto             context.Context
	cancelar             context.CancelFunc
}

func (p *Pantalla) migrarContenido() {
	p.cancelarMigracion()
	if p.svc == nil || p.activo.Crudo.ReadOnly || p.clave == nil || !p.activo.Cifrado || !p.necesitaMigracion() {
		return
	}
	p.migrando = true
	ctx, cancel := context.WithCancel(context.Background())
	p.migracionCancel = cancel
	m := migracionNotas{pantalla: p, epoch: p.cryptoEpoch, cuaderno: p.activo.ID, generacion: p.generacionCrypto(), crypto: bytes.Clone(p.activo.Crudo.Crypto), clave: bytes.Clone(p.clave), contexto: ctx, cancelar: cancel}
	go m.ejecutar()
}

// vigente se evalúa exclusivamente en el hilo de la interfaz.
func (m *migracionNotas) vigente() bool {
	p := m.pantalla
	return p.cryptoEpoch == m.epoch && p.activo.ID == m.cuaderno && p.generacionCrypto() == m.generacion && bytes.Equal(p.activo.Crudo.Crypto, m.crypto) && p.clave != nil
}

func (m *migracionNotas) puedeContinuar() bool {
	seguir := false
	fyne.DoAndWait(func() { seguir = m.vigente() })
	return seguir && m.contexto.Err() == nil
}

func (m *migracionNotas) ejecutar() {
	defer m.cancelar()
	defer clear(m.clave)
	if !m.puedeContinuar() {
		return
	}
	lectura, cerrar := context.WithTimeout(m.contexto, m.pantalla.Plazo)
	lista, err := m.pantalla.svc.ListNotesForEncryption(lectura, m.cuaderno)
	cerrar()
	if err == nil {
		fyne.DoAndWait(func() {
			if m.vigente() {
				m.pantalla.notas = m.pantalla.deNotas(lista)
			}
		})
		for _, n := range lista {
			if err = m.migrarNota(n); err != nil {
				break
			}
		}
	}
	enHilo(func() { m.terminar(err) })
}

func (m *migracionNotas) migrarNota(n api.Note) error {
	claro, needed, err := api.AbrirContenidoNota(m.clave, n)
	if err != nil || !needed {
		return err
	}
	in := api.NoteInput{Title: claro.Title, Content: claro.Content, Data: claro.Data}
	if err := api.CifrarContenidoNota(m.clave, &in); err != nil {
		return err
	}
	if !m.puedeContinuar() {
		return context.Canceled
	}
	ctx, cancel := context.WithTimeout(m.contexto, m.pantalla.Plazo)
	defer cancel()
	nueva, err := m.pantalla.svc.MigrateEncryptedContent(ctx, n.ID, api.NoteCipherMigration{CipherVersion: 2, ExpectedUpdatedAt: n.UpdatedAt, ExpectedCrypto: m.crypto, ContentCipher: in.ContentCipher, Data: in.Data})
	if err != nil {
		return err
	}
	fyne.DoAndWait(func() { m.aceptar(n, nueva) })
	return nil
}

func (m *migracionNotas) aceptar(old, nueva api.Note) {
	if !m.vigente() {
		return
	}
	p := m.pantalla
	for i, n := range p.notas {
		if n.ID == old.ID && n.Crudo.UpdatedAt.Equal(old.UpdatedAt) {
			p.notas[i] = p.deNota(nueva)
		}
	}
}

func (m *migracionNotas) terminar(err error) {
	if !m.vigente() {
		return
	}
	p := m.pantalla
	p.migracionCancel = nil
	if err != nil {
		// La confirmación puede haberse perdido: conservar original y exigir una
		// lectura nueva al volver a desbloquear; no repetir el POST de forma ciega.
		p.migracionFallida = true
		p.fallo("crypto.migrationFailed", err)
	} else {
		p.migrando = false
		p.aplicarEntrante()
	}
	p.Repintar()
}
