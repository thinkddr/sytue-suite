// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
)

// arbolFalso es la carpeta elegida en el teléfono, en memoria: lo que contestaría
// ficheros.Arbol con el proveedor del almacenamiento.
type arbolFalso struct {
	mu      sync.Mutex
	nombre  string
	n       int
	docs    map[string]*docFalso
	vistos  []string // lo que se pidió enseñar: «<id> abrir|compartir|en <paquete> <tipo>»
	sinMove bool
	// apps son los paquetes de Sytue Office instalados (VerEn).
	apps map[string]bool
}

type docFalso struct {
	ficheros.DocumentoArbol
	padre string
	datos []byte
}

func nuevoArbolFalso(nombre string) *arbolFalso {
	a := &arbolFalso{nombre: nombre, docs: map[string]*docFalso{}}
	a.docs["r"] = &docFalso{DocumentoArbol: ficheros.DocumentoArbol{ID: "r", Nombre: nombre, Carpeta: true}}
	return a
}

func (a *arbolFalso) get(id string) (*docFalso, error) {
	if d, ok := a.docs[id]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("%w: %s", fs.ErrNotExist, id)
}

func (a *arbolFalso) Raiz() (ficheros.DocumentoArbol, error) { return a.Datos("r") }
func (a *arbolFalso) Datos(id string) (ficheros.DocumentoArbol, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, err := a.get(id)
	if err != nil {
		return ficheros.DocumentoArbol{}, err
	}
	out := d.DocumentoArbol
	out.Tamano = int64(len(d.datos))
	return out, nil
}

func (a *arbolFalso) Hijos(id string) ([]ficheros.DocumentoArbol, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []ficheros.DocumentoArbol
	for _, d := range a.docs {
		if d.padre == id && d.ID != "r" {
			x := d.DocumentoArbol
			x.Tamano = int64(len(d.datos))
			out = append(out, x)
		}
	}
	return out, nil
}

func (a *arbolFalso) Crear(padre, nombre string, carpeta bool) (ficheros.DocumentoArbol, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.n++
	d := &docFalso{DocumentoArbol: ficheros.DocumentoArbol{ID: fmt.Sprint("d", a.n), Nombre: nombre, Carpeta: carpeta}, padre: padre}
	a.docs[d.ID] = d
	return d.DocumentoArbol, nil
}

func (a *arbolFalso) Renombrar(id, nombre string) (ficheros.DocumentoArbol, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, err := a.get(id)
	if err != nil {
		return ficheros.DocumentoArbol{}, err
	}
	d.Nombre = nombre
	return d.DocumentoArbol, nil
}

func (a *arbolFalso) Mover(id, _, destino string) (ficheros.DocumentoArbol, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sinMove {
		return ficheros.DocumentoArbol{}, ficheros.ErrNoSoportado
	}
	d, err := a.get(id)
	if err != nil {
		return ficheros.DocumentoArbol{}, err
	}
	d.padre = destino
	return d.DocumentoArbol, nil
}

func (a *arbolFalso) Borrar(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.docs, id)
	return nil
}

func (a *arbolFalso) Leer(id string) (io.ReadCloser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, err := a.get(id)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(d.datos)), nil
}

type escritorFalso struct {
	a   *arbolFalso
	id  string
	buf bytes.Buffer
}

func (w *escritorFalso) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *escritorFalso) Close() error {
	w.a.mu.Lock()
	defer w.a.mu.Unlock()
	d, err := w.a.get(w.id)
	if err == nil {
		d.datos = bytes.Clone(w.buf.Bytes())
	}
	return err
}

func (a *arbolFalso) Escribir(id string) (io.WriteCloser, error) {
	return &escritorFalso{a: a, id: id}, nil
}
func (a *arbolFalso) Espacio() (int64, int64, error) { return 0, 0, ficheros.ErrNoDisponible }
func (a *arbolFalso) Ver(id string, abrir bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	que := "compartir"
	if abrir {
		que = "abrir"
	}
	a.vistos = append(a.vistos, id+" "+que)
	return nil
}

func (a *arbolFalso) VerEn(id, paquete, tipo string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.apps[paquete] {
		return false, nil
	}
	a.vistos = append(a.vistos, id+" en "+paquete+" "+tipo)
	return true, nil
}

// sistemaFalso es lo del sistema: qué permisos hay guardados y qué se suelta.
type sistemaFalso struct {
	arboles   map[string]*arbolFalso
	vigentes  map[string]bool
	soltados  []string
	elegido   ficheros.ArbolElegido
	errElegir error
}

func carpetaDePrueba(t *testing.T, s *sistemaFalso) (*carpetaDrive, string) {
	t.Helper()
	datos := t.TempDir()
	c := nuevaCarpetaDrive(datos, filepath.Join(datos, "Drive"))
	c.vigente = func(uri string) bool { return s.vigentes[uri] }
	c.soltar = func(uri string) { s.soltados = append(s.soltados, uri); delete(s.vigentes, uri) }
	c.abrir = func(uri string) proveedorArbol { return s.arboles[uri] }
	c.elegir = func() (ficheros.ArbolElegido, error) { return s.elegido, s.errElegir }
	return c, datos
}

func TestLaCarpetaElegidaSeUsaSeRecuerdaYSeSuelta(t *testing.T) {
	descargas, tarjeta := nuevoArbolFalso("Download"), nuevoArbolFalso("Fotos")
	s := &sistemaFalso{
		arboles:  map[string]*arbolFalso{"content://t/descargas": descargas, "content://t/tarjeta": tarjeta},
		vigentes: map[string]bool{},
	}
	c, datos := carpetaDePrueba(t, s)
	ctx := t.Context()

	// Sin nada elegido: la carpeta de la app.
	disco, perdida, err := c.abrirDisco(archivos.Opciones{Instalacion: "i"})
	if err != nil || perdida != "" || disco.Raiz() != filepath.Join(datos, "Drive") {
		t.Fatal(disco.Raiz(), perdida, err)
	}
	if _, ok := c.enArbol(filepath.Join(datos, "Drive", "x.txt")); ok {
		t.Fatal("la carpeta de la app no es un árbol")
	}

	// Elegir Descargas: Drive pasa a ella en el acto y la recuerda.
	s.vigentes["content://t/descargas"] = true
	recordada, err := c.usar(ficheros.ArbolElegido{URI: "content://t/descargas", Nombre: "Download", Persistente: true})
	if err != nil || !recordada {
		t.Fatal(recordada, err)
	}
	raiz, err := disco.Root(ctx)
	if err != nil || raiz.Name != "Download" || disco.Raiz() != "/content/Download" {
		t.Fatal("la raíz nueva", raiz, disco.Raiz(), err)
	}
	f, err := disco.Upload(ctx, raiz.ID, "plan.txt", "", strings.NewReader("plan"), 4)
	if err != nil {
		t.Fatal(err)
	}
	if got := leerCarpeta(datos); got.URI != "content://t/descargas" || got.Nombre != "Download" {
		t.Fatal("no se recuerda:", got)
	}
	// Abrir un fichero de ahí es dárselo a la app del sistema por su documento.
	ver, ok := c.enArbol(f.Path)
	if !ok {
		t.Fatal("un fichero del árbol no se reconoce:", f.Path)
	}
	if err := ver(true); err != nil {
		t.Fatal(err)
	}
	if len(descargas.vistos) != 1 || !strings.HasSuffix(descargas.vistos[0], " abrir") {
		t.Fatal(descargas.vistos)
	}
	if _, ok := c.enArbol("/content/Downloads/otro"); ok {
		t.Fatal("una ruta que solo empieza igual")
	}

	// Al volver a abrir Drive, sigue en Descargas.
	c2, _ := carpetaDePrueba(t, s)
	c2.datos, c2.app = datos, c.app
	disco2, perdida, err := c2.abrirDisco(archivos.Opciones{Instalacion: "i"})
	if err != nil || perdida != "" || disco2.Raiz() != "/content/Download" {
		t.Fatal("al volver a abrir", disco2.Raiz(), perdida, err)
	}
	if got := leer(t, disco2, f.ID); got != "plan" {
		t.Fatal(got)
	}

	// Elegir otra suelta el permiso de la de antes.
	s.vigentes["content://t/tarjeta"] = true
	if _, err := c2.usar(ficheros.ArbolElegido{URI: "content://t/tarjeta", Nombre: "Fotos", Persistente: true}); err != nil {
		t.Fatal(err)
	}
	if len(s.soltados) != 1 || s.soltados[0] != "content://t/descargas" {
		t.Fatal("no soltó la de antes:", s.soltados)
	}

	// Volver a la de la app suelta la elegida y la olvida.
	if err := c2.volver(); err != nil {
		t.Fatal(err)
	}
	if disco2.Raiz() != filepath.Join(datos, "Drive") || leerCarpeta(datos).URI != "" || c2.nombre() != "" {
		t.Fatal("no volvió a la de la app")
	}
	if s.soltados[len(s.soltados)-1] != "content://t/tarjeta" {
		t.Fatal(s.soltados)
	}
}

func TestSinPermisoDriveAbreLaDeLaAppYLoDice(t *testing.T) {
	s := &sistemaFalso{arboles: map[string]*arbolFalso{"content://t/x": nuevoArbolFalso("X")}, vigentes: map[string]bool{}}
	c, datos := carpetaDePrueba(t, s)
	if err := guardarCarpeta(datos, carpetaElegida{URI: "content://t/x", Nombre: "X"}); err != nil {
		t.Fatal(err)
	}
	disco, perdida, err := c.abrirDisco(archivos.Opciones{Instalacion: "i"})
	if err != nil || perdida != "X" || disco.Raiz() != filepath.Join(datos, "Drive") {
		t.Fatal(disco.Raiz(), perdida, err)
	}
	if leerCarpeta(datos).URI != "" {
		t.Fatal("sin permiso se sigue recordando")
	}

	// Con permiso pero sin poder abrirse (la tarjeta no está), se recuerda para la próxima.
	if err := guardarCarpeta(datos, carpetaElegida{URI: "content://t/fuera", Nombre: "Tarjeta"}); err != nil {
		t.Fatal(err)
	}
	s.vigentes["content://t/fuera"] = true
	s.arboles["content://t/fuera"] = &arbolFalso{nombre: "Tarjeta", docs: map[string]*docFalso{}} // sin raíz
	_, perdida, err = c.abrirDisco(archivos.Opciones{Instalacion: "i"})
	if err != nil || perdida != "Tarjeta" || leerCarpeta(datos).URI != "content://t/fuera" {
		t.Fatal(perdida, err, leerCarpeta(datos))
	}
}

func TestUnPermisoQueNoSeGuardaNoSeRecuerda(t *testing.T) {
	s := &sistemaFalso{arboles: map[string]*arbolFalso{"content://t/nube": nuevoArbolFalso("Nube")}, vigentes: map[string]bool{}}
	c, datos := carpetaDePrueba(t, s)
	if _, _, err := c.abrirDisco(archivos.Opciones{Instalacion: "i"}); err != nil {
		t.Fatal(err)
	}
	recordada, err := c.usar(ficheros.ArbolElegido{URI: "content://t/nube", Nombre: "Nube"})
	if err != nil || recordada || leerCarpeta(datos).URI != "" || c.nombre() != "Nube" {
		t.Fatal(recordada, err)
	}
}

func TestDocumentosTraduceLosErroresDelProveedor(t *testing.T) {
	a := nuevoArbolFalso("R")
	a.sinMove = true
	d := documentos{a}
	if _, err := d.Mover("r", "r", "r"); !errors.Is(err, archivos.ErrNoSoportado) {
		t.Fatal("mover sin saber:", err)
	}
	if _, err := d.Datos("nada"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if r, err := d.Raiz(); err != nil || r.Nombre != "R" || !r.Carpeta {
		t.Fatal(r, err)
	}
}

func TestLaRaizDeUnArbolNoTapaRutasDeVerdad(t *testing.T) {
	for nombre, quiero := range map[string]string{
		"Download": "/content/Download",
		"a/b":      "/content/a_b",
		"":         "/content/Drive",
		"..":       "/content/Drive",
	} {
		if got := raizDeArbol(nombre); got != quiero {
			t.Errorf("%q → %q, no %q", nombre, got, quiero)
		}
	}
	if _, err := os.Stat("/content"); err == nil {
		t.Skip("este equipo tiene /content")
	}
}

func TestLeerCarpetaIgnoraLoQueNoEsUnArbol(t *testing.T) {
	datos := t.TempDir()
	if err := os.WriteFile(filepath.Join(datos, ficheroCarpeta), []byte(`{"uri":"file:///etc","nombre":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := leerCarpeta(datos); got.URI != "" {
		t.Fatal(got)
	}
	if err := guardarCarpeta(datos, carpetaElegida{}); err != nil {
		t.Fatal(err)
	}
	if err := guardarCarpeta(datos, carpetaElegida{}); err != nil {
		t.Fatal("olvidar dos veces:", err)
	}
}

func leer(t *testing.T, d *archivos.Archivos, id string) string {
	t.Helper()
	rc, err := d.Download(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
