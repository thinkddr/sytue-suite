// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // las imágenes de un escaneo llegan en JPEG o PNG
	_ "image/png"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/ocrwindows"
)

// El OCR de la edición local (P-07). Hoy el reconocimiento de la suite es un servicio
// aparte con Tesseract; aquí no hay servicio:
//
//   - si el equipo tiene `tesseract` instalado, se usa ese: nada sale del equipo y da la
//     caja de cada palabra, así que la capa de texto cae encima de lo escrito;
//   - si no, en Windows, el OCR del propio sistema (`Windows.Media.Ocr`, con los idiomas
//     de reconocimiento que ya trae: `nucleo/sistema/ocrwindows`), que también da las cajas
//     y tampoco saca nada del equipo. Cualquier fallo suyo cae a lo siguiente;
//   - si no, y la persona configuró un modelo de IA con visión, se le manda la imagen de
//     cada página para que la transcriba (con el aviso de primera vez de `avisoIA`). El
//     modelo no da cajas: cada línea se coloca en su franja de la página, de arriba abajo,
//     que basta para buscar y copiar pero no para seleccionar palabra a palabra;
//   - sin ninguno de los dos, «reconocer texto» no se ofrece.
//
// Pendiente: el OCR propio de Apple (Vision).

// reconocedor elige en cada llamada: la IA puede cambiar en Ajustes, y tesseract
// instalarse con la app abierta.
type reconocedor struct {
	// asistir es la IA de la persona, ya con el aviso de primera vez; nil sin IA.
	asistir func() func(context.Context, api.PeticionIA) (string, error)
	// buscar encuentra tesseract (exec.LookPath; las pruebas ponen otro).
	buscar func() (string, bool)
	// sistema es el OCR propio del sistema (el de Windows); nil, ninguno.
	sistema ocrDelSistema
}

// ocrDelSistema es el OCR que trae el sistema operativo: hoy el de Windows
// (`ocrWindows`); las pruebas ponen otro.
type ocrDelSistema interface {
	Disponible() bool
	Idiomas() ([]string, error)
	Reconocer(ctx context.Context, imagen []byte, idiomas string) ([]api.PDFPalabra, error)
}

// ocrWindows es `nucleo/sistema/ocrwindows` con las palabras de la API. Fuera de Windows
// no está disponible y no se usa.
type ocrWindows struct{}

func (ocrWindows) Disponible() bool           { return ocrwindows.Disponible() }
func (ocrWindows) Idiomas() ([]string, error) { return ocrwindows.Idiomas() }
func (ocrWindows) Reconocer(ctx context.Context, imagen []byte, idiomas string) ([]api.PDFPalabra, error) {
	ps, err := ocrwindows.Reconocer(ctx, imagen, idiomas)
	if err != nil {
		return nil, err
	}
	out := make([]api.PDFPalabra, len(ps))
	for i, p := range ps {
		out[i] = api.PDFPalabra{Texto: p.Texto, X: p.X, Y: p.Y, Ancho: p.Ancho, Alto: p.Alto}
	}
	return out, nil
}

// delSistema es el OCR del sistema si lo hay y está listo.
func (r *reconocedor) delSistema() ocrDelSistema {
	if r.sistema != nil && r.sistema.Disponible() {
		return r.sistema
	}
	return nil
}

func buscarTesseract() (string, bool) {
	r, err := exec.LookPath("tesseract")
	return r, err == nil
}

// Idiomas son los de tesseract o, sin él, los del OCR del sistema (con los mismos
// códigos); con el modelo, ninguno que elegir.
func (r *reconocedor) Idiomas(ctx context.Context) ([]string, error) {
	if ruta, ok := r.buscar(); ok {
		out, err := exec.CommandContext(ctx, ruta, "--list-langs").Output()
		if err != nil {
			return nil, err
		}
		var idiomas []string
		for _, l := range strings.Split(string(out), "\n") {
			l = strings.TrimSpace(l)
			// La primera línea es «List of available languages in "…" (N):».
			if l == "" || strings.ContainsAny(l, " :\"") {
				continue
			}
			idiomas = append(idiomas, l)
		}
		return idiomas, nil
	}
	if s := r.delSistema(); s != nil {
		if idiomas, err := s.Idiomas(); err == nil {
			return idiomas, nil
		}
	}
	if r.asistir() != nil {
		return nil, nil
	}
	return nil, api.ErrOCRNoDisponible
}

// Reconocer lee una imagen con tesseract o, si no hay, con el OCR del sistema y, si no
// hay o falla, con el modelo de la persona.
func (r *reconocedor) Reconocer(ctx context.Context, imagen []byte, tipo, idiomas string) ([]api.PDFPalabra, error) {
	if ruta, ok := r.buscar(); ok {
		return conTesseract(ctx, ruta, imagen, idiomas)
	}
	var errSistema error
	if s := r.delSistema(); s != nil {
		palabras, err := s.Reconocer(ctx, imagen, idiomas)
		if err == nil || ctx.Err() != nil {
			return palabras, err
		}
		slog.Warn("certipdf: el OCR del sistema ha fallado", "error", err)
		errSistema = err
	}
	if f := r.asistir(); f != nil {
		return conModelo(ctx, f, imagen, tipo)
	}
	if errSistema != nil {
		return nil, errSistema
	}
	return nil, api.ErrOCRNoDisponible
}

// descripcion es la nota de la herramienta, en el idioma de la app: dónde se reconoce.
func (r *reconocedor) descripcion(idioma i18n.Locale) string {
	if _, ok := r.buscar(); ok {
		return propio(idioma, "ocrTesseract")
	}
	if r.delSistema() != nil {
		return propio(idioma, "ocrWindows")
	}
	return propio(idioma, "ocrIA")
}

// conTesseract pasa la imagen por la entrada estándar y lee el TSV de palabras.
func conTesseract(ctx context.Context, ruta string, imagen []byte, idiomas string) ([]api.PDFPalabra, error) {
	args := []string{"stdin", "stdout"}
	if idiomas != "" {
		args = append(args, "-l", idiomas)
	}
	//nolint:gosec // G204: el tesseract del PATH (buscarTesseract) con argumentos sueltos, sin intérprete de órdenes
	cmd := exec.CommandContext(ctx, ruta, append(args, "tsv")...)
	cmd.Stdin = bytes.NewReader(imagen)
	// Con sus hilos de OpenMP, tesseract se pelea consigo mismo y con la app: una página
	// pasa de un segundo a más de un minuto (medido). Uno basta para una página.
	if os.Getenv("OMP_THREAD_LIMIT") == "" {
		cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")
	}
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tesseract: %w: %s", err, strings.TrimSpace(errOut.String()))
	}
	return palabrasDeTSV(out), nil
}

// palabrasDeTSV lee las filas de nivel 5 (palabra) del TSV de tesseract:
// level page block par line word left top width height conf text.
func palabrasDeTSV(tsv []byte) []api.PDFPalabra {
	var out []api.PDFPalabra
	s := bufio.NewScanner(bytes.NewReader(tsv))
	s.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for s.Scan() {
		c := strings.Split(s.Text(), "\t")
		if len(c) < 12 || c[0] != "5" {
			continue
		}
		texto := strings.TrimSpace(c[11])
		conf, err := strconv.ParseFloat(c[10], 64)
		if texto == "" || err != nil || conf < 0 {
			continue
		}
		n := make([]int, 4)
		ok := true
		for i := range n {
			if n[i], err = strconv.Atoi(c[6+i]); err != nil {
				ok = false
			}
		}
		if ok {
			out = append(out, api.PDFPalabra{Texto: texto, X: n[0], Y: n[1], Ancho: n[2], Alto: n[3]})
		}
	}
	return out
}

// instruccionTranscribir pide líneas, no párrafos: cada una va a su franja.
const instruccionTranscribir = "Transcribe literalmente el texto de esta página escaneada, línea a línea, de arriba abajo: una línea del papel en cada línea de tu respuesta. " +
	"No resumas, no corrijas y no completes lo que no se lea. Si la página no tiene texto, responde con una cadena vacía. Devuelve solo la transcripción."

// conModelo le pide la transcripción al modelo y reparte las líneas por la imagen.
func conModelo(ctx context.Context, asistir func(context.Context, api.PeticionIA) (string, error), imagen []byte, tipo string) ([]api.PDFPalabra, error) {
	conf, _, err := image.DecodeConfig(bytes.NewReader(imagen))
	if err != nil || conf.Width <= 0 || conf.Height <= 0 {
		return nil, errors.New("ocr: la imagen de la página no se puede leer")
	}
	texto, err := asistir(ctx, api.PeticionIA{
		Accion: "transcribir", Instruccion: instruccionTranscribir, Tipo: "pdf", Modo: "calidad",
		Imagen: imagen, ImagenTipo: tipo,
	})
	if err != nil {
		return nil, err
	}
	return lineasEnFranjas(texto, conf.Width, conf.Height), nil
}

// lineasEnFranjas coloca cada línea no vacía en una franja horizontal de la imagen, con
// un margen del 5 %: el orden de lectura se conserva y el texto cae sobre la página.
func lineasEnFranjas(texto string, ancho, alto int) []api.PDFPalabra {
	var lineas []string
	for _, l := range strings.Split(texto, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lineas = append(lineas, l)
		}
	}
	if len(lineas) == 0 {
		return nil
	}
	mx, my := ancho/20, alto/20
	franja := (alto - 2*my) / len(lineas)
	if franja < 1 {
		franja = 1
	}
	out := make([]api.PDFPalabra, 0, len(lineas))
	for i, l := range lineas {
		out = append(out, api.PDFPalabra{Texto: l, X: mx, Y: my + i*franja, Ancho: max(1, ancho-2*mx), Alto: max(1, franja*4/5)})
	}
	return out
}
