// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package baseshttp

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// plazoDelFlujo es cuánto se espera un evento. Holgado a propósito: el flujo entrega un
// cambio cuando ninguna transacción de escritura anterior sigue abierta EN TODO el
// Postgres, y el de los tests se comparte con otros que corren a la vez.
const plazoDelFlujo = 30 * time.Second

// evento es un evento SSE leído del flujo.
type evento struct {
	ID, Nombre, Datos string
}

// oyente es una pestaña con el flujo de cambios de una base abierto.
type oyente struct {
	t        *testing.T
	eventos  chan evento
	cerrado  chan struct{}
	cancelar context.CancelFunc
	estado   int
}

// escuchar abre el flujo como lo haría el BFF: con la identidad en las cabeceras del
// test. `cabeceras` son pares nombre, valor.
func escuchar(t *testing.T, url, sub, tenant string, cabeceras ...string) *oyente {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	req.Header.Set("X-Test-Sub", sub)
	req.Header.Set("X-Test-Tenant", tenant)
	for i := 0; i+1 < len(cabeceras); i += 2 {
		req.Header.Set(cabeceras[i], cabeceras[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	o := &oyente{t: t, eventos: make(chan evento, 256), cerrado: make(chan struct{}), cancelar: cancel, estado: resp.StatusCode}
	t.Cleanup(cancel)
	go func() {
		defer close(o.cerrado)
		defer resp.Body.Close()
		var e evento
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			l := sc.Text()
			switch {
			case strings.HasPrefix(l, "id: "):
				e.ID = l[4:]
			case strings.HasPrefix(l, "event: "):
				e.Nombre = l[7:]
			case strings.HasPrefix(l, "data: "):
				e.Datos = l[6:]
			case l == "":
				if e.Nombre != "" {
					o.eventos <- e
				}
				e = evento{}
			}
		}
	}()
	return o
}

// hasta lee eventos hasta el primero con ese nombre (los de presencia se saltan salvo
// que se pidan). Falla si el flujo se cierra o no llega en plazoDelFlujo.
func (o *oyente) hasta(nombre string) evento {
	o.t.Helper()
	plazo := time.After(plazoDelFlujo)
	for {
		select {
		case e := <-o.eventos:
			if e.Nombre == nombre {
				return e
			}
			if e.Nombre == "bye" {
				o.t.Fatalf("el flujo se despidió (%s) esperando %q", e.Datos, nombre)
			}
		case <-plazo:
			o.t.Fatalf("no llegó ningún evento %q", nombre)
		}
	}
}

// cambio lee hasta el primer `change` cuyo contenido incluye `con`. Al abrir, el flujo
// puede contar algún cambio de justo antes (empieza en la transacción más antigua aún en
// curso, que es lo que garantiza que no se salta ninguno): esos se dejan pasar.
func (o *oyente) cambio(con string) evento {
	o.t.Helper()
	for range 50 {
		if e := o.hasta("change"); strings.Contains(e.Datos, con) {
			return e
		}
	}
	o.t.Fatalf("no llegó ningún cambio con %q", con)
	return evento{}
}

// seCierra espera a que el servidor cierre el flujo.
func (o *oyente) seCierra() {
	o.t.Helper()
	select {
	case <-o.cerrado:
	case <-time.After(plazoDelFlujo):
		o.t.Fatal("el flujo sigue abierto")
	}
}

func TestTiempoRealPorHTTP(t *testing.T) {
	p := &pega{}
	h := servidorCon(t, p.montar(t))
	srv := httptest.NewServer(h)
	// Cleanup y no defer: Close espera a que acaben los flujos abiertos, y los oyentes se
	// cancelan en SUS cleanups, que corren antes que este (orden inverso al de registro).
	t.Cleanup(srv.Close)
	base := idDeRespuesta(t, llamar(t, h, "ana", "acme", "POST", "/bases", `{"name":"Clientes"}`))
	tabla := idDeRespuesta(t, llamar(t, h, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"T"}`))
	campo := idDeRespuesta(t, llamar(t, h, "ana", "acme", "POST", "/tables/"+tabla+"/fields", `{"name":"Nombre","kind":"text"}`))
	reg := idDeRespuesta(t, llamar(t, h, "ana", "acme", "POST", "/tables/"+tabla+"/records", `{"data":{"`+campo+`":"uno"}}`))
	flujo := srv.URL + "/api/v1/bases/" + base + "/events"
	conceder := func(quien, rol string) string {
		t.Helper()
		w := llamar(t, h, "ana", "acme", "POST", "/bases/"+base+"/permissions", `{"principal_kind":"user","principal_id":"`+quien+`","role":"`+rol+`"}`)
		if w.Code != 201 {
			t.Fatalf("conceder: %d", w.Code)
		}
		return idDeRespuesta(t, w)
	}

	// Sin acceso, de otra organización o con otra versión del contrato: no hay flujo.
	for _, q := range [][2]string{{"bea", "acme"}, {"gus", "globex"}} {
		if o := escuchar(t, flujo, q[0], q[1]); o.estado != 404 {
			t.Errorf("%s abre el flujo sin acceso: %d", q[0], o.estado)
		}
	}
	if o := escuchar(t, flujo+"?v=2", "ana", "acme"); o.estado != 400 {
		t.Errorf("otra versión del contrato: %d", o.estado)
	}
	for _, malo := range []string{"basura", "1-2-3", "-1-1"} {
		if o := escuchar(t, flujo, "ana", "acme", "Last-Event-ID", malo); o.estado != 422 {
			t.Errorf("cursor %q: %d", malo, o.estado)
		}
	}
	if o := escuchar(t, flujo+"?conn=<script>", "ana", "acme"); o.estado != 422 {
		t.Errorf("pestaña no válida: %d", o.estado)
	}

	permiso := conceder("bea", "viewer")
	bea := escuchar(t, flujo+"?v=1&conn=pestana-de-bea", "bea", "acme")
	hola := bea.hasta("hello")
	var saludo struct {
		V      int    `json:"v"`
		Cursor string `json:"cursor"`
		Role   string `json:"role"`
	}
	if json.Unmarshal([]byte(hola.Datos), &saludo) != nil || saludo.V != 1 || saludo.Cursor == "" || saludo.Role != "viewer" {
		t.Fatalf("hello: %s", hola.Datos)
	}

	// ana escribe una celda con su operación: bea lo ve llegar, con el id de la operación y SIN el valor.
	op := "7b1c6d3e-9f2a-4c5b-8d7e-0a1b2c3d4e5f"
	celda := func(sub, cuerpo, clave string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/records/"+reg+"/cells/"+campo, strings.NewReader(cuerpo))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Sub", sub)
		req.Header.Set("X-Test-Tenant", "acme")
		if clave != "" {
			req.Header.Set("Idempotency-Key", clave)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if w := celda("ana", `{"value":"dos secreto","expected":"uno"}`, op); w.Code != 200 || !strings.Contains(w.Body.String(), `"conflict":false`) || !strings.Contains(w.Body.String(), `"replayed":false`) {
		t.Fatalf("operación de celda: %d %s", w.Code, w.Body)
	}
	cambio := bea.cambio(op)
	if cambio.ID == "" || !strings.Contains(cambio.Datos, `"action":"record.update"`) || !strings.Contains(cambio.Datos, reg) ||
		!strings.Contains(cambio.Datos, `"op_id":"`+op+`"`) || !strings.Contains(cambio.Datos, `"actor":"ana"`) {
		t.Errorf("cambio: %+v", cambio)
	}
	if strings.Contains(cambio.Datos, "secreto") || strings.Contains(cambio.Datos, "value") {
		t.Errorf("el flujo lleva el valor de la celda: %s", cambio.Datos)
	}
	// Repetir la operación (un reintento tras un corte) no la aplica otra vez.
	if w := celda("ana", `{"value":"dos secreto","expected":"uno"}`, op); w.Code != 200 || !strings.Contains(w.Body.String(), `"replayed":true`) {
		t.Errorf("repetición: %d %s", w.Code, w.Body)
	}
	// Quien veía un valor que ya no está pisa y se le avisa.
	if w := celda("ana", `{"value":"tres","expected":"uno"}`, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"conflict":true`) {
		t.Errorf("último escritor con aviso: %d %s", w.Code, w.Body)
	}
	ultimo := bea.cambio(`"record.update"`)
	// Una lectora no escribe celdas; una clave de idempotencia que no es un UUID no vale; otra versión, tampoco.
	if c := celda("bea", `{"value":"x"}`, "").Code; c != 403 {
		t.Errorf("la lectora escribe una celda: %d", c)
	}
	if c := celda("ana", `{"value":"x"}`, "no-es-uuid").Code; c != 422 {
		t.Errorf("clave de idempotencia basura: %d", c)
	}
	if c := celda("ana", `{"value":"x","rev":1,"sub":"bea"}`, "").Code; c != 400 && c != 422 {
		t.Errorf("campos desconocidos en la operación: %d", c)
	}
	// El registro que el flujo dice que ha cambiado se pide por la API, que autoriza.
	if w := llamar(t, h, "bea", "acme", "GET", "/records/"+reg, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"tres"`) {
		t.Errorf("leer el registro cambiado: %d %s", w.Code, w.Body)
	}
	if c := llamar(t, h, "gus", "globex", "GET", "/records/"+reg, "").Code; c != 404 {
		t.Errorf("otra organización lee el registro: %d", c)
	}

	// Presencia: ana dice en qué celda está; quién es lo pone el token, no el cuerpo.
	presencia := "/bases/" + base + "/presence"
	if c := llamar(t, h, "ana", "acme", "PUT", presencia, `{"conn":"pestana-de-ana","table_id":"`+tabla+`","record_id":"`+reg+`","field_id":"`+campo+`"}`).Code; c != 204 {
		t.Fatalf("latido: %d", c)
	}
	for cuerpo, quiero := range map[string]int{
		`{"conn":"pestana-de-ana","sub":"bea"}`:   400, // quién es no se dice
		`{"conn":"pestana-de-ana","name":"Otra"}`: 400,
		`{"conn":"x"}`: 422,
		`{"conn":"pestana-de-ana","table_id":"basura"}`:            422,
		`{"conn":"pestana-de-ana","record_id":"` + reg + `"}`:      422, // una celda sin su tabla
		`{"conn":"pestana-de-bea-falsa","table_id":"` + reg + `"}`: 404, // un id que no es una tabla
	} {
		if c := llamar(t, h, "ana", "acme", "PUT", presencia, cuerpo).Code; c != quiero && (quiero != 400 || c != 422) {
			t.Errorf("latido %s: %d, esperado %d", cuerpo, c, quiero)
		}
	}
	if c := llamar(t, h, "bea", "acme", "PUT", presencia, `{"conn":"pestana-de-ana"}`).Code; c != 422 {
		t.Errorf("bea usa la pestaña de ana: %d", c)
	}
	if c := llamar(t, h, "gus", "globex", "PUT", presencia, `{"conn":"pestana-de-gus"}`).Code; c != 404 {
		t.Errorf("presencia desde otra organización: %d", c)
	}
	var visto bool
	for i := 0; i < 20 && !visto; i++ {
		e := bea.hasta("presence")
		visto = strings.Contains(e.Datos, `"sub":"ana"`) && strings.Contains(e.Datos, `"name":"ana"`) && strings.Contains(e.Datos, campo)
	}
	if !visto {
		t.Error("bea no ve a ana en su celda")
	}

	// Reconexión con replay: bea corta, pasan cosas, y al volver con su último id se las cuentan.
	bea.cancelar()
	bea.seCierra()
	celda("ana", `{"value":"cuatro"}`, "")
	llamar(t, h, "ana", "acme", "POST", "/records/"+reg+"/comments", `{"body":"mientras no estabas"}`)
	bea = escuchar(t, flujo, "bea", "acme", "Last-Event-ID", ultimo.ID)
	bea.hasta("hello")
	// En orden y sin repetir lo que ya había visto: primero la celda, después el comentario.
	if e := bea.hasta("change"); !strings.Contains(e.Datos, "record.update") || e.ID == ultimo.ID {
		t.Errorf("replay, primer cambio: %+v", e)
	}
	if e := bea.hasta("change"); !strings.Contains(e.Datos, "comment.create") {
		t.Errorf("replay, segundo cambio: %s", e.Datos)
	}

	// Un permiso retirado corta el flujo abierto: se despide con «access» y cierra.
	if c := llamar(t, h, "ana", "acme", "DELETE", "/bases/"+base+"/permissions/"+permiso, "").Code; c != 204 {
		t.Fatalf("revocar: %d", c)
	}
	adios := evento{}
	for adios.Nombre != "bye" {
		select {
		case adios = <-bea.eventos:
		case <-time.After(plazoDelFlujo):
			t.Fatal("el flujo sigue abierto tras retirar el permiso")
		}
	}
	if !strings.Contains(adios.Datos, `"reason":"access"`) {
		t.Errorf("despedida: %s", adios.Datos)
	}
	bea.seCierra()
	celda("ana", `{"value":"esto ya no lo ve bea"}`, "")
	select {
	case e := <-bea.eventos:
		t.Errorf("llegó un evento tras retirar el permiso: %+v", e)
	case <-time.After(200 * time.Millisecond):
	}

	// El flujo no sobrevive a su token: se despide con «token» y cierra (el navegador reconecta con uno nuevo).
	corto := escuchar(t, flujo, "ana", "acme", "X-Test-Exp", "300ms")
	corto.hasta("hello")
	for adios = (evento{}); adios.Nombre != "bye"; {
		select {
		case adios = <-corto.eventos:
		case <-time.After(plazoDelFlujo):
			t.Fatal("el flujo sobrevive a su token")
		}
	}
	if !strings.Contains(adios.Datos, `"reason":"token"`) {
		t.Errorf("despedida por token: %s", adios.Datos)
	}
	corto.seCierra()

	// Tope de flujos por persona: el noveno no entra, y al cerrar uno vuelve a caber.
	var abiertos []*oyente
	for i := 0; i < maxFlujosPorPersona; i++ {
		o := escuchar(t, flujo, "ana", "acme")
		if o.estado != 200 {
			t.Fatalf("flujo %d: %d", i, o.estado)
		}
		o.hasta("hello")
		abiertos = append(abiertos, o)
	}
	if o := escuchar(t, flujo, "ana", "acme"); o.estado != 429 {
		t.Errorf("un flujo de más: %d", o.estado)
	}
	abiertos[0].cancelar()
	abiertos[0].seCierra()
	time.Sleep(100 * time.Millisecond)
	if o := escuchar(t, flujo, "ana", "acme"); o.estado != 200 {
		t.Errorf("tras cerrar uno: %d", o.estado)
	}
}

// Alguien de otra organización sigue en vivo la base a la que está invitada, y solo esa.
func TestTiempoRealParaInvitadosDeFuera(t *testing.T) {
	p := &pega{}
	h := servidorCon(t, p.montar(t))
	srv := httptest.NewServer(h)
	// Cleanup y no defer: Close espera a que acaben los flujos abiertos, y los oyentes se
	// cancelan en SUS cleanups, que corren antes que este (orden inverso al de registro).
	t.Cleanup(srv.Close)
	base := idDeRespuesta(t, llamar(t, h, "ana", "acme", "POST", "/bases", `{"name":"Clientes"}`))
	tabla := idDeRespuesta(t, llamar(t, h, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"T"}`))
	otra := idDeRespuesta(t, llamar(t, h, "ana", "acme", "POST", "/bases", `{"name":"Nóminas"}`))
	llamar(t, h, "ana", "acme", "POST", "/bases/"+otra+"/permissions", `{"principal_kind":"tenant","principal_id":"acme","role":"editor"}`)
	w := llamar(t, h, "ana", "acme", "POST", "/bases/"+base+"/permissions", `{"principal_kind":"external","query":"@gus","role":"viewer"}`)
	if w.Code != 201 {
		t.Fatalf("invitar: %d %s", w.Code, w.Body)
	}
	permiso := idDeRespuesta(t, w)

	if o := escuchar(t, srv.URL+"/api/v1/bases/"+otra+"/events", "gus", "globex"); o.estado != 404 {
		t.Errorf("el invitado sigue otra base de la anfitriona: %d", o.estado)
	}
	gus := escuchar(t, srv.URL+"/api/v1/bases/"+base+"/events", "gus", "globex")
	if gus.estado != 200 {
		t.Fatalf("el invitado abre el flujo: %d", gus.estado)
	}
	gus.hasta("hello")
	llamar(t, h, "ana", "acme", "POST", "/tables/"+tabla+"/records", `{"data":{}}`)
	gus.cambio("record.create")
	llamar(t, h, "ana", "acme", "DELETE", "/bases/"+base+"/permissions/"+permiso, "")
	gus.seCierra()
}
