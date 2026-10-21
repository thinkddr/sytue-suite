// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package notas es lo que Notepads guarda en el equipo para no perderlo sin red —la
// cola de capturas y los borradores de la captura de cada cuaderno— y cómo se sube.
//
// Vive en el núcleo y no en la app porque lo necesitan las trece: cerrar sesión, en
// cualquiera de ellas, avisa de lo que haya sin subir, ofrece subirlo y después lo
// borra del disco (NAT-08, decisión del dueño del 2026-10-16). La app de Notepads lo
// usa igual que antes; esto es su cola, no una copia.
package notas

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// Los espacios del almacén van con guiones y no con puntos: con forma de clave, el
// guardián de `catalogos` los buscaría en los mensajes.
const (
	EspacioCapturas   = "notes-capturas"
	EspacioBorradores = "notes-borradores"
	// PrefijoDia es el cuaderno de un día que aún no existe en el servicio: sin red se
	// captura en él y se crea al enviar (POST /daily).
	PrefijoDia = "dia:"
)

// Captura es la `CapturaPendiente` de cola.ts, con los mismos nombres JSON.
type Captura struct {
	ClientID  string        `json:"client_id"`
	Cuaderno  string        `json:"notebook_id,omitempty"`
	Fecha     string        `json:"date,omitempty"`
	Titulo    string        `json:"title,omitempty"`
	In        api.NoteInput `json:"input"`
	At        int64         `json:"at"`
	Rechazada bool          `json:"rechazada,omitempty"`
}

// Envio es lo que la cola necesita del servicio.
type Envio struct {
	Crear  func(ctx context.Context, cuaderno string, in api.NoteInput) (api.Note, error)
	Diario func(ctx context.Context, date, title string, carry bool) (api.Notebook, error)
	Hoy    string
}

// EnvioDe es el Envio de un cliente de Notepads.
func EnvioDe(n *api.Notes, hoy string) Envio {
	return Envio{Crear: n.CreateNote, Diario: n.DailyNotebook, Hoy: hoy}
}

// EsRechazo es un 4xx que reintentar no arregla; 401, 408 y 429 sí se arreglan.
func EsRechazo(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && e.Status >= 400 && e.Status < 500 &&
		e.Status != http.StatusUnauthorized && e.Status != http.StatusRequestTimeout &&
		e.Status != http.StatusTooManyRequests
}

// EsFalloDeRed es un error sin respuesta del servicio: lo único que se encola al vuelo.
func EsFalloDeRed(err error) bool {
	var e *api.Error
	return err != nil && !errors.As(err, &e)
}

// Pendientes es la cola de un dueño, la más antigua primero.
func Pendientes(st *offline.Store, dueño string) []Captura {
	rs, err := st.List(dueño, EspacioCapturas)
	if err != nil {
		slog.Warn("notas: no se ha podido leer la cola de capturas", "error", err)
		return nil
	}
	out := make([]Captura, 0, len(rs))
	for _, r := range rs {
		var c Captura
		if json.Unmarshal(r.Value, &c) == nil {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b Captura) int { return int(a.At - b.At) })
	return out
}

// Vaciar envía en orden. Para en el primer fallo que no sea un rechazo (el resto
// fallaría igual) y aparta las rechazadas. Devuelve los cuadernos que han recibido notas
// y, con PrefijoDia, los días provisionales que ya existen.
func Vaciar(ctx context.Context, st *offline.Store, dueño string, e Envio) map[string]bool {
	tocados := map[string]bool{}
	for _, c := range Pendientes(st, dueño) {
		if c.Rechazada {
			continue
		}
		nb := c.Cuaderno
		var err error
		if nb == "" {
			fecha := cmp.Or(c.Fecha, e.Hoy)
			var dia api.Notebook
			if dia, err = e.Diario(ctx, fecha, c.Titulo, fecha == e.Hoy); err == nil {
				nb = dia.ID
				tocados[PrefijoDia+fecha] = true
			}
		}
		if err == nil {
			_, err = e.Crear(ctx, nb, c.In)
		}
		if err == nil {
			tocados[nb] = true
			_ = st.Delete(dueño, EspacioCapturas, c.ClientID)
			continue
		}
		if !EsRechazo(err) {
			break
		}
		c.Rechazada = true
		_ = st.Write(dueño, EspacioCapturas, c.ClientID, c)
	}
	return tocados
}

// Subir es lo que se hace al cerrar sesión con «Subir»: cada borrador pasa a la cola
// como la captura que habría hecho pulsar Intro en su cuaderno, y la cola se envía.
// Devuelve cuántas quedan sin subir de ese dueño (sin red, o rechazadas).
func Subir(ctx context.Context, st *offline.Store, dueño string, e Envio, nuevoID func() string) int {
	if bs, err := st.List(dueño, EspacioBorradores); err == nil {
		for _, b := range bs {
			var texto string
			if json.Unmarshal(b.Value, &texto) != nil {
				continue
			}
			in, ok := CapturaRapida(texto, "")
			if !ok {
				_ = st.Delete(dueño, EspacioBorradores, b.ID)
				continue
			}
			in = in.ConClientID(nuevoID())
			c := Captura{ClientID: in.ClientID(), In: in, At: b.UpdatedAt.UnixMilli()}
			if fecha, ok := strings.CutPrefix(b.ID, PrefijoDia); ok {
				c.Fecha = fecha
			} else {
				c.Cuaderno = b.ID
			}
			if st.Write(dueño, EspacioCapturas, c.ClientID, c) == nil {
				_ = st.Delete(dueño, EspacioBorradores, b.ID)
			}
		}
	}
	Vaciar(ctx, st, dueño, e)
	quedan := len(Pendientes(st, dueño))
	if bs, err := st.List(dueño, EspacioBorradores); err == nil {
		quedan += len(bs)
	}
	return quedan
}

var (
	marcaTarea = regexp.MustCompile(`(?s)^(?:-\s*)?\[([ xX]?)\]\s+(.+)$`)
	// El guion solo vale en UNA línea: un texto pegado con viñetas es un texto.
	marcaGuion = regexp.MustCompile(`^-[ \t]+([^\n]+)$`)
	parrafos   = regexp.MustCompile(`\n{2,}`)
)

// CapturaRapida interpreta los mismos marcadores explícitos que client/diario.ts: `[ ] `,
// `[] `, `[x] ` o `- ` hacen una tarea y `---` solo es un separador.
func CapturaRapida(texto, tipo string) (api.NoteInput, bool) {
	s := strings.TrimSpace(texto)
	if s == "" {
		return api.NoteInput{}, false
	}
	if s == "---" {
		return api.NoteInput{Kind: "text", Content: "<hr>"}, true
	}
	m := marcaTarea.FindStringSubmatch(s)
	var g []string
	if m == nil {
		g = marcaGuion.FindStringSubmatch(s)
	}
	if tipo == "todo" || m != nil || g != nil {
		titulo, hecha := s, false
		switch {
		case m != nil:
			titulo, hecha = strings.TrimSpace(m[2]), strings.TrimSpace(m[1]) != ""
		case g != nil:
			titulo = strings.TrimSpace(g[1])
		}
		return api.NoteInput{Kind: "todo", Title: titulo, Checked: &hecha}, titulo != ""
	}
	return InterpretarCaptura(s)
}

// InterpretarCaptura es la nota que crea un texto pegado (`interpretarCaptura` y
// `capturar` de la web).
func InterpretarCaptura(texto string) (api.NoteInput, bool) {
	t := strings.TrimSpace(strings.ReplaceAll(texto, "\r\n", "\n"))
	if t == "" {
		return api.NoteInput{}, false
	}
	// Un texto que es UNA sola URL es un enlace, no un párrafo con una URL dentro. El
	// título es el host, que es lo que se lee de un vistazo en la tarjeta.
	if u, err := url.Parse(t); err == nil && !strings.ContainsFunc(t, esBlanco) &&
		(u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" {
		data, _ := json.Marshal(map[string]string{"url": t})
		return api.NoteInput{Kind: "link", Title: strings.ToLower(u.Hostname()), Data: data}, true
	}
	return api.NoteInput{Kind: "text", Content: TextoAHtml(t)}, true
}

func esBlanco(r rune) bool { return strings.ContainsRune(" \t\n\r\f\v ", r) }

// TextoAHtml: línea en blanco separa párrafos; salto simple es `<br>`.
func TextoAHtml(t string) string {
	var b strings.Builder
	for _, p := range parrafos.Split(t, -1) {
		b.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(strings.TrimSpace(p)), "\n", "<br>") + "</p>")
	}
	return b.String()
}
