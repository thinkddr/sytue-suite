// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/miniatura"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

const maxTamanoRecurso = 20 << 20 // 20 MiB

func (h *Handlers) listarRecursos(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	recursos, err := h.Store.ListarRecursos(r.Context(), siteID)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	if recursos == nil {
		recursos = []domain.Recurso{}
	}
	httpx.JSON(w, r, http.StatusOK, recursos)
}

func (h *Handlers) subirRecurso(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	cuerpo, err := io.ReadAll(io.LimitReader(r.Body, maxTamanoRecurso+1))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("read_error").Wrap(err))
		return
	}
	if len(cuerpo) > maxTamanoRecurso {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("file_too_large").WithMessage("El recurso supera el límite de 20 MiB"))
		return
	}
	if len(cuerpo) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("empty_file"))
		return
	}

	nombre := strings.TrimSpace(r.URL.Query().Get("name"))
	if len(nombre) > 200 {
		nombre = nombre[:200]
	}

	// Comprobar formato por bytes mágicos (A7, §6.3).
	if bytes.HasPrefix(cuerpo, []byte("%PDF-")) {
		h.procesarPDF(w, r, siteID, cuerpo, nombre)
		return
	}
	if esJPEG(cuerpo) || esPNG(cuerpo) {
		h.procesarImagen(w, r, siteID, cuerpo, nombre)
		return
	}

	httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("unsupported_media_type").WithMessage("Solo se admiten imágenes JPEG, PNG o documentos PDF"))
}

func esJPEG(b []byte) bool {
	return len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF
}

func esPNG(b []byte) bool {
	return len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n"
}

func (h *Handlers) procesarPDF(w http.ResponseWriter, r *http.Request, siteID uuid.UUID, datos []byte, nombre string) {
	if _, err := h.Store.SitioCompleto(r.Context(), siteID); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	sum := sha256.Sum256(datos)
	fileHash := hex.EncodeToString(sum[:16])

	if nombre == "" {
		nombre = fileHash + ".pdf"
	}
	if !strings.HasSuffix(strings.ToLower(nombre), ".pdf") {
		nombre += ".pdf"
	}

	// Subir a S3 si hay almacén configurado.
	if h.Objetos != nil {
		t, _ := tenancy.From(r.Context())
		clave := fmt.Sprintf("shared/sitios/%s/recursos/%s.pdf", siteID.String(), fileHash)
		if err := h.Objetos.Put(r.Context(), t, clave, bytes.NewReader(datos), int64(len(datos)), objectstore.PutOptions{ContentType: "application/pdf"}); err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
	}

	rec, err := h.Store.GuardarRecurso(r.Context(), domain.Recurso{
		SiteID:    siteID,
		File:      fileHash,
		Ext:       "pdf",
		Width:     0,
		Height:    0,
		Bytes:     int64(len(datos)),
		Variants:  []int{},
		Origin:    "upload",
		Name:      nombre,
		CreatedBy: sub(r),
	})
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, rec)
}

func (h *Handlers) procesarImagen(w http.ResponseWriter, r *http.Request, siteID uuid.UUID, datos []byte, nombre string) {
	// Guarda anti bomba de descompresión: por la memoria que pide, no solo por
	// los píxeles (miniatura.Cabe, FILE-07).
	cfg, _, err := image.DecodeConfig(bytes.NewReader(datos))
	if err != nil || !miniatura.Cabe(cfg) {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("invalid_image").WithMessage("Dimensiones o formato de imagen no soportados"))
		return
	}

	if _, err := h.Store.SitioCompleto(r.Context(), siteID); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	img, formato, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("decode_error").Wrap(err))
		return
	}

	ext := "jpg"
	mimeType := "image/jpeg"
	if formato == "png" || esPNG(datos) {
		ext = "png"
		mimeType = "image/png"
	}

	// Recodificar para normalizar y eliminar metadatos EXIF.
	var limpia bytes.Buffer
	if ext == "png" {
		if err := png.Encode(&limpia, img); err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
	} else {
		if err := jpeg.Encode(&limpia, img, &jpeg.Options{Quality: 82}); err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
	}

	normalizados := limpia.Bytes()
	sum := sha256.Sum256(normalizados)
	fileHash := hex.EncodeToString(sum[:16])

	if nombre == "" {
		nombre = fileHash + "." + ext
	}

	anchoOriginal := img.Bounds().Dx()
	altoOriginal := img.Bounds().Dy()
	ladoMayor := max(anchoOriginal, altoOriginal)

	// Variantes estándar (§5.1, §13.2): 480, 960, 1600, 2400 (solo las menores que el original).
	escalones := []int{480, 960, 1600, 2400}
	var admitidos []int
	for _, e := range escalones {
		if e < ladoMayor {
			admitidos = append(admitidos, e)
		}
	}

	// Subir original y variantes a S3.
	if h.Objetos != nil {
		t, _ := tenancy.From(r.Context())
		claveMaster := fmt.Sprintf("shared/sitios/%s/recursos/%s.%s", siteID.String(), fileHash, ext)
		_ = h.Objetos.Put(r.Context(), t, claveMaster, bytes.NewReader(normalizados), int64(len(normalizados)), objectstore.PutOptions{ContentType: mimeType})

		if len(admitidos) > 0 {
			cascada := miniatura.Cascada(img, admitidos)
			for lado, varImg := range cascada {
				var bVar bytes.Buffer
				if ext == "png" {
					_ = png.Encode(&bVar, varImg)
				} else {
					_ = jpeg.Encode(&bVar, varImg, &jpeg.Options{Quality: 82})
				}
				claveVar := fmt.Sprintf("shared/sitios/%s/recursos/%s-%d.%s", siteID.String(), fileHash, lado, ext)
				_ = h.Objetos.Put(r.Context(), t, claveVar, bytes.NewReader(bVar.Bytes()), int64(bVar.Len()), objectstore.PutOptions{ContentType: mimeType})
			}
		}
	}

	sort.Ints(admitidos)
	rec, err := h.Store.GuardarRecurso(r.Context(), domain.Recurso{
		SiteID:    siteID,
		File:      fileHash,
		Ext:       ext,
		Width:     anchoOriginal,
		Height:    altoOriginal,
		Bytes:     int64(len(normalizados)),
		Variants:  admitidos,
		Origin:    "upload",
		Name:      nombre,
		CreatedBy: sub(r),
	})
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, rec)
}

func (h *Handlers) borrarRecurso(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	aid, err := uuid.Parse(chi.URLParam(r, "aid"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_asset_id").Wrap(err))
		return
	}

	// El maestro y cada ancho salen del almacén en la transacción que borra la fila: si
	// el almacén falla, la fila se queda y se puede volver a borrar (WEB-04).
	var errAlmacen error
	confirmado := r.URL.Query().Get("confirm") == "published"
	err = h.Store.BorrarRecurso(r.Context(), siteID, aid, confirmado, func(ctx context.Context, rec domain.Recurso) error {
		if h.Objetos == nil {
			return nil
		}
		t, _ := tenancy.From(ctx)
		for _, nombre := range rec.Ficheros() {
			if err := h.Objetos.Delete(ctx, t, fmt.Sprintf("shared/sitios/%s/recursos/%s", siteID, nombre)); err != nil {
				errAlmacen = err
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, domain.ErrRecursoEnUso) {
			httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "asset_in_use", "The asset is currently in use in the site draft."))
			return
		}
		// La web publicada lo usa: quien lo pide avisa de que se dejará de ver allí y, si se
		// confirma, repite con ?confirm=published (WEB-04, «borrar de verdad, avisando»).
		if errors.Is(err, domain.ErrRecursoPublicado) {
			httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "asset_published",
				"The published site uses this asset: it will stop showing there. Repeat with ?confirm=published to delete it.").
				WithParams(map[string]any{"confirm": "published"}))
			return
		}
		if errAlmacen != nil {
			httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err))
			return
		}
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
