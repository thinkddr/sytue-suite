// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package galeriacifrada

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/exif"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/render"
)

// # Ver y descargar una foto cifrada
//
// Lo que el servidor hace con sus fotos (la variante «grande» del visor, la foto entera
// para el tamaño real y la exportación de «Descargar»), hecho en el dispositivo con el
// original descifrado en memoria. Las tres orientan la foto por su EXIF, como el
// servidor, y ninguna escribe nada: ni en el Drive ni en disco. Lo que devuelven es de
// quien llama, que decide dónde va (la descarga, solo a donde elige la persona).

// LadoVista es el lado mayor de la foto grande del visor: el de la variante «grande»
// de la galería del servidor.
const LadoVista = 2048

// conOriginal descifra el original de `f`, lo decodifica orientado y se lo da a
// `hacer`, con el turno de decodificar de miniatura cogido. Los bytes descifrados se
// borran al terminar.
func (g *Galeria) conOriginal(ctx context.Context, f api.File, hacer func(image.Image) error) error {
	if !idValido(f.ID) {
		return ErrSinFoto
	}
	if f.SizeBytes > render.MaxOriginal {
		return miniatura.ErrMedioNoSoportado
	}
	release, err := miniatura.TurnoConContexto(ctx)
	if err != nil {
		return err
	}
	defer release()
	datos, err := g.leerFichero(ctx, f.ID, render.MaxOriginal)
	if err != nil {
		return err
	}
	defer clear(datos)
	meta, _ := exif.Leer(datos) // sin EXIF no es un fallo
	img, err := render.Imagen(ctx, datos, model.OrientacionDe(int(meta.Orientacion)), model.RecetaVacia())
	if err != nil {
		return err
	}
	return hacer(img)
}

// Vista es la foto grande del visor: orientada, con `lado` px de lado mayor como mucho
// (0, [LadoVista]) y en JPEG.
func (g *Galeria) Vista(ctx context.Context, f api.File, lado int) ([]byte, error) {
	if lado <= 0 {
		lado = LadoVista
	}
	var out []byte
	err := g.conOriginal(ctx, f, func(img image.Image) error {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, render.AplanarSobreBlanco(miniatura.Reducir(img, lado)), &jpeg.Options{Quality: render.CalidadExportacion}); err != nil {
			return fmt.Errorf("galeriacifrada: codificar la vista: %w", err)
		}
		out = buf.Bytes()
		return nil
	})
	return out, err
}

// Imagen es la foto entera, orientada y decodificada: el tamaño real del visor.
func (g *Galeria) Imagen(ctx context.Context, f api.File) (image.Image, error) {
	var out image.Image
	err := g.conOriginal(ctx, f, func(img image.Image) error {
		out = img
		return nil
	})
	return out, err
}

// Exportar escribe en `out` la foto orientada en `formato` ("png" o, si no, JPEG con la
// calidad de exportar de Photos), como «Descargar» con las fotos del servidor. Sin EXIF:
// la exportación no lleva ni la ubicación ni la cámara.
func (g *Galeria) Exportar(ctx context.Context, f api.File, formato string, out io.Writer) error {
	return g.conOriginal(ctx, f, func(img image.Image) error {
		var err error
		if formato == "png" {
			err = png.Encode(out, img)
		} else {
			err = jpeg.Encode(out, render.AplanarSobreBlanco(img), &jpeg.Options{Quality: render.CalidadExportacion})
		}
		if err != nil {
			return fmt.Errorf("galeriacifrada: codificar la exportación: %w", err)
		}
		return ctx.Err()
	})
}
