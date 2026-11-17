// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package correo es lo que el redactor de Mail guarda en el equipo para no perder lo
// escrito sin red o tras un cierre forzado: la copia local del redactor, la
// contrapartida de `mail.compose` (`writeOfflineValue`) de ComposeModal.tsx.
//
// Vive en el núcleo y no en la app por lo mismo que `notas`: cerrar sesión, en
// cualquiera de las trece apps, cuenta lo que hay sin subir, ofrece subirlo y después
// lo borra del disco (NAT-08). Por eso la copia lleva ya compuesto el .eml del
// borrador: subirla es un `SaveDraft` y no hace falta el redactor para ello.
package correo

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// EspacioRedactor es el espacio del almacén. Con guion y no con punto, como los de
// `notas`: con forma de clave, el guardián de `catalogos` lo buscaría en los mensajes.
const EspacioRedactor = "mail-redactor"

// TopeAdjuntos es lo más que la copia guarda de adjuntos (en bytes, sumados). La web
// los guarda todos en IndexedDB; aquí cada copia es un fichero cifrado que se reescribe
// en cada autoguardado, y uno de cientos de megas no puede ir y venir del disco cada dos
// segundos. Lo que no cabe se dice (`Fuera`) y se vuelve a adjuntar a mano.
const TopeAdjuntos = 5 << 20

// Copia es un redactor guardado en el equipo. Una por redactor (su `ID` en el almacén),
// no una sola como en la web: abrir otro redactor sin red no pisa lo que dejó el
// anterior.
type Copia struct {
	// Borrador es el estado del redactor como lo serializa la app, SIN los datos de los
	// adjuntos, que ya van dentro de `Crudo`: se leen de ahí al restaurar.
	Borrador json.RawMessage `json:"borrador"`
	// De y Crudo son lo que pide `SaveDraft`: la dirección y el .eml del borrador.
	De    string `json:"de"`
	Crudo []byte `json:"crudo"`
	// Servidor es el borrador de Borradores al que sustituye, o vacío.
	Servidor string `json:"servidor,omitempty"`
	// Fuera es cuántos adjuntos no cupieron en la copia (TopeAdjuntos).
	Fuera int `json:"fuera,omitempty"`
}

// Guardada es una copia leída del almacén, con su id y cuándo se escribió.
type Guardada struct {
	ID string
	Copia
	En int64 // milisegundos
}

// Copias son las del dueño, la más reciente primero. Las que no se pueden leer no
// cuentan aquí (siguen en el disco y cerrar sesión las borra).
func Copias(st *offline.Store, dueño string) []Guardada {
	if st == nil || dueño == "" {
		return nil
	}
	rs, err := st.List(dueño, EspacioRedactor)
	if err != nil {
		slog.Warn("correo: no se han podido leer las copias del redactor", "error", err)
		return nil
	}
	out := make([]Guardada, 0, len(rs))
	for _, r := range rs {
		var c Copia
		if json.Unmarshal(r.Value, &c) == nil {
			out = append(out, Guardada{ID: r.ID, Copia: c, En: r.UpdatedAt.UnixMilli()})
		}
	}
	slices.SortStableFunc(out, func(a, b Guardada) int { return cmp.Compare(b.En, a.En) })
	return out
}

// Guardar escribe (o reescribe) la copia `id` del dueño.
func Guardar(st *offline.Store, dueño, id string, c Copia) error {
	if st == nil || dueño == "" || id == "" {
		return errors.New("correo: sin almacén o sin dueño")
	}
	return st.Write(dueño, EspacioRedactor, id, c)
}

// Borrar quita la copia `id` del dueño. Que no exista no es un error.
func Borrar(st *offline.Store, dueño, id string) {
	if st == nil || dueño == "" || id == "" {
		return
	}
	if err := st.Delete(dueño, EspacioRedactor, id); err != nil {
		slog.Warn("correo: no se ha podido borrar la copia del redactor", "error", err)
	}
}

// Subida es `SaveDraft` de `api.Mail`.
type Subida func(ctx context.Context, id, de string, crudo []byte) (api.MailMessage, error)

// Subir es lo que se hace al cerrar sesión con «Subir»: cada copia del dueño va a
// Borradores y, si llega, se borra del equipo. La que sustituía a un borrador que ya
// no existe (404) se sube como uno nuevo: lo escrito vale más que el hueco. Devuelve
// cuántas quedan de ese dueño (sin red, o rechazadas).
func Subir(ctx context.Context, st *offline.Store, dueño string, subir Subida) int {
	quedan := 0
	for _, c := range Copias(st, dueño) {
		if ctx.Err() != nil {
			quedan++
			continue
		}
		_, err := subir(ctx, c.Servidor, c.De, c.Crudo)
		var e *api.Error
		if err != nil && c.Servidor != "" && errors.As(err, &e) && e.Status == http.StatusNotFound {
			_, err = subir(ctx, "", c.De, c.Crudo)
		}
		if err != nil {
			quedan++
			continue
		}
		Borrar(st, dueño, c.ID)
	}
	return quedan
}
