// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package auditlog es la lectura admin del registro de auditoría de compliance:
// una página por keyset con filtros, y la exportación CSV del mismo conjunto.
//
// POR QUÉ EXISTE. Estaba escrito DOS VECES, en Sytue Mail y en Sytue Drive, y
// medido: de ~490 líneas solo 36 diferían, y de esas la mayoría eran el nombre
// del paquete y el comentario de cabecera. Ya había empezado a divergir donde
// más se nota: un mensaje de error salía en español en un servicio y en inglés
// en el otro, y el rango vacío se escribía `>=` en uno y `≥` en el otro. Eso es
// una API de cumplimiento contestando distinto según por dónde se entre.
//
// El riesgo real no es el tamaño: es que el filtrado o la paginación se
// arreglen en un sitio y no en el otro, y que dos exportaciones de la misma
// organización no contengan lo mismo. Aquí es un solo objeto.
//
// Lo que NO entra aquí, porque de verdad es de cada servicio: quién puede
// llamar (el guard admin), de dónde salen las filas (su store) y cómo se
// traduce su error de dominio a HTTP.
package auditlog

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

// VentanaMax acota la ventana que se le pide al store. El audit_log de
// compliance es de bajo volumen (holds y búsquedas/exports de eDiscovery), así
// que en la práctica es «toda» la traza de la organización.
//
// ponytail: el filtro y el keyset se aplican EN MEMORIA sobre esta ventana. Si
// la tabla creciera, empujarlos a una consulta del store —una ListAuditPage con
// su filtro, como en Sytue ID— en vez de filtrar aquí.
const VentanaMax = 500

// Entrada es una fila de la traza. Los servicios que la exponen la aliasan desde
// su `domain`, así que los tags JSON de aquí son los que ve el cliente.
type Entrada struct {
	ID         int64           `json:"id"`
	OccurredAt time.Time       `json:"occurred_at"`
	Tenant     string          `json:"-"`
	Actor      string          `json:"actor"`
	Action     string          `json:"action"`
	Target     string          `json:"target,omitempty"`
	Detail     json.RawMessage `json:"detail,omitempty"`
}

// Posicion es la posición keyset (occurred_at, id) DESC de la última fila
// devuelta.
type Posicion struct {
	OccurredAt time.Time
	ID         int64
}

// Consulta son los criterios ya validados del listado o la exportación.
type Consulta struct {
	Action string    // prefijo de acción (sin distinguir mayúsculas); "" = todas
	Actor  string    // subcadena del actor (sin distinguir mayúsculas); "" = todos
	Since  time.Time // occurred_at >= Since; cero = sin límite inferior
	Until  time.Time // occurred_at <  Until; cero = sin límite superior
	Limit  int       // 1..500 (100 por defecto)
	After  *Posicion // posición keyset de la página anterior; nil = primera
	CSV    bool      // ?format=csv => exportación en vez de listado JSON
}

// LeerConsulta extrae y valida los filtros de la query. Rechaza con 400 las
// fechas ilegibles, el rango invertido y el cursor corrupto: servir una página
// engañosa sería peor que fallar, y un 500 escondería un error del cliente.
func LeerConsulta(r *http.Request) (Consulta, error) {
	v := r.URL.Query()
	var q Consulta
	q.Action = strings.TrimSpace(v.Get("action"))
	if len(q.Action) > 100 {
		return q, httpx.ErrBadRequest.WithMessage("action is too long.")
	}
	q.Actor = strings.TrimSpace(v.Get("actor"))
	since, err := LeerFecha(strings.TrimSpace(v.Get("since")))
	if err != nil {
		return q, err
	}
	until, err := leerHasta(strings.TrimSpace(v.Get("until")))
	if err != nil {
		return q, err
	}
	if !since.IsZero() && !until.IsZero() && !since.Before(until) {
		return q, httpx.ErrBadRequest.WithMessage("The date range is empty (since ≥ until).")
	}
	q.Since, q.Until = since, until

	q.Limit = 100
	if s := v.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 500 {
			return q, httpx.ErrBadRequest.WithMessage("Invalid limit (1..500).")
		}
		q.Limit = n
	}
	after, err := LeerCursor(v.Get("cursor"))
	if err != nil {
		return q, httpx.ErrBadRequest.WithMessage("Invalid cursor.")
	}
	q.After = after
	q.CSV = v.Get("format") == "csv"
	return q, nil
}

// LeerFecha acepta RFC3339 o AAAA-MM-DD. La cadena vacía es «sin límite», no un
// error. Se exporta porque el resto de endpoints de compliance de cada servicio
// parsean sus fechas igual.
func LeerFecha(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, httpx.ErrBadRequest.WithMessage("Invalid date (use RFC3339 or YYYY-MM-DD).")
}

// leerHasta parsea el límite superior. Una fecha suelta (AAAA-MM-DD) se toma
// INCLUSIVA del día —avanza al siguiente, porque el filtro usa occurred_at <
// Until—, de modo que «hasta el 10» incluye el día 10; un RFC3339 va tal cual.
func leerHasta(s string) (time.Time, error) {
	t, err := LeerFecha(s)
	if err != nil || t.IsZero() {
		return t, err
	}
	if len(s) == len("2006-01-02") {
		t = t.AddDate(0, 0, 1)
	}
	return t, nil
}

// Filtrar aplica acción, actor y rango a la ventana, que el store ya devuelve
// ordenada (occurred_at, id) DESC.
//
// Reutiliza el array de `entradas` (`entradas[:0]`), así que el slice recibido
// queda pisado: no vale conservarlo para otra cosa después de llamar.
func Filtrar(entradas []Entrada, q Consulta) []Entrada {
	action := buscar.Plegar(q.Action)
	actor := buscar.Plegar(q.Actor)
	out := entradas[:0]
	for _, e := range entradas {
		if action != "" && !strings.HasPrefix(buscar.Plegar(e.Action), action) {
			continue
		}
		if actor != "" && !strings.Contains(buscar.Plegar(e.Actor), actor) {
			continue
		}
		if !q.Since.IsZero() && e.OccurredAt.Before(q.Since) {
			continue
		}
		if !q.Until.IsZero() && !e.OccurredAt.Before(q.Until) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Paginar aplica el cursor keyset y corta a limit, devolviendo el cursor
// siguiente (vacío si no quedan más). `entradas` viene ordenado DESC.
func Paginar(entradas []Entrada, after *Posicion, limit int) ([]Entrada, string) {
	if after != nil {
		i := 0
		for i < len(entradas) {
			e := entradas[i]
			// Primera fila ESTRICTAMENTE anterior a la última vista (orden DESC).
			if e.OccurredAt.Before(after.OccurredAt) ||
				(e.OccurredAt.Equal(after.OccurredAt) && e.ID < after.ID) {
				break
			}
			i++
		}
		entradas = entradas[i:]
	}
	next := ""
	if len(entradas) > limit {
		last := entradas[limit-1]
		next = FormatearCursor(last.OccurredAt, last.ID)
		entradas = entradas[:limit]
	}
	return entradas, next
}

// EscribirCSV exporta el conjunto filtrado con columnas normalizadas para SIEM.
// `servicio` solo nombra el fichero (`audit-<servicio>-AAAAMMDD.csv`).
//
// Las cabeceras salen antes del primer byte; si el writer falla a media
// escritura solo se registra, porque el 200 ya viajó.
func EscribirCSV(w http.ResponseWriter, r *http.Request, servicio string, entradas []Entrada) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="audit-`+servicio+`-`+time.Now().UTC().Format("20060102")+`.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"ts", "action", "actor", "target", "detail"})
	for _, e := range entradas {
		if err := cw.Write([]string{
			e.OccurredAt.UTC().Format(time.RFC3339), validation.CSVSafe(e.Action), validation.CSVSafe(e.Actor),
			validation.CSVSafe(e.Target), validation.CSVSafe(string(e.Detail)),
		}); err != nil {
			log.From(r.Context()).Warn("audit-log CSV: fallo al escribir fila", "error", err)
			return
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		log.From(r.Context()).Warn("audit-log CSV: fallo al vaciar", "error", err)
	}
}

// FormatearCursor codifica la posición keyset como token opaco URL-safe:
// base64url("<RFC3339Nano>|<id>").
func FormatearCursor(ts time.Time, id int64) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(ts.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(id, 10)),
	)
}

// LeerCursor invierte FormatearCursor. Token vacío => primera página (nil).
// Un token corrupto es entrada inválida del cliente (400 en el llamador), no un
// 500.
func LeerCursor(token string) (*Posicion, error) {
	if token == "" {
		return nil, nil //nolint:nilnil // contrato: token vacío ⇒ primera página, no un error.
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	tsRaw, idRaw, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, errors.New("cursor sin separador")
	}
	ts, err := time.Parse(time.RFC3339Nano, tsRaw)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		return nil, err
	}
	return &Posicion{OccurredAt: ts, ID: id}, nil
}
