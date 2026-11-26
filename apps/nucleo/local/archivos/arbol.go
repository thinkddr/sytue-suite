// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// # Un árbol de documentos como raíz (DR-12)
//
// En Android, la carpeta de Drive puede ser una que la persona elige con el selector del
// sistema (Descargas, una tarjeta, la carpeta de otra app de ficheros): el Storage Access
// Framework no da una ruta sino un ÁRBOL de documentos, `content://…/tree/…`, donde cada
// documento tiene un identificador del proveedor y se crea, se renombra, se mueve y se
// borra con DocumentsContract. [Arbol] presenta ese árbol como un [Sistema] de rutas:
//
//   - La raíz del árbol cuelga de una ruta VIRTUAL (la que se pasa a Abrir), y cada ruta de
//     debajo se resuelve nombre a nombre desde ella, listando la carpeta la primera vez.
//     Lo que no cuelga de esa ruta (un fichero suelto abierto por su ruta, D-24) va al
//     disco, como siempre.
//   - Los identificadores del proveedor son opacos: aquí no se deduce nada de ellos, y se
//     olvidan al renombrar, mover o borrar (en el almacenamiento del teléfono el de cada
//     hijo cambia con el de su carpeta).
//   - Rename sustituye un fichero que ya esté en el destino, como `os.Rename`, borrándolo
//     antes: un proveedor no sabe renombrar encima. Lo que se pierde es la atomicidad: un
//     corte justo entre el borrado y el renombrado deja el temporal (`.nombre.*.tmp`) y la
//     versión anterior en `.sytue/versiones`, no un fichero a medias.
//   - Si el proveedor cambia un nombre (porque ya existe o porque no lo admite: `:` o `?`
//     en una tarjeta FAT), la operación se deshace y falla: el nombre de la ruta es el que
//     tiene que quedar.
//   - Un proveedor que no sabe mover (FLAG_SUPPORTS_MOVE) mueve copiando y borrando.

// ErrNoSoportado lo devuelve un proveedor de [Documentos] que no sabe hacer algo.
var ErrNoSoportado = errors.New("archivos: el proveedor de documentos no lo admite")

// errNombreCambiado: el proveedor no dejó el nombre que se le pidió.
var errNombreCambiado = fmt.Errorf("archivos: el proveedor de documentos cambió el nombre: %w", fs.ErrInvalid)

// Documento es un elemento de un árbol de documentos tal como lo describe su proveedor.
type Documento struct {
	ID         string
	Nombre     string
	Carpeta    bool
	Tamano     int64
	Modificado time.Time
}

// Documentos es el proveedor de un árbol de documentos: en Android, el de la carpeta que
// la persona eligió con el selector del sistema (`ficheros.Arbol`). Un documento que ya no
// está da un error que cumple `errors.Is(err, fs.ErrNotExist)`.
type Documentos interface {
	// Raiz es la carpeta elegida.
	Raiz() (Documento, error)
	Datos(id string) (Documento, error)
	Hijos(id string) ([]Documento, error)
	// Crear crea en padre un fichero vacío o una carpeta. El proveedor puede dejarlo con
	// otro nombre; el Documento devuelto dice cuál.
	Crear(padre, nombre string, carpeta bool) (Documento, error)
	// Renombrar cambia el nombre; el identificador puede cambiar.
	Renombrar(id, nombre string) (Documento, error)
	// Mover lleva id de la carpeta origen a destino; ErrNoSoportado si no sabe.
	Mover(id, origen, destino string) (Documento, error)
	// Borrar borra un documento; una carpeta, con todo lo que tenga.
	Borrar(id string) error
	// Leer abre el contenido; Escribir lo abre vaciándolo antes.
	Leer(id string) (io.ReadCloser, error)
	Escribir(id string) (io.WriteCloser, error)
	// Espacio del almacenamiento del árbol: total y libre.
	Espacio() (total, libre int64, err error)
}

// FrescuraListado es lo que vale el listado de una carpeta para decir que algo NO está en
// ella sin volver a preguntar: Archivos busca a menudo lo que no existe (el historial de
// versiones de cada fichero que lista) y cada pregunta al proveedor es una llamada entre
// procesos. Lo que cambie otra app se ve pasado este tiempo.
const FrescuraListado = 2 * time.Second

// Arbol es un árbol de documentos montado en una ruta virtual. Ver [NuevoArbol].
type Arbol struct {
	raiz  string
	docs  Documentos
	resto Sistema
	// Frescura es lo que vale un listado para decir que algo no está (FrescuraListado).
	Frescura time.Duration
	ahora    func() time.Time

	mu      sync.Mutex
	raizDoc *Documento
	ids     map[string]string    // ruta → identificador del proveedor
	listado map[string]time.Time // carpeta → cuándo se listó entera
}

var _ Sistema = (*Arbol)(nil)

// NuevoArbol monta en la ruta virtual raiz (absoluta) el árbol de docs. Lo que no cuelgue
// de raiz va al disco ([DelDisco]).
func NuevoArbol(raiz string, docs Documentos) *Arbol {
	return &Arbol{
		raiz: filepath.Clean(raiz), docs: docs, resto: DelDisco, Frescura: FrescuraListado, ahora: time.Now,
		ids: map[string]string{}, listado: map[string]time.Time{},
	}
}

// Raiz es la ruta virtual donde está montado.
func (t *Arbol) Raiz() string { return t.raiz }

// Documento es el identificador del proveedor de lo que hay en ruta: lo que hace falta
// para dárselo a otra app (`content://…/document/<id>`).
func (t *Arbol) Documento(ruta string) (string, error) {
	if !t.bajo(ruta) {
		return "", &fs.PathError{Op: "documento", Path: ruta, Err: fs.ErrInvalid}
	}
	return t.resolver(ruta)
}

func (t *Arbol) bajo(r string) bool {
	r = filepath.Clean(r)
	return r == t.raiz || strings.HasPrefix(r, t.raiz+string(filepath.Separator))
}

// ----- Resolver rutas -----

func (t *Arbol) docRaiz() (Documento, error) {
	t.mu.Lock()
	d := t.raizDoc
	t.mu.Unlock()
	if d != nil {
		return *d, nil
	}
	r, err := t.docs.Raiz()
	if err != nil {
		return Documento{}, err
	}
	t.mu.Lock()
	t.raizDoc = &r
	t.mu.Unlock()
	return r, nil
}

// resolver da el identificador de una ruta de debajo de la raíz, nombre a nombre.
func (t *Arbol) resolver(r string) (string, error) {
	r = filepath.Clean(r)
	raiz, err := t.docRaiz()
	if err != nil {
		return "", err
	}
	if r == t.raiz {
		return raiz.ID, nil
	}
	rel, err := filepath.Rel(t.raiz, r)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", &fs.PathError{Op: "abrir", Path: r, Err: fs.ErrNotExist}
	}
	actual, id := t.raiz, raiz.ID
	for _, nombre := range strings.Split(rel, string(filepath.Separator)) {
		hijo := filepath.Join(actual, nombre)
		t.mu.Lock()
		h, ok := t.ids[hijo]
		fresco := t.Frescura > 0 && t.ahora().Sub(t.listado[actual]) < t.Frescura
		t.mu.Unlock()
		if !ok && !fresco {
			if _, err := t.listar(actual, id); err != nil {
				return "", err
			}
			t.mu.Lock()
			h, ok = t.ids[hijo]
			t.mu.Unlock()
		}
		if !ok {
			return "", &fs.PathError{Op: "abrir", Path: r, Err: fs.ErrNotExist}
		}
		actual, id = hijo, h
	}
	return id, nil
}

// listar pide los hijos de una carpeta y deja apuntados sus identificadores; lo que ya
// no está se olvida, con lo que colgara de ello.
func (t *Arbol) listar(carpeta, id string) ([]Documento, error) {
	hijos, err := t.docs.Hijos(id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			t.olvidar(carpeta)
		}
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	vivos := map[string]bool{}
	for _, h := range hijos {
		r := filepath.Join(carpeta, h.Nombre)
		if vivos[r] {
			continue // dos con el mismo nombre (un proveedor de nube): vale el primero
		}
		vivos[r] = true
		if t.ids[r] != h.ID {
			t.olvidarBloqueado(r)
			t.ids[r] = h.ID
		}
	}
	for r := range t.ids {
		if filepath.Dir(r) == carpeta && !vivos[r] {
			t.olvidarBloqueado(r)
		}
	}
	t.listado[carpeta] = t.ahora()
	return hijos, nil
}

func (t *Arbol) apuntar(r, id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.olvidarBloqueado(r)
	t.ids[r] = id
}

func (t *Arbol) olvidar(r string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.olvidarBloqueado(r)
}

// olvidarBloqueado borra lo apuntado de r y de lo que cuelga de él. Con t.mu tomado.
func (t *Arbol) olvidarBloqueado(r string) {
	prefijo := r + string(filepath.Separator)
	for k := range t.ids {
		if k == r || strings.HasPrefix(k, prefijo) {
			delete(t.ids, k)
		}
	}
	for k := range t.listado {
		if k == r || strings.HasPrefix(k, prefijo) {
			delete(t.listado, k)
		}
	}
}

// datos son los del documento de r. Si el identificador apuntado ya no vale (otra app lo
// borró o lo movió), se olvida y se vuelve a buscar una vez.
func (t *Arbol) datos(r string) (Documento, error) {
	for intento := 0; ; intento++ {
		id, err := t.resolver(r)
		if err != nil {
			return Documento{}, err
		}
		d, err := t.docs.Datos(id)
		if errors.Is(err, fs.ErrNotExist) && intento == 0 && filepath.Clean(r) != t.raiz {
			t.olvidar(filepath.Clean(r))
			t.mu.Lock()
			delete(t.listado, filepath.Dir(filepath.Clean(r)))
			t.mu.Unlock()
			continue
		}
		if err != nil {
			return Documento{}, &fs.PathError{Op: "stat", Path: r, Err: err}
		}
		return d, nil
	}
}

// ----- Sistema -----

// Stat es os.Stat. La raíz se llama como la llama el proveedor.
func (t *Arbol) Stat(r string) (fs.FileInfo, error) {
	if !t.bajo(r) {
		return t.resto.Stat(r)
	}
	d, err := t.datos(r)
	if err != nil {
		return nil, err
	}
	nombre := filepath.Base(r)
	if filepath.Clean(r) == t.raiz {
		nombre = d.Nombre
	}
	return infoDe(nombre, d), nil
}

// Lstat es Stat: en un árbol de documentos no hay enlaces.
func (t *Arbol) Lstat(r string) (fs.FileInfo, error) {
	if !t.bajo(r) {
		return t.resto.Lstat(r)
	}
	return t.Stat(r)
}

// ReadDir es os.ReadDir: ordenado por nombre.
func (t *Arbol) ReadDir(r string) ([]fs.DirEntry, error) {
	if !t.bajo(r) {
		return t.resto.ReadDir(r)
	}
	r = filepath.Clean(r)
	id, err := t.resolver(r)
	if err != nil {
		return nil, err
	}
	hijos, err := t.listar(r, id)
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: r, Err: err}
	}
	vistos := map[string]bool{}
	out := make([]fs.DirEntry, 0, len(hijos))
	for _, h := range hijos {
		if vistos[h.Nombre] {
			continue
		}
		vistos[h.Nombre] = true
		out = append(out, fs.FileInfoToDirEntry(infoDe(h.Nombre, h)))
	}
	slices.SortFunc(out, func(x, y fs.DirEntry) int { return strings.Compare(x.Name(), y.Name()) })
	return out, nil
}

// ReadFile es os.ReadFile.
func (t *Arbol) ReadFile(r string) ([]byte, error) {
	if !t.bajo(r) {
		return t.resto.ReadFile(r)
	}
	f, err := t.Open(r)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// Open es os.Open, solo para ficheros.
func (t *Arbol) Open(r string) (Fichero, error) {
	if !t.bajo(r) {
		return t.resto.Open(r)
	}
	d, err := t.datos(r)
	if err != nil {
		return nil, err
	}
	if d.Carpeta {
		return nil, &fs.PathError{Op: "open", Path: r, Err: syscall.EISDIR}
	}
	rc, err := t.docs.Leer(d.ID)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: r, Err: err}
	}
	return &ficheroArbol{ReadCloser: rc, info: infoDe(filepath.Base(r), d)}, nil
}

// Mkdir es os.Mkdir.
func (t *Arbol) Mkdir(r string, p fs.FileMode) error {
	if !t.bajo(r) {
		return t.resto.Mkdir(r, p)
	}
	r = filepath.Clean(r)
	if _, err := t.datos(r); err == nil {
		return &fs.PathError{Op: "mkdir", Path: r, Err: fs.ErrExist}
	}
	padre, err := t.resolver(filepath.Dir(r))
	if err != nil {
		return &fs.PathError{Op: "mkdir", Path: r, Err: fs.ErrNotExist}
	}
	if _, err := t.crear(padre, r, true); err != nil {
		return &fs.PathError{Op: "mkdir", Path: r, Err: err}
	}
	return nil
}

// crear crea r en la carpeta padre y comprueba que se llama como se pidió; si no, lo
// borra y falla (con fs.ErrExist si fue porque ya había uno).
func (t *Arbol) crear(padre, r string, carpeta bool) (Documento, error) {
	nombre := filepath.Base(r)
	d, err := t.docs.Crear(padre, nombre, carpeta)
	if err != nil {
		return Documento{}, err
	}
	if d.Nombre != nombre {
		_ = t.docs.Borrar(d.ID)
		t.mu.Lock()
		delete(t.listado, filepath.Dir(r))
		t.mu.Unlock()
		if _, err := t.resolver(r); err == nil {
			return Documento{}, fs.ErrExist
		}
		return Documento{}, errNombreCambiado
	}
	t.apuntar(r, d.ID)
	return d, nil
}

// MkdirAll es os.MkdirAll.
func (t *Arbol) MkdirAll(r string, p fs.FileMode) error {
	if !t.bajo(r) {
		return t.resto.MkdirAll(r, p)
	}
	r = filepath.Clean(r)
	if fi, err := t.Stat(r); err == nil {
		if fi.IsDir() {
			return nil
		}
		return &fs.PathError{Op: "mkdir", Path: r, Err: syscall.ENOTDIR}
	} else if !errors.Is(err, fs.ErrNotExist) || r == t.raiz {
		return err
	}
	if err := t.MkdirAll(filepath.Dir(r), p); err != nil {
		return err
	}
	if err := t.Mkdir(r, p); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return nil
}

// CreateTemp es os.CreateTemp: el `*` del patrón (o el final) es un número al azar.
func (t *Arbol) CreateTemp(dir, patron string) (Temporal, error) {
	if !t.bajo(dir) {
		return t.resto.CreateTemp(dir, patron)
	}
	dir = filepath.Clean(dir)
	padre, err := t.resolver(dir)
	if err != nil {
		return nil, &fs.PathError{Op: "createtemp", Path: dir, Err: err}
	}
	antes, despues := patron, ""
	if i := strings.LastIndex(patron, "*"); i >= 0 {
		antes, despues = patron[:i], patron[i+1:]
	}
	for range 100 {
		r := filepath.Join(dir, antes+alAzar()+despues)
		if _, err := t.resolver(r); err == nil {
			continue
		}
		d, err := t.crear(padre, r, false)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return nil, &fs.PathError{Op: "createtemp", Path: r, Err: err}
		}
		w, err := t.docs.Escribir(d.ID)
		if err != nil {
			_ = t.docs.Borrar(d.ID)
			t.olvidar(r)
			return nil, &fs.PathError{Op: "createtemp", Path: r, Err: err}
		}
		return &temporalArbol{WriteCloser: w, nombre: r}, nil
	}
	return nil, &fs.PathError{Op: "createtemp", Path: dir + string(filepath.Separator) + patron, Err: fs.ErrExist}
}

// Rename es os.Rename: un fichero que ya esté en el destino se sustituye. Entre el árbol y
// el disco no se puede (como entre dos discos): quien lo necesita copia y borra.
func (t *Arbol) Rename(de, a string) error {
	enDe, enA := t.bajo(de), t.bajo(a)
	if !enDe && !enA {
		return t.resto.Rename(de, a)
	}
	de, a = filepath.Clean(de), filepath.Clean(a)
	fallo := func(err error) error { return &fs.PathError{Op: "rename", Path: de + " → " + a, Err: err} }
	if enDe != enA {
		return fallo(syscall.EXDEV)
	}
	if de == a {
		return nil
	}
	if de == t.raiz || a == t.raiz || strings.HasPrefix(a, de+string(filepath.Separator)) {
		return fallo(fs.ErrInvalid)
	}
	origen, err := t.datos(de)
	if err != nil {
		return fallo(fs.ErrNotExist)
	}
	padreDe, err := t.resolver(filepath.Dir(de))
	if err != nil {
		return fallo(err)
	}
	padreA, err := t.resolver(filepath.Dir(a))
	if err != nil {
		return fallo(fs.ErrNotExist)
	}
	if ya, err := t.datos(a); err == nil {
		if ya.Carpeta || origen.Carpeta {
			return fallo(fs.ErrExist)
		}
		if err := t.docs.Borrar(ya.ID); err != nil {
			return fallo(err)
		}
		t.olvidar(a)
	}
	d, err := t.llevar(origen, de, padreDe, a, padreA)
	t.olvidar(de)
	t.olvidar(a)
	if err != nil {
		// A medias puede haber quedado con otro nombre o en otra carpeta: lo que se sabía
		// de las dos ya no vale.
		t.mu.Lock()
		delete(t.listado, filepath.Dir(de))
		delete(t.listado, filepath.Dir(a))
		t.mu.Unlock()
		return fallo(err)
	}
	t.apuntar(a, d.ID)
	return nil
}

// llevar deja en a (dentro de padreA, donde ya no hay nada con ese nombre) el documento
// que está en la ruta de (dentro de padreDe).
func (t *Arbol) llevar(origen Documento, de, padreDe, a, padreA string) (Documento, error) {
	nombreDe, nombreA := filepath.Base(de), filepath.Base(a)
	if filepath.Dir(de) == filepath.Dir(a) {
		return t.renombrar(origen.ID, nombreDe, nombreA)
	}
	id, nombre := origen.ID, nombreDe
	if nombreDe != nombreA {
		// Se renombra antes de mover si en el destino ya hay algo con el nombre de ahora, y
		// por un nombre de paso si además en el origen lo hay con el de después.
		_, chocaEnA := t.resolver(filepath.Join(filepath.Dir(a), nombreDe))
		if chocaEnA == nil {
			paso := nombreA
			if _, err := t.resolver(filepath.Join(filepath.Dir(de), nombreA)); err == nil {
				paso = "." + nombreDe + "." + alAzar() + ".mover"
			}
			d, err := t.renombrar(id, nombreDe, paso)
			if err != nil {
				return Documento{}, err
			}
			id, nombre = d.ID, paso
		}
	}
	d, err := t.docs.Mover(id, padreDe, padreA)
	if errors.Is(err, ErrNoSoportado) {
		d, err = t.moverCopiando(id, filepath.Join(filepath.Dir(de), nombre), padreA, filepath.Join(filepath.Dir(a), nombre))
	}
	if err != nil {
		return Documento{}, err
	}
	if nombre != nombreA {
		return t.renombrar(d.ID, nombre, nombreA)
	}
	return d, nil
}

// renombrar cambia el nombre y comprueba que quedó el pedido; si no, lo devuelve al de
// antes y falla.
func (t *Arbol) renombrar(id, antes, despues string) (Documento, error) {
	d, err := t.docs.Renombrar(id, despues)
	if err != nil {
		return Documento{}, err
	}
	if d.Nombre != despues {
		_, _ = t.docs.Renombrar(d.ID, antes)
		return Documento{}, errNombreCambiado
	}
	return d, nil
}

// moverCopiando es mover para un proveedor que no sabe: copia de (con su árbol) a a, en la
// carpeta padreA, y después borra de. Si la copia falla, lo copiado se retira.
func (t *Arbol) moverCopiando(id, de, padreA, a string) (Documento, error) {
	d, err := t.copiarDoc(id, de, padreA, a)
	if err != nil {
		return Documento{}, err
	}
	if err := t.docs.Borrar(id); err != nil {
		return Documento{}, err
	}
	return d, nil
}

func (t *Arbol) copiarDoc(id, de, padreA, a string) (Documento, error) {
	origen, err := t.docs.Datos(id)
	if err != nil {
		return Documento{}, err
	}
	nuevo, err := t.crear(padreA, a, origen.Carpeta)
	if err != nil {
		return Documento{}, err
	}
	if !origen.Carpeta {
		err = t.copiarContenido(origen.ID, nuevo.ID)
	} else {
		var hijos []Documento
		if hijos, err = t.docs.Hijos(origen.ID); err == nil {
			for _, h := range hijos {
				if _, err = t.copiarDoc(h.ID, filepath.Join(de, h.Nombre), nuevo.ID, filepath.Join(a, h.Nombre)); err != nil {
					break
				}
			}
		}
	}
	if err != nil {
		_ = t.docs.Borrar(nuevo.ID)
		t.olvidar(a)
		return Documento{}, err
	}
	return t.docs.Datos(nuevo.ID)
}

func (t *Arbol) copiarContenido(de, a string) error {
	in, err := t.docs.Leer(de)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := t.docs.Escribir(a)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// Remove es os.Remove: una carpeta, solo si está vacía.
func (t *Arbol) Remove(r string) error {
	if !t.bajo(r) {
		return t.resto.Remove(r)
	}
	r = filepath.Clean(r)
	if r == t.raiz {
		return &fs.PathError{Op: "remove", Path: r, Err: fs.ErrPermission}
	}
	d, err := t.datos(r)
	if err != nil {
		return &fs.PathError{Op: "remove", Path: r, Err: fs.ErrNotExist}
	}
	if d.Carpeta {
		hijos, err := t.docs.Hijos(d.ID)
		if err != nil {
			return &fs.PathError{Op: "remove", Path: r, Err: err}
		}
		if len(hijos) > 0 {
			return &fs.PathError{Op: "remove", Path: r, Err: syscall.ENOTEMPTY}
		}
	}
	if err := t.docs.Borrar(d.ID); err != nil {
		return &fs.PathError{Op: "remove", Path: r, Err: err}
	}
	t.olvidar(r)
	return nil
}

// RemoveAll es os.RemoveAll: lo que no está no es un error. La raíz no se borra nunca:
// es la carpeta de la persona, no de la app.
func (t *Arbol) RemoveAll(r string) error {
	if !t.bajo(r) {
		return t.resto.RemoveAll(r)
	}
	r = filepath.Clean(r)
	if r == t.raiz {
		return &fs.PathError{Op: "removeall", Path: r, Err: fs.ErrPermission}
	}
	d, err := t.datos(r)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := t.docs.Borrar(d.ID); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return &fs.PathError{Op: "removeall", Path: r, Err: err}
	}
	t.olvidar(r)
	return nil
}

// Chmod no hace nada en el árbol: los documentos no tienen permisos de Unix.
func (t *Arbol) Chmod(r string, p fs.FileMode) error {
	if !t.bajo(r) {
		return t.resto.Chmod(r, p)
	}
	return nil
}

// WalkDir es filepath.WalkDir sobre el árbol.
func (t *Arbol) WalkDir(raiz string, fn fs.WalkDirFunc) error {
	if !t.bajo(raiz) {
		return t.resto.WalkDir(raiz, fn)
	}
	info, err := t.Lstat(raiz)
	if err != nil {
		err = fn(raiz, nil, err)
	} else {
		err = t.recorrer(raiz, fs.FileInfoToDirEntry(info), fn)
	}
	if errors.Is(err, fs.SkipDir) || errors.Is(err, fs.SkipAll) {
		return nil
	}
	return err
}

func (t *Arbol) recorrer(r string, d fs.DirEntry, fn fs.WalkDirFunc) error {
	if err := fn(r, d, nil); err != nil || !d.IsDir() {
		if errors.Is(err, fs.SkipDir) && d.IsDir() {
			err = nil
		}
		return err
	}
	entradas, err := t.ReadDir(r)
	if err != nil {
		if err = fn(r, d, err); err != nil {
			if errors.Is(err, fs.SkipDir) && d.IsDir() {
				err = nil
			}
			return err
		}
	}
	for _, e := range entradas {
		if err := t.recorrer(filepath.Join(r, e.Name()), e, fn); err != nil {
			if errors.Is(err, fs.SkipDir) {
				break
			}
			return err
		}
	}
	return nil
}

// EscribirAtomico es el de siempre (temporal y renombrado) sobre el árbol.
func (t *Arbol) EscribirAtomico(r string, datos []byte) error {
	if !t.bajo(r) {
		return t.resto.EscribirAtomico(r, datos)
	}
	return escribirAtomicoEn(t, r, datos)
}

// Espacio es el que dice el proveedor del árbol.
func (t *Arbol) Espacio(r string) (total, libre int64, err error) {
	if !t.bajo(r) {
		return t.resto.Espacio(r)
	}
	return t.docs.Espacio()
}

// alAzar es un número al azar para un nombre de paso, como el de os.CreateTemp.
func alAzar() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return strconv.FormatUint(uint64(binary.LittleEndian.Uint32(b[:])), 10)
}

// ----- Lo que devuelve -----

type infoArbol struct {
	nombre string
	d      Documento
}

func infoDe(nombre string, d Documento) fs.FileInfo { return infoArbol{nombre: nombre, d: d} }

func (i infoArbol) Name() string       { return i.nombre }
func (i infoArbol) Size() int64        { return max(i.d.Tamano, 0) }
func (i infoArbol) ModTime() time.Time { return i.d.Modificado }
func (i infoArbol) IsDir() bool        { return i.d.Carpeta }
func (i infoArbol) Sys() any           { return i.d }
func (i infoArbol) Mode() fs.FileMode {
	if i.d.Carpeta {
		return fs.ModeDir | 0o755
	}
	return 0o644
}

type ficheroArbol struct {
	io.ReadCloser
	info fs.FileInfo
}

func (f *ficheroArbol) Stat() (fs.FileInfo, error) { return f.info, nil }

type temporalArbol struct {
	io.WriteCloser
	nombre string
}

func (w *temporalArbol) Name() string { return w.nombre }

// Sync vuelca a disco si el proveedor da un fichero de verdad; una tubería (un proveedor
// de nube) no sabe y no es un error: lo escrito llega al cerrarla.
func (w *temporalArbol) Sync() error {
	s, ok := w.WriteCloser.(interface{ Sync() error })
	if !ok {
		return nil
	}
	if err := s.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) {
		return err
	}
	return nil
}
