// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

// # Reconocer texto en el dispositivo (P-07)
//
// `/pdf/ocr` como el de `services/docs` (pdfstudio.go): `estado` dice qué páginas no
// tienen texto y si hay reconocedor; `capa` escribe un texto ya reconocido; `reconocer`
// saca la imagen de cada página escaneada, se la da al reconocedor que ponga la app
// ([PDFLocal.Reconocedor]: el OCR del sistema o el modelo de visión de la persona) y
// escribe la capa invisible con [localpdf.CapaDeTextoPDF]. La conversión de coordenadas
// es la del servicio: el reconocedor solo ve una imagen.

// PDFPalabra es lo que el reconocedor encuentra en una imagen: el texto y su caja en
// PÍXELES de esa imagen, con el origen arriba a la izquierda.
type PDFPalabra struct {
	Texto             string
	X, Y, Ancho, Alto int
}

// PDFReconocedor es un OCR del dispositivo.
type PDFReconocedor interface {
	// Idiomas son los que sabe reconocer, con los códigos de Tesseract (`spa`, `eng`).
	// Vacío: no se elige idioma.
	Idiomas(ctx context.Context) ([]string, error)
	// Reconocer lee una imagen (`tipo` es su MIME) en `idiomas` («spa+eng», o vacío).
	Reconocer(ctx context.Context, imagen []byte, tipo, idiomas string) ([]PDFPalabra, error)
}

// ErrOCRNoDisponible lo devuelve un reconocedor que ya no puede trabajar (se desinstaló,
// se quitó el modelo).
var ErrOCRNoDisponible = errors.New("api: no hay reconocimiento de texto en este equipo")

// sondeoOCRLocal acota la pregunta de «¿enseño el botón?».
const sondeoOCRLocal = 3 * time.Second

// maxPaginasOCRLocal es la tanda del servicio: la interfaz pide el resto después.
const maxPaginasOCRLocal = 25

func (p *PDF) localOCR(ctx context.Context, fields map[string]string, docs ...PDFFichero) (PDFRespuesta, error) {
	if len(docs) != 1 {
		return PDFRespuesta{}, localPDFError(http.StatusBadRequest, "bad_request", "Send a single PDF.")
	}
	rec := p.confLocal().Reconocedor
	switch op := strings.TrimSpace(fields["op"]); op {
	case "", "estado":
		parsed, err := p.readLocalPDFs(ctx, fields["clave"], docs...)
		if err != nil {
			return PDFRespuesta{}, err
		}
		respuesta := map[string]any{
			"paginas_sin_texto": parsed[0].PaginasSinTexto(),
			"total":             parsed[0].NumPaginas(),
			"disponible":        false,
		}
		if rec != nil {
			sondeo, parar := context.WithTimeout(ctx, sondeoOCRLocal)
			idiomas, err := rec.Idiomas(sondeo)
			parar()
			if err == nil {
				respuesta["disponible"] = true
				if len(idiomas) > 0 {
					respuesta["idiomas"] = idiomas
				}
			}
		}
		out, err := localJSON(respuesta)
		if err != nil {
			return PDFRespuesta{}, err
		}
		return out, p.authorizeLocalPDF(ctx)
	case "capa":
		var textos map[int][]localpdf.TextoReconocido
		if err := localpdf.DecodeOperationJSON(fields["textos"], "textos", &textos); err != nil {
			return PDFRespuesta{}, localOperationFailure(err)
		}
		return p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
			out, err := localpdf.CapaDeTextoPDF(doc, textos)
			return localPDFResponse("application/pdf", out), err
		}, docs...)
	case "reconocer":
		if rec == nil {
			return PDFRespuesta{}, localPDFError(http.StatusServiceUnavailable, "ocr_not_configured", "Text recognition is not available on this device.")
		}
		return p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
			return reconocerLocal(ctx, rec, doc, fields)
		}, docs...)
	default:
		return PDFRespuesta{}, localPDFError(http.StatusBadRequest, "bad_request", strconv.Quote(op)+" is not a valid operation. Use estado, reconocer or capa.")
	}
}

// reconocerLocal es `reconocerYEscribir` del servicio con el reconocedor del equipo.
func reconocerLocal(ctx context.Context, rec PDFReconocedor, doc *localpdf.PDFDoc, fields map[string]string) (PDFRespuesta, error) {
	indices, err := paginasParaOCRLocal(doc, fields)
	if err != nil {
		return PDFRespuesta{}, err
	}
	idiomas := strings.TrimSpace(fields["idiomas"])
	porPagina := map[int][]localpdf.TextoReconocido{}
	var reconocidas, sinImagen int
	for _, i := range indices {
		if err := ctx.Err(); err != nil {
			return PDFRespuesta{}, err
		}
		trozos := trozosParaOCRLocal(doc, i)
		if len(trozos) == 0 {
			sinImagen++
			continue
		}
		var textos []localpdf.TextoReconocido
		for _, trozo := range trozos {
			delTrozo, err := reconocerTrozoLocal(ctx, rec, trozo, idiomas)
			if err != nil {
				return PDFRespuesta{}, err
			}
			textos = append(textos, delTrozo...)
		}
		if len(textos) > 0 {
			porPagina[i] = textos
			reconocidas++
		}
	}
	if len(porPagina) == 0 {
		return PDFRespuesta{}, &localpdf.OperationFailure{Status: http.StatusBadRequest, Code: "pdf_ocr_empty", Message: "No text could be recognised on those pages."}
	}
	out, err := localpdf.CapaDeTextoPDF(doc, porPagina)
	r := localPDFResponse("application/pdf", out)
	r.Cabecera.Set("X-Sytue-Ocr-Paginas", strconv.Itoa(reconocidas))
	r.Cabecera.Set("X-Sytue-Ocr-Sin-Imagen", strconv.Itoa(sinImagen))
	return r, err
}

// paginasParaOCRLocal son las páginas que se reconocen: las que se piden en `paginas` o,
// si no se pide ninguna, las que no tienen texto; como mucho maxPaginasOCRLocal.
func paginasParaOCRLocal(doc *localpdf.PDFDoc, fields map[string]string) ([]int, error) {
	var indices []int
	if lista := strings.TrimSpace(fields["paginas"]); lista != "" {
		var err error
		if indices, err = localpdf.ParsePageSelection(lista, doc.NumPaginas()); err != nil {
			return nil, err
		}
	} else {
		indices = doc.PaginasSinTexto()
	}
	if len(indices) == 0 {
		return nil, &localpdf.OperationFailure{Status: http.StatusBadRequest, Code: "pdf_already_has_text", Message: "This document already has text on every page."}
	}
	if len(indices) > maxPaginasOCRLocal {
		indices = indices[:maxPaginasOCRLocal]
	}
	return indices, nil
}

// trozosParaOCRLocal son las imágenes que se reconocen de la página i: el escaneo entero
// si la página lo es o, si no, sus imágenes una a una.
func trozosParaOCRLocal(doc *localpdf.PDFDoc, i int) []localpdf.ImagenColocada {
	if imagen, ok := doc.ImagenDeEscaneo(i); ok {
		return []localpdf.ImagenColocada{imagen}
	}
	return doc.ImagenesParaOCR(i)
}

// reconocerTrozoLocal reconoce una imagen y da sus palabras en coordenadas del PDF. Un
// fallo del reconocedor se traduce al error que daría el servicio.
func reconocerTrozoLocal(ctx context.Context, rec PDFReconocedor, trozo localpdf.ImagenColocada, idiomas string) ([]localpdf.TextoReconocido, error) {
	palabras, err := rec.Reconocer(ctx, trozo.Datos, trozo.Tipo, idiomas)
	if err != nil {
		if cancelado := ctx.Err(); cancelado != nil {
			return nil, cancelado
		}
		if errors.Is(err, ErrOCRNoDisponible) {
			return nil, &localpdf.OperationFailure{Status: http.StatusServiceUnavailable, Code: "ocr_not_configured", Message: "Text recognition is not available on this device."}
		}
		return nil, &localpdf.OperationFailure{Status: http.StatusBadGateway, Code: "ocr_failed", Message: "The text recognition could not complete the request."}
	}
	var textos []localpdf.TextoReconocido
	for _, w := range palabras {
		if strings.TrimSpace(w.Texto) == "" || w.Ancho <= 0 || w.Alto <= 0 {
			continue
		}
		textos = append(textos, localpdf.TextoReconocido{Texto: w.Texto, Rect: trozo.RectDePDF(w.X, w.Y, w.Ancho, w.Alto)})
	}
	return textos, nil
}
