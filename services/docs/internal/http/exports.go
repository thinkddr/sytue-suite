// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

func (h *Handler) listExports(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer); err != nil {
		fail(w, r, err)
		return
	}
	blobs, err := h.store.ListBlobs(r.Context(), docID(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"exports": blobs})
}

type createExportReq struct {
	Format domain.BlobFormat `json:"format"`
	// Template pide el paquete como PLANTILLA de Office u OpenDocument (.dotx .xltx .potx
	// .ott .ots .otp). Solo con ooxml u odf.
	Template bool `json:"template,omitempty"`
	// Accessible elige el PDF: etiquetado y conforme a PDF/UA-1 (por defecto, si falta) o el
	// PDF simple de siempre, de catorce fuentes estándar y sin estructura. Solo con pdf.
	Accessible *bool `json:"accessible,omitempty"`
}

// createExport materializa el documento al formato pedido (OOXML/ODF/PDF/HTML/MD)
// y lo sube a Object Storage, registrando sus metadatos. Requiere lectura.
func (h *Handler) createExport(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	doc, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer)
	if err != nil {
		fail(w, r, err)
		return
	}
	var req createExportReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !req.Format.Valid() {
		fail(w, r, domain.ErrInvalidFormat)
		return
	}
	if h.objects == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Object storage is unavailable."))
		return
	}

	state, seq, err := h.svc.LoadFull(r.Context(), subj, doc.ID, h.hydrateFromS3)
	if err != nil {
		fail(w, r, err)
		return
	}
	comments, err := h.commentsForExport(r, doc.ID, doc.Kind, req.Format)
	if err != nil {
		fail(w, r, err)
		return
	}
	data, contentType, ext, informe, err := materializarExport(doc, state, req, comments, httpx.LanguageFromRequest(r))
	if err == nil && req.Template {
		data, contentType, ext, err = domain.ComoPlantilla(doc.Kind, req.Format, data)
	}
	if err != nil {
		fail(w, r, err)
		return
	}

	key := exportKey(doc.ID, seq, uuid.NewString(), ext)
	if err := h.uploadExport(r.Context(), key, contentType, data); err != nil {
		fail(w, r, err)
		return
	}
	blob, err := h.store.RecordBlob(r.Context(), domain.Blob{
		DocumentID:  doc.ID,
		Format:      req.Format,
		BlobKey:     key,
		Seq:         seq,
		SizeBytes:   int64(len(data)),
		ContentType: contentType,
		CreatedBy:   subj.Sub,
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	resp := map[string]any{"export": blob}
	if informe != nil {
		// Lo que el PDF accesible no pudo llevar, para que la interfaz avise.
		resp["pdf"] = map[string]any{
			"accessible":         true,
			"images_without_alt": informe.ImagenesSinAlt,
			"images_lost":        informe.ImagenesPerdidas,
			"replaced_chars":     len(informe.Sustituidos),
		}
	}
	httpx.JSON(w, r, http.StatusCreated, resp)
}

// materializarExport serializa el documento al formato pedido. El PDF sale accesible
// (etiquetado, PDF/UA-1) salvo que se pida lo contrario; el resto de formatos, como siempre.
// El informe solo viene con el PDF accesible: lo que no pudo llevar al fichero.
func materializarExport(doc *domain.Document, state *domain.State, req createExportReq, comments []domain.Comment, lang string) (data []byte, contentType, ext string, informe *domain.InformePDF, err error) {
	if req.Format == domain.FormatPDF && (req.Accessible == nil || *req.Accessible) {
		var inf domain.InformePDF
		data, inf, err = domain.ExportPDFAccesible(doc.Kind, doc.Title, lang, state)
		return data, "application/pdf", "pdf", &inf, err
	}
	data, contentType, ext, err = domain.ExportWithComments(doc.Kind, doc.Title, state, req.Format, comments)
	return data, contentType, ext, nil, err
}

// downloadExport sirve un export ya materializado. Para descargas grandes
// devuelve una URL prefirmada de S3; si el cliente pide ?inline=true hace
// streaming a través del servicio (útil en dev sin acceso directo a S3).
func (h *Handler) downloadExport(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer); err != nil {
		fail(w, r, err)
		return
	}
	if h.objects == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	blob, err := h.store.GetBlob(r.Context(), docID(r), chi.URLParam(r, "blobId"))
	if err != nil {
		fail(w, r, err)
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}

	if r.URL.Query().Get("inline") == "true" {
		rc, err := h.objects.Get(r.Context(), t, blob.BlobKey)
		if err != nil {
			fail(w, r, err)
			return
		}
		defer rc.Close()
		cabecerasDeExportacion(w.Header(), blob.ContentType)
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, rc)
		return
	}

	u, err := h.objects.PresignGet(r.Context(), t, blob.BlobKey, 10*time.Minute)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"url":          u.String(),
		"content_type": blob.ContentType,
		"size_bytes":   blob.SizeBytes,
		"format":       blob.Format,
	})
}

// cabecerasDeExportacion son las de un export servido desde ESTE origen. Es contenido de
// quien edita el documento —el HTML exportado es el del editor, sin sanear—, así que se
// DESCARGA y no se pinta: `attachment`, y un `sandbox` por si algo lo abre igualmente.
// Hoy solo lo pide el BFF, que lo vuelve a servir como adjunto; esto es para que no
// dependa de quién lo pida.
func cabecerasDeExportacion(h http.Header, contentType string) {
	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", "attachment")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
}

// commentsForExport son los comentarios del documento para los formatos que los llevan
// (DOCX y ODT), con el nombre de su autor; los demás no los piden.
func (h *Handler) commentsForExport(r *http.Request, docID string, kind domain.Kind, format domain.BlobFormat) ([]domain.Comment, error) {
	if kind != domain.KindDocument || (format != domain.FormatOOXML && format != domain.FormatODF) {
		return nil, nil
	}
	comments, err := h.store.ListComments(r.Context(), docID, true)
	if err != nil {
		return nil, err
	}
	h.conNombres(r, comments)
	return comments, nil
}
