// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command precomprimir escribe, junto a cada fichero que recibe, su versión en
// brotli (calidad 11, ventana de 16 MiB) y en gzip (nivel 9): `x` → `x.br` y `x.gz`.
//
//	go run ./tools/precomprimir <ficheros…>
//
// POR QUÉ. Los WebAssembly del núcleo de cliente (scripts/client-core-assets.sh)
// pesan de 7 a 27 MB y Traefik los comprimía en cada descarga, a un brotli de
// calidad media: 0,6 s de CPU por cada `core` en frío y 1,2 s por cada `mail`,
// para quedar un 14–16 % por encima de lo que da brotli 11. Comprimirlos una vez al
// generarlos cuesta lo mismo una sola vez, y Rooms sirve los bytes ya comprimidos
// (platform/pkg/httpx/compiler_assets.go). Traefik no vuelve a comprimir una
// respuesta que ya trae Content-Encoding (docs/decisions.md#0086).
//
// La salida es determinista: gzip sin nombre ni fecha, brotli sin metadatos. Cada
// variante se descomprime antes de darla por buena; si no devuelve los mismos bytes
// no se escribe y la orden falla. Los ficheros se procesan en paralelo.
package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/andybalholm/brotli"
)

// Variante es una codificación de contenido y cómo se produce y se comprueba.
type variante struct {
	extension string
	comprimir func(io.Writer, []byte) error
	leer      func(io.Reader) (io.Reader, error)
}

var variantes = []variante{
	{".br", func(w io.Writer, crudo []byte) error {
		// Ventana de 2^24: los navegadores la admiten y en un wasm de 20 MB se nota.
		z := brotli.NewWriterOptions(w, brotli.WriterOptions{Quality: brotli.BestCompression, LGWin: 24})
		if _, err := z.Write(crudo); err != nil {
			return err
		}
		return z.Close()
	}, func(r io.Reader) (io.Reader, error) { return brotli.NewReader(r), nil }},
	{".gz", func(w io.Writer, crudo []byte) error {
		z, err := gzip.NewWriterLevel(w, gzip.BestCompression)
		if err != nil {
			return err
		}
		if _, err := z.Write(crudo); err != nil {
			return err
		}
		return z.Close()
	}, func(r io.Reader) (io.Reader, error) { return gzip.NewReader(r) }},
}

// Precomprimir escribe las variantes de fichero y devuelve sus tamaños.
func precomprimir(fichero string) (map[string]int, error) {
	crudo, err := os.ReadFile(fichero) //nolint:gosec // G703: herramienta de build; las rutas las pone scripts/client-core-assets.sh.
	if err != nil {
		return nil, err
	}
	tamanos := map[string]int{"": len(crudo)}
	for _, v := range variantes {
		var comprimido bytes.Buffer
		if err := v.comprimir(&comprimido, crudo); err != nil {
			return nil, fmt.Errorf("%s%s: %w", fichero, v.extension, err)
		}
		lector, err := v.leer(bytes.NewReader(comprimido.Bytes()))
		if err != nil {
			return nil, fmt.Errorf("%s%s: %w", fichero, v.extension, err)
		}
		vuelta, err := io.ReadAll(lector)
		if err != nil || !bytes.Equal(vuelta, crudo) {
			return nil, fmt.Errorf("%s%s: descomprimido no devuelve el original", fichero, v.extension)
		}
		if err := escribir(fichero+v.extension, comprimido.Bytes()); err != nil {
			return nil, err
		}
		tamanos[v.extension] = comprimido.Len()
	}
	return tamanos, nil
}

// escribir no deja nunca un fichero a medias donde Rooms lo incrustaría.
func escribir(destino string, datos []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(destino), "."+filepath.Base(destino)+".*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(datos)
	err = errors.Join(err, tmp.Chmod(0o644), tmp.Close())
	if err == nil {
		err = os.Rename(tmp.Name(), destino) //nolint:gosec // G703: temporal propio junto al destino.
	}
	if err != nil {
		_ = os.Remove(tmp.Name()) //nolint:gosec // G703: temporal propio junto al destino.
	}
	return err
}

func main() {
	ficheros := os.Args[1:]
	if len(ficheros) == 0 {
		fmt.Fprintln(os.Stderr, "uso: precomprimir <ficheros…>")
		os.Exit(2)
	}
	tamanos := make([]map[string]int, len(ficheros))
	fallos := make([]error, len(ficheros))
	var espera sync.WaitGroup
	for i, f := range ficheros {
		espera.Go(func() { tamanos[i], fallos[i] = precomprimir(f) })
	}
	espera.Wait()
	if err := errors.Join(fallos...); err != nil {
		fmt.Fprintln(os.Stderr, "precomprimir:", err)
		os.Exit(1)
	}
	for i, f := range ficheros {
		fmt.Printf("%s: %d bytes, br %d, gz %d\n", f, tamanos[i][""], tamanos[i][".br"], tamanos[i][".gz"])
	}
}
