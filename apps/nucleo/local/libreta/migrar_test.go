// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// libretaDeAntes llena con el repositorio de Contacts el almacén de registros de dir con
// un poco de todo: dos libretas (una borrada después), etiquetas con miembros resueltos
// y sin resolver, favoritos (uno de un UID que ya no está), la papelera, un contacto
// eliminado para siempre y una foto.
func libretaDeAntes(t *testing.T, dir string) {
	t.Helper()
	d, cerrar := implementaciones[0].abrir(t, dir)
	defer cerrar()
	ctx := t.Context()
	libro := libretaNueva(t, d, "Personal")
	otra := libretaNueva(t, d, "Antigua")
	ana := alta(t, d, libro, api.ContactInput{FormattedName: "Ana García", GivenName: "Ana", FamilyName: "García", Emails: []api.ContactEmail{{Email: "ana@x.es"}}, BirthdayNoYear: "--02-03"})
	bea := alta(t, d, libro, api.ContactInput{FormattedName: "Bea Núñez", Organization: "Bodegas"})
	cris := alta(t, d, libro, api.ContactInput{FormattedName: "Cris"})
	dani := alta(t, d, libro, api.ContactInput{FormattedName: "Dani"})
	alta(t, d, otra, api.ContactInput{FormattedName: "En la antigua"})
	if _, err := d.ImportCard(ctx, libro, tarjetaConExtras); err != nil {
		t.Fatal(err)
	}
	g, err := d.CreateGroup(ctx, libro, "Familia")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.RenameGroup(ctx, g.ID, "Familia", []string{ana.UID, bea.UID, dani.UID, "de-fuera"}); err != nil {
		t.Fatal(err)
	}
	if err := d.BatchFavorite(ctx, []string{ana.UID, dani.UID}, true); err != nil {
		t.Fatal(err)
	}
	if err := d.SetPhoto(ctx, bea.UID, fotoPNG); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, cris.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, dani.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.Purge(ctx, dani.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteAddressBook(ctx, otra); err != nil {
		t.Fatal(err)
	}
}

// vistaEntera es todo lo que se ve de una libreta, en JSON y con sus identificadores
// (la migración los conserva).
func vistaEntera(t *testing.T, d datos) string {
	t.Helper()
	ctx := t.Context()
	var b strings.Builder
	poner := func(que string, v any, err error) {
		if err != nil {
			t.Fatal(que, err)
		}
		raw, _ := json.Marshal(v)
		b.WriteString(que + ": " + string(raw) + "\n")
	}
	ls, err := d.AddressBooks(ctx)
	poner("libretas", ls, err)
	for _, l := range ls {
		gs, err := d.Groups(ctx, l.ID)
		poner("etiquetas", gs, err)
		p, err := d.InBookPage(ctx, l.ID, "")
		poner("página", p.Contacts, err)
		ts, err := d.TarjetasCrudas(ctx, l.ID)
		poner("tarjetas", ts, err)
	}
	todos, siguiente, err := d.All(ctx, 500, 0)
	poner("todos", []any{todos, siguiente}, err)
	hits, err := d.Search(ctx, "a", 50)
	poner("buscar", hits, err)
	favoritos, err := d.Favorites(ctx)
	poner("favoritos", favoritos, err)
	tr, err := d.Trash(ctx, 50)
	poner("papelera", tr, err)
	_, csv, err := d.ExportCSV(ctx)
	poner("csv", string(csv), err)
	return b.String()
}

func huellaDe(t *testing.T, ruta string) [32]byte {
	t.Helper()
	raw, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(raw)
}

func TestMigrarPasaLaLibretaEnteraYNoTocaElAlmacen(t *testing.T) {
	dir := t.TempDir()
	libretaDeAntes(t, dir)
	diario := filepath.Join(dir, "registros.jsonl")
	antes := huellaDe(t, diario)
	vieja, cerrar := implementaciones[0].abrir(t, dir)
	quiero := vistaEntera(t, vieja)
	cerrar()

	x, err := libreta.Abrir(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	if got := vistaEntera(t, x); got != quiero {
		t.Fatalf("la libreta migrada no es la de antes:\n%s\nfrente a\n%s", got, quiero)
	}
	inf, ok, err := x.Migracion(t.Context())
	if err != nil || !ok || inf.Origen != "registros" || inf.Contactos != 5 || inf.Libretas != 2 || inf.Etiquetas != 1 || inf.Favoritos != 2 || inf.Huella == "" {
		t.Fatalf("la marca = %+v, %v, %v", inf, ok, err)
	}
	if err := x.Cerrar(); err != nil {
		t.Fatal(err)
	}
	if huellaDe(t, diario) != antes {
		t.Fatal("migrar cambió el almacén de antes")
	}
	// Abrir otra vez no migra otra vez (ni duplica nada).
	x, err = libreta.Abrir(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	if got := vistaEntera(t, x); got != quiero {
		t.Fatal("abrir otra vez cambió la libreta")
	}
	otra, ok, err := x.Migracion(t.Context())
	if err != nil || !ok || !otra.Fecha.Equal(inf.Fecha) {
		t.Fatal("la marca cambió al abrir otra vez", otra, err)
	}
}

// La migración dice cómo va, como la de Photos: HayQueMigrar lo sabe antes de abrir, y
// AbrirCon pasa por leer el almacén, copiar (de 0 a todo, sin volver atrás) y
// comprobar; después ya no hay que migrar ni se dice nada.
func TestMigrarDiceComoVa(t *testing.T) {
	dir := t.TempDir()
	if libreta.HayQueMigrar(dir) {
		t.Fatal("sin almacén no hay que migrar")
	}
	libretaDeAntes(t, dir)
	if !libreta.HayQueMigrar(dir) {
		t.Fatal("con el almacén de antes hay que migrar")
	}
	var avances []libreta.Avance
	x, err := libreta.AbrirCon(t.Context(), dir, sujeto, libreta.Opciones{Avance: func(a libreta.Avance) { avances = append(avances, a) }})
	if err != nil {
		t.Fatal(err)
	}
	_ = x.Cerrar()
	var fases []string
	hechas := -1
	for _, a := range avances {
		if len(fases) == 0 || fases[len(fases)-1] != a.Fase {
			fases = append(fases, a.Fase)
		}
		if a.Fase == libreta.FaseCopiar {
			if a.Hechas < hechas || a.Total == 0 || a.Hechas > a.Total {
				t.Errorf("avance que no cuadra: %+v tras %d", a, hechas)
			}
			hechas = a.Hechas
		}
	}
	if !slices.Equal(fases, []string{libreta.FaseAbrir, libreta.FaseCopiar, libreta.FaseComprobar}) {
		t.Errorf("fases: %v", fases)
	}
	if copiar := avances[len(avances)-2]; copiar.Fase != libreta.FaseCopiar || copiar.Hechas != copiar.Total {
		t.Errorf("el último avance al copiar no llega al final: %+v", copiar)
	}
	if libreta.HayQueMigrar(dir) {
		t.Error("migrada, sigue habiendo que migrar")
	}
	avances = nil
	x, err = libreta.AbrirCon(t.Context(), dir, sujeto, libreta.Opciones{Avance: func(a libreta.Avance) { avances = append(avances, a) }})
	if err != nil {
		t.Fatal(err)
	}
	_ = x.Cerrar()
	if len(avances) != 0 {
		t.Errorf("abrir la migrada dice avances: %+v", avances)
	}
}

func TestMigrarSinAlmacenDeAntes(t *testing.T) {
	dir := t.TempDir()
	x, err := libreta.Abrir(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	inf, ok, err := x.Migracion(t.Context())
	if err != nil || !ok || inf.Origen != "ninguno" {
		t.Fatal("la marca sin almacén", inf, ok, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "registros.jsonl")); !os.IsNotExist(err) {
		t.Fatal("abrir la libreta creó un almacén de registros")
	}
}

// Un corte a mitad (aquí, la comprobación que falla) no deja nada: ni filas ni marca, y
// la próxima vez se migra entera.
func TestMigrarCortadaNoDejaNadaYSeRepite(t *testing.T) {
	dir := t.TempDir()
	libretaDeAntes(t, dir)
	libreta.FallarAlComprobar(func() error { return fs.ErrClosed })
	_, err := libreta.Abrir(t.Context(), dir, sujeto)
	libreta.FallarAlComprobar(nil)
	if err == nil {
		t.Fatal("la migración cortada no falló")
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, libreta.Fichero)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	var filas, marcas int
	if err := db.QueryRowContext(t.Context(), `SELECT (SELECT COUNT(*) FROM fila), (SELECT COUNT(*) FROM meta WHERE clave = 'migrada')`).Scan(&filas, &marcas); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if filas != 0 || marcas != 0 {
		t.Fatalf("la migración cortada dejó %d filas y %d marcas", filas, marcas)
	}
	if _, err := libreta.AbrirLectura(t.Context(), dir, sujeto); err == nil {
		t.Fatal("otra app lee una libreta sin migrar")
	}
	x, err := libreta.Abrir(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	todos, _, err := x.All(t.Context(), 100, 0)
	if err != nil || len(todos) != 3 {
		t.Fatalf("tras repetir la migración = %v, %v", nombres(todos), err)
	}
}

// Una base con contactos y sin marca (no debería pasar) no se mezcla con lo de antes.
func TestMigrarNoMezclaUnaBaseConDatos(t *testing.T) {
	dir := t.TempDir()
	libretaDeAntes(t, dir)
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, libreta.Fichero))
	if err != nil {
		t.Fatal(err)
	}
	x, err := libreta.Abrir(t.Context(), t.TempDir(), sujeto)
	if err != nil {
		t.Fatal(err)
	}
	_ = x.Cerrar()
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE fila (clase TEXT, id TEXT, json TEXT); INSERT INTO fila VALUES ('book', 'x', '{}')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if _, err := libreta.Abrir(t.Context(), dir, sujeto); err == nil {
		t.Fatal("migró sobre una base con contactos")
	}
}

// La libreta en solo lectura de otra app lee lo que Contacts confirma mientras sigue
// abierto escribiendo, y no puede escribir.
func TestOtraAppLeeMientrasContactsEscribe(t *testing.T) {
	dir := t.TempDir()
	x, err := libreta.Abrir(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	ctx := t.Context()
	libro := libretaNueva(t, x, "Personal")
	lec, err := libreta.AbrirLectura(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer lec.Cerrar()
	alta(t, x, libro, api.ContactInput{FormattedName: "Ana", Emails: []api.ContactEmail{{Email: "ana@x.es"}}})
	if hits, err := lec.Search(ctx, "ana@", 10); err != nil || len(hits) != 1 {
		t.Fatal("la otra app no ve lo confirmado", hits, err)
	}
	antes := lec.Revision(ctx)
	alta(t, x, libro, api.ContactInput{FormattedName: "Bea"})
	if lec.Revision(ctx) <= antes {
		t.Fatal("la revisión no sube con una escritura")
	}
	if _, err := lec.Create(ctx, libro, api.ContactInput{FormattedName: "No"}); err == nil {
		t.Fatal("la libreta en solo lectura escribió")
	}
	if _, err := libreta.AbrirLectura(t.Context(), t.TempDir(), sujeto); err == nil {
		t.Fatal("AbrirLectura sin libreta")
	}
}

// «Eliminar para siempre» quita al contacto del disco (la base y su diario) sin cerrar.
func TestEliminarParaSiempreSeVaDelDisco(t *testing.T) {
	antes := libreta.EsperaVaciar
	libreta.EsperaVaciar = 10 * time.Millisecond
	t.Cleanup(func() { libreta.EsperaVaciar = antes })
	dir := t.TempDir()
	x, err := libreta.Abrir(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	ctx := t.Context()
	libro := libretaNueva(t, x, "Personal")
	alta(t, x, libro, api.ContactInput{FormattedName: "Ana Ruiz"})
	secreto := alta(t, x, libro, api.ContactInput{FormattedName: "Ex Pareja Secreta", Note: "Nota muy privada"})
	if err := x.Delete(ctx, secreto.ID); err != nil {
		t.Fatal(err)
	}
	if err := x.Purge(ctx, secreto.ID); err != nil {
		t.Fatal(err)
	}
	limite := time.Now().Add(10 * time.Second)
	for {
		sigue := false
		for _, f := range []string{libreta.Fichero, libreta.Fichero + "-wal"} {
			raw, _ := os.ReadFile(filepath.Join(dir, f))
			sigue = sigue || bytes.Contains(raw, []byte("Pareja Secreta")) || bytes.Contains(raw, []byte("pareja secreta")) || bytes.Contains(raw, []byte("muy privada"))
		}
		if !sigue {
			break
		}
		if time.Now().After(limite) {
			t.Fatal("lo eliminado para siempre sigue en el disco con la app abierta")
		}
		time.Sleep(20 * time.Millisecond)
	}
	todos, _, err := x.All(ctx, 10, 0)
	if err != nil || len(todos) != 1 {
		t.Fatal("tras vaciar", todos, err)
	}
}

// La base, su diario y su índice son solo de la persona (0600), también con Mail leyendo.
func TestLaBaseEsSoloDeLaPersona(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sin permisos de Unix")
	}
	dir := t.TempDir()
	x, err := libreta.Abrir(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	libro := libretaNueva(t, x, "Personal")
	alta(t, x, libro, api.ContactInput{FormattedName: "Ana"})
	lec, err := libreta.AbrirLectura(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer lec.Cerrar()
	if _, _, err := lec.All(t.Context(), 10, 0); err != nil {
		t.Fatal(err)
	}
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		fi, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(e.Name(), libreta.Fichero) && fi.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s es %v: la pueden leer otros", e.Name(), fi.Mode().Perm())
		}
	}
}

// La copia en caliente de la base abierta se abre como libreta y tiene lo mismo.
func TestCopiaEnCaliente(t *testing.T) {
	dir := t.TempDir()
	x, err := libreta.Abrir(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	libro := libretaNueva(t, x, "Personal")
	alta(t, x, libro, api.ContactInput{FormattedName: "Ana"})
	copia := t.TempDir()
	if err := x.CopiarEnCaliente(context.Background(), filepath.Join(copia, libreta.Fichero)); err != nil {
		t.Fatal(err)
	}
	y, err := libreta.AbrirLectura(t.Context(), copia, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer y.Cerrar()
	if todos, _, err := y.All(t.Context(), 10, 0); err != nil || len(todos) != 1 {
		t.Fatal("la copia", todos, err)
	}
}
