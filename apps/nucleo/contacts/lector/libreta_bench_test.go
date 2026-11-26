// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package lector_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/internal/libretaprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
)

// # La libreta con 20.000 contactos (R-MEDIR)
//
// Mientras Contacts siga en el almacén de registros de antes, Mail y Calendar
// autocompletan con su libreta en memoria (Libreta), que se relee entera cuando el diario
// de Contacts cambia (con la libreta SQLite no hay relectura: lo mide
// `contacts/carga_libreta_test.go`). Estas medidas son
// las de SYTUE-OFFICE.md («Mail»): cuánto tarda releerla, cuánta memoria ocupa y cuánto
// tarda una letra del autocompletado, con la lista ya leída y justo después de un
// cambio en Contacts.
//
//	go test -tags goolm -run '^$' -bench Libreta -benchtime 3x ./contacts/lector/
//
// La libreta se escribe directamente con el puerto del almacén (`libretaprueba`, el
// mismo generador que miden las altas de Contacts en `contacts/rapidez_test.go`).

// ContactosGrande es el tamaño de la libreta medida.
const ContactosGrande = 20_000

var (
	grandeUna sync.Once
	grandeDir string
	grandeErr error
)

func TestMain(m *testing.M) {
	codigo := m.Run()
	if grandeDir != "" {
		_ = os.RemoveAll(grandeDir)
	}
	os.Exit(codigo)
}

// libretaGrande es la carpeta de Contacts con ContactosGrande contactos, escrita una vez
// por proceso.
func libretaGrande(tb testing.TB) string {
	tb.Helper()
	grandeUna.Do(func() {
		grandeDir, grandeErr = os.MkdirTemp("", "sytue-libreta-grande-")
		if grandeErr == nil {
			_, grandeErr = libretaprueba.Escribir(grandeDir, "instalacion-1", ContactosGrande)
		}
	})
	if grandeErr != nil {
		tb.Fatal(grandeErr)
	}
	return grandeDir
}

// lo que se teclea en un campo de direcciones: dos o más letras de un nombre, de un
// apellido (con y sin tilde), de una empresa o de un dominio, y algo que no está.
var tecleos = []string{"ma", "mar", "gar", "lopez", "lópez", "etxe", "construc", "@gmail", "ana.m", "+34 6", "zzz"}

// BenchmarkLibretaReleer es releer la libreta entera: lo que pasa cuando cambia el
// diario de Contacts. Da también la memoria que se queda en uso con la lista y con sus
// textos de búsqueda (los de Libreta).
func BenchmarkLibretaReleer(b *testing.B) {
	dir := libretaGrande(b)
	ctx := context.Background()
	l := lector.Nuevo(dir, "instalacion-1")
	var antes, despues, conTextos runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&antes)
	lista, err := l.Contactos(ctx)
	if err != nil || len(lista) != ContactosGrande {
		b.Fatal(len(lista), err)
	}
	runtime.GC()
	runtime.ReadMemStats(&despues)
	textos := contacts.TextosBuscables(lista)
	runtime.GC()
	runtime.ReadMemStats(&conTextos)
	runtime.KeepAlive(lista)
	runtime.KeepAlive(textos)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := l.Contactos(ctx); err != nil {
			b.Fatal(err)
		}
	}
	// Después del bucle: ResetTimer borra las métricas propias.
	b.ReportMetric(float64(despues.HeapAlloc-antes.HeapAlloc)/(1<<20), "MiB-lista")
	b.ReportMetric(float64(conTextos.HeapAlloc-despues.HeapAlloc)/(1<<20), "MiB-textos")
}

// BenchmarkLibretaAutocompletar es una letra del redactor de Mail (Search, 8) con la
// libreta ya en memoria.
func BenchmarkLibretaAutocompletar(b *testing.B) {
	lib := libretaLeida(b)
	ctx := context.Background()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if _, err := lib.Search(ctx, tecleos[i%len(tecleos)], 8); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

// BenchmarkLibretaAutocompletarSinNada es el peor caso de una letra: nada casa y se
// recorre la libreta entera.
func BenchmarkLibretaAutocompletarSinNada(b *testing.B) {
	lib := libretaLeida(b)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if cs, err := lib.Search(ctx, "zzz", 8); err != nil || len(cs) != 0 {
			b.Fatal(len(cs), err)
		}
	}
}

// BenchmarkLibretaInvitados es una letra de los invitados de Calendar (Buscar, 5).
func BenchmarkLibretaInvitados(b *testing.B) {
	lib := libretaLeida(b)
	ctx := context.Background()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if _, err := lib.Buscar(ctx, tecleos[i%len(tecleos)], 5); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

// BenchmarkLibretaAutocompletarTrasCambio es la primera letra después de que Contacts
// cambie: lo que espera la persona en el peor momento.
func BenchmarkLibretaAutocompletarTrasCambio(b *testing.B) {
	dir := libretaGrande(b)
	lib := libretaLeida(b)
	ctx := context.Background()
	diario := filepath.Join(dir, "registros.jsonl")
	b.ReportAllocs()
	n := 0
	for b.Loop() {
		b.StopTimer()
		n++
		// Contacts escribió: el diario tiene otra fecha (el contenido da igual).
		cuando := time.Now().Add(time.Duration(n) * time.Second)
		if err := os.Chtimes(diario, cuando, cuando); err != nil {
			b.Fatal(err)
		}
		lector.Caducar(lib)
		b.StartTimer()
		if _, err := lib.Search(ctx, "gar", 8); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	lector.EsperarRelectura(lib)
}

// libretaLeida es la libreta de 20.000 contactos ya leída en memoria.
func libretaLeida(b *testing.B) *lector.Libreta {
	b.Helper()
	lib := lector.Nuevo(libretaGrande(b), "instalacion-1").Libreta()
	if cs, err := lib.Contactos(context.Background()); err != nil || len(cs) != ContactosGrande {
		b.Fatal(len(cs), err)
	}
	return lib
}
