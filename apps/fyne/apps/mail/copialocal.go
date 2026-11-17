// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
)

// # La copia local del redactor (`LOCAL_DRAFT_NAMESPACE` de ComposeModal.tsx)
//
// Sin red, guardar y enviar fallan; un cierre forzado se lleva lo que el autoguardado
// aún no había subido. Por eso, cada vez que el autoguardado salta —y al cerrar el
// redactor—, lo escrito se guarda también en el almacén cifrado del equipo
// (`apps/nucleo/correo`), y se borra en cuanto el servidor lo tiene: al guardar el
// borrador, al enviar o al programar, y al descartar. «Redactar» recupera la que haya
// y lo dice (`compose.draftRestoredOffline`).
//
// Cerrar sesión la cuenta como pendiente, ofrece subirla a Borradores y después la
// borra con todo lo demás (NAT-08), como hace con lo de Notepads.

// esFalloDeRed es un error que no viene del servicio: no hubo respuesta. Lo que el
// servicio rechaza (`*api.Error`) no se arregla guardándolo en el equipo.
func esFalloDeRed(err error) bool {
	var e *api.Error
	return err != nil && !errors.As(err, &e)
}

// dueñoCopia es persona y organización, el mismo dueño con el que `sesion` sube y
// cuenta lo pendiente al cerrar sesión. Sin sesión, ninguno: no hay copia.
func (b *Buzon) dueñoCopia() string {
	partes := strings.Split(b.identidad, "\x00")
	if len(partes) != 3 || partes[1] == "" {
		return ""
	}
	return partes[1] + "/" + partes[2]
}

// conCopia dice si este buzón guarda copias locales.
func (b *Buzon) conCopia() bool { return b.Almacen != nil && b.dueñoCopia() != "" }

// copiaDe compone la copia de un borrador: el .eml que subiría «Guardar borrador» y el
// estado del redactor sin los datos de los adjuntos, que ya van en el .eml. Los
// adjuntos que pasan de `correo.TopeAdjuntos` no se guardan y se cuentan en `Fuera`.
func (b *Buzon) copiaDe(br Borrador) (correo.Copia, error) {
	c := correo.Copia{Servidor: br.ID}
	dentro, total := make([]eml.Adjunto, 0, len(br.Adjuntos)), 0
	for _, a := range br.Adjuntos {
		if total+len(a.Datos) > correo.TopeAdjuntos {
			c.Fuera++
			continue
		}
		dentro = append(dentro, a)
		total += len(a.Datos)
	}
	br.Adjuntos = dentro
	de, _, crudo, err := b.componer(br, true)
	if err != nil {
		return c, err
	}
	br.Adjuntos, br.Copia = nil, ""
	estado, err := json.Marshal(br)
	if err != nil {
		return c, err
	}
	c.Borrador, c.De, c.Crudo = estado, de, crudo
	return c, nil
}

// guardarCopia deja lo escrito en el equipo. Sin contenido, quita la que hubiera: no
// se recupera un redactor vacío. Devuelve si quedó guardada y cuántos adjuntos no
// cupieron.
func (b *Buzon) guardarCopia(rd *redaccion) (bool, int) {
	if !b.conCopia() {
		return false, 0
	}
	br := rd.r.B
	if !br.ConContenido() {
		b.borrarCopia(br.Copia)
		return false, 0
	}
	if br.Copia == "" {
		br.Copia = rand.Text()
		rd.r.B.Copia = br.Copia
	}
	c, err := b.copiaDe(br)
	if err != nil || correo.Guardar(b.Almacen, b.dueñoCopia(), br.Copia, c) != nil {
		return false, 0
	}
	return true, c.Fuera
}

// borrarCopia quita la copia `id`: el servidor ya tiene lo escrito, o se descartó.
func (b *Buzon) borrarCopia(id string) {
	if b.conCopia() {
		correo.Borrar(b.Almacen, b.dueñoCopia(), id)
	}
}

// copiaEnVuelo aparta la copia `id` de «Redactar» mientras su guardado no conteste.
func (b *Buzon) copiaEnVuelo(id string) {
	if id == "" {
		return
	}
	if b.copiasEnVuelo == nil {
		b.copiasEnVuelo = map[string]bool{}
	}
	b.copiasEnVuelo[id] = true
}

// copiaGuardada es la copia más reciente de quien está dentro, lista para el redactor:
// su estado, sus adjuntos (leídos del .eml) y el borrador del servidor al que
// sustituye. Devuelve también cuántos adjuntos no cupieron.
func (b *Buzon) copiaGuardada() (Borrador, int, bool) {
	if !b.conCopia() {
		return Borrador{}, 0, false
	}
	for _, c := range correo.Copias(b.Almacen, b.dueñoCopia()) {
		if b.copiasEnVuelo[c.ID] {
			continue
		}
		var br Borrador
		if json.Unmarshal(c.Borrador, &br) != nil {
			continue
		}
		if x, err := eml.Leer(bytes.NewReader(c.Crudo)); err == nil {
			br.Adjuntos = adjuntosDe(x.Adjuntos)
		}
		br.ID, br.Copia = c.Servidor, c.ID
		return br, c.Fuera, true
	}
	return Borrador{}, 0, false
}

// restaurarCopia es el efecto de apertura de ComposeModal: «Redactar» abre la copia
// que dejó un redactor sin red o un cierre forzado, y lo dice.
func (b *Buzon) restaurarCopia() bool {
	br, fuera, ok := b.copiaGuardada()
	if !ok {
		return false
	}
	b.abrirRedactor(br)
	b.avisarCopia(ui.AvisoInfo, "compose.draftRestoredOffline", fuera)
	return true
}

// avisarCopia es el aviso de la copia local; si quedaron adjuntos fuera, lo añade.
func (b *Buzon) avisarCopia(tono ui.TonoAviso, clave string, fuera int) {
	t := b.P.T
	desc := ""
	if fuera > 0 {
		desc = t.T("compose.draftAttachmentsNotKept", nil)
	}
	b.P.Avisar(tono, t.T(clave, nil), desc)
}

// quedaSinRed es el envío (o la programación) que no llegó al servicio: lo escrito se
// queda en el equipo y el redactor, abierto, para volver a enviarlo con red
// (`compose.sendOffline`). Devuelve false si el fallo no es de red o no hay copia, y
// entonces el aviso es el de siempre.
func (b *Buzon) quedaSinRed(rd *redaccion, err error) bool {
	if !esFalloDeRed(err) {
		return false
	}
	ok, fuera := b.guardarCopia(rd)
	if ok {
		b.avisarCopia(ui.AvisoInfo, "compose.sendOffline", fuera)
	}
	return ok
}

// guardadoSinRed es el guardado del borrador que no llegó al servicio: lo escrito se
// queda en el equipo (`compose.draftSavedOffline`) y, con el botón, el redactor se
// cierra igual que si hubiera llegado. Devuelve false si el fallo no es de red o no
// hay copia, y entonces el aviso es el de siempre.
func (b *Buzon) guardadoSinRed(rd *redaccion, err error, silencioso, yCerrar bool) bool {
	if !esFalloDeRed(err) {
		return false
	}
	ok, fuera := b.guardarCopia(rd)
	if !ok {
		return false
	}
	if !silencioso {
		b.avisarCopia(ui.AvisoInfo, "compose.draftSavedOffline", fuera)
	}
	if yCerrar {
		b.cerrar(rd)
	}
	return true
}
