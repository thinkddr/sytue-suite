// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/internal/libretaprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// # Dar de alta con la libreta llena (R-MEDIR)
//
// Cuánto tarda dar de alta un contacto con 300, 2.000 y 20.000 en la libreta, importar
// un CSV de 2.000 filas en una libreta vacía y leer la instantánea sin cambios, en el
// almacén en claro de Sytue Office (como `cmd-local/contacts`). Los números están en
// SYTUE-OFFICE.md («Pendiente», «Mail»):
//
//	go test -tags goolm -run '^$' -bench 'Alta|ImportarCSV|SnapshotSinCambios' -benchtime 10x ./contacts/

var (
	plantillasMu sync.Mutex
	plantillas   = map[int]plantilla{}
)

// plantilla es una carpeta de Contacts con n contactos, escrita una vez por proceso.
type plantilla struct{ dir, libro string }

func libretaDe(tb testing.TB, n int) plantilla {
	tb.Helper()
	plantillasMu.Lock()
	defer plantillasMu.Unlock()
	if p, ok := plantillas[n]; ok {
		return p
	}
	dir, err := os.MkdirTemp("", "sytue-contacts-rapidez-")
	if err != nil {
		tb.Fatal(err)
	}
	libro, err := libretaprueba.Escribir(dir, "instalacion-1", n)
	if err != nil {
		_ = os.RemoveAll(dir)
		tb.Fatal(err)
	}
	p := plantilla{dir: dir, libro: libro}
	plantillas[n] = p
	return p
}

// copiaDe es una copia de la plantilla de n contactos que la prueba puede cambiar.
func copiaDe(tb testing.TB, n int) (string, string) {
	tb.Helper()
	p := libretaDe(tb, n)
	dir := tb.TempDir()
	err := filepath.WalkDir(p.dir, func(ruta string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(p.dir, ruta)
		if err != nil {
			return err
		}
		destino := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(destino, 0o700)
		}
		if rel == "cerrojo" {
			return nil
		}
		origen, err := os.Open(ruta)
		if err != nil {
			return err
		}
		defer origen.Close()
		f, err := os.OpenFile(destino, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, origen); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	})
	if err != nil {
		tb.Fatal(err)
	}
	return dir, p.libro
}

func abrirEn(tb testing.TB, dir string) *contacts.Repository {
	tb.Helper()
	ctx := context.Background()
	rec, err := almacen.Abrir(ctx, dir, "contacts", "instalacion-1")
	if err != nil {
		tb.Fatal(err)
	}
	repo, err := contacts.New(rec.Config)
	if err != nil {
		rec.Cerrar()
		tb.Fatal(err)
	}
	tb.Cleanup(func() { repo.Close(); rec.Cerrar() })
	if err := almacen.Preparar(ctx, repo); err != nil {
		tb.Fatal(err)
	}
	return repo
}

// BenchmarkAlta es dar de alta un contacto con la libreta llena.
func BenchmarkAlta(b *testing.B) {
	for _, n := range []int{300, 2_000, 20_000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			dir, libro := copiaDe(b, n)
			repo := abrirEn(b, dir)
			ctx := context.Background()
			// La app ya ha enseñado la libreta: la primera lectura no es del alta.
			if _, err := repo.AddressBooks(ctx); err != nil {
				b.Fatal(err)
			}
			r := rand.New(rand.NewPCG(7, uint64(n)))
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				c := libretaprueba.Realista(r, n+i)
				i++
				if _, err := repo.Create(ctx, libro, entradaDe(c)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkAltaConIndices es dar de alta y editar con favoritos y papelera en la libreta
// (2.000 contactos): la libreta de después sigue saliendo de lo escrito, sin releer.
func BenchmarkAltaConIndices(b *testing.B) {
	dir, libro := copiaDe(b, 2_000)
	repo := abrirEn(b, dir)
	ctx := context.Background()
	todos, _, err := repo.All(ctx, 10, 0)
	if err != nil || len(todos) < 3 {
		b.Fatal(len(todos), err)
	}
	if err := repo.SetFavorite(ctx, todos[0].UID, true); err != nil {
		b.Fatal(err)
	}
	if err := repo.Delete(ctx, todos[1].ID); err != nil {
		b.Fatal(err)
	}
	if _, err := repo.AddressBooks(ctx); err != nil {
		b.Fatal(err)
	}
	r := rand.New(rand.NewPCG(8, 2_000))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		c, err := repo.Create(ctx, libro, entradaDe(libretaprueba.Realista(r, 2_000+i)))
		i++
		if err != nil {
			b.Fatal(err)
		}
		cambio := c.Input()
		cambio.Note = "Editado"
		if _, err := repo.Update(ctx, c.ID, cambio, c.ETag); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSnapshotSinCambios es leer la instantánea cuando nada ha cambiado: lo que
// hace cada consulta de la pantalla (lista, búsqueda, ficha).
func BenchmarkSnapshotSinCambios(b *testing.B) {
	dir, _ := copiaDe(b, 20_000)
	repo := abrirEn(b, dir)
	ctx := context.Background()
	if _, err := repo.Snapshot(ctx); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := repo.Snapshot(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkImportarCSV es importar un CSV de 2.000 filas en una libreta vacía.
func BenchmarkImportarCSV(b *testing.B) {
	csv := csvDe(b, 2_000)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		dir, libro := copiaDe(b, 0)
		repo := abrirEn(b, dir)
		b.StartTimer()
		hechos, saltados, err := repo.ImportCSV(ctx, libro, csv)
		if err != nil || hechos != 2_000 || saltados != 0 {
			b.Fatal(hechos, saltados, err)
		}
	}
}

// csvDe es un CSV de Google Contacts con n personas realistas.
func csvDe(tb testing.TB, n int) []byte {
	tb.Helper()
	r := rand.New(rand.NewPCG(2026, 12))
	gente := make([]card.Contact, n)
	for i := range gente {
		gente[i] = libretaprueba.Realista(r, i)
		// Cada fila con sus propios correos: la importación salta las que repiten uno
		// de la libreta, y aquí se mide importarlas todas.
		for j := range gente[i].Emails {
			gente[i].Emails[j].Email = fmt.Sprintf("%d.%d.%s", i, j, gente[i].Emails[j].Email)
		}
	}
	return csvDeGente(tb, gente)
}

// csvDeGente es el CSV de Google Contacts de gente.
func csvDeGente(tb testing.TB, gente []card.Contact) []byte {
	tb.Helper()
	var buf bytes.Buffer
	if err := card.WriteContactsCSV(&buf, gente); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

// entradaDe es c como lo manda la ficha de la pantalla.
func entradaDe(c card.Contact) api.ContactInput {
	in := api.ContactInput{
		GivenName: c.GivenName, FamilyName: c.FamilyName, FormattedName: c.FormattedName,
		Nickname: c.Nickname, Organization: c.Organization, Title: c.Title, Note: c.Note, BirthdayNoYear: c.BirthdayNoYear,
	}
	for _, e := range c.Emails {
		in.Emails = append(in.Emails, api.ContactEmail{Email: e.Email, Type: string(e.Type), IsPrimary: e.IsPrimary})
	}
	for _, p := range c.Phones {
		in.Phones = append(in.Phones, api.ContactPhone{Phone: p.Phone, Type: string(p.Type), IsPrimary: p.IsPrimary})
	}
	for _, a := range c.Addresses {
		in.Addresses = append(in.Addresses, api.ContactAddress{Street: a.Street, Locality: a.Locality, PostalCode: a.PostalCode, Country: a.Country, Type: string(a.Type)})
	}
	return in
}

func TestMain(m *testing.M) {
	codigo := m.Run()
	for _, p := range plantillas {
		_ = os.RemoveAll(p.dir)
	}
	os.Exit(codigo)
}
