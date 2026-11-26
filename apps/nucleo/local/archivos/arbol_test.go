// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// sobreArbol, puesto, hace que TODAS las pruebas del paquete abran su raíz sobre un
// [Arbol] (con [docsDisco], un proveedor como el del almacenamiento del teléfono) en vez
// de sobre el disco: así la lógica de siempre se prueba también por el camino de Android.
const sobreArbol = "SYTUE_ARCHIVOS_SOBRE_ARBOL"

func TestMain(m *testing.M) {
	if os.Getenv(sobreArbol) != "" {
		porDefecto = func(raiz string) Sistema {
			t := NuevoArbol(raiz, &docsDisco{dir: raiz})
			// Las pruebas cambian el disco por fuera y lo miran en el acto.
			t.Frescura = 0
			return t
		}
	}
	os.Exit(m.Run())
}

// TestLasPruebasDeSiempreSobreUnArbolDeDocumentos vuelve a pasar el paquete entero con
// la raíz en un árbol de documentos.
func TestLasPruebasDeSiempreSobreUnArbolDeDocumentos(t *testing.T) {
	if os.Getenv(sobreArbol) != "" {
		t.Skip("ya es la pasada sobre el árbol")
	}
	if testing.Short() {
		t.Skip("-short")
	}
	cmd := exec.Command(os.Args[0], "-test.count=1", "-test.timeout=5m") //nolint:gosec // el propio binario de la prueba
	cmd.Env = append(os.Environ(), sobreArbol+"=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sobre el árbol de documentos: %v\n%s", err, out)
	}
}

// ----- Un proveedor de documentos sobre una carpeta del disco -----

// docsDisco es un proveedor como el ExternalStorageProvider de Android: los
// identificadores salen de la ruta (y cambian con ella), Crear y Renombrar buscan otro
// nombre si el pedido ya está, y los caracteres que FAT no admite se cambian por `_`.
type docsDisco struct {
	dir      string
	sinMover bool
}

func (d *docsDisco) ruta(id string) (string, error) {
	rel, ok := strings.CutPrefix(id, "d:")
	if !ok {
		return "", fs.ErrInvalid
	}
	return filepath.Join(d.dir, filepath.FromSlash(rel)), nil
}

func (d *docsDisco) id(r string) string {
	rel, _ := filepath.Rel(d.dir, r)
	if rel == "." {
		rel = ""
	}
	return "d:" + filepath.ToSlash(rel)
}

func (d *docsDisco) doc(r string) (Documento, error) {
	fi, err := os.Stat(r)
	if err != nil {
		return Documento{}, err
	}
	return Documento{ID: d.id(r), Nombre: fi.Name(), Carpeta: fi.IsDir(), Tamano: fi.Size(), Modificado: fi.ModTime()}, nil
}

func (d *docsDisco) Raiz() (Documento, error) {
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return Documento{}, err
	}
	return d.doc(d.dir)
}

func (d *docsDisco) Datos(id string) (Documento, error) {
	r, err := d.ruta(id)
	if err != nil {
		return Documento{}, err
	}
	return d.doc(r)
}

func (d *docsDisco) Hijos(id string) ([]Documento, error) {
	r, err := d.ruta(id)
	if err != nil {
		return nil, err
	}
	es, err := os.ReadDir(r)
	if err != nil {
		return nil, err
	}
	var out []Documento
	for _, e := range es {
		if doc, err := d.doc(filepath.Join(r, e.Name())); err == nil {
			out = append(out, doc)
		}
	}
	return out, nil
}

func fat(nombre string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`"*:<>?|`, r) {
			return '_'
		}
		return r
	}, nombre)
}

// unico es buildUniqueFile de Android: «nombre (1).ext», «nombre (2).ext», etc.
func unico(existe func(string) bool, nombre string) string {
	if !existe(nombre) {
		return nombre
	}
	ext := path.Ext(nombre)
	base := strings.TrimSuffix(nombre, ext)
	for n := 1; ; n++ {
		if c := base + " (" + strconv.Itoa(n) + ")" + ext; !existe(c) {
			return c
		}
	}
}

func (d *docsDisco) Crear(padre, nombre string, carpeta bool) (Documento, error) {
	p, err := d.ruta(padre)
	if err != nil {
		return Documento{}, err
	}
	nombre = unico(func(n string) bool { _, err := os.Lstat(filepath.Join(p, n)); return err == nil }, fat(nombre))
	r := filepath.Join(p, nombre)
	if carpeta {
		err = os.Mkdir(r, 0o755)
	} else {
		err = os.WriteFile(r, nil, 0o644)
	}
	if err != nil {
		return Documento{}, err
	}
	return d.doc(r)
}

func (d *docsDisco) Renombrar(id, nombre string) (Documento, error) {
	r, err := d.ruta(id)
	if err != nil {
		return Documento{}, err
	}
	nombre = unico(func(n string) bool { _, err := os.Lstat(filepath.Join(filepath.Dir(r), n)); return err == nil }, fat(nombre))
	nueva := filepath.Join(filepath.Dir(r), nombre)
	if err := os.Rename(r, nueva); err != nil {
		return Documento{}, err
	}
	return d.doc(nueva)
}

func (d *docsDisco) Mover(id, origen, destino string) (Documento, error) {
	if d.sinMover {
		return Documento{}, ErrNoSoportado
	}
	r, err := d.ruta(id)
	if err != nil {
		return Documento{}, err
	}
	if o, _ := d.ruta(origen); o != filepath.Dir(r) {
		return Documento{}, errors.New("no cuelga de esa carpeta")
	}
	dst, err := d.ruta(destino)
	if err != nil {
		return Documento{}, err
	}
	nueva := filepath.Join(dst, filepath.Base(r))
	if _, err := os.Lstat(nueva); err == nil {
		return Documento{}, errors.New("ya existe en el destino")
	}
	if err := os.Rename(r, nueva); err != nil {
		return Documento{}, err
	}
	return d.doc(nueva)
}

func (d *docsDisco) Borrar(id string) error {
	r, err := d.ruta(id)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(r); err != nil {
		return err
	}
	return os.RemoveAll(r)
}

func (d *docsDisco) Leer(id string) (io.ReadCloser, error) {
	r, err := d.ruta(id)
	if err != nil {
		return nil, err
	}
	return os.Open(r)
}

func (d *docsDisco) Escribir(id string) (io.WriteCloser, error) {
	r, err := d.ruta(id)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(r, os.O_WRONLY|os.O_TRUNC, 0)
}

func (d *docsDisco) Espacio() (int64, int64, error) { return espacioDisco(d.dir) }

// ----- Un proveedor en memoria, con identificadores opacos -----

type docMem struct {
	Documento
	padre string
	datos []byte
}

// docsMemoria es un proveedor en memoria cuyos identificadores no dicen nada de la ruta
// (como uno de nube), con las mismas manías que docsDisco.
type docsMemoria struct {
	mu       sync.Mutex
	n        int
	docs     map[string]*docMem
	sinMover bool
	hijos    int // veces que se ha listado una carpeta
	ahora    time.Time
}

func nuevaMemoria() *docsMemoria {
	m := &docsMemoria{docs: map[string]*docMem{}, ahora: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)}
	m.docs["raiz"] = &docMem{Documento: Documento{ID: "raiz", Nombre: "Descargas", Carpeta: true, Modificado: m.ahora}}
	return m
}

func (m *docsMemoria) nuevoID() string {
	m.n++
	return fmt.Sprintf("doc-%04d", m.n)
}

func (m *docsMemoria) get(id string) (*docMem, error) {
	d, ok := m.docs[id]
	if !ok {
		return nil, fmt.Errorf("%s: %w", id, fs.ErrNotExist)
	}
	return d, nil
}

func (m *docsMemoria) existe(padre, nombre, salvo string) bool {
	for _, d := range m.docs {
		if d.padre == padre && d.Nombre == nombre && d.ID != salvo && d.ID != "raiz" {
			return true
		}
	}
	return false
}

func (m *docsMemoria) Raiz() (Documento, error) { return m.Datos("raiz") }

func (m *docsMemoria) Datos(id string) (Documento, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, err := m.get(id)
	if err != nil {
		return Documento{}, err
	}
	out := d.Documento
	if !d.Carpeta {
		out.Tamano = int64(len(d.datos))
	}
	return out, nil
}

func (m *docsMemoria) Hijos(id string) ([]Documento, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hijos++
	p, err := m.get(id)
	if err != nil {
		return nil, err
	}
	if !p.Carpeta {
		return nil, errors.New("no es una carpeta")
	}
	var out []Documento
	for _, d := range m.docs {
		if d.padre == id && d.ID != "raiz" {
			c := d.Documento
			c.Tamano = int64(len(d.datos))
			out = append(out, c)
		}
	}
	// El orden de un proveedor no es el de los nombres.
	slices.SortFunc(out, func(x, y Documento) int { return strings.Compare(y.ID, x.ID) })
	return out, nil
}

func (m *docsMemoria) Crear(padre, nombre string, carpeta bool) (Documento, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.get(padre); err != nil {
		return Documento{}, err
	}
	nombre = unico(func(n string) bool { return m.existe(padre, n, "") }, fat(nombre))
	d := &docMem{Documento: Documento{ID: m.nuevoID(), Nombre: nombre, Carpeta: carpeta, Modificado: m.ahora}, padre: padre}
	m.docs[d.ID] = d
	return d.Documento, nil
}

func (m *docsMemoria) Renombrar(id, nombre string) (Documento, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, err := m.get(id)
	if err != nil {
		return Documento{}, err
	}
	d.Nombre = unico(func(n string) bool { return m.existe(d.padre, n, id) }, fat(nombre))
	// Como en el teléfono, el identificador cambia con el nombre.
	delete(m.docs, id)
	d.ID = m.nuevoID()
	m.docs[d.ID] = d
	for _, h := range m.docs {
		if h.padre == id {
			h.padre = d.ID
		}
	}
	return d.Documento, nil
}

func (m *docsMemoria) Mover(id, origen, destino string) (Documento, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sinMover {
		return Documento{}, ErrNoSoportado
	}
	d, err := m.get(id)
	if err != nil {
		return Documento{}, err
	}
	if d.padre != origen {
		return Documento{}, errors.New("no cuelga de esa carpeta")
	}
	if m.existe(destino, d.Nombre, id) {
		return Documento{}, errors.New("ya existe en el destino")
	}
	d.padre = destino
	return d.Documento, nil
}

func (m *docsMemoria) Borrar(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.get(id); err != nil {
		return err
	}
	m.borrar(id)
	return nil
}

func (m *docsMemoria) borrar(id string) {
	for _, h := range m.docs {
		if h.padre == id && h.ID != "raiz" {
			m.borrar(h.ID)
		}
	}
	delete(m.docs, id)
}

func (m *docsMemoria) Leer(id string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, err := m.get(id)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(slices.Clone(d.datos))), nil
}

// escritorMem es una tubería: no tiene Sync y lo escrito se ve al cerrar.
type escritorMem struct {
	m   *docsMemoria
	id  string
	buf bytes.Buffer
}

func (w *escritorMem) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *escritorMem) Close() error {
	w.m.mu.Lock()
	defer w.m.mu.Unlock()
	d, err := w.m.get(w.id)
	if err != nil {
		return err
	}
	d.datos = slices.Clone(w.buf.Bytes())
	return nil
}

func (m *docsMemoria) Escribir(id string) (io.WriteCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, err := m.get(id)
	if err != nil {
		return nil, err
	}
	d.datos = nil
	return &escritorMem{m: m, id: id}, nil
}

func (m *docsMemoria) Espacio() (int64, int64, error) { return 1000, 400, nil }

// nombres son los hijos de una carpeta del proveedor, por nombre.
func (m *docsMemoria) nombres(t *testing.T, padre string) []string {
	t.Helper()
	hs, err := m.Hijos(padre)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(hs))
	for _, h := range hs {
		out = append(out, h.Nombre)
	}
	slices.Sort(out)
	return out
}

// ----- Pruebas del árbol -----

const raizVirtual = "/sytue-arbol/prueba"

func escribirEn(t *testing.T, s Sistema, r, texto string) {
	t.Helper()
	if err := s.EscribirAtomico(r, []byte(texto)); err != nil {
		t.Fatal(r, err)
	}
}

func leerDe(t *testing.T, s Sistema, r string) string {
	t.Helper()
	b, err := s.ReadFile(r)
	if err != nil {
		t.Fatal(r, err)
	}
	return string(b)
}

func TestArbolComoSistemaDeFicheros(t *testing.T) {
	m := nuevaMemoria()
	s := NuevoArbol(raizVirtual, m)
	r := func(p string) string { return filepath.Join(raizVirtual, filepath.FromSlash(p)) }

	if fi, err := s.Stat(raizVirtual); err != nil || !fi.IsDir() || fi.Name() != "Descargas" {
		t.Fatal("la raíz", fi, err)
	}
	if err := s.MkdirAll(r("a/b/c"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Mkdir(r("a"), 0o755); !errors.Is(err, fs.ErrExist) {
		t.Fatal("Mkdir de lo que ya está:", err)
	}
	escribirEn(t, s, r("a/b/uno.txt"), "uno")
	escribirEn(t, s, r("a/b/dos.txt"), "dos")
	if got := leerDe(t, s, r("a/b/uno.txt")); got != "uno" {
		t.Fatal(got)
	}
	es, err := s.ReadDir(r("a/b"))
	if err != nil {
		t.Fatal(err)
	}
	nombres := make([]string, 0, len(es))
	for _, e := range es {
		nombres = append(nombres, e.Name())
	}
	if !slices.Equal(nombres, []string{"c", "dos.txt", "uno.txt"}) {
		t.Fatal("ReadDir en orden y sin temporales:", nombres)
	}
	if fi, err := s.Stat(r("a/b/uno.txt")); err != nil || fi.Size() != 3 || fi.IsDir() || !fi.Mode().IsRegular() {
		t.Fatal("Stat", fi, err)
	}
	if _, err := s.Stat(r("a/nada")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("lo que no está:", err)
	}

	// Rename sustituye un fichero del destino, como os.Rename.
	if err := s.Rename(r("a/b/dos.txt"), r("a/b/uno.txt")); err != nil {
		t.Fatal(err)
	}
	if got := leerDe(t, s, r("a/b/uno.txt")); got != "dos" {
		t.Fatal("no sustituyó:", got)
	}
	if got := m.nombres(t, mustID(t, s, r("a/b"))); !slices.Equal(got, []string{"c", "uno.txt"}) {
		t.Fatal(got)
	}
	// A otra carpeta y con otro nombre, también cuando el nombre de antes ya está allí.
	escribirEn(t, s, r("a/x.txt"), "estorbo")
	escribirEn(t, s, r("a/b/x.txt"), "x")
	if err := s.Rename(r("a/b/x.txt"), r("a/y.txt")); err != nil {
		t.Fatal(err)
	}
	if leerDe(t, s, r("a/y.txt")) != "x" || leerDe(t, s, r("a/x.txt")) != "estorbo" {
		t.Fatal("mover y renombrar a la vez")
	}
	// Y cuando además el nombre de después ya está en el origen.
	escribirEn(t, s, r("a/b/y.txt"), "otro y")
	escribirEn(t, s, r("a/b/x.txt"), "x2")
	if err := s.Rename(r("a/b/x.txt"), r("a/c/y.txt")); err == nil {
		t.Fatal("a/c no existe y se movió")
	}
	if err := s.Rename(r("a/x.txt"), r("a/b/y2.txt")); err != nil {
		t.Fatal(err)
	}
	if got := m.nombres(t, mustID(t, s, r("a/b"))); !slices.Equal(got, []string{"c", "uno.txt", "x.txt", "y.txt", "y2.txt"}) {
		t.Fatal(got)
	}
	// El nombre de antes está en el destino y el de después en el origen: por uno de paso.
	escribirEn(t, s, r("a/p"), "p")
	escribirEn(t, s, r("a/q"), "q")
	escribirEn(t, s, r("a/b/p"), "otra p")
	if err := s.Rename(r("a/p"), r("a/b/q")); err != nil {
		t.Fatal(err)
	}
	if leerDe(t, s, r("a/b/q")) != "p" || leerDe(t, s, r("a/b/p")) != "otra p" || leerDe(t, s, r("a/q")) != "q" {
		t.Fatal("con nombre de paso")
	}
	if got := m.nombres(t, mustID(t, s, r("a"))); !slices.Equal(got, []string{"b", "q", "y.txt"}) {
		t.Fatal("quedó un nombre de paso:", got)
	}
	// Una carpeta con lo que lleva: lo de dentro se vuelve a encontrar con su nueva ruta.
	if err := s.Rename(r("a/b"), r("b")); err != nil {
		t.Fatal(err)
	}
	if got := leerDe(t, s, r("b/uno.txt")); got != "dos" {
		t.Fatal(got)
	}
	if _, err := s.Stat(r("a/b/uno.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("la ruta vieja sigue:", err)
	}
	if err := s.Rename(r("b"), r("b/c/b")); err == nil {
		t.Fatal("una carpeta dentro de sí misma")
	}

	// Remove no borra una carpeta con cosas; RemoveAll sí, y lo que no está no es error.
	if err := s.Remove(r("b")); err == nil {
		t.Fatal("Remove de una carpeta llena")
	}
	if err := s.RemoveAll(r("b")); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveAll(r("b")); err != nil {
		t.Fatal("RemoveAll de lo que no está:", err)
	}
	if err := s.RemoveAll(raizVirtual); err == nil {
		t.Fatal("se puede borrar la carpeta de la persona")
	}
	if total, libre, err := s.Espacio(raizVirtual); err != nil || total != 1000 || libre != 400 {
		t.Fatal("Espacio", total, libre, err)
	}

	// WalkDir en orden, con SkipDir.
	escribirEn(t, s, r("w/.oculta/z"), "z")
	escribirEn(t, s, r("w/a.txt"), "a")
	escribirEn(t, s, r("w/b/c.txt"), "c")
	var vistos []string
	err = s.WalkDir(r("w"), func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && strings.HasPrefix(e.Name(), ".") {
			return fs.SkipDir
		}
		rel, _ := filepath.Rel(raizVirtual, p)
		vistos = append(vistos, filepath.ToSlash(rel))
		return nil
	})
	if err != nil || !slices.Equal(vistos, []string{"w", "w/a.txt", "w/b", "w/b/c.txt"}) {
		t.Fatal("WalkDir", vistos, err)
	}

	// Lo que no cuelga de la raíz va al disco.
	fuera := filepath.Join(t.TempDir(), "suelto.txt")
	escribirEn(t, s, fuera, "del disco")
	if b, err := os.ReadFile(fuera); err != nil || string(b) != "del disco" {
		t.Fatal("fuera de la raíz", err)
	}
	if err := s.Rename(fuera, r("suelto.txt")); err == nil {
		t.Fatal("renombrar entre el disco y el árbol")
	}
}

func mustID(t *testing.T, s *Arbol, r string) string {
	t.Helper()
	id, err := s.Documento(r)
	if err != nil {
		t.Fatal(r, err)
	}
	return id
}

func TestArbolNoDejaNombresQueElProveedorCambia(t *testing.T) {
	m := nuevaMemoria()
	s := NuevoArbol(raizVirtual, m)
	if err := s.Mkdir(filepath.Join(raizVirtual, "a:b"), 0o755); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal("Mkdir con «:»", err)
	}
	escribirEn(t, s, filepath.Join(raizVirtual, "bien.txt"), "x")
	if err := s.Rename(filepath.Join(raizVirtual, "bien.txt"), filepath.Join(raizVirtual, "mal?.txt")); err == nil {
		t.Fatal("renombró a un nombre que el proveedor cambia")
	}
	if got := m.nombres(t, "raiz"); !slices.Equal(got, []string{"bien.txt"}) {
		t.Fatal("quedó algo a medias:", got)
	}
	if got := leerDe(t, s, filepath.Join(raizVirtual, "bien.txt")); got != "x" {
		t.Fatal(got)
	}
}

func TestArbolMueveCopiandoSiElProveedorNoSabe(t *testing.T) {
	m := nuevaMemoria()
	m.sinMover = true
	s := NuevoArbol(raizVirtual, m)
	r := func(p string) string { return filepath.Join(raizVirtual, filepath.FromSlash(p)) }
	escribirEn(t, s, r("de/x.txt"), "x")
	escribirEn(t, s, r("de/sub/y.txt"), "y")
	if err := s.MkdirAll(r("a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename(r("de/x.txt"), r("a/x2.txt")); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename(r("de/sub"), r("a/sub")); err != nil {
		t.Fatal(err)
	}
	if leerDe(t, s, r("a/x2.txt")) != "x" || leerDe(t, s, r("a/sub/y.txt")) != "y" {
		t.Fatal("no se copió")
	}
	if got := m.nombres(t, mustID(t, s, r("de"))); len(got) != 0 {
		t.Fatal("no se borró el origen:", got)
	}
}

func TestArbolNoPreguntaDeMasYVeLoQueCambiaPorFuera(t *testing.T) {
	m := nuevaMemoria()
	s := NuevoArbol(raizVirtual, m)
	reloj := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	s.ahora = func() time.Time { return reloj }
	r := func(p string) string { return filepath.Join(raizVirtual, filepath.FromSlash(p)) }
	escribirEn(t, s, r("v/uno"), "1")
	if _, err := s.ReadDir(r("v")); err != nil {
		t.Fatal(err)
	}
	antes := m.hijos
	for i := range 50 {
		if _, err := s.Stat(r("v/no-esta-" + strconv.Itoa(i))); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
	}
	if m.hijos != antes {
		t.Fatalf("buscar lo que no está listó %d veces la carpeta recién listada", m.hijos-antes)
	}
	// Otra app crea algo: se ve cuando el listado deja de estar fresco.
	vid := mustID(t, s, r("v"))
	if _, err := m.Crear(vid, "nuevo", false); err != nil {
		t.Fatal(err)
	}
	reloj = reloj.Add(FrescuraListado)
	if _, err := s.Stat(r("v/nuevo")); err != nil {
		t.Fatal("lo creado por fuera no se ve:", err)
	}
	// Otra app borra algo que ya se conocía: deja de estar.
	m.mu.Lock()
	for id, d := range m.docs {
		if d.Nombre == "uno" {
			delete(m.docs, id)
		}
	}
	m.mu.Unlock()
	reloj = reloj.Add(FrescuraListado)
	if _, err := s.Stat(r("v/uno")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("lo borrado por fuera sigue:", err)
	}
}

// TestArchivosSobreUnArbolEnMemoria es Drive entero sobre un proveedor de identificadores
// opacos: subir, listar, renombrar, mover, versiones, papelera, copiar, buscar y la cuota.
func TestArchivosSobreUnArbolEnMemoria(t *testing.T) {
	m := nuevaMemoria()
	a, err := Abrir(raizVirtual, Opciones{Instalacion: "inst-1", MaxVersiones: 3, Sistema: NuevoArbol(raizVirtual, m)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	raiz, err := a.Root(ctx)
	if err != nil || raiz.Name != "Descargas" {
		t.Fatal("Root", raiz, err)
	}
	docs, err := a.CreateFolder(ctx, raiz.ID, "Docs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateFolder(ctx, raiz.ID, "Docs"); !errors.Is(err, ErrExiste) {
		t.Fatal("carpeta repetida:", err)
	}
	f, err := a.Upload(ctx, docs.ID, "plan.txt", "text/plain", strings.NewReader("v1"), 2)
	if err != nil {
		t.Fatal(err)
	}
	if f, err = a.ReplaceFile(ctx, f, "text/plain", strings.NewReader("v2"), 2); err != nil {
		t.Fatal(err)
	}
	vs, _, err := a.Versions(ctx, f.ID, "")
	if err != nil || len(vs) != 2 || vs[0].Version != 2 {
		t.Fatal("versiones", vs, err)
	}
	if got := leer(a.DownloadVersion(ctx, f.ID, 1))(t); got != "v1" {
		t.Fatal(got)
	}
	hijos, err := a.Children(ctx, raiz.ID, "")
	if err != nil || len(hijos.Folders) != 1 || len(hijos.Files) != 0 {
		t.Fatal("la raíz enseña .sytue o le falta algo:", hijos, err)
	}
	if err := a.Rename(ctx, api.DriveFile, f.ID, "plan final.txt"); err != nil {
		t.Fatal(err)
	}
	if err := a.Move(ctx, api.DriveFile, f.ID, raiz.ID); err != nil {
		t.Fatal(err)
	}
	movido, err := a.File(ctx, f.ID) // el identificador viejo sigue valiendo
	if err != nil || movido.Name != "plan final.txt" || movido.Version != 2 {
		t.Fatal("tras renombrar y mover:", movido, err)
	}
	copia, err := a.CopyFolder(ctx, docs.ID)
	if err != nil || copia.Name != "Docs (2)" {
		t.Fatal("CopyFolder", copia, err)
	}
	if err := a.Delete(ctx, api.DriveFile, movido.ID); err != nil {
		t.Fatal(err)
	}
	papelera, err := a.Trash(ctx)
	if err != nil || len(papelera.Files) != 1 {
		t.Fatal("papelera", papelera, err)
	}
	if err := a.Restore(ctx, api.DriveFile, papelera.Files[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := leer(a.Download(ctx, movido.ID))(t); got != "v2" {
		t.Fatal(got)
	}
	encontrados, err := a.Search(ctx, "plan")
	if err != nil || len(encontrados.Files) != 1 {
		t.Fatal("Search", encontrados, err)
	}
	doc, err := a.CrearDocumento(ctx, "", api.DocText, "Informe")
	if err != nil || doc.Name != "Informe.docx" || doc.SizeBytes == 0 {
		t.Fatal("CrearDocumento", doc, err)
	}
	q, err := a.Quota(ctx)
	if err != nil || q.LimitBytes != 1000 || q.AvailableBytes != 400 {
		t.Fatal("Quota", q, err)
	}
	// Nada de temporales en el proveedor: ni en la raíz ni en .sytue.
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.docs {
		if strings.HasSuffix(d.Nombre, ".tmp") || strings.HasSuffix(d.Nombre, ".mover") {
			t.Fatal("quedó un temporal:", d.Nombre)
		}
	}
}

// TestCambiarRaizEnCaliente: la misma Archivos pasa de una raíz a otra (DR-12).
func TestCambiarRaizEnCaliente(t *testing.T) {
	a, base := abrirPrueba(t)
	ctx := t.Context()
	raiz, _ := a.Root(ctx)
	if _, err := a.Upload(ctx, raiz.ID, "en-el-disco.txt", "", strings.NewReader("d"), 1); err != nil {
		t.Fatal(err)
	}
	m := nuevaMemoria()
	if err := a.CambiarRaiz(raizVirtual, NuevoArbol(raizVirtual, m)); err != nil {
		t.Fatal(err)
	}
	nueva, err := a.Root(ctx)
	if err != nil || nueva.Name != "Descargas" || a.Raiz() != raizVirtual {
		t.Fatal("la raíz nueva", nueva, err)
	}
	if _, ok := a.Sistema().(*Arbol); !ok {
		t.Fatal("el sistema no es el árbol")
	}
	if _, err := a.Children(ctx, raiz.ID, ""); err == nil {
		t.Fatal("la raíz vieja sigue accesible")
	}
	if _, err := a.Upload(ctx, nueva.ID, "en-el-arbol.txt", "", strings.NewReader("a"), 1); err != nil {
		t.Fatal(err)
	}
	if got := m.nombres(t, "raiz"); !slices.Equal(got, []string{".sytue", "en-el-arbol.txt"}) {
		t.Fatal(got)
	}
	// Y de vuelta al disco, donde estaba todo.
	if err := a.CambiarRaiz(filepath.Join(base, "Drive"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "Drive", "en-el-disco.txt")); err != nil {
		t.Fatal(err)
	}
	if a.Sistema() != DelDisco && os.Getenv(sobreArbol) == "" {
		t.Fatal("no volvió al disco")
	}
}
