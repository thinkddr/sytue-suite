// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// ficherosTemporales escribe n ficheros en una carpeta temporal y devuelve sus rutas.
func ficherosTemporales(t *testing.T, n int) []string {
	t.Helper()
	dir := t.TempDir()
	rutas := make([]string, 0, n)
	for i := range n {
		ruta := filepath.Join(dir, "doc"+strconv.Itoa(i)+".txt")
		if err := os.WriteFile(ruta, []byte("hola"), 0o600); err != nil {
			t.Fatal(err)
		}
		rutas = append(rutas, ruta)
	}
	return rutas
}

// Soltar dos ficheros con un canal abierto pone dos fichas con su porcentaje y los sube
// al ámbito de ESE canal, como el `onDrop` del compositor de la web.
//
// VISTO EN ROJO (2026-11-09, saboteado a mano): con `Soltado` sin llamar a
// AdjuntarSoltados, la caja se queda sin fichas y no se sube nada.
func TestSoltarFicherosLosAdjuntaAlCanalAbierto(t *testing.T) {
	sa, _ := salaDePrueba(t, servicioConDos())
	fs := &ficherosFalso{}
	sa.Ficheros = fs
	sa.Arrancar(context.Background())
	var pendiente []func()
	sa.Lanzar = func(f func()) { pendiente = append(pendiente, f) }
	c := sa.P.Canal.Compositor

	rutas := ficherosTemporales(t, 2)
	// Lo que no es un fichero local (un enlace http) no se sube.
	uris := []fyne.URI{storage.NewFileURI(rutas[0]), storage.NewFileURI(rutas[1])}
	if web, err := storage.ParseURI("https://ejemplo.eu/a.txt"); err == nil {
		uris = append(uris, web)
	}
	// Ni una carpeta, que dejaría una ficha fallida.
	uris = append(uris, storage.NewFileURI(t.TempDir()))
	sa.Soltado(fyne.NewPos(10, 10), uris)
	if len(c.Pendientes) != 2 {
		t.Fatalf("soltar dos ficheros deja %d fichas", len(c.Pendientes))
	}
	for _, p := range c.Pendientes {
		if e := NuevaFichaPendiente(sa.P.T, p, nil).estado(); e != "0%" {
			t.Errorf("la ficha de %s dice %q mientras sube", p.Nombre, e)
		}
	}
	for _, f := range pendiente {
		f()
	}
	if len(fs.llamadas) != 2 || fs.llamadas[0] != "subir c1 doc0.txt text/plain; charset=utf-8 hola" {
		t.Errorf("se suben %q", fs.llamadas)
	}
	if len(c.Subidos()) != 2 {
		t.Errorf("subidos quedan %+v", c.Pendientes)
	}
}

// Soltar más de los que caben en un mensaje sube hasta el tope y lo dice.
func TestSoltarMasDeLaCuentaAvisa(t *testing.T) {
	sa, _ := salaDePrueba(t, servicioConDos())
	sa.Ficheros = &ficherosFalso{}
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor
	sa.AdjuntarSoltados(ficherosTemporales(t, MaxAdjuntos+1))
	if len(c.Pendientes) != MaxAdjuntos {
		t.Errorf("con %d soltados quedan %d fichas", MaxAdjuntos+1, len(c.Pendientes))
	}
	if want := sa.P.T.T("composer.maxAttachments", map[string]any{"count": MaxAdjuntos}); c.aviso != want {
		t.Errorf("el aviso dice %q y no %q", c.aviso, want)
	}
}

// Sin canal abierto, soltar no hace nada: ni fichas, ni subidas, ni velo.
func TestSoltarSinCanalNoHaceNada(t *testing.T) {
	sa, _ := salaDePrueba(t, servicioConDos())
	fs := &ficherosFalso{}
	sa.Ficheros = fs
	c := sa.P.Canal.Compositor
	sa.Arrastrando(true)
	if c.Arrastrando() {
		t.Error("sin canal, la caja ofrece soltar")
	}
	sa.AdjuntarSoltados(ficherosTemporales(t, 2))
	if len(c.Pendientes) != 0 || len(fs.llamadas) != 0 {
		t.Errorf("sin canal, soltar deja %+v y sube %q", c.Pendientes, fs.llamadas)
	}
	if sa.SoltarTexto("hola", fyne.NewPos(10, 700)) {
		t.Error("sin canal, el texto soltado entra en la caja")
	}
}

// Con ficheros encima, la caja dice «Suelta para adjuntar» por encima de todo; al
// soltarlos (o al salir), se va.
func TestArrastrarEnsenaElVeloDeSoltar(t *testing.T) {
	sa, _ := salaDePrueba(t, servicioConDos())
	sa.Ficheros = &ficherosFalso{}
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor
	sa.Arrastrando(true)
	if !c.Arrastrando() {
		t.Fatal("con ficheros encima no se ofrece soltar")
	}
	objs := c.objetos
	tx, ok := objs[len(objs)-1].(*canvas.Text)
	if !ok || tx.Text != sa.P.T.T("composer.dropToAttach", nil) {
		t.Fatalf("lo último que se pinta no es «Suelta para adjuntar»: %#v", objs[len(objs)-1])
	}
	if velo := objs[len(objs)-3]; velo.Position().X != tema.Espacio(1) || velo.Size().Width != c.Size().Width-2*tema.Espacio(1) {
		t.Errorf("el velo no va a `inset-1`: %v %v en %v", velo.Position(), velo.Size(), c.Size())
	}
	sa.Arrastrando(false)
	if c.Arrastrando() || len(c.objVelo) != 0 {
		t.Error("al salir el arrastre, el velo se queda")
	}
	sa.Arrastrando(true)
	sa.AdjuntarSoltados(nil)
	if c.Arrastrando() {
		t.Error("al soltar, el velo se queda")
	}
	// Con la caja apagada (sin conexión o sin permiso), no se ofrece.
	c.Inerte = true
	sa.Arrastrando(true)
	if c.Arrastrando() {
		t.Error("con la caja apagada se ofrece soltar")
	}
}

// El texto o el enlace soltado sobre el compositor entra en la caja; fuera, no.
func TestSoltarTextoEnLaCaja(t *testing.T) {
	sa, _ := salaDePrueba(t, servicioConDos())
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor
	d := fyne.CurrentApp().Driver()
	a := d.AbsolutePositionForObject(c.area)
	if !sa.SoltarTexto("https://sytue.eu", a.AddXY(5, 5)) || c.Texto() != "https://sytue.eu" {
		t.Fatalf("soltado en el área queda %q", c.Texto())
	}
	// En la franja pero fuera del área: al cursor.
	franja := d.AbsolutePositionForObject(c)
	if !sa.SoltarTexto(" hola", franja.AddXY(2, 2)) || c.Texto() != "https://sytue.eu hola" {
		t.Errorf("soltado en la franja queda %q", c.Texto())
	}
	if sa.SoltarTexto("x", fyne.NewPos(franja.X, franja.Y-20)) || c.Texto() != "https://sytue.eu hola" {
		t.Errorf("soltado fuera del compositor, entra: %q", c.Texto())
	}
}
