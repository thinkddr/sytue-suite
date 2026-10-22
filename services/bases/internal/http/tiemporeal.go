// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

// COEDICIÓN EN TIEMPO REAL — contrato v1 (domain.VersionTiempoReal)
//
// El patrón es el de Docs (contrato versionado, cambios en orden con replay al
// reconectar, operaciones por celda con identificador e idempotencia, presencia y
// autorización del servicio); el TRANSPORTE no: nació cuando la API de Bases no tenía
// Ingress (solo la alcanzaba el BFF) y no había WebSocket posible desde el navegador.
// Desde el 2026-10-05 `/api/v1` sale por el Ingress para las apps nativas, que abren
// este mismo flujo con el token de la persona.
//
//	GET  /bases/{id}/events        flujo SSE, que el BFF deja pasar tal cual:
//	       hello    {v, cursor, role}   al abrir; el cliente (re)carga entonces sus datos
//	       change   {action, table_id, record_id, field_id, actor, op_id}   id: <cursor>
//	       presence [{conn, sub, name, table_id, record_id, field_id}]
//	       resync   {}                  se quedó demasiado atrás: recargar
//	       bye      {reason}            access | token | error; el servidor cierra
//	     `Last-Event-ID` (lo manda el navegador al reconectar) reanuda desde ahí.
//	PUT  /bases/{id}/presence      latido: {conn, table_id?, record_id?, field_id?}
//	PUT  /records/{rid}/cells/{fid} operación de celda: {value | links, expected?},
//	       con `Idempotency-Key: <uuid>`. Contesta {record, conflict, replayed}.
//	GET  /records/{rid}            el registro que el flujo dice que ha cambiado
//
// Por el flujo NO viajan datos: solo qué ha cambiado. El valor se pide por la API, que
// autoriza la lectura. Y el flujo se vuelve a autorizar en CADA tanda, dentro de la
// transacción que la lee: un permiso retirado lo cierra en la siguiente.

var (
	errVersionDeContrato = errors.New("bases: versión del contrato de tiempo real no admitida")
	errDemasiadosFlujos  = errors.New("bases: demasiados flujos abiertos")
)

const (
	// maxFlujosPorPersona y maxFlujos acotan lo que cuesta el tiempo real: cada flujo
	// abierto es una transacción corta por tic contra un pool pequeño.
	//
	// ponytail: un sondeo por flujo, 200 por réplica. Si una base junta muchas pestañas,
	// una sala por base con UN sondeo que reparte (cada oyente sigue filtrando por su ACL).
	maxFlujosPorPersona = 8
	maxFlujos           = 200
	// latidoDelFlujo mantiene viva la conexión a través de los proxies cuando no pasa nada.
	latidoDelFlujo = 20 * time.Second
)

// contadorDeFlujos cuenta los flujos abiertos en esta réplica.
type contadorDeFlujos struct {
	mu    sync.Mutex
	por   map[string]int
	total int
}

func (c *contadorDeFlujos) entrar(clave string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.total >= maxFlujos || c.por[clave] >= maxFlujosPorPersona {
		return false
	}
	if c.por == nil {
		c.por = map[string]int{}
	}
	c.por[clave]++
	c.total++
	return true
}

func (c *contadorDeFlujos) salir(clave string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.por[clave]--
	if c.por[clave] <= 0 {
		delete(c.por, clave)
	}
	c.total--
}

// versionAdmitida lee la versión del contrato que pide el cliente (`?v=`): vacía o la actual.
func versionAdmitida(r *http.Request) bool {
	v := r.URL.Query().Get("v")
	return v == "" || v == strconv.Itoa(domain.VersionTiempoReal)
}

// parametrosDelFlujo lee lo que el cliente dice al abrir: la versión del contrato, desde
// dónde reanuda (`Last-Event-ID`) y su pestaña.
func parametrosDelFlujo(r *http.Request) (desde *domain.Cursor, conn string, err error) {
	if !versionAdmitida(r) {
		return nil, "", errVersionDeContrato
	}
	if s := cmp.Or(r.Header.Get("Last-Event-ID"), r.URL.Query().Get("cursor")); s != "" {
		c, err := domain.LeerCursor(s)
		if err != nil {
			return nil, "", err
		}
		desde = &c
	}
	conn = r.URL.Query().Get("conn")
	if conn != "" && !domain.ConexionValida(conn) {
		return nil, "", domain.ErrInvalido
	}
	return desde, conn, nil
}

func (h *Handlers) flujoDeCambios(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	desde, conn, err := parametrosDelFlujo(r)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	// La primera tanda autoriza ANTES de abrir el flujo: sin acceso es un 404 normal.
	nov, err := h.Store.Novedades(r.Context(), a, bid, desde)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	clave := a.Tenant + "|" + a.Sub
	if !h.flujos.entrar(clave) {
		httpx.Fail(w, r, errorAPI(errDemasiadosFlujos))
		return
	}
	defer h.flujos.salir(clave)
	flush, ok := httpx.AbrirSSE(w)
	if !ok {
		httpx.Fail(w, r, errors.New("bases: la respuesta no admite un flujo"))
		return
	}
	// El flujo no sobrevive a su token: al caducar se cierra y, al reconectar, el nuevo
	// dice lo que vale AHORA (sus grupos, su organización).
	ctx := r.Context()
	if c, ok := authn.FromContext(ctx); ok && !c.Expira.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, c.Expira)
		defer cancel()
	}
	if conn != "" {
		// Al irse, su presencia se va con ella (con un contexto que no muera con la petición).
		defer func(fin context.Context) {
			fin, cancel := context.WithTimeout(fin, 3*time.Second)
			defer cancel()
			_ = h.Store.Irse(fin, a, bid, conn)
		}(context.WithoutCancel(r.Context()))
	}
	f := flujo{w: w, flush: flush}
	f.evento("hello", "", map[string]any{"v": domain.VersionTiempoReal, "cursor": nov.Cursor.String(), "role": nov.RolBase})
	f.tanda(nov)
	h.seguirFlujo(ctx, &f, a, bid, nov.Cursor)
}

// seguirFlujo es el bucle de un flujo abierto: en cada tic lee (y autoriza) una tanda,
// hasta que quien escucha se va, pierde el acceso o caduca su token.
func (h *Handlers) seguirFlujo(ctx context.Context, f *flujo, a domain.Actor, base string, cursor domain.Cursor) {
	tic := time.NewTicker(cmp.Or(h.tic, time.Second))
	defer tic.Stop()
	callado := time.Now()
	for f.err == nil {
		select {
		case <-ctx.Done():
			f.despedir(ctx, nil)
			return
		case <-tic.C:
		}
		nov, err := h.Store.Novedades(ctx, a, base, &cursor)
		switch {
		case errors.Is(err, domain.ErrNoExiste), errors.Is(err, domain.ErrSinPermiso):
			f.evento("bye", "", map[string]string{"reason": "access"})
			return
		case err != nil:
			f.despedir(ctx, err)
			return
		}
		cursor = nov.Cursor
		if f.tanda(nov) {
			callado = time.Now()
		} else if time.Since(callado) >= latidoDelFlujo {
			// Un bloque con solo `id:` no dispara ningún evento, pero deja al navegador el
			// cursor al día para cuando reconecte, y mantiene abierta la conexión.
			f.linea("id: " + cursor.String() + "\n\n")
			callado = time.Now()
		}
	}
}

// flujo escribe los eventos SSE de una conexión.
type flujo struct {
	w         io.Writer
	flush     http.Flusher
	presencia string // la última lista de presentes enviada, para no repetirla
	err       error
}

func (f *flujo) linea(s string) {
	if f.err != nil {
		return
	}
	if _, f.err = io.WriteString(f.w, s); f.err == nil {
		f.flush.Flush()
	}
}

func (f *flujo) evento(nombre, cursor string, datos any) {
	b, err := json.Marshal(datos)
	if err != nil {
		return
	}
	var s strings.Builder
	if cursor != "" {
		s.WriteString("id: " + cursor + "\n")
	}
	// json.Marshal no deja saltos de línea en la salida: un `data:` de una línea.
	fmt.Fprintf(&s, "event: %s\ndata: %s\n\n", nombre, b)
	f.linea(s.String())
}

// despedir dice por qué se cierra el flujo, si queda alguien a quien decírselo: `token`
// si ha caducado el de quien escucha —también cuando el plazo vence a media lectura, que
// entonces llega aquí como un error de la consulta—, `error` si ha fallado la lectura. Si
// es quien escucha quien se ha ido, nada.
func (f *flujo) despedir(ctx context.Context, err error) {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		f.evento("bye", "", map[string]string{"reason": "token"})
	case ctx.Err() == nil && err != nil:
		f.evento("bye", "", map[string]string{"reason": "error"})
	}
}

// tanda escribe lo que trae una tanda. Dice si ha escrito algo.
func (f *flujo) tanda(nov store.Novedades) bool {
	escrito := false
	if nov.Atras {
		f.evento("resync", nov.Cursor.String(), struct{}{})
		escrito = true
	}
	for _, c := range nov.Cambios {
		f.evento("change", c.Cursor, c)
		escrito = true
	}
	if b, err := json.Marshal(nov.Presentes); err == nil && string(b) != f.presencia {
		f.presencia = string(b)
		f.evento("presence", "", nov.Presentes)
		escrito = true
	}
	return escrito
}

func (h *Handlers) latir(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	bid, err2 := id(r, "id")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Conn     string `json:"conn"`
		TableID  string `json:"table_id"`
		RecordID string `json:"record_id"`
		FieldID  string `json:"field_id"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p := domain.Presente{Conn: in.Conn}
	for _, par := range []struct {
		de string
		a  *string
	}{{in.TableID, &p.TableID}, {in.RecordID, &p.RecordID}, {in.FieldID, &p.FieldID}} {
		if par.de == "" {
			continue
		}
		if *par.a, err = idDe(par.de); err != nil {
			httpx.Fail(w, r, errorAPI(domain.ErrInvalido))
			return
		}
	}
	if p.TableID == "" && (p.RecordID != "" || p.FieldID != "") {
		httpx.Fail(w, r, errorAPI(domain.ErrInvalido)) // una celda sin su tabla no se puede autorizar
		return
	}
	// Quién es lo dice el token, no el cuerpo.
	if c, ok := authn.FromContext(r.Context()); ok {
		p.Name = validation.TruncateRunes(cmp.Or(strings.TrimSpace(c.Name), c.Email), 200)
	}
	h.hecho(w, r, h.Store.Latir(r.Context(), a, bid, p))
}

func (h *Handlers) obtenerRegistro(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	rid, err2 := id(r, "rid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	reg, err := h.Store.ObtenerRegistro(r.Context(), a, rid)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, reg)
}

func (h *Handlers) escribirCelda(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	rid, err2 := id(r, "rid")
	fid, err3 := id(r, "fid")
	if err = firstErr(err, err2, err3); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !versionAdmitida(r) {
		httpx.Fail(w, r, errorAPI(errVersionDeContrato))
		return
	}
	var in struct {
		Value    json.RawMessage `json:"value"`
		Links    []string        `json:"links"`
		Expected json.RawMessage `json:"expected"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c := store.Celda{FieldID: fid, Value: in.Value, Links: in.Links, Expected: in.Expected}
	if k := r.Header.Get("Idempotency-Key"); k != "" {
		u, err := uuid.Parse(k)
		if err != nil {
			httpx.Fail(w, r, errorAPI(domain.ErrInvalido))
			return
		}
		c.OpID = u.String()
	}
	out, err := h.Store.EscribirCelda(r.Context(), a, rid, c)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
