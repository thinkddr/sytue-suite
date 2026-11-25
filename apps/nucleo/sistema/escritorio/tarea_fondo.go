// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// # El trabajo de una app con la app cerrada, en el escritorio
//
// En Android lo despierta el sistema (un JobService) y en iOS su tarea de refresco; en el
// escritorio no hay quien despierte a una app cerrada, salvo el Hub, que se queda en la
// bandeja y arranca al iniciar sesión (inicio.go). Así que el Hub lanza la app hermana
// con [ArgTareaEnFondo] cada cierto tiempo: la app hace su tarea en segundo plano —la
// misma que en el móvil (`sesion.TareaEnFondo`), con la sesión guardada—, sin abrir
// ninguna ventana, y sale. La primera es la copia de seguridad de Photos.

// ArgTareaEnFondo es el argumento con el que el Hub lanza una app para su tarea en
// segundo plano: sin ventana, una ronda y fuera.
const ArgTareaEnFondo = "--tarea-en-fondo"

// PlazoTareaEnFondo es lo más que dura una tarea lanzada así: lo que no termine se
// retoma en la siguiente, como en el móvil.
const PlazoTareaEnFondo = 30 * time.Minute

// EjecutableDeHermana es el ejecutable de la app hermana `app` (`photos`): el de su
// misma carpeta —las del mismo bundle, en Linux y en Windows— o el de [Hermanas] —en
// macOS, el de dentro de su .app—. Vacío si no está instalada.
func EjecutableDeHermana(app string) string {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return ""
	}
	nombre := "sytue-" + app
	if runtime.GOOS == "windows" {
		nombre += ".exe"
	}
	if junto := filepath.Join(filepath.Dir(exe), nombre); esEjecutable(junto) {
		return junto
	}
	for _, h := range hermanasDe(exe) {
		if h.App == app {
			return ejecutableDe(h.Destino, nombre)
		}
	}
	return ""
}

// ejecutableDe es el ejecutable de una hermana: su Destino si ya lo es, o el de dentro
// del .app.
func ejecutableDe(destino, nombre string) string {
	if strings.HasSuffix(destino, ".app") {
		dentro := filepath.Join(destino, "Contents", "MacOS", nombre)
		if esEjecutable(dentro) {
			return dentro
		}
		return ""
	}
	if esEjecutable(destino) {
		return destino
	}
	return ""
}

func esEjecutable(ruta string) bool {
	info, err := os.Stat(ruta)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode().Perm()&0o100 != 0
}

// LanzarTareaEnFondo ejecuta `exe` con [ArgTareaEnFondo] y espera a que acabe, como
// mucho [PlazoTareaEnFondo] o lo que deje `ctx`. Un código distinto de cero es un
// error: la tarea lo dice así cuando falla y conviene reintentar.
func LanzarTareaEnFondo(ctx context.Context, exe string) error {
	if exe == "" {
		return ErrNoDisponible
	}
	ctx, cancelar := context.WithTimeout(ctx, PlazoTareaEnFondo)
	defer cancelar()
	return exec.CommandContext(ctx, exe, ArgTareaEnFondo).Run() //nolint:gosec // el ejecutable de una app hermana de la suite
}
