// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/miniatura"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

// Adjuntos: subir, descargar y borrar los ficheros de un campo de adjuntos. Las rutas
// cuelgan del REGISTRO (`/records/{rid}/attachments…`): el permiso es el de leer o
// escribir ese registro, y el store comprueba que el adjunto es suyo. La descarga la
// sirve este servicio (no una URL firmada del almacén): así el permiso se comprueba en
// CADA descarga y la clave del objeto no sale nunca.

// Almacen es lo que los adjuntos necesitan del almacén de objetos (objectstore.Client).
type Almacen interface {
	Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts objectstore.PutOptions) error
	Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, t tenancy.Tenant, key string) error
}

// AlmacenDe envuelve el cliente; con nil devuelve un Almacen nil (sin almacén), no una
// interfaz con un puntero nil dentro.
func AlmacenDe(c *objectstore.Client) Almacen {
	if c == nil {
		return nil
	}
	return c
}

// BorrarObjeto es lo que la purga hace con cada clave: borrar el adjunto y su miniatura.
func BorrarObjeto(a Almacen) func(context.Context, store.ObjetoAdjunto) error {
	return func(ctx context.Context, o store.ObjetoAdjunto) error {
		t := tenancy.Tenant{Slug: o.Tenant}
		return errors.Join(a.Delete(ctx, t, o.Clave), a.Delete(ctx, t, o.Clave+store.SufijoMiniatura))
	}
}

const (
	// subidasALaVez acota la memoria: cada subida vive entera en memoria (hasta
	// MaxBytesAdjunto, más la imagen decodificada de su miniatura: ~32 MiB) y el pod
	// tiene 512 Mi.
	//
	// ponytail: fichero entero en memoria tras un semáforo; subida en streaming si el
	// tope por fichero sube.
	subidasALaVez = 2
	// maxPixelesMiniatura: por encima no se hace miniatura (decodificarla pediría
	// cientos de MB). miniatura.Render tiene su propia barrera, más alta.
	maxPixelesMiniatura = 8_000_000
	ladoMiniatura       = 256
	// plazoDeTransferencia es lo que puede durar subir o bajar un fichero (un adjunto,
	// una exportación). Los plazos del servidor (15 s de lectura, 30 s de escritura)
	// valían mientras solo llamaba el BFF, por dentro del clúster; una app nativa llega
	// por internet, y 20 MiB no caben en 15 s por debajo de 11 Mbit/s. Diez minutos son
	// 280 kbit/s para el fichero más grande, y siguen siendo un techo: nadie retiene una
	// de las subidasALaVez para siempre.
	plazoDeTransferencia = 10 * time.Minute
)

// conPlazoDeTransferencia cambia, para ESTA petición, los plazos de lectura y escritura
// del servidor por plazoDeTransferencia. Un ResponseRecorder de test no los tiene.
func conPlazoDeTransferencia(w http.ResponseWriter) {
	rc, fin := http.NewResponseController(w), time.Now().Add(plazoDeTransferencia)
	_ = rc.SetReadDeadline(fin)
	_ = rc.SetWriteDeadline(fin)
}

func (h *Handlers) montarAdjuntos(r chi.Router) {
	subir := r.With(httpx.MaxBodyBytes(domain.MaxBytesAdjunto+1), h.enAnfitrion)
	subir.Post("/records/{rid}/attachments", h.subirAdjunto)
	j := r.With(httpx.MaxBodyBytes(maxCuerpoAPI), h.enAnfitrion)
	j.Get("/records/{rid}/attachments/{aid}", h.descargarAdjunto)
	j.Get("/records/{rid}/attachments/{aid}/thumb", h.descargarAdjunto)
	j.Delete("/records/{rid}/attachments/{aid}", h.borrarAdjunto)
}

// sinAlmacen responde 503 si el servicio no tiene almacén de objetos configurado.
func (h *Handlers) sinAlmacen(w http.ResponseWriter, r *http.Request) bool {
	if h.Objetos != nil {
		return false
	}
	httpx.Fail(w, r, httpx.NewError(http.StatusServiceUnavailable, "attachments_unavailable", "los adjuntos no están disponibles"))
	return true
}

func (h *Handlers) subirAdjunto(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	rid, err2 := id(r, "rid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	campo, err := idDe(r.URL.Query().Get("field"))
	nombre, err2 := domain.NombreDeAdjunto(r.URL.Query().Get("name"))
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, errorAPI(domain.ErrInvalido))
		return
	}
	if h.sinAlmacen(w, r) {
		return
	}
	// El permiso va ANTES de leer el cuerpo: si no, cualquiera con sesión haría leer
	// 20 MiB al servicio por petición.
	if err := h.Store.PuedeAdjuntar(r.Context(), a, rid, campo); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	select {
	case h.subidas <- struct{}{}:
		defer func() { <-h.subidas }()
	case <-r.Context().Done():
		return
	}
	// Con el turno ya en la mano: la espera en la cola no gasta el plazo de la subida.
	conPlazoDeTransferencia(w)
	datos, err := io.ReadAll(io.LimitReader(r.Body, domain.MaxBytesAdjunto+1))
	if err != nil || len(datos) > domain.MaxBytesAdjunto {
		httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "attachment_too_large", "adjunto demasiado grande"))
		return
	}
	// El tipo lo dice el contenido; el Content-Type de la petición ni se mira.
	tipo, err := domain.TipoDeContenido(nombre, datos)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	reserva, err := h.Store.ReservarAdjunto(r.Context(), a, rid, campo, int64(len(datos)))
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	t := tenancy.Tenant{Slug: reserva.Objeto.Tenant}
	if err := h.Objetos.Put(r.Context(), t, reserva.Objeto.Clave, bytes.NewReader(datos), int64(len(datos)), objectstore.PutOptions{ContentType: tipo}); err != nil {
		httpx.Fail(w, r, fmt.Errorf("subir adjunto: %w", err))
		return
	}
	mini := miniaturaDe(tipo, datos)
	if mini != nil {
		if err := h.Objetos.Put(r.Context(), t, reserva.Objeto.Clave+store.SufijoMiniatura, bytes.NewReader(mini), int64(len(mini)), objectstore.PutOptions{ContentType: "image/jpeg"}); err != nil {
			mini = nil // sin miniatura; el adjunto sigue valiendo
		}
	}
	huella := sha256.Sum256(datos)
	adj, err := h.Store.ConfirmarAdjunto(r.Context(), a, reserva, nombre, tipo, hex.EncodeToString(huella[:]), int64(len(datos)), mini != nil)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err)) // el objeto ya subido lo borra la purga
		return
	}
	httpx.JSON(w, r, http.StatusCreated, adj)
}

// miniaturaDe devuelve la miniatura JPEG de una imagen, o nil si no es una imagen que
// se pueda reducir sin riesgo.
func miniaturaDe(tipo string, datos []byte) []byte {
	if tipo != "image/png" && tipo != "image/jpeg" && tipo != "image/gif" {
		return nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(datos))
	if err != nil || cfg.Width*cfg.Height > maxPixelesMiniatura {
		return nil
	}
	mini, err := miniatura.Render(datos, ladoMiniatura, "image/jpeg")
	if err != nil {
		return nil
	}
	return mini
}

func (h *Handlers) descargarAdjunto(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	rid, err2 := id(r, "rid")
	aid, err3 := id(r, "aid")
	if err = firstErr(err, err2, err3); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if h.sinAlmacen(w, r) {
		return
	}
	adj, obj, err := h.Store.AdjuntoParaLeer(r.Context(), a, rid, aid)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	clave, tipo := obj.Clave, domain.TipoAlServir(adj.Mime)
	esMiniatura := strings.HasSuffix(r.URL.Path, "/thumb")
	if esMiniatura {
		if !adj.HasThumb {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		clave, tipo = clave+store.SufijoMiniatura, "image/jpeg"
	}
	cuerpo, err := h.Objetos.Get(r.Context(), tenancy.Tenant{Slug: obj.Tenant}, clave)
	if err != nil {
		httpx.Fail(w, r, fmt.Errorf("leer adjunto: %w", err))
		return
	}
	defer func() { _ = cuerpo.Close() }()
	conPlazoDeTransferencia(w)
	// Siempre como descarga, sin que el navegador adivine el tipo y sin poder ejecutar
	// nada si aun así lo abriera: un adjunto es un fichero de otra persona.
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, no-store")
	disposicion := "attachment"
	if esMiniatura {
		disposicion = "inline" // la generamos nosotros: es un JPEG recodificado
	}
	w.Header().Set("Content-Disposition", disposicion+"; filename*=UTF-8''"+url.PathEscape(adj.Name))
	_, _ = io.Copy(w, cuerpo)
}

func (h *Handlers) borrarAdjunto(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	rid, err2 := id(r, "rid")
	aid, err3 := id(r, "aid")
	if err = firstErr(err, err2, err3); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.BorrarAdjunto(r.Context(), a, rid, aid))
}

// nombresDeAdjuntos es la celda de un campo de adjuntos en la exportación: sus nombres.
func nombresDeAdjuntos(adjuntos []domain.Adjunto) string {
	nombres := make([]string, len(adjuntos))
	for i, x := range adjuntos {
		nombres[i] = x.Name
	}
	return strings.Join(nombres, "; ")
}
