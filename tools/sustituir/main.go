// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sustituye un bloque de texto EXACTO en un fichero, y falla si no aparece las
// veces esperadas.
//
// Por qué existe: las ediciones automatizadas que no verifican lo que van a
// tocar son la forma más rápida de romper un fichero sin enterarse. Un `sed`
// que no casa devuelve 0 y sigue como si nada —pasó el 2026-08-19 con un `\b`,
// que BSD no soporta: imprimió «limpiado» sin haber sustituido nada—. Aquí, si
// el texto no está o está más veces de las declaradas, no se escribe nada.
//
// Uso:
//
//	go run ./tools/sustituir fichero <<'FIN'
//	@@VIEJO@@
//	el texto tal cual está
//	@@NUEVO@@
//	el texto que lo reemplaza
//	FIN
//
// Con -n se declara cuántas apariciones se esperan (1 por defecto), y con
// -borrar se omite el bloque nuevo para eliminar el viejo.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	marcaVieja = "@@VIEJO@@\n"
	marcaNueva = "@@NUEVO@@\n"
)

func main() {
	veces := flag.Int("n", 1, "apariciones esperadas del texto a sustituir")
	borrar := flag.Bool("borrar", false, "eliminar el bloque en vez de sustituirlo")
	flag.Parse()

	if flag.NArg() != 1 {
		fatal("uso: sustituir [-n N] [-borrar] fichero < especificación")
	}
	ruta := flag.Arg(0)

	spec, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatal("no se pudo leer la especificación: %v", err)
	}
	viejo, nuevo, err := partir(string(spec), *borrar)
	if err != nil {
		fatal("%v", err)
	}

	if err := aplicar(ruta, viejo, nuevo, *veces); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("%s: %d sustitución(es)\n", ruta, *veces)
}

// aplicar vive fuera de main() para poder probarla: main termina en os.Exit y un
// test no puede ejercitarla sin abortar el propio test.
func aplicar(ruta, viejo, nuevo string, veces int) error {
	original, err := os.ReadFile(ruta)
	if err != nil {
		return err
	}
	contenido := string(original)
	if n := strings.Count(contenido, viejo); n != veces {
		return fmt.Errorf("%s: el texto aparece %d veces y se esperaban %d; no se ha tocado nada", ruta, n, veces)
	}
	// Se conservan los permisos: escribir con 0644 a ciegas convierte un script
	// ejecutable en uno que no arranca, y el fallo aparece lejos de aquí.
	info, err := os.Stat(ruta)
	if err != nil {
		return err
	}
	//nolint:gosec // G703: `ruta` sale del recorrido que hace esta misma herramienta sobre el árbol que le pasan por argv; no hay entrada de red ni de usuario ajeno
	return os.WriteFile(ruta, []byte(strings.ReplaceAll(contenido, viejo, nuevo)), info.Mode().Perm())
}

func partir(spec string, borrar bool) (viejo, nuevo string, err error) {
	i := strings.Index(spec, marcaVieja)
	if i != 0 {
		return "", "", fmt.Errorf("la especificación debe empezar por %s", strings.TrimSpace(marcaVieja))
	}
	resto := spec[len(marcaVieja):]

	if borrar {
		return resto, "", nil
	}
	j := strings.Index(resto, marcaNueva)
	if j < 0 {
		return "", "", fmt.Errorf("falta %s (o usa -borrar)", strings.TrimSpace(marcaNueva))
	}
	return resto[:j], resto[j+len(marcaNueva):], nil
}

func fatal(formato string, args ...any) {
	fmt.Fprintf(os.Stderr, formato+"\n", args...)
	os.Exit(1)
}
