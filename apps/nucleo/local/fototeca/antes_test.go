// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca_test

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/contrato"
)

// ficherosDe son las rutas (relativas) de los ficheros que hay bajo raiz.
func ficherosDe(t *testing.T, raiz string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(raiz, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(raiz, ruta)
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

// envejecer pone a los ficheros de objetos/ una fecha de hace una hora: los de un
// almacén de antes de verdad lo son, y lo de los últimos minutos no se barre nunca (puede
// ser de una importación en curso).
func envejecer(t *testing.T, dir string) {
	t.Helper()
	hace := time.Now().Add(-time.Hour)
	for _, f := range ficherosDe(t, filepath.Join(dir, "objetos")) {
		if err := os.Chtimes(filepath.Join(dir, "objetos", filepath.FromSlash(f)), hace, hace); err != nil {
			t.Fatal(err)
		}
	}
}

// citados son los ficheros de objetos/ que cita la base de la fototeca, leídos de ella.
func citados(t *testing.T, dir string) []string {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, fototeca.Fichero)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	filas, err := db.QueryContext(t.Context(), `SELECT huella FROM fotos WHERE retenido = 1 AND purgada IS NULL UNION SELECT sha FROM derivadas`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = filas.Close() }()
	var out []string
	for filas.Next() {
		var sha string
		if err := filas.Scan(&sha); err != nil {
			t.Fatal(err)
		}
		out = append(out, sha[:2]+"/"+sha)
	}
	if err := filas.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

// «Borrar los datos de antes de la migración» (Ajustes → Datos): sin la migración no hay
// nada que borrar ni se borra; migrada, quita el diario conservado, lo demás del almacén
// y, de objetos/ (que es de las dos), solo lo que la fototeca ya no cita, aunque esté
// abierta; lo vivo —la base, cada original y cada derivada, lo de antes y lo de después
// de migrar, y lo que no es del almacén— se queda, y la fototeca se ve y se lee igual.
func TestBorrarLoDeAntesNoTocaLoVivo(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	antes := almacenDeAntes(t, dir)
	// Sin migrar: ni se ofrece ni se borra nada.
	if a, err := fototeca.MedirAntes(ctx, dir); err != nil || a.Migrada || a.Ficheros != 0 {
		t.Fatal("sin migrar:", a, err)
	}
	todo := ficherosDe(t, dir)
	if _, err := fototeca.BorrarAntes(ctx, dir); !errors.Is(err, fototeca.ErrSinMigrar) {
		t.Fatal("borrar sin migrar:", err)
	}
	if got := ficherosDe(t, dir); !slices.Equal(got, todo) {
		t.Fatalf("sin migrar se ha tocado algo:\n%v\n%v", todo, got)
	}

	f := abrirEn(t, dir)
	defer f.Close()
	// Lo de después de migrar: una foto nueva, una editada otra vez y una purgada.
	nueva := contrato.JPEG(60, 40, 99)
	cli, err := api.NewPhotos("https://sin-red.invalid", nil, nil).WithPrivateData(f)
	if err != nil {
		t.Fatal(err)
	}
	x, err := cli.ImportarFoto(ctx, uuid.NewString(), "nueva.jpg", "image/jpeg", bytes.NewReader(nueva), int64(len(nueva)))
	if err != nil {
		t.Fatal(err)
	}
	antes.originales[x.ID] = nueva
	if _, err := cli.Rotar(ctx, antes.editada); err != nil {
		t.Fatal(err)
	}
	purgada := antes.linea[0].ID
	if err := cli.Trash(ctx, purgada); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.Purgar(ctx, purgada); err != nil {
		t.Fatal(err)
	}
	delete(antes.originales, purgada)
	delete(antes.originales, antes.purgada)
	envejecer(t, dir)
	// Lo que no es del almacén ni de la fototeca, un huérfano viejo con nombre de huella
	// (lo dejó un corte) y la subida en curso de otra importación.
	huerfano := strings.Repeat("ab", 32)
	if err := os.WriteFile(filepath.Join(dir, "objetos", "ab", huerfano), []byte("de nadie"), 0o600); err != nil {
		t.Fatal(err)
	}
	hace := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "objetos", "ab", huerfano), hace, hace); err != nil {
		t.Fatal(err)
	}
	enCurso := filepath.Join(dir, "objetos", ".subida-en-curso")
	ajeno := filepath.Join(dir, "léeme.txt")
	for _, r := range []string{enCurso, ajeno} {
		if err := os.WriteFile(r, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	vivos := citados(t, dir)
	visto := ver(t, f)

	medido, err := fototeca.MedirAntes(ctx, dir)
	if err != nil || !medido.Migrada || medido.Ficheros == 0 || medido.Bytes == 0 {
		t.Fatal("medir:", medido, err)
	}
	// Con el almacén de antes abierto en otra app (su cerrojo), nada.
	todo = ficherosDe(t, dir)
	soltar, err := almacen.CerrojoExclusivo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fototeca.BorrarAntes(ctx, dir); !errors.Is(err, fototeca.ErrEnUso) {
		t.Fatal("con el almacén abierto:", err)
	}
	soltar()
	if got := ficherosDe(t, dir); !slices.Equal(got, todo) {
		t.Fatalf("con el almacén abierto se ha borrado algo:\n%v\n%v", todo, got)
	}

	hecho, err := fototeca.BorrarAntes(ctx, dir)
	if err != nil || hecho != medido {
		t.Fatalf("borrar: %+v, %v; medido %+v", hecho, err, medido)
	}
	quedan := ficherosDe(t, dir)
	for _, r := range quedan {
		base := filepath.Base(r)
		if strings.HasPrefix(base, "registros") || strings.HasPrefix(r, "diario") || base == "origen.json" || base == almacen.FicheroCerrojo {
			t.Errorf("sigue %s, del almacén de antes", r)
		}
	}
	var objetos []string
	for _, r := range quedan {
		if o, ok := strings.CutPrefix(r, "objetos/"); ok && !strings.HasPrefix(filepath.Base(o), ".") {
			objetos = append(objetos, o)
		}
	}
	if !slices.Equal(objetos, vivos) {
		t.Errorf("en objetos/ quedan\n%v\ny la fototeca cita\n%v", objetos, vivos)
	}
	for _, r := range []string{fototeca.Fichero, "léeme.txt", "objetos/.subida-en-curso"} {
		if !slices.Contains(quedan, r) {
			t.Errorf("se ha borrado %s", r)
		}
	}
	// La fototeca abierta sigue igual: se ve, y cada original y miniatura se lee.
	compararVisto(t, visto, ver(t, f))
	for id, datos := range antes.originales {
		var b bytes.Buffer
		if err := f.Original(ctx, id, &b); err != nil || !bytes.Equal(b.Bytes(), datos) {
			t.Error("el original de", id, err)
		}
	}
	for _, x := range append(slices.Clone(visto.linea), visto.papelera...) {
		for _, v := range []string{api.PhotoThumb, api.PhotoPreview} {
			r, err := f.VariantDe(ctx, x.ID, v, "")
			if err != nil {
				t.Fatal(v, "de", x.Name, err)
			}
			_, _ = io.Copy(io.Discard, r)
			_ = r.Close()
		}
	}
	if a, err := fototeca.MedirAntes(ctx, dir); err != nil || !a.Migrada || a.Ficheros != 0 {
		t.Error("después de borrar queda:", a, err)
	}
	// Borrado, objetos/ no se vuelve a recorrer: lo que un corte deje sin citar desde
	// entonces es cosa de vaciar la papelera (barrer), no de los datos de antes.
	otro := filepath.Join(dir, "objetos", "cd", strings.Repeat("cd", 32))
	if err := os.MkdirAll(filepath.Dir(otro), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otro, []byte("de un corte"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(otro, hace, hace); err != nil {
		t.Fatal(err)
	}
	if a, err := fototeca.MedirAntes(ctx, dir); err != nil || !a.Migrada || a.Ficheros != 0 {
		t.Error("borrado lo de antes, se vuelve a ofrecer:", a, err)
	}
	f.Close()
	if fototeca.HayQueMigrar(dir) {
		t.Fatal("borrado lo de antes, dice que hay que migrar")
	}
	g := abrirEn(t, dir)
	defer g.Close()
	compararVisto(t, visto, ver(t, g))
}

// Una fototeca que nació sin almacén de antes no tiene nada que borrar.
func TestSinAlmacenDeAntesNoHayNadaQueBorrar(t *testing.T) {
	dir := t.TempDir()
	f := abrirEn(t, dir)
	f.Close()
	if a, err := fototeca.MedirAntes(t.Context(), dir); err != nil || a.Migrada {
		t.Fatal(a, err)
	}
	if _, err := fototeca.BorrarAntes(t.Context(), dir); !errors.Is(err, fototeca.ErrSinMigrar) {
		t.Fatal(err)
	}
	if a, err := fototeca.MedirAntes(t.Context(), t.TempDir()); err != nil || a.Migrada {
		t.Fatal("sin fototeca:", a, err)
	}
}
