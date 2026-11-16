// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// La miniatura de la portada (domain/miniatura.go) se dibuja AL GUARDAR, pero no en
// cada guardado: el editor guarda a ritmo de tecla, y dibujar la página entera por cada
// lote sería gastar CPU en imágenes que nadie verá. Cada escritura (re)arma un
// temporizador por documento; cuando el documento lleva `esperaMiniatura` sin cambios,
// se dibuja una vez con lo último. Quien vuelve a la portada al acabar de escribir ve su
// documento tal como lo dejó.
//
// ponytail: el temporizador vive en la réplica que recibió la escritura. Si se reinicia
// antes de que venza, esa miniatura no se dibuja hasta la próxima edición (la portada
// enseña entretanto la anterior, o el glifo). Una cola persistente lo arreglaría; para
// una vista previa no compensa.
const esperaMiniatura = 8 * time.Second

// maxMiniaturasALaVez acota cuántas se dibujan a la vez en una réplica: una miniatura es
// CPU pura y no puede competir con las peticiones de quien está editando.
const maxMiniaturasALaVez = 2

// limiteMiniatura es el tiempo máximo para cargar, dibujar y subir una.
const limiteMiniatura = time.Minute

// almacenMiniaturas lo implementa *store.Store (store/miniaturas.go).
type almacenMiniaturas interface {
	GuardarMiniatura(ctx context.Context, docID string, seq int64, clave string) (anterior string, guardada bool, err error)
	MiniaturaDe(ctx context.Context, docID string) (clave string, seq int64, err error)
}

// objetosMiniatura es lo que se usa de Object Storage (lo cumple *objectstore.Client).
type objetosMiniatura interface {
	Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts objectstore.PutOptions) error
	Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, t tenancy.Tenant, key string) error
}

type miniaturas struct {
	espera  time.Duration
	estado  func(ctx context.Context, docID string) (*domain.Document, *domain.State, int64, error)
	almacen almacenMiniaturas
	objetos objetosMiniatura
	turno   chan struct{}

	mu         sync.Mutex
	pendientes map[string]*time.Timer // «organización/documento» → su temporizador
}

func nuevasMiniaturas(svc *domain.Service, almacen almacenMiniaturas, objetos objetosMiniatura) *miniaturas {
	return &miniaturas{
		espera: esperaMiniatura, estado: svc.EstadoVigente, almacen: almacen, objetos: objetos,
		turno: make(chan struct{}, maxMiniaturasALaVez), pendientes: map[string]*time.Timer{},
	}
}

// programar es domain.Service.AlGuardar: arma (o rearma) el temporizador del documento y
// vuelve enseguida. La organización se toma del contexto de la escritura, porque el
// dibujo ocurre después, fuera de la petición.
func (m *miniaturas) programar(ctx context.Context, docID string) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return
	}
	// Desligado de la petición (que acaba enseguida) pero con sus valores: la organización
	// y el registro con el id de la petición que la provocó.
	fondo := context.WithoutCancel(ctx)
	clave := t.Slug + "/" + docID
	m.mu.Lock()
	defer m.mu.Unlock()
	if tm, ok := m.pendientes[clave]; ok {
		tm.Reset(m.espera)
		return
	}
	m.pendientes[clave] = time.AfterFunc(m.espera, func() {
		m.mu.Lock()
		delete(m.pendientes, clave)
		m.mu.Unlock()
		m.dibujar(fondo, t, docID)
	})
}

// dibujar carga el estado de ahora, dibuja la miniatura, la sube y la anota; la que
// sustituye se borra. Un fallo se registra y no pasa nada más: la portada sigue con la
// miniatura anterior (o el glifo) hasta la próxima edición.
func (m *miniaturas) dibujar(ctx context.Context, t tenancy.Tenant, docID string) {
	lg := log.From(ctx)
	m.turno <- struct{}{}
	defer func() { <-m.turno }()
	// Un estado que el dibujo no espera no puede tirar el servicio: esto corre fuera de
	// cualquier petición y nada más arriba recuperaría el pánico.
	defer func() {
		if p := recover(); p != nil {
			lg.Error("docs: la miniatura del documento entró en pánico", "doc", docID, "panic", fmt.Sprint(p))
		}
	}()
	ctx, cancel := context.WithTimeout(tenancy.Into(ctx, t), limiteMiniatura)
	defer cancel()
	if err := m.generar(ctx, t, docID); err != nil {
		lg.Warn("docs: no se pudo dibujar la miniatura del documento", "doc", docID, "error", err)
	}
}

func (m *miniaturas) generar(ctx context.Context, t tenancy.Tenant, docID string) error {
	doc, st, seq, err := m.estado(ctx, docID)
	if err != nil {
		return err
	}
	if seq == 0 {
		return nil // nada escrito todavía: la portada pinta el glifo
	}
	if _, vigente, err := m.almacen.MiniaturaDe(ctx, docID); err != nil {
		return err
	} else if vigente >= seq {
		return nil // ya dibujada con esto (u otra réplica fue más rápida)
	}
	png, err := domain.Miniatura(doc.Kind, st)
	if err != nil {
		return err
	}
	// Con un sufijo aleatorio y no solo la secuencia: dos réplicas que dibujan la misma
	// secuencia no escriben la misma clave, y la que pierde borra SOLO la suya.
	tok, err := domain.NewToken()
	if err != nil {
		return err
	}
	clave := fmt.Sprintf("docs/%s/miniatura/%d-%s.png", docID, seq, tok[:12])
	if err := m.objetos.Put(ctx, t, clave, bytes.NewReader(png), int64(len(png)),
		objectstore.PutOptions{ContentType: "image/png"}); err != nil {
		return err
	}
	anterior, guardada, err := m.almacen.GuardarMiniatura(ctx, docID, seq, clave)
	switch {
	case err != nil || !guardada:
		_ = m.objetos.Delete(ctx, t, clave)
		return err
	case anterior != "":
		_ = m.objetos.Delete(ctx, t, anterior)
	}
	return nil
}

// getMiniatura sirve la miniatura de un documento a quien puede verlo:
// GET /api/v1/documents/{id}/thumbnail[?v=<thumbnail_seq>].
//
// Con `v` igual a la secuencia vigente, la respuesta es inmutable y el navegador no la
// vuelve a pedir: la lista trae la secuencia y la pone en la URL, así que un cambio en el
// documento es una URL nueva. Sin `v` (o con una vieja), se revalida siempre. Privada en
// los dos casos: es contenido del documento, no puede quedarse en una caché compartida.
func (h *Handler) getMiniatura(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer); err != nil {
		fail(w, r, err)
		return
	}
	if h.miniaturas == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	clave, seq, err := h.miniaturas.almacen.MiniaturaDe(r.Context(), docID(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	if clave == "" {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	rc, err := h.miniaturas.objetos.Get(r.Context(), t, clave)
	if err != nil {
		fail(w, r, err)
		return
	}
	defer rc.Close()
	cab := w.Header()
	cab.Set("Content-Type", "image/png")
	cab.Set("X-Content-Type-Options", "nosniff")
	cab.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if r.URL.Query().Get("v") == strconv.FormatInt(seq, 10) {
		cab.Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		cab.Set("Cache-Control", "private, no-cache")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}
