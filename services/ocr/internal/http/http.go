// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ocrhttp es la superficie HTTP de Sytue OCR: un endpoint que recibe la
// imagen de una página y devuelve sus palabras con su posición.
//
// NO SE EXPONE POR EL INGRESS. Solo lo llama Sytue Docs desde dentro del
// clúster. Aun así exige el JWT del usuario —que Docs reenvía— en vez de fiarse
// de la red: un servicio que reconoce el contenido de nóminas y contratos no
// debe estar abierto a cualquier pod del namespace.
package ocrhttp

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/ocr/internal/config"
	"git.sytue.eu/sytue/one/services/ocr/internal/tesseract"
)

// Deps son las dependencias del router.
type Deps struct {
	Cfg      config.Config
	Verifier *authn.Verifier
	Motor    *tesseract.Motor
}

type handlers struct {
	cfg   config.Config
	motor *tesseract.Motor
	// turnos limita cuántos reconocimientos corren a la vez. Es un semáforo y no
	// un rate-limit: lo que hay que proteger no es la frecuencia, es la CPU.
	turnos chan struct{}
}

// NewRouter construye el router de Sytue OCR.
func NewRouter(d Deps) http.Handler {
	// El plazo del router supera el del motor para que gane el handler, que
	// devuelve un error legible, y no un corte seco del transporte.
	r := httpx.NewRouter(httpx.RouterOptions{RequestTimeout: d.Cfg.Timeout + 30*time.Second})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second)

	h := &handlers{
		cfg:    d.Cfg,
		motor:  d.Motor,
		turnos: make(chan struct{}, d.Cfg.MaxConcurrentes),
	}

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)
		api.Post("/recognize", h.reconocer)
		// Qué idiomas hay instalados. Lo consulta la interfaz para ofrecer solo
		// los que de verdad van a funcionar en vez de una lista que decepciona.
		api.Get("/languages", h.idiomas)
	})
	return r
}

func (h *handlers) idiomas(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, r, http.StatusOK, map[string]any{"idiomas": h.motor.Idiomas()})
}

// maxCuerpo acota la imagen que se acepta, con margen para el envoltorio
// multipart.
const maxCuerpo = 44 << 20

func (h *handlers) reconocer(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpo)
	//nolint:gosec // G120: el cuerpo ya está acotado por http.MaxBytesReader
	if err := r.ParseMultipartForm(maxCuerpo); err != nil {
		var grande *http.MaxBytesError
		if errors.As(err, &grande) {
			httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge,
				"image_too_large", "The image is too large."))
			return
		}
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			`Send the image as multipart/form-data in the "file" field.`,
		))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	fichero, _, err := r.FormFile("file")
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(`The "file" field is missing.`))
		return
	}
	defer func() { _ = fichero.Close() }()
	imagen, err := io.ReadAll(io.LimitReader(fichero, maxCuerpo))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The image could not be read."))
		return
	}

	// El turno se coge ANTES de arrancar Tesseract y se espera con el contexto de
	// la petición: si quien llama se cansa y corta, el trabajo no llega a
	// empezar. Sin esto, una avalancha que ya nadie está esperando seguiría
	// ocupando la CPU durante minutos.
	select {
	case h.turnos <- struct{}{}:
		defer func() { <-h.turnos }()
	case <-r.Context().Done():
		httpx.Fail(w, r, httpx.NewError(http.StatusServiceUnavailable,
			"ocr_busy", "The recognition queue is full; try again in a moment."))
		return
	}

	resultado, err := h.motor.Reconocer(r.Context(), imagen, r.FormValue("idiomas"))
	if err != nil {
		httpx.Fail(w, r, traducirError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, resultado)
}

// traducirError convierte los errores del motor en respuestas que la interfaz
// pueda distinguir. Un idioma que falta lo arregla el usuario; un motor que no
// está lo arregla quien opera, y confundirlos manda a la persona equivocada.
func traducirError(err error) error {
	switch {
	case errors.Is(err, tesseract.ErrIdiomaDesconocido):
		return httpx.ErrBadRequest.
			WithMessage("That language is not installed on this server.").
			WithCode("ocr_language_unavailable")
	case errors.Is(err, tesseract.ErrTiempoAgotado):
		return httpx.NewError(http.StatusGatewayTimeout, "ocr_timeout",
			"Recognising this page took too long.")
	case errors.Is(err, tesseract.ErrNoDisponible):
		return httpx.NewError(http.StatusServiceUnavailable, "ocr_engine_missing",
			"The recognition engine is not available on this server.")
	default:
		return httpx.ErrBadRequest.WithMessage(err.Error())
	}
}
