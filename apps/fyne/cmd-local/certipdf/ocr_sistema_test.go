// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// sistemaFalso es el OCR del sistema de mentira (el de Windows, en las pruebas).
type sistemaFalso struct {
	esta     bool
	idiomas  []string
	palabras []api.PDFPalabra
	err      error
	pedidos  []string
}

func (s *sistemaFalso) Disponible() bool { return s.esta }
func (s *sistemaFalso) Idiomas() ([]string, error) {
	return s.idiomas, s.err
}

func (s *sistemaFalso) Reconocer(_ context.Context, _ []byte, idiomas string) ([]api.PDFPalabra, error) {
	s.pedidos = append(s.pedidos, idiomas)
	return s.palabras, s.err
}

func sinIA() func(context.Context, api.PeticionIA) (string, error) { return nil }

// Sin tesseract, el OCR del sistema va antes que el modelo: da sus idiomas y sus palabras,
// y la nota dice que no sale nada del equipo. Con tesseract, tesseract sigue mandando.
func TestElOCRDelSistemaVaSinTesseractYAntesQueElModelo(t *testing.T) {
	s := &sistemaFalso{esta: true, idiomas: []string{"spa", "eng"}, palabras: []api.PDFPalabra{{Texto: "Contrato", X: 1, Y: 2, Ancho: 3, Alto: 4}}}
	usado := false
	ia := func(context.Context, api.PeticionIA) (string, error) { usado = true; return "x", nil }
	r := &reconocedor{asistir: func() func(context.Context, api.PeticionIA) (string, error) { return ia }, buscar: sinTesseract, sistema: s}
	if idiomas, err := r.Idiomas(t.Context()); err != nil || !slices.Equal(idiomas, []string{"spa", "eng"}) {
		t.Fatal(idiomas, err)
	}
	got, err := r.Reconocer(t.Context(), pngDe(t, 10, 10), "image/png", "spa+eng")
	if err != nil || len(got) != 1 || got[0].Texto != "Contrato" || usado || !slices.Equal(s.pedidos, []string{"spa+eng"}) {
		t.Fatal(got, err, usado, s.pedidos)
	}
	if nota := r.descripcion("es"); !strings.Contains(nota, "OCR de Windows") || !strings.Contains(nota, "no sale") {
		t.Fatal(nota)
	}
	if nota := r.descripcion("en"); !strings.Contains(nota, "Windows OCR") {
		t.Fatal(nota)
	}
}

// Cualquier fallo del OCR del sistema cae a lo de siempre: el modelo si lo hay; sin él, el
// error. Y si no está (otro sistema, sin idiomas), es como si no existiera.
func TestUnFalloDelOCRDelSistemaCaeALoDeSiempre(t *testing.T) {
	roto := errors.New("RecognizeAsync: HRESULT 0x80004005")
	s := &sistemaFalso{esta: true, err: roto}
	ia := func(context.Context, api.PeticionIA) (string, error) { return "Línea uno", nil }
	conIA := &reconocedor{asistir: func() func(context.Context, api.PeticionIA) (string, error) { return ia }, buscar: sinTesseract, sistema: s}
	got, err := conIA.Reconocer(t.Context(), pngDe(t, 100, 100), "image/png", "")
	if err != nil || len(got) != 1 || got[0].Texto != "Línea uno" {
		t.Fatal("con el modelo detrás:", got, err)
	}
	// Idiomas que fallan: los del modelo (ninguno que elegir).
	if idiomas, err := conIA.Idiomas(t.Context()); err != nil || len(idiomas) != 0 {
		t.Fatal(idiomas, err)
	}
	sinNada := &reconocedor{asistir: sinIA, buscar: sinTesseract, sistema: s}
	if _, err := sinNada.Reconocer(t.Context(), pngDe(t, 10, 10), "image/png", ""); !errors.Is(err, roto) {
		t.Fatal("sin modelo, el error del sistema:", err)
	}

	s.esta = false
	if _, err := sinNada.Idiomas(t.Context()); !errors.Is(err, api.ErrOCRNoDisponible) {
		t.Fatal("sin el OCR del sistema ni modelo no se ofrece:", err)
	}
	if _, err := sinNada.Reconocer(t.Context(), nil, "", ""); !errors.Is(err, api.ErrOCRNoDisponible) {
		t.Fatal(err)
	}
	if !strings.Contains(conIA.descripcion("es"), "se le envía la imagen") {
		t.Fatal("sin el del sistema, la nota es la del modelo")
	}
}

// El de verdad, fuera de Windows, no está y no cambia nada.
func TestElOCRDeWindowsFueraDeWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("en Windows depende de los idiomas instalados")
	}
	r := &reconocedor{asistir: sinIA, buscar: sinTesseract, sistema: ocrWindows{}}
	if r.delSistema() != nil {
		t.Fatal("el OCR de Windows fuera de Windows")
	}
	if _, err := r.Idiomas(t.Context()); !errors.Is(err, api.ErrOCRNoDisponible) {
		t.Fatal(err)
	}
}
