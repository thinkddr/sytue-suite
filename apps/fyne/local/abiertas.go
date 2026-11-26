// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"encoding/csv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// # Qué apps de la edición están abiertas
//
// Lo preguntan las cosas que escriben en los datos de TODAS las apps y necesitan que
// estén cerradas: «Traer desde Sytue» y «Exportar todo» del Hub (HU-07) y mover la
// carpeta de datos (Ajustes → Datos). Una app abierta responde en el enchufe de su copia
// única (instancia.go); la lista de procesos de la persona, por el nombre del ejecutable
// (`sytue-office-<app>`), cubre las que se abrieron de otra forma. Sin hablar con nadie.

// AppsDeLaEdicion son las apps de Sytue Office: su carpeta en `cmd-local` (el final de su
// identificador `eu.sytue.office.<carpeta>` y de su ejecutable) y su clave de producto
// (la de su esquema, `sytueoffice-<producto>`). TestAppsDeLaEdicionSonLasDeCmdLocal la
// compara con `cmd-local`.
var AppsDeLaEdicion = map[string]string{
	"bases": "bases", "calcus": "sheets", "calendar": "calendar", "cast": "cast", "certipdf": "pdf",
	"contacts": "contacts", "diapos": "slides", "docus": "docs", "drive": "drive", "erp": "erp",
	"hub": "one", "mail": "mail", "notes": "notes", "photos": "photos", "rooms": "rooms",
}

// NombreDeApp es el nombre corto de la app de esa carpeta («Calcus»).
func NombreDeApp(carpeta string) string {
	if m, ok := iconos.ProductoDe(AppsDeLaEdicion[carpeta]); ok {
		return m.NombreCorto()
	}
	if carpeta == "hub" {
		return "Hub"
	}
	return carpeta
}

// Procesos son los nombres de ejecutable de los procesos de la persona, tal cual (con
// `.exe` en Windows). Sin poder saberlo, nil y false.
func Procesos(goos string) ([]string, bool) {
	switch goos {
	case "linux", "freebsd", "openbsd", "netbsd":
		return ProcesosProc("/proc")
	case "darwin":
		return procesosOrden(func(salida string) []string {
			var out []string
			for l := range strings.SplitSeq(salida, "\n") {
				if l = strings.TrimSpace(l); l != "" {
					out = append(out, filepath.Base(l))
				}
			}
			return out
		}, "ps", "-axo", "comm=")
	case "windows":
		return procesosOrden(func(salida string) []string {
			filas, _ := csv.NewReader(strings.NewReader(salida)).ReadAll()
			var out []string
			for _, f := range filas {
				if len(f) > 0 {
					out = append(out, f[0])
				}
			}
			return out
		}, "tasklist", "/FO", "CSV", "/NH")
	}
	return nil, false
}

// ProcesosProc lee /proc (raiz): el ejecutable de cada proceso que se deja leer (los de
// la persona) o, si no, el primer argumento.
func ProcesosProc(raiz string) ([]string, bool) {
	es, err := os.ReadDir(raiz)
	if err != nil {
		return nil, false
	}
	var out []string
	for _, e := range es {
		if !e.IsDir() || strings.Trim(e.Name(), "0123456789") != "" {
			continue
		}
		if exe, err := os.Readlink(filepath.Join(raiz, e.Name(), "exe")); err == nil {
			out = append(out, filepath.Base(strings.TrimSuffix(exe, " (deleted)")))
			continue
		}
		if raw, err := os.ReadFile(filepath.Join(raiz, e.Name(), "cmdline")); err == nil && len(raw) > 0 { //nolint:gosec // G304: /proc de la persona
			arg0, _, _ := strings.Cut(string(raw), "\x00")
			out = append(out, filepath.Base(arg0))
		}
	}
	return out, true
}

func procesosOrden(leer func(string) []string, orden ...string) ([]string, bool) {
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	salida, err := exec.CommandContext(ctx, orden[0], orden[1:]...).Output() //nolint:gosec // G204: órdenes fijas
	if err != nil {
		return nil, false
	}
	return leer(string(salida)), true
}

// Abiertas son, de esas apps (por su carpeta de `cmd-local`, en el orden dado), las que
// están corriendo ahora. sabido es falso si el sistema no deja ver la lista de procesos
// (entonces solo se sabe de las que responden en su enchufe).
func Abiertas(carpetas []string, goos string) (abiertas []string, sabido bool) {
	ps, ok := Procesos(goos)
	nombres := map[string]bool{}
	for _, p := range ps {
		nombres[strings.TrimSuffix(strings.ToLower(p), ".exe")] = true
	}
	for _, c := range carpetas {
		if CopiaViva(espacio.ServicioLlavero+"."+c) || nombres[espacio.PrefijoEjecutable+c] {
			abiertas = append(abiertas, c)
		}
	}
	return abiertas, ok
}
