// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// Carpeta es una carpeta que ya sincroniza un cliente del sistema.
type Carpeta struct {
	Nombre string // «OneDrive», «Google Drive»…
	Ruta   string
}

// Sincronizadas busca, sin red, las carpetas de los clientes de nube instalados.
func Sincronizadas() []Carpeta {
	casa, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var out []Carpeta
	vistas := map[string]bool{}
	añadir := func(nombre, ruta string) {
		if ruta == "" || vistas[ruta] {
			return
		}
		if st, err := os.Stat(ruta); err == nil && st.IsDir() { //nolint:gosec // G703: del entorno y la carpeta personal de quien ejecuta la app; solo se mira si existe
			vistas[ruta] = true
			out = append(out, Carpeta{Nombre: nombre, Ruta: ruta})
		}
	}
	// Windows publica las de OneDrive en variables de entorno.
	añadir("OneDrive", os.Getenv("OneDriveConsumer"))
	añadir("OneDrive (Microsoft 365)", os.Getenv("OneDriveCommercial"))
	añadir("OneDrive", os.Getenv("OneDrive"))
	if runtime.GOOS == "windows" {
		for _, u := range []string{"G:", "H:", "I:"} {
			añadir("Google Drive", filepath.Join(u+`\`, "My Drive"))
			añadir("Google Drive", filepath.Join(u+`\`, "Mi unidad"))
		}
	}
	enCloudStorage(casa, añadir)
	añadir("iCloud Drive", filepath.Join(casa, "Library", "Mobile Documents", "com~apple~CloudDocs"))
	enCasa(casa, añadir)
	slices.SortStableFunc(out, func(a, b Carpeta) int { return strings.Compare(a.Nombre, b.Nombre) })
	return out
}

// enCloudStorage añade las de macOS: File Provider deja todas en ~/Library/CloudStorage.
func enCloudStorage(casa string, añadir func(nombre, ruta string)) {
	entradas, err := os.ReadDir(filepath.Join(casa, "Library", "CloudStorage"))
	if err != nil {
		return
	}
	for _, e := range entradas {
		n := e.Name()
		ruta := filepath.Join(casa, "Library", "CloudStorage", n)
		switch {
		case strings.HasPrefix(n, "GoogleDrive-"):
			for _, sub := range []string{"My Drive", "Mi unidad"} {
				añadir("Google Drive ("+strings.TrimPrefix(n, "GoogleDrive-")+")", filepath.Join(ruta, sub))
			}
		case strings.HasPrefix(n, "OneDrive-"):
			añadir("OneDrive ("+strings.TrimPrefix(n, "OneDrive-")+")", ruta)
		case strings.HasPrefix(n, "SytueDrive") || strings.HasPrefix(n, "Sytue"):
			añadir("Sytue Drive", ruta)
		}
	}
}

// enCasa añade las carpetas con el nombre de siempre en la carpeta personal (Linux,
// Windows).
func enCasa(casa string, añadir func(nombre, ruta string)) {
	entradas, err := os.ReadDir(casa)
	if err != nil {
		return
	}
	for _, e := range entradas {
		n := e.Name()
		switch {
		case n == "OneDrive" || strings.HasPrefix(n, "OneDrive - "):
			añadir(n, filepath.Join(casa, n))
		case n == "Google Drive" || n == "GoogleDrive":
			añadir("Google Drive", filepath.Join(casa, n))
		case strings.HasPrefix(n, "Sytue Drive"):
			// La carpeta que sincroniza la edición conectada (`nucleo/nube`).
			añadir(n, filepath.Join(casa, n))
		}
	}
}

func ordenar(es []Elemento) { Ordenar(es) }

// Ordenar deja el contenido de una carpeta como lo lista cada proveedor: las carpetas
// primero y por nombre; entre los que se llaman igual (Google Drive lo admite), el creado
// antes y, si no se sabe, el de identificador menor. Así el primero de los repetidos es
// siempre el mismo: el que se ve con su nombre y el que sustituye Subir por el nombre.
func Ordenar(es []Elemento) {
	slices.SortStableFunc(es, func(a, b Elemento) int {
		if a.Carpeta != b.Carpeta {
			if a.Carpeta {
				return -1
			}
			return 1
		}
		if c := strings.Compare(strings.ToLower(a.Nombre), strings.ToLower(b.Nombre)); c != 0 {
			return c
		}
		if c := strings.Compare(a.Nombre, b.Nombre); c != 0 {
			return c
		}
		if c := a.Creado.Compare(b.Creado); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

var errorTamano = errors.New("nube: el fichero supera los 250 MB que admite una subida sencilla")
