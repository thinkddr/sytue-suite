// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// El reflejo de «Guardar como…» en el móvil, sin móvil: la copia de trabajo llega al
// destino cuando la app la escribe y cada vez que cambia, y deja de hacerlo cuando el
// destino ya no vale.

type destinoFalso struct {
	mu     sync.Mutex
	copias []string
	err    error
}

func (d *destinoFalso) copiar(ruta, _ string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	b, err := os.ReadFile(ruta)
	if err != nil {
		return err
	}
	d.copias = append(d.copias, string(b))
	return nil
}

func (d *destinoFalso) vistas() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.copias...)
}

func esperarQue(t *testing.T, que string, f func() bool) {
	t.Helper()
	limite := time.Now().Add(3 * time.Second)
	for time.Now().Before(limite) {
		if f() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(que)
}

// reflejosDePrueba miran cada 5 ms y se paran al acabar la prueba.
func reflejosDePrueba(t *testing.T, base string) *reflejos {
	r := nuevosReflejos(base)
	r.paso, r.espera = 5*time.Millisecond, time.Second
	t.Cleanup(func() {
		r.mu.Lock()
		r.vivos = map[string]string{}
		r.fichero = ""
		r.mu.Unlock()
	})
	return r
}

// escribir como la biblioteca: un temporal y un renombrado, con la fecha de ahora.
func escribir(t *testing.T, ruta, texto string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		t.Fatal(err)
	}
	tmp := ruta + ".tmp"
	if err := os.WriteFile(tmp, []byte(texto), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, ruta); err != nil {
		t.Fatal(err)
	}
	ahora := time.Now().Add(time.Duration(len(texto)) * time.Millisecond)
	_ = os.Chtimes(ruta, ahora, ahora)
}

func TestElReflejoLlevaCadaGuardarAlDestino(t *testing.T) {
	base := t.TempDir()
	r := reflejosDePrueba(t, base)
	ruta := copiaDeTrabajo(base, "Informe", "content://proveedor/doc/1", []string{"docx"})
	if filepath.Base(ruta) != "Informe.docx" || !strings.HasPrefix(ruta, carpetaGuardados(base)) {
		t.Fatalf("copia de trabajo: %s", ruta)
	}
	d := &destinoFalso{}
	r.vigilar(ruta, "content://proveedor/doc/1", true, d.copiar)
	escribir(t, ruta, "uno")
	esperarQue(t, "la primera copia no llega", func() bool { return len(d.vistas()) == 1 })
	escribir(t, ruta, "dos dos")
	esperarQue(t, "el segundo Guardar no llega", func() bool { v := d.vistas(); return len(v) == 2 && v[1] == "dos dos" })
	time.Sleep(50 * time.Millisecond)
	if n := len(d.vistas()); n != 2 {
		t.Fatalf("sin cambios no se copia otra vez: %d copias", n)
	}
	// Recordado para el próximo arranque.
	if m := nuevosReflejos(base).recordados(); m[ruta] != "content://proveedor/doc/1" {
		t.Fatalf("recordados: %v", m)
	}
}

func TestUnDestinoQueYaNoValeAcabaElReflejo(t *testing.T) {
	base := t.TempDir()
	r := reflejosDePrueba(t, base)
	ruta := copiaDeTrabajo(base, "x.md", "content://a", nil)
	d := &destinoFalso{err: errors.New("permiso revocado")}
	r.vigilar(ruta, "content://a", true, d.copiar)
	escribir(t, ruta, "hola")
	esperarQue(t, "el reflejo sigue vivo", func() bool { _, ok := r.destino(ruta); return !ok })
	if m := nuevosReflejos(base).recordados(); len(m) != 0 {
		t.Fatalf("un reflejo acabado no se recuerda: %v", m)
	}
}

func TestReintentarNoDaPorLlevadoElCambio(t *testing.T) {
	base := t.TempDir()
	r := reflejosDePrueba(t, base)
	ruta := copiaDeTrabajo(base, "x.md", "content://b", nil)
	d := &destinoFalso{err: errReintentar}
	r.vigilar(ruta, "content://b", false, d.copiar)
	escribir(t, ruta, "hola")
	time.Sleep(30 * time.Millisecond)
	d.mu.Lock()
	d.err = nil
	d.mu.Unlock()
	esperarQue(t, "tras reintentar no se copió", func() bool { return len(d.vistas()) == 1 })
}

// Al arrancar, lo que ya está en la copia se toma por llevado: abrir la app no vuelve a
// subir cada documento a su nube.
func TestRestaurarNoVuelveACopiarLoQueYaEstaba(t *testing.T) {
	base := t.TempDir()
	ruta := copiaDeTrabajo(base, "x.md", "content://c", nil)
	escribir(t, ruta, "ya llevado")
	r := reflejosDePrueba(t, base)
	d := &destinoFalso{}
	r.vigilar(ruta, "content://c", true, d.copiar)
	time.Sleep(40 * time.Millisecond)
	if n := len(d.vistas()); n != 0 {
		t.Fatalf("al restaurar se copió %d veces", n)
	}
	escribir(t, ruta, "cambio nuevo")
	esperarQue(t, "el cambio tras restaurar no llega", func() bool { return len(d.vistas()) == 1 })
}

func TestSinEscribirNuncaElReflejoSeOlvida(t *testing.T) {
	base := t.TempDir()
	r := reflejosDePrueba(t, base)
	r.espera = 20 * time.Millisecond
	ruta := copiaDeTrabajo(base, "x.md", "content://d", nil)
	r.vigilar(ruta, "content://d", true, (&destinoFalso{}).copiar)
	esperarQue(t, "un reflejo sin copia no caduca", func() bool { _, ok := r.destino(ruta); return !ok })
}

func TestDosDestinosConElMismoNombreNoSePisan(t *testing.T) {
	base := t.TempDir()
	a := copiaDeTrabajo(base, "Acta.docx", "content://x/1", []string{"docx"})
	b := copiaDeTrabajo(base, "Acta.docx", "content://x/2", []string{"docx"})
	if a == b || filepath.Base(a) != "Acta.docx" {
		t.Fatalf("%s %s", a, b)
	}
	if c := copiaDeTrabajo(base, "../../fuera", "content://x/3", []string{"md"}); !strings.HasPrefix(c, carpetaGuardados(base)) {
		t.Fatalf("un nombre con barras sale de la carpeta: %s", c)
	}
	if tipoMovil("a.DOCX") == "application/octet-stream" || tipoMovil("a.docus") != "application/octet-stream" {
		t.Fatal("tipos MIME del selector")
	}
}
