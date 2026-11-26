// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// entradasDe son las rutas (relativas) de lo que hay bajo raiz, carpetas incluidas.
func entradasDe(t *testing.T, raiz string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(raiz, func(ruta string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if rel, _ := filepath.Rel(raiz, ruta); rel != "." {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

// «Borrar los datos de antes de la migración» (Ajustes → Datos): sin la migración no se
// ofrece ni se borra nada; migrada, se va el almacén de registros entero (su diario,
// objetos/ y lo demás) aunque Contacts esté abierta, y la base, el estado de las cuentas
// (`agenda/`) y lo que no es del almacén se quedan: la libreta se ve igual.
func TestBorrarLoDeAntesNoTocaLaLibreta(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	libretaDeAntes(t, dir)
	if a, err := libreta.MedirAntes(ctx, dir); err != nil || a.Migrada || a.Ficheros != 0 {
		t.Fatal("sin migrar:", a, err)
	}
	todo := entradasDe(t, dir)
	if _, err := libreta.BorrarAntes(ctx, dir); !errors.Is(err, libreta.ErrSinMigrar) {
		t.Fatal("borrar sin migrar:", err)
	}
	if got := entradasDe(t, dir); !slices.Equal(got, todo) {
		t.Fatalf("sin migrar se ha tocado algo:\n%v\n%v", todo, got)
	}

	x, err := libreta.Abrir(ctx, dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	quiero := vistaEntera(t, x)
	// Lo que no es del almacén: el estado de una cuenta y un fichero de la persona.
	if err := os.MkdirAll(filepath.Join(dir, "agenda"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{filepath.Join(dir, "agenda", "cuenta-1.json"), filepath.Join(dir, "léeme.txt")} {
		if err := os.WriteFile(r, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	medido, err := libreta.MedirAntes(ctx, dir)
	if err != nil || !medido.Migrada || medido.Ficheros == 0 || medido.Bytes == 0 {
		t.Fatal("medir:", medido, err)
	}
	// Con el almacén abierto en otra app (su cerrojo), nada.
	todo = entradasDe(t, dir)
	soltar, err := almacen.CerrojoExclusivo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := libreta.BorrarAntes(ctx, dir); !errors.Is(err, libreta.ErrEnUso) {
		t.Fatal("con el almacén abierto:", err)
	}
	soltar()
	if got := entradasDe(t, dir); !slices.Equal(got, todo) {
		t.Fatalf("con el almacén abierto se ha borrado algo:\n%v\n%v", todo, got)
	}

	hecho, err := libreta.BorrarAntes(ctx, dir)
	if err != nil || hecho != medido {
		t.Fatalf("borrar: %+v, %v; medido %+v", hecho, err, medido)
	}
	var quedan []string
	for _, r := range entradasDe(t, dir) {
		if !strings.HasPrefix(r, libreta.Fichero) {
			quedan = append(quedan, r)
		}
	}
	if !slices.Equal(quedan, []string{"agenda", "agenda/cuenta-1.json", "léeme.txt"}) {
		t.Errorf("quedan %v", quedan)
	}
	if got := vistaEntera(t, x); got != quiero {
		t.Fatal("la libreta abierta ha cambiado al borrar lo de antes")
	}
	if a, err := libreta.MedirAntes(ctx, dir); err != nil || !a.Migrada || a.Ficheros != 0 {
		t.Error("después de borrar queda:", a, err)
	}
	if err := x.Cerrar(); err != nil {
		t.Fatal(err)
	}
	if libreta.HayQueMigrar(dir) {
		t.Fatal("borrado lo de antes, dice que hay que migrar")
	}
	y, err := libreta.Abrir(ctx, dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer y.Cerrar()
	if got := vistaEntera(t, y); got != quiero {
		t.Fatal("al volver a abrir, la libreta no es la misma")
	}
}

// Una libreta que nació sin almacén de antes no tiene nada que borrar, aunque luego
// aparezca uno (no se ha comprobado contra él).
func TestSinAlmacenDeAntesNoHayNadaQueBorrar(t *testing.T) {
	dir := t.TempDir()
	x, err := libreta.Abrir(t.Context(), dir, sujeto)
	if err != nil {
		t.Fatal(err)
	}
	_ = x.Cerrar()
	if err := os.WriteFile(filepath.Join(dir, "registros.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if a, err := libreta.MedirAntes(t.Context(), dir); err != nil || a.Migrada {
		t.Fatal(a, err)
	}
	if _, err := libreta.BorrarAntes(t.Context(), dir); !errors.Is(err, libreta.ErrSinMigrar) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "registros.jsonl")); err != nil {
		t.Fatal("se ha borrado un almacén que no se comprobó:", err)
	}
	if a, err := libreta.MedirAntes(t.Context(), t.TempDir()); err != nil || a.Migrada {
		t.Fatal("sin libreta:", a, err)
	}
}
