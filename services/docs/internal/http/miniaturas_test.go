// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// almacenMiniaturasFalso guarda la miniatura vigente como el de verdad: solo avanza.
type almacenMiniaturasFalso struct {
	mu    sync.Mutex
	clave string
	seq   int64
}

func (a *almacenMiniaturasFalso) GuardarMiniatura(_ context.Context, _ string, seq int64, clave string) (string, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.seq >= seq {
		return "", false, nil
	}
	anterior := a.clave
	a.clave, a.seq = clave, seq
	return anterior, true, nil
}

func (a *almacenMiniaturasFalso) MiniaturaDe(context.Context, string) (string, int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.clave, a.seq, nil
}

// objetosFalsos es Object Storage en memoria, por organización.
type objetosFalsos struct {
	mu    sync.Mutex
	datos map[string][]byte
	tipos map[string]string
}

func nuevosObjetos() *objetosFalsos {
	return &objetosFalsos{datos: map[string][]byte{}, tipos: map[string]string{}}
}

func (o *objetosFalsos) Put(_ context.Context, t tenancy.Tenant, key string, r io.Reader, _ int64, opts objectstore.PutOptions) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.datos[t.Slug+"/"+key], o.tipos[t.Slug+"/"+key] = b, opts.ContentType
	return nil
}

func (o *objetosFalsos) Get(_ context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	b, ok := o.datos[t.Slug+"/"+key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (o *objetosFalsos) Delete(_ context.Context, t tenancy.Tenant, key string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.datos, t.Slug+"/"+key)
	return nil
}

func (o *objetosFalsos) claves() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]string, 0, len(o.datos))
	for k := range o.datos {
		out = append(out, k)
	}
	return out
}

// estadoFijo es un documento con una secuencia dada y un párrafo de texto.
func estadoFijo(seq *int64) func(context.Context, string) (*domain.Document, *domain.State, int64, error) {
	return func(context.Context, string) (*domain.Document, *domain.State, int64, error) {
		op, err := domain.ContentOp(domain.KindDocument, "<h1>Acta</h1><p>Hola</p>")
		if err != nil {
			return nil, nil, 0, err
		}
		st := domain.NewState()
		if _, err := st.Apply(op); err != nil {
			return nil, nil, 0, err
		}
		return &domain.Document{ID: "d1", Kind: domain.KindDocument}, st, *seq, nil
	}
}

func esperar(t *testing.T, cond func() bool) {
	t.Helper()
	for range 400 {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no se cumplió a tiempo")
}

var acme = tenancy.Tenant{Slug: "acme"}

// Guardar NO dibuja en el acto: una ráfaga de guardados se convierte en UNA miniatura,
// con lo último, cuando el documento se queda quieto. Y la que sustituye se borra.
func TestLaMiniaturaSeDibujaUnaVezCuandoElDocumentoSeQuedaQuieto(t *testing.T) {
	seq := int64(3)
	alm, obj := &almacenMiniaturasFalso{}, nuevosObjetos()
	m := &miniaturas{
		espera: 40 * time.Millisecond, estado: estadoFijo(&seq), almacen: alm, objetos: obj,
		turno: make(chan struct{}, 1), pendientes: map[string]*time.Timer{},
	}
	ctx := tenancy.Into(context.Background(), acme)
	for range 5 {
		m.programar(ctx, "d1") // cinco guardados seguidos
		time.Sleep(5 * time.Millisecond)
	}
	if len(obj.claves()) != 0 {
		t.Fatal("se dibujó antes de que el documento se quedara quieto")
	}
	esperar(t, func() bool { _, s, _ := alm.MiniaturaDe(ctx, "d1"); return s == 3 })
	ks := obj.claves()
	if len(ks) != 1 || !strings.HasPrefix(ks[0], "acme/docs/d1/miniatura/3-") || obj.tipos[ks[0]] != "image/png" {
		t.Fatalf("una miniatura PNG bajo el prefijo del documento, y hay %v", ks)
	}
	if _, err := png.Decode(bytes.NewReader(obj.datos[ks[0]])); err != nil {
		t.Fatalf("lo subido no es un PNG: %v", err)
	}

	seq = 8
	m.programar(ctx, "d1")
	esperar(t, func() bool { _, s, _ := alm.MiniaturaDe(ctx, "d1"); return s == 8 })
	esperar(t, func() bool { return len(obj.claves()) == 1 })
	if ks := obj.claves(); !strings.HasPrefix(ks[0], "acme/docs/d1/miniatura/8-") {
		t.Fatalf("la de la secuencia 3 debe borrarse al llegar la de la 8: %v", ks)
	}

	// Sin cambios desde la última, no se vuelve a dibujar ni a subir nada.
	antes := obj.claves()[0]
	if err := m.generar(ctx, acme, "d1"); err != nil {
		t.Fatal(err)
	}
	if ks := obj.claves(); len(ks) != 1 || ks[0] != antes {
		t.Fatalf("sin cambios no se sube otra: %v", ks)
	}
}

// Si otra réplica guardó antes una miniatura igual o más nueva, la recién subida sobra y
// se borra (solo esa).
func TestLaMiniaturaQuePierdeLaCarreraSeBorra(t *testing.T) {
	seq := int64(4)
	alm, obj := &carreraPerdida{}, nuevosObjetos()
	m := &miniaturas{estado: estadoFijo(&seq), almacen: alm, objetos: obj, turno: make(chan struct{}, 1)}
	if err := m.generar(tenancy.Into(context.Background(), acme), acme, "d1"); err != nil {
		t.Fatal(err)
	}
	if ks := obj.claves(); len(ks) != 0 {
		t.Fatalf("la miniatura perdedora debe borrarse: %v", ks)
	}
}

// carreraPerdida: al leer no hay ninguna, y al guardar otra réplica ya se adelantó.
type carreraPerdida struct{ almacenMiniaturasFalso }

func (*carreraPerdida) GuardarMiniatura(context.Context, string, int64, string) (string, bool, error) {
	return "", false, nil
}

// Escribir operaciones llama a AlGuardar; leer, no.
func TestEscribirOperacionesProgramaLaMiniatura(t *testing.T) {
	fs := &fakeStore{doc: ownedDoc()}
	h := newHandler(fs)
	var veces int
	h.svc.AlGuardar = func(context.Context, string) { veces++ }
	rec := httptest.NewRecorder()
	h.postOps(rec, req(t, "POST", "/v1/documents/d1/ops", ownerClaims(), map[string]string{"id": "d1"},
		`{"ops":[{"kind":"set","key":"doc:html","value":"\"<p>x</p>\"","dot":{"a":"a","c":1},"lamport":1}]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("ops: code=%d body=%s", rec.Code, rec.Body)
	}
	if veces != 1 {
		t.Fatalf("una escritura debe programar la miniatura una vez, y van %d", veces)
	}
	h.getDocument(httptest.NewRecorder(), req(t, "GET", "/v1/documents/d1", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if veces != 1 {
		t.Fatal("leer un documento no debe programar su miniatura")
	}
}

// La miniatura se sirve a quien puede ver el documento, y a nadie más; con la secuencia
// vigente en la URL es inmutable, y sin ella se revalida.
func TestGetMiniatura(t *testing.T) {
	alm, obj := &almacenMiniaturasFalso{}, nuevosObjetos()
	h := newHandler(&fakeStore{doc: ownedDoc()})
	h.miniaturas = &miniaturas{almacen: alm, objetos: obj}
	pedir := func(c *authn.Claims, consulta string) *httptest.ResponseRecorder {
		r := req(t, "GET", "/v1/documents/d1/thumbnail"+consulta, c, map[string]string{"id": "d1"}, "")
		rec := httptest.NewRecorder()
		h.getMiniatura(rec, r.WithContext(tenancy.Into(r.Context(), acme)))
		return rec
	}

	if rec := pedir(ownerClaims(), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("sin miniatura todavía: 404, y da %d", rec.Code)
	}
	ctx := tenancy.Into(context.Background(), acme)
	_ = obj.Put(ctx, acme, "docs/d1/miniatura/7-x.png", strings.NewReader("PNG"), 3, objectstore.PutOptions{})
	_, _, _ = alm.GuardarMiniatura(ctx, "d1", 7, "docs/d1/miniatura/7-x.png")

	rec := pedir(ownerClaims(), "?v=7")
	if rec.Code != http.StatusOK || rec.Body.String() != "PNG" {
		t.Fatalf("con miniatura: code=%d body=%q", rec.Code, rec.Body)
	}
	cab := rec.Header()
	if cab.Get("Content-Type") != "image/png" || cab.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(cab.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("cabeceras de imagen inertes: %v", cab)
	}
	if cc := cab.Get("Cache-Control"); !strings.Contains(cc, "immutable") || !strings.Contains(cc, "private") {
		t.Errorf("con la secuencia vigente se cachea para siempre (en privado): %q", cc)
	}
	if cc := pedir(ownerClaims(), "?v=6").Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
		t.Errorf("con una secuencia vieja no puede quedarse cacheada: %q", cc)
	}
	if rec := pedir(&authn.Claims{Subject: "stranger", Tenant: "acme"}, "?v=7"); rec.Code != http.StatusForbidden {
		t.Errorf("quien no puede ver el documento no ve su miniatura: %d", rec.Code)
	}

	h.miniaturas = nil
	if rec := pedir(ownerClaims(), "?v=7"); rec.Code != http.StatusNotFound {
		t.Errorf("sin Object Storage no hay miniaturas: %d", rec.Code)
	}
}

// Un fallo al dibujar (o un pánico) no tira el servicio: se registra y ya.
func TestUnaMiniaturaQueFallaNoTiraElServicio(t *testing.T) {
	m := &miniaturas{
		estado: func(context.Context, string) (*domain.Document, *domain.State, int64, error) {
			panic("estado imposible")
		},
		turno: make(chan struct{}, 1),
	}
	m.dibujar(log.Into(context.Background(), slog.New(slog.DiscardHandler)), acme, "d1")
	m.estado = func(context.Context, string) (*domain.Document, *domain.State, int64, error) {
		return nil, nil, 0, errors.New("sin base")
	}
	m.dibujar(log.Into(context.Background(), slog.New(slog.DiscardHandler)), acme, "d1")
}
