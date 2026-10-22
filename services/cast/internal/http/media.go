// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

// Media — GET /media/{id}/{token}/{ruta...} (CAST-ENCARGO §5.1 y §5.3): los
// ficheros del vídeo (HLS, WebM, póster, original) desde el almacén en streaming,
// con Range cuando viene, `Cache-Control: private, max-age=3600` y el Content-Type
// que toca. El token firmado es la verja: lo emitió `play` tras pasar la ACL.
//
// ponytail: proxy por el servicio; el relevo es una CDN o URL firmadas de S3
// cuando haya tráfico real.
func (h *Handlers) Media(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ruta := chi.URLParam(r, "*")
	f, err := h.svc.Abrir(r.Context(), id, chi.URLParam(r, "token"), ruta, r.Header.Get("Range"))
	if err != nil {
		var rango *service.RangoNoSatisfacible
		switch {
		case errors.As(err, &rango):
			w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(rango.Total, 10))
			httpx.Fail(w, r, httpx.NewError(http.StatusRequestedRangeNotSatisfiable, "range_not_satisfiable", "The requested range is not satisfiable."))
		case errors.Is(err, domain.ErrNoExiste):
			httpx.Fail(w, r, httpx.ErrNotFound)
		case errors.Is(err, service.ErrSinAlmacen):
			httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Media storage is not available."))
		default:
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		}
		return
	}
	defer f.Cuerpo.Close()

	hdr := w.Header()
	hdr.Set("Content-Type", f.ContentType)
	hdr.Set("Cache-Control", "private, max-age=3600")
	hdr.Set("Accept-Ranges", "bytes")
	hdr.Set("X-Content-Type-Options", "nosniff")
	if !f.Modificado.IsZero() {
		hdr.Set("Last-Modified", f.Modificado.UTC().Format(http.TimeFormat))
	}
	if f.Tamano > 0 {
		hdr.Set("Content-Length", strconv.FormatInt(f.Tamano, 10))
	}
	if f.ContentRange != "" {
		hdr.Set("Content-Range", f.ContentRange)
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, f.Cuerpo); err != nil {
		// Quien cierra el reproductor a medias corta la copia: no es un error del servicio.
		log.From(r.Context()).Debug("cast/media: copia interrumpida", "video", id, "ruta", ruta, "error", err)
	}
}
