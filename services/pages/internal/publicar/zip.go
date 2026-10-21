// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package publicar

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrDemasiadosFicheros salta si el zip supera el tope de 2000 ficheros (§7.6).
	ErrDemasiadosFicheros = errors.New("publicar: el archivo zip supera el límite de 2000 ficheros")
	// ErrRutaInvalida salta si una ruta no cumple con la defensa contra zip slip (§7.6).
	ErrRutaInvalida = errors.New("publicar: ruta inválida en archivo zip")
)

// regexRutaZip valida que las rutas del zip sean relativas, seguras y sin caracteres raros:
// solo A-Za-z0-9._- separados por /.
var regexRutaZip = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)

// ArchivoZip representa un archivo en memoria que se incluirá en el zip.
type ArchivoZip struct {
	Ruta       string
	Contenido  []byte
	Modificado time.Time
}

// RecursoZip representa un recurso que se escribe en streaming desde un io.Reader.
type RecursoZip struct {
	Ruta       string
	Lector     io.Reader
	Modificado time.Time
}

// ValidarRutaZip comprueba las reglas de defensa contra zip slip (§7.6):
// 1. Debe casar con ^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$
// 2. Ningún segmento puede ser "." o ".."
// 3. Longitud máxima de 300 caracteres.
func ValidarRutaZip(ruta string) error {
	if ruta == "" || len(ruta) > 300 {
		return fmt.Errorf("%w: longitud fuera de límites (1-300): %d", ErrRutaInvalida, len(ruta))
	}
	if !regexRutaZip.MatchString(ruta) {
		return fmt.Errorf("%w: no cumple el patrón permitido: %q", ErrRutaInvalida, ruta)
	}
	segmentos := strings.Split(ruta, "/")
	for _, seg := range segmentos {
		if seg == "." || seg == ".." {
			return fmt.Errorf("%w: segmento prohibido %q en %q", ErrRutaInvalida, seg, ruta)
		}
	}
	return nil
}

// EscribirZip escribe un ZIP sobre w en streaming (PAGES-ENCARGO §7.6):
// rechaza, para todos los llamantes, rutas que no casen con ^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$,
// con segmentos . o .., de más de 300 caracteres, o más de 2000 ficheros (zip slip).
func EscribirZip(w io.Writer, archivos []ArchivoZip, recursos []RecursoZip) error {
	total := len(archivos) + len(recursos)
	if total > 2000 {
		return fmt.Errorf("%w: %d ficheros", ErrDemasiadosFicheros, total)
	}

	zw := zip.NewWriter(w)

	// Escribir archivos en memoria
	for _, a := range archivos {
		if err := ValidarRutaZip(a.Ruta); err != nil {
			_ = zw.Close()
			return err
		}
		modTime := a.Modificado
		if modTime.IsZero() {
			modTime = time.Now().UTC()
		}
		header := &zip.FileHeader{
			Name:     a.Ruta,
			Method:   zip.Deflate,
			Modified: modTime,
		}
		fw, err := zw.CreateHeader(header)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("publicar: crear entrada zip %q: %w", a.Ruta, err)
		}
		if _, err := fw.Write(a.Contenido); err != nil {
			_ = zw.Close()
			return fmt.Errorf("publicar: escribir entrada zip %q: %w", a.Ruta, err)
		}
	}

	// Escribir recursos en streaming
	for _, r := range recursos {
		if err := ValidarRutaZip(r.Ruta); err != nil {
			_ = zw.Close()
			return err
		}
		modTime := r.Modificado
		if modTime.IsZero() {
			modTime = time.Now().UTC()
		}
		header := &zip.FileHeader{
			Name:     r.Ruta,
			Method:   zip.Deflate,
			Modified: modTime,
		}
		fw, err := zw.CreateHeader(header)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("publicar: crear entrada recurso zip %q: %w", r.Ruta, err)
		}
		if _, err := io.Copy(fw, r.Lector); err != nil {
			_ = zw.Close()
			return fmt.Errorf("publicar: copiar recurso zip %q: %w", r.Ruta, err)
		}
	}

	return zw.Close()
}
