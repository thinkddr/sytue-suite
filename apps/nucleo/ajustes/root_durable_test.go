//go:build linux || android

// SPDX-License-Identifier: Apache-2.0
package ajustes

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func abrirAnclaPrueba(t *testing.T) (string, *os.Root) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return dir, root
}

func operacionesPrueba() operacionesDurables {
	op := operacionesReales()
	op.syncDir = func(*os.File) error { return nil }
	return op
}

func TestRaizDurableSincronizaCadenaYUsaElPadreAnclado(t *testing.T) {
	_, ancla := abrirAnclaPrueba(t)
	op := operacionesPrueba()
	var orden []string
	op.syncDir = func(f *os.File) error { orden = append(orden, filepath.Clean(f.Name())); return nil }
	r, err := abrirRaizDurable(ancla, "uno/dos", op)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if len(orden) != 5 {
		t.Fatalf("faltan barreras de ancla/hijos/padres: %v", orden)
	}
	if orden[0] != orden[2] || orden[1] != orden[4] || orden[3] == orden[1] {
		t.Fatalf("orden de directorios incorrecto: %v", orden)
	}
	orden = nil
	if err := r.Escribir("data", []byte("hola")); err != nil {
		t.Fatal(err)
	}
	if len(orden) != 2 || orden[0] != orden[1] {
		t.Fatalf("rename sin barrera del mismo directorio: %v", orden)
	}
	got, err := ancla.ReadFile("uno/dos/data")
	if err != nil || string(got) != "hola" {
		t.Fatal(string(got), err)
	}
}

func TestRaizDurableFalloAntesYDespuesDeRename(t *testing.T) {
	_, ancla := abrirAnclaPrueba(t)
	fallo := errors.New("fallo artificial")
	op := operacionesPrueba()
	var failFile, failAfter bool
	var syncs int
	op.syncFile = func(f *os.File) error {
		if failFile {
			return fallo
		}
		return f.Sync()
	}
	op.syncDir = func(*os.File) error {
		syncs++
		if failAfter && syncs == 2 {
			return fallo
		}
		return nil
	}
	r, err := abrirRaizDurable(ancla, "dir", op)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Escribir("data", []byte("anterior")); err != nil {
		t.Fatal(err)
	}
	failFile = true
	if err := r.Escribir("data", []byte("nuevo")); !errors.Is(err, fallo) || errors.Is(err, ErrPersistenciaIncierta) {
		t.Fatal(err)
	}
	got, err := ancla.ReadFile("dir/data")
	if err != nil || string(got) != "anterior" {
		t.Fatal(string(got), err)
	}
	failFile, failAfter, syncs = false, true, 0
	if err := r.Escribir("data", []byte("nuevo")); !errors.Is(err, fallo) || !errors.Is(err, ErrPersistenciaIncierta) {
		t.Fatal(err)
	}
	got, err = ancla.ReadFile("dir/data")
	if err != nil || string(got) != "nuevo" {
		t.Fatal(string(got), err)
	}
	failAfter = false
	if err := r.Escribir("data", []byte("nuevo")); err != nil {
		t.Fatal(err)
	}
}

func TestRaizDurableRenameConErrorConservaTemporalAjeno(t *testing.T) {
	_, ancla := abrirAnclaPrueba(t)
	fallo := errors.New("respuesta perdida")
	op := operacionesPrueba()
	var temporal string
	op.rename = func(root *os.Root, oldName, newName string) error {
		temporal = oldName
		if err := root.Rename(oldName, newName); err != nil {
			return err
		}
		if err := root.WriteFile(oldName, []byte("otro"), 0o600); err != nil {
			return err
		}
		return fallo
	}
	r, err := abrirRaizDurable(ancla, "dir", op)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Escribir("data", []byte("nuevo")); !errors.Is(err, fallo) || !errors.Is(err, ErrPersistenciaIncierta) {
		t.Fatal(err)
	}
	got, err := ancla.ReadFile(filepath.Join("dir", temporal))
	if err != nil || string(got) != "otro" {
		t.Fatal(string(got), err)
	}
	got, err = ancla.ReadFile("dir/data")
	if err != nil || string(got) != "nuevo" {
		t.Fatal(string(got), err)
	}
}

func TestRaizDurableBorradoInciertoReintentaYConfirmaAusencia(t *testing.T) {
	_, ancla := abrirAnclaPrueba(t)
	fallo := errors.New("respuesta perdida")
	op := operacionesPrueba()
	var perder bool
	var syncs int
	op.syncDir = func(*os.File) error { syncs++; return nil }
	op.remove = func(root *os.Root, nombre string) error {
		err := root.Remove(nombre)
		if err == nil && perder {
			return fallo
		}
		return err
	}
	r, err := abrirRaizDurable(ancla, "dir", op)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Escribir("data", []byte("valor")); err != nil {
		t.Fatal(err)
	}
	perder = true
	if err := r.Borrar("data"); !errors.Is(err, fallo) || !errors.Is(err, ErrPersistenciaIncierta) {
		t.Fatal(err)
	}
	perder, syncs = false, 0
	if err := r.Borrar("data"); err != nil {
		t.Fatal(err)
	}
	if syncs != 2 {
		t.Fatalf("ausencia sin barrera posterior: %d", syncs)
	}
	if _, err := ancla.Stat("dir/data"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestRaizDurableAperturaFallaCerradaYRechazaEnlaces(t *testing.T) {
	dir, ancla := abrirAnclaPrueba(t)
	fallo := errors.New("fsync no disponible")
	preflight := operacionesPrueba()
	preflight.syncDir = func(*os.File) error { return fallo }
	if _, err := abrirRaizDurable(ancla, "no-creado", preflight); !errors.Is(err, fallo) || errors.Is(err, ErrPersistenciaIncierta) {
		t.Fatal(err)
	}
	if _, err := ancla.Stat("no-creado"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	op := operacionesPrueba()
	var llamadas int
	op.syncDir = func(*os.File) error {
		llamadas++
		if llamadas == 2 {
			return fallo
		}
		return nil
	}
	if _, err := abrirRaizDurable(ancla, "nuevo", op); !errors.Is(err, fallo) || !errors.Is(err, ErrPersistenciaIncierta) {
		t.Fatal(err)
	}
	if _, err := ancla.Stat("nuevo"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := abrirRaizDurable(ancla, "link", operacionesPrueba()); !errors.Is(err, ErrRaizDurableCambiada) {
		t.Fatal(err)
	}
}

func TestRaizDurableDetectaRutaRenombradaAntesDeOperacion(t *testing.T) {
	dir, ancla := abrirAnclaPrueba(t)
	r, err := abrirRaizDurable(ancla, "a/b", operacionesPrueba())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "retirada")); err != nil {
		t.Fatal(err)
	}
	if err := r.Escribir("data", []byte("privado")); !errors.Is(err, ErrRaizDurableCambiada) {
		t.Fatal(err)
	}
	if _, err := ancla.Stat("retirada/b/data"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	for _, nombre := range []string{"", ".", "../otro", "a/b", `a\b`} {
		if err := r.Escribir(nombre, []byte("x")); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("nombre %q: %v", nombre, err)
		}
	}
}

func TestRaizDurableDetectaSustitucionPorEnlace(t *testing.T) {
	dir, ancla := abrirAnclaPrueba(t)
	r, err := abrirRaizDurable(ancla, "a", operacionesPrueba())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "retirada")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("retirada", filepath.Join(dir, "a")); err != nil {
		t.Fatal(err)
	}
	if err := r.Escribir("data", []byte("privado")); !errors.Is(err, ErrRaizDurableCambiada) {
		t.Fatal(err)
	}
	if _, err := ancla.Stat("retirada/data"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestRaizDurableConservaTemporalSiRenameFalla(t *testing.T) {
	_, ancla := abrirAnclaPrueba(t)
	fallo := errors.New("rename fallo")
	op := operacionesPrueba()
	op.rename = func(*os.Root, string, string) error { return fallo }
	r, err := abrirRaizDurable(ancla, "dir", op)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Escribir("data", []byte("contenido")); !errors.Is(err, ErrPersistenciaIncierta) || !errors.Is(err, fallo) {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(ancla.FS(), "dir")
	if err != nil || len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "data.tmp-") {
		t.Fatal(entries, err)
	}
}

func TestRaizDurableUsaBarrerasReales(t *testing.T) {
	_, ancla := abrirAnclaPrueba(t)
	r, err := AbrirRaizDurable(ancla, "una/dos")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Escribir("record", []byte("cifrado")); err != nil {
		t.Fatal(err)
	}
	if err := r.Borrar("record"); err != nil {
		t.Fatal(err)
	}
}

func TestRaizDurableErroresDeCierreYCambioDeRutaTrasRename(t *testing.T) {
	dir, ancla := abrirAnclaPrueba(t)
	fallo := errors.New("fallo al cerrar")
	op := operacionesPrueba()
	var failFile, failDir bool
	var cerrados int
	op.closeFile = func(f *os.File) error {
		err := f.Close()
		if failFile {
			return errors.Join(err, fallo)
		}
		return err
	}
	op.closeDir = func(f *os.File) error {
		err := f.Close()
		cerrados++
		if failDir && cerrados == 2 {
			return errors.Join(err, fallo)
		}
		return err
	}
	r, err := abrirRaizDurable(ancla, "dir", op)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Escribir("data", []byte("anterior")); err != nil {
		t.Fatal(err)
	}
	failFile = true
	if err := r.Escribir("data", []byte("nuevo")); !errors.Is(err, fallo) || errors.Is(err, ErrPersistenciaIncierta) {
		t.Fatal(err)
	}
	got, err := ancla.ReadFile("dir/data")
	if err != nil || string(got) != "anterior" {
		t.Fatal(string(got), err)
	}
	failFile, failDir, cerrados = false, true, 0
	if err := r.Escribir("data", []byte("nuevo")); !errors.Is(err, fallo) || !errors.Is(err, ErrPersistenciaIncierta) {
		t.Fatal(err)
	}
	got, err = ancla.ReadFile("dir/data")
	if err != nil || string(got) != "nuevo" {
		t.Fatal(string(got), err)
	}
	failDir = false
	r.op.rename = func(root *os.Root, oldName, newName string) error {
		if err := root.Rename(oldName, newName); err != nil {
			return err
		}
		return os.Rename(filepath.Join(dir, "dir"), filepath.Join(dir, "movido"))
	}
	if err := r.Escribir("data", []byte("traslado")); !errors.Is(err, ErrRaizDurableCambiada) || !errors.Is(err, ErrPersistenciaIncierta) {
		t.Fatal(err)
	}
	got, err = ancla.ReadFile("movido/data")
	if err != nil || string(got) != "traslado" {
		t.Fatal(string(got), err)
	}
}

func TestRaizDurableRechazaRutaInvalidaSinCrearPadres(t *testing.T) {
	_, ancla := abrirAnclaPrueba(t)
	for _, ruta := range []string{"", "../outside", "a/../b", "a/b\x00", `/absolute`, strings.Repeat("a/", 64) + "a"} {
		if _, err := abrirRaizDurable(ancla, ruta, operacionesPrueba()); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("%q: %v", ruta, err)
		}
	}
	if _, err := ancla.Stat("a"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestRaizDurableNoOcultaErroresDeFsync(t *testing.T) {
	for _, fallo := range []error{syscall.EINVAL, syscall.ENOTSUP} {
		t.Run(fallo.Error(), func(t *testing.T) {
			_, ancla := abrirAnclaPrueba(t)
			op := operacionesPrueba()
			var llamadas int
			var fallar bool
			op.syncDir = func(*os.File) error {
				llamadas++
				if fallar && llamadas == 2 {
					return fallo
				}
				return nil
			}
			r, err := abrirRaizDurable(ancla, "dir", op)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			llamadas, fallar = 0, true
			if err := r.Escribir("data", []byte("valor")); !errors.Is(err, fallo) || !errors.Is(err, ErrPersistenciaIncierta) {
				t.Fatal(err)
			}
		})
	}
}
