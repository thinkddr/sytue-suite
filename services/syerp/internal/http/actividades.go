// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// Tareas y documentos, colgados de cualquier registro (las «actividades» y los
// adjuntos de Odoo). Quien puede ver el registro ve sus tareas y sus documentos;
// quien puede editarlo, los crea.

var (
	errAdjuntoTipo = httpx.NewError(http.StatusUnsupportedMediaType, "adjunto_tipo",
		"Upload a PDF, an image (JPEG, PNG or WebP) or an e-invoice XML.")
	errAdjuntoGrande = httpx.NewError(http.StatusRequestEntityTooLarge, "adjunto_grande",
		"The file is larger than 10 MB.")
)

// tiposAdjunto son los que se aceptan, por su tipo DETECTADO en el contenido: la
// extensión y la cabecera las decide quien sube, y un «factura.pdf» que es un HTML
// es exactamente el fichero que no se quiere servir luego a otra persona.
var tiposAdjunto = map[string]string{
	"application/pdf": ".pdf", "image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp",
	"application/xml": ".xml",
}

// accesoRegistro comprueba el nivel sobre lo que cuelga una tarea o un documento.
func (h *Handlers) accesoRegistro(ctx context.Context, modelo, nivel string) bool {
	modulo := h.moduloDe(modelo)
	return modulo != "" && h.puede(ctx, modulo, nivel)
}

// Actividades — GET /api/v1/actividades?estado=&modelo=&registro=&mias=1&hasta=.
func (h *Handlers) Actividades(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.FiltroActividades{
		Estado: q.Get("estado"), Modelo: q.Get("modelo"), Registro: q.Get("registro"),
		HastaVencimiento: q.Get("hasta"),
	}
	if f.HastaVencimiento != "" {
		if _, err := time.Parse(time.DateOnly, f.HastaVencimiento); err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("hasta must be YYYY-MM-DD."))
			return
		}
	}
	ctx := r.Context()
	if q.Get("mias") == "1" {
		f.ResponsableSub = sub(r)
	}
	lista, err := h.store.Actividades(ctx, f)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Se ven las tareas de lo que se puede ver, y las propias aunque no.
	visibles := make([]store.Actividad, 0, len(lista))
	var ids []string
	for _, a := range lista {
		if a.Modelo == "" || a.ResponsableSub == sub(r) || h.accesoRegistro(ctx, a.Modelo, permisos.Lectura) {
			visibles = append(visibles, a)
			if _, err := uuid.Parse(a.Registro); err == nil {
				ids = append(ids, a.Registro)
			}
		}
	}
	if etiquetas, err := h.store.ResolverEtiquetas(ctx, ids); err == nil {
		for i := range visibles {
			visibles[i].RegistroEtiqueta = etiquetas[visibles[i].Registro]
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"actividades": visibles})
}

// CrearActividad — POST /api/v1/actividades.
func (h *Handlers) CrearActividad(w http.ResponseWriter, r *http.Request) {
	var a store.Actividad
	if err := httpx.DecodeJSON(w, r, &a); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a.Resumen = validation.TruncateRunes(strings.TrimSpace(a.Resumen), 300)
	a.Notas = validation.TruncateRunes(strings.TrimSpace(a.Notas), 4000)
	v := validation.New().Required("resumen", a.Resumen)
	if !slices.Contains(store.TiposDeActividad, a.Tipo) {
		v.Add("tipo", "tipo desconocido")
	}
	if _, err := time.Parse(time.DateOnly, a.Vence); err != nil {
		v.Add("vence", "fecha AAAA-MM-DD")
	}
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, errorDeValidacion(err))
		return
	}
	ctx := r.Context()
	if a.Modelo != "" && !h.accesoRegistro(ctx, a.Modelo, permisos.Usuario) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	if a.ResponsableSub == "" || a.ResponsableSub == sub(r) {
		claims, _ := authn.FromContext(ctx)
		a.ResponsableSub, a.ResponsableNombre = claims.Subject, cmp.Or(claims.Name, claims.Email)
	} else {
		// Otra persona: tiene que ser de la organización —Sytue ID la resuelve con la
		// sesión de quien pide, uno a uno y sin el tope de una página del directorio—, y
		// su nombre es el de allí, no el que mande el cliente (es el que sale en su campana).
		correo, nombre, err := peticiones.Persona(ctx, h.cfg.IDBaseURL, tokenDe(r), a.ResponsableSub)
		switch {
		case errors.Is(err, peticiones.ErrSinPersona):
			httpx.Fail(w, r, errorDeValidacion(validation.New().Add("responsable_sub", "no es de la organización").Err()))
			return
		case err != nil:
			log.From(ctx).Warn("erp: sytue id no resuelve al responsable", "error", err)
			httpx.Fail(w, r, httpx.ErrUnavailable)
			return
		}
		a.ResponsableNombre = cmp.Or(nombre, correo)
	}
	creada, err := h.store.CrearActividad(ctx, a, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	if creada.ResponsableSub != sub(r) {
		h.avisarTarea(ctx, creada) // una tarea que te pone otro
	}
	httpx.JSON(w, r, http.StatusCreated, creada)
}

// CerrarActividad — POST /api/v1/actividades/{id}/hecha y /cancelar.
func (h *Handlers) CerrarActividad(estado string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		a, err := h.store.Actividad(ctx, chi.URLParam(r, "id"))
		if err != nil {
			httpx.Fail(w, r, errorDeEscritura(err))
			return
		}
		if a.ResponsableSub != sub(r) && a.Modelo != "" && !h.accesoRegistro(ctx, a.Modelo, permisos.Usuario) {
			httpx.Fail(w, r, errSinPermiso)
			return
		}
		cerrada, err := h.store.CerrarActividad(ctx, a.ID, estado)
		if err != nil {
			httpx.Fail(w, r, errorDeEscritura(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, cerrada)
	}
}

// Adjuntos — GET /api/v1/adjuntos?modelo=&registro=.
func (h *Handlers) Adjuntos(w http.ResponseWriter, r *http.Request) {
	modelo, registro := r.URL.Query().Get("modelo"), r.URL.Query().Get("registro")
	if !h.accesoRegistro(r.Context(), modelo, permisos.Lectura) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	lista, err := h.store.Adjuntos(r.Context(), modelo, registro)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"adjuntos": lista})
}

// SubirAdjunto — POST /api/v1/adjuntos?modelo=&registro=&nombre= con el fichero en
// el cuerpo. El nombre va en la consulta y no en una cabecera: el BFF de la app
// solo reenvía las cabeceras que conoce.
func (h *Handlers) SubirAdjunto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modelo, registro := r.URL.Query().Get("modelo"), strings.TrimSpace(r.URL.Query().Get("registro"))
	if registro == "" || len(registro) > 200 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("registro is required."))
		return
	}
	if !h.accesoRegistro(ctx, modelo, permisos.Usuario) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	if _, err := uuid.Parse(registro); err == nil {
		if _, err := h.store.Obtener(ctx, modelo, registro); err != nil {
			httpx.Fail(w, r, errorDeEscritura(err))
			return
		}
	}
	contenido, err := io.ReadAll(http.MaxBytesReader(w, r.Body, store.MaxAdjunto+1))
	if err != nil || len(contenido) > store.MaxAdjunto {
		httpx.Fail(w, r, errAdjuntoGrande)
		return
	}
	tipo := tipoDetectado(contenido)
	ext, ok := tiposAdjunto[tipo]
	if len(contenido) == 0 || !ok {
		httpx.Fail(w, r, errAdjuntoTipo)
		return
	}
	nombre := r.URL.Query().Get("nombre")
	nombre = validation.TruncateRunes(strings.TrimSpace(filepath.Base(validation.SanitizeText(nombre))), 200)
	if nombre == "" || nombre == "." || nombre == "/" {
		nombre = "documento"
	}
	if !strings.EqualFold(filepath.Ext(nombre), ext) {
		nombre += ext
	}
	a, cerradas, err := h.store.GuardarAdjunto(ctx, modelo, registro, nombre, tipo, contenido, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"adjunto": a, "tareas_hechas": cerradas})
}

// tipoDetectado es el tipo del contenido, sin parámetros («text/xml» de una
// factura electrónica se guarda como application/xml).
func tipoDetectado(b []byte) string {
	t, _, _ := mime.ParseMediaType(http.DetectContentType(b))
	if t == "text/xml" {
		return "application/xml"
	}
	return t
}

// DescargarAdjunto — GET /api/v1/adjuntos/{id}.
func (h *Handlers) DescargarAdjunto(w http.ResponseWriter, r *http.Request) {
	a, contenido, err := h.store.AdjuntoConContenido(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	if !h.accesoRegistro(r.Context(), a.Modelo, permisos.Lectura) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	disposicion := "attachment"
	if r.URL.Query().Get("ver") == "1" && (a.Tipo == "application/pdf" || strings.HasPrefix(a.Tipo, "image/")) {
		disposicion = "inline"
	}
	w.Header().Set("Content-Type", a.Tipo)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposicion, map[string]string{"filename": a.Nombre}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, a.Nombre, a.CreadoEn, bytes.NewReader(contenido))
}

// BorrarAdjunto — DELETE /api/v1/adjuntos/{id}.
func (h *Handlers) BorrarAdjunto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, _, err := h.store.AdjuntoConContenido(ctx, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	if !h.accesoRegistro(ctx, a.Modelo, permisos.Usuario) {
		httpx.Fail(w, r, errSinPermiso)
		return
	}
	if err := h.store.BorrarAdjunto(ctx, a.ID); err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
