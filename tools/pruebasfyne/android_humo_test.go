// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// El juicio de la prueba de humo del emulador (`android-humo.sh revisar`): qué logcat
// es una app caída y cuál no. Se prueba sin emulador, con logcats de mentira con la
// forma de `adb logcat -v threadtime`; si el juicio dejara de ver una caída, el job
// semanal saldría en verde con las apps rotas.
func TestAndroidHumoRevisaElLogcat(t *testing.T) {
	const paquete = "eu.sytue.mail"
	linea := func(pid, nivel, etiqueta, texto string) string {
		return "11-25 10:00:00.000  " + pid + "  " + pid + " " + nivel + " " + etiqueta + ": " + texto
	}
	casos := []struct {
		nombre string
		logcat []string
		cae    bool
		aviso  bool
	}{
		{"arranca sin más", []string{
			linea("1234", "I", "Fyne", "sesion: abierta"),
			linea("1234", "I", "ActivityTaskManager", "Displayed eu.sytue.mail/org.golang.app.GoNativeActivity"),
		}, false, false},
		{"FATAL EXCEPTION suya", []string{
			linea("1234", "E", "AndroidRuntime", "FATAL EXCEPTION: main"),
			linea("1234", "E", "AndroidRuntime", "Process: eu.sytue.mail, PID: 1234"),
			linea("1234", "E", "AndroidRuntime", "java.lang.RuntimeException: Unable to start activity"),
		}, true, false},
		{"FATAL EXCEPTION de otra app de la imagen", []string{
			linea("999", "E", "AndroidRuntime", "FATAL EXCEPTION: main"),
			linea("999", "E", "AndroidRuntime", "Process: com.google.android.apps.messaging, PID: 999"),
		}, false, true},
		{"FATAL EXCEPTION sin proceso", []string{
			linea("1234", "E", "AndroidRuntime", "FATAL EXCEPTION: GLThread 12"),
		}, true, false},
		{"pánico de Go", []string{
			linea("1234", "E", "Fyne", "panic: runtime error: invalid memory address or nil pointer dereference"),
			linea("1234", "E", "Fyne", "[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x7a]"),
		}, true, false},
		{"error del runtime de Go", []string{
			linea("1234", "E", "Fyne", "fatal error: concurrent map writes"),
		}, true, false},
		{"caída nativa suya", []string{
			linea("1234", "F", "libc", "Fatal signal 11 (SIGSEGV), code 1 (SEGV_MAPERR), fault addr 0x0 in tid 1250 (GLThread), pid 1234 (eu.sytue.mail)"),
		}, true, false},
		{"caída nativa de otro proceso", []string{
			linea("777", "F", "libc", "Fatal signal 6 (SIGABRT), code -1 (SI_QUEUE) in tid 777 (binder), pid 777 (com.android.phone)"),
		}, false, true},
		{"«panic» dentro de otra palabra no es un pánico", []string{
			linea("1234", "I", "Fyne", "nopanic: todo bien"),
		}, false, false},
	}
	dir := t.TempDir()
	for _, c := range casos {
		ruta := filepath.Join(dir, strings.ReplaceAll(c.nombre, " ", "-")+".txt")
		guiontest.Escribir(t, ruta, strings.Join(c.logcat, "\n")+"\n")
		r := guiontest.Guion(t, guiontest.Opciones{}, "apps/fyne/scripts/android-humo.sh", "revisar", ruta, paquete)
		if cae := r.Codigo != 0; cae != c.cae {
			t.Errorf("%s: cae=%v y debía ser %v\n%s", c.nombre, cae, c.cae, r.Todo())
		}
		if aviso := strings.Contains(r.Error, "aviso, de otro proceso"); aviso != c.aviso {
			t.Errorf("%s: aviso=%v y debía ser %v\n%s", c.nombre, aviso, c.aviso, r.Todo())
		}
		if c.cae && strings.TrimSpace(r.Salida) == "" {
			t.Errorf("%s: cae y no dice por qué", c.nombre)
		}
	}
}
