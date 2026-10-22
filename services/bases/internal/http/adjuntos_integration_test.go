// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package baseshttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

// almacenFalso es un almacén de objetos en memoria, con el prefijo de organización en la
// clave como el de verdad.
type almacenFalso struct {
	mu      sync.Mutex
	objetos map[string][]byte
	fallar  bool
}

func (a *almacenFalso) Put(_ context.Context, t tenancy.Tenant, key string, r io.Reader, _ int64, _ objectstore.PutOptions) error {
	if a.fallar {
		return errors.New("almacén caído")
	}
	b, err := io.ReadAll(r)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.objetos[t.Slug+"/"+key] = b
	return err
}

func (a *almacenFalso) Get(_ context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.objetos[t.Slug+"/"+key]
	if !ok {
		return nil, errors.New("no existe")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (a *almacenFalso) Delete(_ context.Context, t tenancy.Tenant, key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.objetos, t.Slug+"/"+key)
	return nil
}

func (a *almacenFalso) cuantos() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.objetos)
}

type entornoAdjuntos struct {
	t                            *testing.T
	srv                          http.Handler
	alm                          *almacenFalso
	st                           *store.Store
	base, tabla, campo, registro string
}

func nuevoEntornoDeAdjuntos(t *testing.T) *entornoAdjuntos {
	e := &entornoAdjuntos{t: t, alm: &almacenFalso{objetos: map[string][]byte{}}}
	e.srv = servidorCon(t, func(d *Deps) { d.Objetos, e.st = e.alm, d.Store })
	e.base = idDeRespuesta(t, llamar(t, e.srv, "ana", "acme", "POST", "/bases", `{"name":"Expedientes"}`))
	e.tabla = idDeRespuesta(t, llamar(t, e.srv, "ana", "acme", "POST", "/bases/"+e.base+"/tables", `{"name":"Casos"}`))
	e.campo = idDeRespuesta(t, llamar(t, e.srv, "ana", "acme", "POST", "/tables/"+e.tabla+"/fields", `{"name":"Ficheros","kind":"attachment"}`))
	e.registro = idDeRespuesta(t, llamar(t, e.srv, "ana", "acme", "POST", "/tables/"+e.tabla+"/records", `{"data":{}}`))
	for quien, rol := range map[string]string{"bea": "editor", "cleo": "viewer"} {
		if c := llamar(t, e.srv, "ana", "acme", "POST", "/bases/"+e.base+"/permissions", `{"principal_kind":"user","principal_id":"`+quien+`","role":"`+rol+`"}`).Code; c != 201 {
			t.Fatalf("permiso de %s: %d", quien, c)
		}
	}
	return e
}

// subir manda `datos` como adjunto con un Content-Type cualquiera (no se mira).
func (e *entornoAdjuntos) subir(sub, tenant, registro, campo, nombre string, datos []byte) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/records/"+registro+"/attachments?field="+campo+"&name="+url.QueryEscape(nombre), bytes.NewReader(datos))
	req.Header.Set("Content-Type", "image/png") // miente a propósito: el tipo sale del contenido
	req.Header.Set("X-Test-Sub", sub)
	req.Header.Set("X-Test-Tenant", tenant)
	w := httptest.NewRecorder()
	e.srv.ServeHTTP(w, req)
	return w
}

func (e *entornoAdjuntos) purgar() {
	e.t.Helper()
	if _, err := e.st.PurgarAdjuntos(context.Background(), BorrarObjeto(e.alm)); err != nil {
		e.t.Fatal(err)
	}
}

// lectorEspia es un cuerpo sin fin que cuenta lo que le leen.
type lectorEspia struct{ n int }

func (l *lectorEspia) Read(p []byte) (int, error) {
	l.n += len(p)
	return len(p), nil
}

func pngDePrueba(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 40, 30))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestAdjuntosFlujo(t *testing.T) {
	e := nuevoEntornoDeAdjuntos(t)
	imagen := pngDePrueba(t)
	w := e.subir("bea", "acme", e.registro, e.campo, "../../foto.png", imagen)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"mime":"image/png"`) || !strings.Contains(w.Body.String(), `"has_thumb":true`) || !strings.Contains(w.Body.String(), `"name":"foto.png"`) {
		t.Fatalf("subir: %d %s", w.Code, w.Body)
	}
	adj := idDeRespuesta(t, w)
	if e.alm.cuantos() != 2 { // el fichero y su miniatura
		t.Errorf("objetos en el almacén: %d", e.alm.cuantos())
	}
	// La clave la pone el servicio: bajo la organización y la base, nunca el nombre.
	for clave := range e.alm.objetos {
		if !strings.HasPrefix(clave, "acme/bases/"+e.base+"/"+adj) {
			t.Errorf("clave inesperada: %s", clave)
		}
	}
	// La ficha viaja con el registro.
	if b := llamar(t, e.srv, "cleo", "acme", "GET", "/tables/"+e.tabla+"/records", ""); !strings.Contains(b.Body.String(), `"attachments":{"`+e.campo+`":[{"id":"`+adj) {
		t.Errorf("el registro no lleva el adjunto: %s", b.Body)
	}
	// Un lector descarga; siempre como descarga y sin que el navegador adivine nada.
	d := llamar(t, e.srv, "cleo", "acme", "GET", "/records/"+e.registro+"/attachments/"+adj, "")
	if d.Code != 200 || !bytes.Equal(d.Body.Bytes(), imagen) {
		t.Fatalf("descargar: %d (%d bytes)", d.Code, d.Body.Len())
	}
	for cab, quiero := range map[string]string{"Content-Type": "image/png", "X-Content-Type-Options": "nosniff", "Content-Security-Policy": "default-src 'none'; sandbox", "Cache-Control": "private, no-store"} {
		if d.Header().Get(cab) != quiero {
			t.Errorf("%s: %q", cab, d.Header().Get(cab))
		}
	}
	if !strings.HasPrefix(d.Header().Get("Content-Disposition"), "attachment; filename*=UTF-8''foto.png") {
		t.Errorf("Content-Disposition: %q", d.Header().Get("Content-Disposition"))
	}
	if m := llamar(t, e.srv, "cleo", "acme", "GET", "/records/"+e.registro+"/attachments/"+adj+"/thumb", ""); m.Code != 200 || m.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("miniatura: %d %s", m.Code, m.Header().Get("Content-Type"))
	}
	// Los valores de un campo de adjuntos no se escriben como datos.
	if c := llamar(t, e.srv, "bea", "acme", "PATCH", "/records/"+e.registro, `{"rev":1,"data":{"`+e.campo+`":"x"}}`).Code; c != 422 {
		t.Errorf("escribir el campo de adjuntos como dato: %d", c)
	}
	// El nombre de un fichero es texto de alguien: en el CSV se neutraliza.
	if c := e.subir("bea", "acme", e.registro, e.campo, "=cmd.txt", []byte("hola")).Code; c != 201 {
		t.Fatalf("subir texto: %d", c)
	}
	if csv := llamar(t, e.srv, "cleo", "acme", "GET", "/tables/"+e.tabla+"/export?format=csv", "").Body.String(); !strings.Contains(csv, "foto.png; =cmd.txt") && !strings.Contains(csv, "'=cmd.txt") {
		t.Errorf("CSV: %q", csv)
	}
	// Borrar el adjunto borra su objeto y su miniatura (por la purga).
	if c := llamar(t, e.srv, "cleo", "acme", "DELETE", "/records/"+e.registro+"/attachments/"+adj, "").Code; c != 403 {
		t.Errorf("un lector borra: %d", c)
	}
	if c := llamar(t, e.srv, "bea", "acme", "DELETE", "/records/"+e.registro+"/attachments/"+adj, "").Code; c != 204 {
		t.Fatalf("borrar: %d", c)
	}
	e.purgar()
	if e.alm.cuantos() != 1 {
		t.Errorf("tras borrar y purgar quedan %d objetos", e.alm.cuantos())
	}
	if c := llamar(t, e.srv, "cleo", "acme", "GET", "/records/"+e.registro+"/attachments/"+adj, "").Code; c != 404 {
		t.Errorf("descargar lo borrado: %d", c)
	}
}

// El tipo lo decide el contenido, y lo que un navegador o un sistema ejecutaría no entra.
func TestAdjuntosHostiles(t *testing.T) {
	e := nuevoEntornoDeAdjuntos(t)
	pdf := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")
	hostiles := map[string]struct {
		nombre string
		datos  []byte
		quiero int
	}{
		"HTML con nombre de imagen":    {"foto.png", []byte("<!DOCTYPE html><html><script>alert(1)</script></html>"), 422},
		"HTML sin extensión":           {"informe", []byte("<html><body>hola</body></html>"), 422},
		"SVG con script":               {"dibujo.txt", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), 422},
		"extensión SVG":                {"logo.svg", []byte("texto"), 422},
		"extensión HTML en mayúsculas": {"PAGINA.HTML", []byte("texto"), 422},
		"script suelto":                {"nota.txt", []byte("hola <script>alert(1)</script>"), 422},
		"ejecutable de Windows":        {"instalador.bin", []byte("MZ\x90\x00\x03\x00\x00\x00"), 422},
		"ejecutable ELF":               {"programa", []byte("\x7fELF\x02\x01\x01\x00"), 422},
		"guion de shell":               {"limpiar", []byte("#!/bin/sh\nrm -rf /\n"), 422},
		"extensión de ejecutable":      {"factura.pdf.exe", pdf, 422},
		"PDF con nombre de imagen":     {"foto.jpg", pdf, 422},
		"vacío":                        {"vacio.txt", nil, 422},
		"nombre vacío":                 {"", pdf, 422},
		"nombre con control":           {"a\x00b.pdf", pdf, 422},
		"nombre enorme":                {strings.Repeat("a", domain.MaxLongNombreAdjunto+1), pdf, 422},
		"demasiado grande":             {"grande.bin", bytes.Repeat([]byte{1}, maxCuerpoImport+1), 413},
	}
	for nombre, h := range hostiles {
		if w := e.subir("ana", "acme", e.registro, e.campo, h.nombre, h.datos); w.Code != h.quiero {
			t.Errorf("%s: %d, esperado %d", nombre, w.Code, h.quiero)
		}
	}
	if e.alm.cuantos() != 0 {
		t.Errorf("un adjunto rechazado ha llegado al almacén: %d objetos", e.alm.cuantos())
	}
	if w := e.subir("ana", "acme", e.registro, e.campo, "factura.pdf", pdf); w.Code != 201 || !strings.Contains(w.Body.String(), `"mime":"application/pdf"`) {
		t.Errorf("un PDF de verdad: %d %s", w.Code, w.Body)
	}
	// Si el almacén falla no queda ficha, y sin almacén la respuesta es 503.
	e.alm.fallar = true
	if w := e.subir("ana", "acme", e.registro, e.campo, "otro.pdf", pdf); w.Code != 500 {
		t.Errorf("con el almacén caído: %d", w.Code)
	}
	e.alm.fallar = false
	if b := llamar(t, e.srv, "ana", "acme", "GET", "/tables/"+e.tabla+"/records", "").Body.String(); strings.Contains(b, "otro.pdf") {
		t.Error("una subida fallida ha dejado ficha")
	}
	sin := servidor(t)
	if c := llamar(t, sin, "ana", "acme", "POST", "/records/"+e.registro+"/attachments?field="+e.campo+"&name=a.txt", "hola").Code; c != 503 {
		t.Errorf("sin almacén: %d", c)
	}
}

// Nadie llega a un adjunto por conocer su id: ni desde otro registro, ni desde otra
// base, ni desde otra organización; y nadie sube donde no puede escribir.
func TestAdjuntosIDOR(t *testing.T) {
	e := nuevoEntornoDeAdjuntos(t)
	pdf := []byte("%PDF-1.7\nsecreto\n")
	adj := idDeRespuesta(t, e.subir("ana", "acme", e.registro, e.campo, "secreto.pdf", pdf))

	// Otra base de la MISMA organización, de bea, con su tabla, su campo y su registro.
	base2 := idDeRespuesta(t, llamar(t, e.srv, "bea", "acme", "POST", "/bases", `{"name":"De Bea"}`))
	tabla2 := idDeRespuesta(t, llamar(t, e.srv, "bea", "acme", "POST", "/bases/"+base2+"/tables", `{"name":"T"}`))
	campo2 := idDeRespuesta(t, llamar(t, e.srv, "bea", "acme", "POST", "/tables/"+tabla2+"/fields", `{"name":"F","kind":"attachment"}`))
	reg2 := idDeRespuesta(t, llamar(t, e.srv, "bea", "acme", "POST", "/tables/"+tabla2+"/records", `{"data":{}}`))
	// Y otra organización.
	baseG := idDeRespuesta(t, llamar(t, e.srv, "gus", "globex", "POST", "/bases", `{"name":"Globex"}`))
	tablaG := idDeRespuesta(t, llamar(t, e.srv, "gus", "globex", "POST", "/bases/"+baseG+"/tables", `{"name":"T"}`))
	campoG := idDeRespuesta(t, llamar(t, e.srv, "gus", "globex", "POST", "/tables/"+tablaG+"/fields", `{"name":"F","kind":"attachment"}`))
	regG := idDeRespuesta(t, llamar(t, e.srv, "gus", "globex", "POST", "/tables/"+tablaG+"/records", `{"data":{}}`))
	// dan no tiene ningún permiso en la base de ana.
	lecturas := map[string][3]string{
		"sin acceso a la base":                   {"dan", "acme", e.registro},
		"desde su propio registro, en otra base": {"bea", "acme", reg2},
		"otra organización":                      {"gus", "globex", e.registro},
		"otra organización, desde su registro":   {"gus", "globex", regG},
	}
	for nombre, l := range lecturas {
		for _, sufijo := range []string{"", "/thumb"} {
			if w := llamar(t, e.srv, l[0], l[1], "GET", "/records/"+l[2]+"/attachments/"+adj+sufijo, ""); w.Code != 404 || strings.Contains(w.Body.String(), "secreto") {
				t.Errorf("leer, %s%s: %d", nombre, sufijo, w.Code)
			}
		}
		if c := llamar(t, e.srv, l[0], l[1], "DELETE", "/records/"+l[2]+"/attachments/"+adj, "").Code; c != 404 {
			t.Errorf("borrar, %s: %d", nombre, c)
		}
	}
	subidas := map[string]struct {
		sub, tenant, registro, campo string
		quiero                       int
	}{
		"un lector":         {"cleo", "acme", e.registro, e.campo, 403},
		"sin acceso":        {"dan", "acme", e.registro, e.campo, 404},
		"otra organización": {"gus", "globex", e.registro, e.campo, 404},
		"a su registro con el campo de otra base": {"bea", "acme", reg2, e.campo, 404},
		"al registro ajeno con su campo":          {"bea", "acme", e.registro, campo2, 404},
		"a su registro con el campo de otra org":  {"gus", "globex", regG, e.campo, 404},
		"campo de la otra org en mi registro":     {"ana", "acme", e.registro, campoG, 404},
	}
	antes := e.alm.cuantos()
	for nombre, s := range subidas {
		if w := e.subir(s.sub, s.tenant, s.registro, s.campo, "x.pdf", pdf); w.Code != s.quiero {
			t.Errorf("subir, %s: %d, esperado %d", nombre, w.Code, s.quiero)
		}
	}
	if e.alm.cuantos() != antes {
		t.Errorf("una subida sin permiso ha escrito en el almacén (%d → %d)", antes, e.alm.cuantos())
	}
	// Y ni se lee: el permiso va ANTES que el cuerpo (20 MiB por petición de cualquiera
	// con sesión serían memoria gratis).
	leido := &lectorEspia{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/records/"+e.registro+"/attachments?field="+e.campo+"&name=x.pdf", leido)
	req.Header.Set("X-Test-Sub", "dan")
	req.Header.Set("X-Test-Tenant", "acme")
	w := httptest.NewRecorder()
	e.srv.ServeHTTP(w, req)
	if w.Code != 404 || leido.n > 0 {
		t.Errorf("subida sin acceso: %d, %d bytes leídos del cuerpo", w.Code, leido.n)
	}
	// Un campo que no es de adjuntos no los admite.
	texto := idDeRespuesta(t, llamar(t, e.srv, "ana", "acme", "POST", "/tables/"+e.tabla+"/fields", `{"name":"Nota","kind":"text"}`))
	if c := e.subir("ana", "acme", e.registro, texto, "x.pdf", pdf).Code; c != 404 {
		t.Errorf("adjunto en un campo de texto: %d", c)
	}
	// El adjunto sigue ahí para quien sí puede.
	if c := llamar(t, e.srv, "ana", "acme", "GET", "/records/"+e.registro+"/attachments/"+adj, "").Code; c != 200 {
		t.Errorf("la propietaria ya no lo lee: %d", c)
	}
}

// Borrado seguro y límites: el objeto no sobrevive a su registro ni a su base, y un
// campo no admite más ficheros de la cuenta.
func TestAdjuntosBorradoYLimites(t *testing.T) {
	e := nuevoEntornoDeAdjuntos(t)
	for i := 0; i < domain.MaxAdjuntosPorCampo; i++ {
		if c := e.subir("ana", "acme", e.registro, e.campo, fmt.Sprintf("n%d.txt", i), []byte("hola")).Code; c != 201 {
			t.Fatalf("subida %d: %d", i, c)
		}
	}
	if c := e.subir("ana", "acme", e.registro, e.campo, "de-mas.txt", []byte("hola")).Code; c != 409 {
		t.Errorf("un adjunto de más: %d", c)
	}
	otro := idDeRespuesta(t, llamar(t, e.srv, "ana", "acme", "POST", "/tables/"+e.tabla+"/records", `{"data":{}}`))
	if c := e.subir("ana", "acme", otro, e.campo, "otro.txt", []byte("hola")).Code; c != 201 {
		t.Fatalf("subir a otro registro: %d", c)
	}
	e.purgar() // la subida rechazada dejó su reserva, con una hora de margen: no borra nada vivo
	if n := e.alm.cuantos(); n != domain.MaxAdjuntosPorCampo+1 {
		t.Fatalf("objetos antes de borrar: %d", n)
	}
	if c := llamar(t, e.srv, "ana", "acme", "DELETE", "/records/"+e.registro, "").Code; c != 204 {
		t.Fatalf("borrar el registro: %d", c)
	}
	e.purgar()
	if n := e.alm.cuantos(); n != 1 {
		t.Errorf("tras borrar el registro quedan %d objetos", n)
	}
	if c := llamar(t, e.srv, "ana", "acme", "DELETE", "/bases/"+e.base, "").Code; c != 204 {
		t.Fatalf("borrar la base: %d", c)
	}
	e.purgar()
	if n := e.alm.cuantos(); n != 0 {
		t.Errorf("tras borrar la base quedan %d objetos", n)
	}
}

// Una app nativa sube por internet: el cuerpo llega despacio y el plazo de lectura del
// servidor (15 s en producción; aquí 300 ms) no puede cortarlo, ni el de escritura la
// descarga. Verlo en rojo: quitar conPlazoDeTransferencia de subirAdjunto o de
// descargarAdjunto.
func TestAdjuntoLentoNoMuereEnLosPlazosDelServidor(t *testing.T) {
	e := nuevoEntornoDeAdjuntos(t)
	srv := httptest.NewUnstartedServer(e.srv)
	srv.Config.ReadTimeout, srv.Config.WriteTimeout = 300*time.Millisecond, 300*time.Millisecond
	srv.Start()
	defer srv.Close()

	datos := bytes.Repeat([]byte("hola, despacio\n"), 64)
	lector, escritor := io.Pipe()
	go func() {
		_, _ = escritor.Write(datos[:10])
		time.Sleep(700 * time.Millisecond) // más que el plazo del servidor
		_, _ = escritor.Write(datos[10:])
		_ = escritor.Close()
	}()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/records/"+e.registro+"/attachments?field="+e.campo+"&name=notas.txt", lector)
	req.ContentLength = int64(len(datos))
	req.Header.Set("X-Test-Sub", "bea")
	req.Header.Set("X-Test-Tenant", "acme")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("subida lenta: %v", err)
	}
	cuerpo, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("subida lenta: %d %s", res.StatusCode, cuerpo)
	}
	adj := string(cuerpo[bytes.Index(cuerpo, []byte(`"id":"`))+6:][:36])

	// La descarga: quien lee tarda más que el plazo de escritura en empezar a leer un
	// cuerpo que no cabe en los búferes del sistema.
	e.alm.mu.Lock()
	for clave := range e.alm.objetos {
		if strings.HasSuffix(clave, adj) {
			e.alm.objetos[clave] = bytes.Repeat([]byte{'x'}, 16<<20)
		}
	}
	e.alm.mu.Unlock()
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/v1/records/"+e.registro+"/attachments/"+adj, http.NoBody)
	req.Header.Set("X-Test-Sub", "bea")
	req.Header.Set("X-Test-Tenant", "acme")
	res, err = (&http.Client{Transport: &http.Transport{ReadBufferSize: 4 << 10}}).Do(req)
	if err != nil {
		t.Fatalf("descarga lenta: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	time.Sleep(700 * time.Millisecond)
	n, err := io.Copy(io.Discard, res.Body)
	if err != nil || n != 16<<20 {
		t.Fatalf("descarga lenta: %d bytes, %v", n, err)
	}
}
