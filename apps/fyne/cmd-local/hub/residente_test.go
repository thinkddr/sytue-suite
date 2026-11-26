// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/local/avisos"
	"git.sytue.eu/sytue/one/apps/nucleo/local/indice"
)

func hoyDeEjemplo() indice.Hoy {
	nueve := time.Date(2026, 11, 24, 9, 0, 0, 0, time.Local)
	return indice.Hoy{
		Eventos: []indice.Ocurrencia{
			{Elemento: indice.Elemento{Producto: "calendar", Clase: indice.Evento, ID: "e1", Titulo: "Cumpleaños de Ana"}, TodoElDia: true},
			{Elemento: indice.Elemento{Producto: "calendar", Clase: indice.Evento, ID: "e2", Titulo: "Dentista", Detalle: "Calle Mayor 3", Fecha: nueve}, Fin: nueve.Add(time.Hour)},
		},
		Recordatorios: []indice.Elemento{{Producto: "notes", Clase: indice.Nota, ID: "n1", Titulo: "Comprar pan", Fecha: nueve.Add(8 * time.Hour)}},
		Tareas: []indice.Elemento{
			{Producto: "notes", Clase: indice.Nota, ID: "n1", Titulo: "Comprar pan"},
			{Producto: "notes", Clase: indice.Nota, ID: "n2", Titulo: "Regar"},
			{Producto: "notes", Clase: indice.Nota, ID: "n3", Titulo: "Barrer"},
			{Producto: "notes", Clase: indice.Nota, ID: "n4", Titulo: "Tender"},
			{Producto: "notes", Clase: indice.Nota, ID: "n5", Titulo: "Planchar"},
		},
		TotalTareas: 6,
		SinLeer:     3,
		Correos:     []indice.Elemento{{Producto: "mail", Clase: indice.Correo, ID: "7", Titulo: "Factura de la luz", Detalle: "Iberdrola"}},
	}
}

// HU-04: la tarjeta dice los eventos con su hora, los recordatorios, como mucho tres
// tareas (sin repetir la que ya sale por su recordatorio) y cuánto más queda, y el
// correo sin leer con el último.
func TestLasFilasDeHoy(t *testing.T) {
	filas := FilasHoy("es", hoyDeEjemplo())
	got := make([]string, 0, len(filas))
	for _, f := range filas {
		got = append(got, f.Cuando+"|"+f.Elemento.Titulo+"|"+f.Resumen)
	}
	want := []string{
		"Todo el día|Cumpleaños de Ana|",
		"9:00–10:00|Dentista|Calle Mayor 3",
		"17:00|Comprar pan|Recordatorio",
		"|Regar|Tarea por hacer", "|Barrer|Tarea por hacer", "|Tender|Tarea por hacer",
		"|Y 2 tareas más|",
		"|3 correos sin leer|Iberdrola · Factura de la luz",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("filas =\n%s\nquiero\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(FilasHoy("es", indice.Hoy{})) != 0 {
		t.Fatal("sin nada, sin filas")
	}
}

// HU-03: la bandeja abre el Hub, enseña lo de hoy (con su hora) y cada app.
func TestElMenuDeLaBandeja(t *testing.T) {
	var abierto, lanzado string
	mostrado := false
	apps := []Instalada{{catalogo[slices.IndexFunc(catalogo, func(a App) bool { return a.Carpeta == "calendar" })], "/x/sytue-office-calendar"}}
	m := MenuBandeja("es", "Sytue Office", hoyDeEjemplo(), apps, AccionesBandeja{
		Mostrar: func() { mostrado = true },
		Abrir:   func(e indice.Elemento) { abierto = e.ID },
		Lanzar:  func(i Instalada) { lanzado = i.Carpeta },
	})
	rotulos := make([]string, 0, len(m.Items))
	for _, i := range m.Items {
		rotulos = append(rotulos, i.Label)
	}
	want := []string{
		"Abrir Sytue Office", "", "Hoy", "Todo el día  Cumpleaños de Ana", "9:00–10:00  Dentista", "17:00  Comprar pan",
		"Regar", "Barrer", "", "Abrir una app",
	}
	if !slices.Equal(rotulos, want) {
		t.Fatalf("bandeja = %q", rotulos)
	}
	m.Items[0].Action()
	m.Items[4].Action()
	m.Items[len(m.Items)-1].ChildMenu.Items[0].Action()
	if !mostrado || abierto != "e2" || lanzado != "calendar" {
		t.Fatalf("mostrado=%v abierto=%q lanzado=%q", mostrado, abierto, lanzado)
	}
	if !m.Items[2].Disabled {
		t.Error("el rótulo «Hoy» no se puede pulsar")
	}
	if r := recortarRotulo(strings.Repeat("a", 100)); len([]rune(r)) != LargoBandeja {
		t.Errorf("rótulo largo = %d runas", len([]rune(r)))
	}
}

// HU-06: el vigía avisa un recordatorio solo si su app está cerrada, y lo avisa como de
// ella; si está abierta, avisa ella.
func TestElVigiaAvisaSoloConLaAppCerrada(t *testing.T) {
	abiertas := map[string]bool{"calendar": true}
	var avisados []string
	v := &Vigia{
		Abierta: func(p string) bool { return abiertas[p] },
		Avisar:  func(p string, a avisos.Aviso) { avisados = append(avisados, p+" "+a.Titulo) },
	}
	v.avisado(avisos.Aviso{Titulo: "Dentista", Enlace: "sytueoffice-calendar:/abrir?clase=evento&id=1"})
	v.avisado(avisos.Aviso{Titulo: "Comprar pan", Enlace: "sytueoffice-notes:/abrir?clase=nota&id=2"})
	if !slices.Equal(avisados, []string{"notes Comprar pan"}) {
		t.Fatalf("avisados = %q", avisados)
	}
	if productoDeEnlace("sytueoffice-calendar:/abrir") != "calendar" {
		t.Fatal("producto del enlace")
	}
}

// El vigía programa en relojes del proceso lo que lee de los almacenes; sin datos, nada.
func TestElVigiaReprogramaSinDatos(t *testing.T) {
	v := &Vigia{Datos: t.TempDir()}
	v.Reprogramar()
	if len(v.Programados("calendar")) != 0 || len(v.Programados("notes")) != 0 {
		t.Fatal("sin almacenes no hay avisos")
	}
}

// Las apps abiertas salen de /proc por el nombre del ejecutable, que es el del catálogo.
func TestLasAppsAbiertasSalenDeProc(t *testing.T) {
	raiz := t.TempDir()
	proceso := func(pid, exe, cmdline string) {
		dir := filepath.Join(raiz, pid)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if exe != "" {
			if err := os.Symlink(exe, filepath.Join(dir, "exe")); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	proceso("10", "/opt/sytue-office/sytue-office-calendar", "")
	proceso("11", "", "/opt/sytue-office/sytue-office-notes\x00--bandeja\x00")
	proceso("12", "/usr/bin/bash", "bash")
	if err := os.MkdirAll(filepath.Join(raiz, "self"), 0o700); err != nil {
		t.Fatal(err)
	}
	ps, ok := local.ProcesosProc(raiz)
	if !ok || !slices.Contains(ps, "sytue-office-calendar") || !slices.Contains(ps, "sytue-office-notes") || len(ps) != 3 {
		t.Fatalf("procesos = %q %v", ps, ok)
	}
	if _, ok := local.ProcesosProc(filepath.Join(raiz, "no")); ok {
		t.Fatal("sin /proc no se sabe")
	}
	if got := carpetasDe([]string{"docs", "notes"}); !slices.Equal(got, []string{"docus", "notes"}) {
		t.Fatalf("carpetas = %q", got)
	}
}
