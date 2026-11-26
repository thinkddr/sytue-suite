// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package archivos es `api.DriveData` sobre el sistema de ficheros, para Sytue Office,
// la edición local de la suite (apps/fyne/PLAN-EDICION-LOCAL.md §6.3 y §6.4, DR-01/DR-02).
//
// Los ficheros de la persona se quedan donde están y tal cual, en claro:
//
//   - La RAÍZ es una carpeta de la carpeta de datos (`~/Sytue Office/Drive`). Lo que hay
//     debajo se lista, se crea, se renombra y se mueve como en Drive.
//   - Un fichero SUELTO de cualquier otra carpeta se abre por su ruta absoluta y NO se
//     copia (D-24): queda en «recientes», y él y su carpeta pasan a ser accesibles (para
//     dejar la copia de una operación a su lado, como en Drive).
//   - Guardar en su sitio (`api.DriveFileReplacer`) escribe a un temporal y renombra, y
//     guarda antes la versión anterior en `<raíz>/.sytue/versiones` (P-02, R-NO-CORROMPE).
//   - La papelera es `<raíz>/.sytue/papelera`; recientes, favoritos y alias de
//     renombrados van en `<raíz>/.sytue/indice.json`, legible.
//   - Todo pasa por el [Sistema] de la raíz (sistema.go): el disco ([DelDisco]) o, en
//     Android, la carpeta elegida con el selector del sistema, un árbol de documentos que
//     [Arbol] presenta como rutas (DR-12). Se puede cambiar en caliente ([Archivos.CambiarRaiz]).
//
// El identificador de cada elemento es opaco para las pantallas: su ruta absoluta
// codificada. Al renombrar o mover desde aquí, el identificador viejo sigue valiendo
// (alias) y el historial de versiones se va con el fichero. Lo que cambie otro programa no
// se versiona.
//
// Las miniaturas y el ZIP se hacen en el dispositivo; lo que no tiene sentido sin
// servidor (etiquetas de sensibilidad de una organización) falla con ErrNoDisponible, que
// envuelve api.ErrPrivateDriveOperation.
package archivos

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Carpeta oculta de la raíz donde van el índice, la papelera y las versiones.
const Meta = ".sytue"

const (
	// MaxRecientes es cuántos ficheros abiertos se recuerdan.
	MaxRecientes = 50
	// MaxVersionesPorDefecto es el tope de versiones guardadas por fichero.
	MaxVersionesPorDefecto = 20
	// RetencionPapelera es lo que se queda algo en la papelera antes de borrarse solo.
	RetencionPapelera = 30 * 24 * time.Hour
	// maxBusqueda corta una búsqueda por nombre en un árbol enorme.
	maxBusqueda = 200
)

var (
	// ErrNoDisponible: la operación no existe en la edición local.
	ErrNoDisponible = fmt.Errorf("archivos: no disponible en este equipo: %w", api.ErrPrivateDriveOperation)
	// ErrDesfasado: el fichero cambió desde que se abrió (otra ventana u otro programa);
	// guardar encima perdería ese cambio.
	ErrDesfasado = errors.New("archivos: el fichero ha cambiado desde que se abrió")
	// ErrFuera: el identificador apunta a algo que ni está bajo la raíz ni se ha abierto.
	ErrFuera = errors.New("archivos: fuera de las carpetas accesibles")
	// ErrNombre: un nombre vacío, con separadores o reservado.
	ErrNombre = errors.New("archivos: nombre no válido")
	// ErrExiste: ya hay algo con ese nombre en la carpeta de destino.
	ErrExiste = errors.New("archivos: ya existe")
)

// Opciones de Abrir.
type Opciones struct {
	// Instalacion es el identificador de esta instalación: el sujeto del alcance.
	Instalacion string
	// Autor es el nombre del perfil, con el que se firman las anotaciones (P-09).
	Autor string
	// MaxVersiones por fichero; 0 es MaxVersionesPorDefecto.
	MaxVersiones int
	// Ahora es el reloj; nil es time.Now.
	Ahora func() time.Time
	// Miniaturas es la carpeta de la caché en disco de miniaturas (se puede borrar sin
	// perder nada); vacía, se hacen cada vez.
	Miniaturas string
	// Sistema es el sistema de ficheros de la raíz; nil es DelDisco, el del equipo. En
	// Android, una carpeta elegida con el selector del sistema es un [Arbol] (DR-12).
	Sistema Sistema
}

// Archivos es una raíz abierta.
type Archivos struct {
	// est es la raíz y su sistema de ficheros: se cambian juntos y en caliente
	// ([Archivos.CambiarRaiz]), así que se leen siempre por raiz(), meta() y fs().
	est   atomic.Pointer[estado]
	scope storage.Scope
	autor string
	maxV  int
	ahora func() time.Time
	minis string

	mu  sync.Mutex
	ind indice
}

type indice struct {
	Recientes []reciente        `json:"recientes,omitempty"`
	Favoritos []string          `json:"favoritos,omitempty"`
	Alias     map[string]string `json:"alias,omitempty"`
	Papelera  []enPapelera      `json:"papelera,omitempty"`
	// Etiquetas libres de la persona, por ruta (DR-09).
	Etiquetas map[string][]string `json:"etiquetas,omitempty"`
}

type reciente struct {
	Ruta    string    `json:"ruta"`
	Abierto time.Time `json:"abierto"`
}

type enPapelera struct {
	Clave   string    `json:"clave"`
	Ruta    string    `json:"ruta"`
	Carpeta bool      `json:"carpeta,omitempty"`
	Borrado time.Time `json:"borrado"`
	// Etiquetas son las que llevaba (y, si es carpeta, lo de dentro), por ruta: vuelven
	// con él al restaurarlo.
	Etiquetas map[string][]string `json:"etiquetas,omitempty"`
}

var (
	_ api.DriveData         = (*Archivos)(nil)
	_ api.DriveFileReplacer = (*Archivos)(nil)
	_ api.DrivePathResolver = (*Archivos)(nil)
	_ api.PDFLister         = (*Archivos)(nil)
	_ api.PDFAuthorNamer    = (*Archivos)(nil)
)

// estado es la raíz abierta: su ruta, la de su `.sytue` y su sistema de ficheros.
type estado struct {
	raiz string
	meta string
	fs   Sistema
}

func (a *Archivos) raiz() string { return a.est.Load().raiz }
func (a *Archivos) meta() string { return a.est.Load().meta }
func (a *Archivos) fs() Sistema  { return a.est.Load().fs }

// Abrir abre (y crea si falta) la raíz. No habla con la red.
func Abrir(raiz string, o Opciones) (*Archivos, error) {
	a := &Archivos{
		scope: storage.Scope{Tenant: "local", Subject: cmp.Or(strings.TrimSpace(o.Instalacion), "local"), Product: "drive"},
		autor: strings.TrimSpace(o.Autor),
		maxV:  cmp.Or(o.MaxVersiones, MaxVersionesPorDefecto),
		ahora: o.Ahora,
		minis: o.Miniaturas,
	}
	if a.ahora == nil {
		a.ahora = time.Now
	}
	if !a.scope.Valid() {
		return nil, fmt.Errorf("archivos: alcance no válido")
	}
	e, ind, err := abrirRaiz(raiz, o.Sistema)
	if err != nil {
		return nil, err
	}
	a.est.Store(e)
	a.ind = ind
	a.mu.Lock()
	a.purgarPapelera()
	a.mu.Unlock()
	return a, nil
}

// abrirRaiz prepara una raíz (la crea si falta, con su `.sytue`) y lee su índice.
func abrirRaiz(raiz string, s Sistema) (*estado, indice, error) {
	abs, err := filepath.Abs(raiz)
	if err != nil {
		return nil, indice{}, err
	}
	if s == nil {
		s = DelDisco
		if porDefecto != nil {
			s = porDefecto(filepath.Clean(abs))
		}
	}
	e := &estado{raiz: filepath.Clean(abs), fs: s}
	e.meta = filepath.Join(e.raiz, Meta)
	for _, d := range []string{e.raiz, filepath.Join(e.meta, "versiones"), filepath.Join(e.meta, "papelera")} {
		if err := s.MkdirAll(d, 0o700); err != nil {
			return nil, indice{}, err
		}
	}
	var ind indice
	raw, err := s.ReadFile(filepath.Join(e.meta, "indice.json"))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, indice{}, err
	default:
		if err := json.Unmarshal(raw, &ind); err != nil {
			return nil, indice{}, fmt.Errorf("archivos: índice ilegible: %w", err)
		}
	}
	return e, ind, nil
}

// porDefecto, si está, da el sistema de ficheros de una raíz abierta sin Opciones.Sistema
// en vez de DelDisco. Solo lo ponen las pruebas, para pasar las de siempre por un [Arbol].
var porDefecto func(raiz string) Sistema

// CambiarRaiz pasa, en caliente, a otra raíz con su sistema de ficheros (nil, el del
// equipo): en Android, la carpeta que la persona elige con el selector del sistema o la
// de la app (DR-12). La nueva se prepara entera antes del cambio; si falla, sigue la de
// antes. Lo que la pantalla tenga abierto de la vieja deja de ser accesible.
func (a *Archivos) CambiarRaiz(raiz string, s Sistema) error {
	e, ind, err := abrirRaiz(raiz, s)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.est.Store(e)
	a.ind = ind
	a.purgarPapelera()
	return nil
}

// Sistema es el sistema de ficheros de la raíz abierta.
func (a *Archivos) Sistema() Sistema { return a.fs() }

// Raiz es la carpeta raíz, absoluta.
func (a *Archivos) Raiz() string { return a.raiz() }

func (a *Archivos) rutaIndice() string { return filepath.Join(a.meta(), "indice.json") }

// guardarIndice escribe el índice; se llama con a.mu tomado.
func (a *Archivos) guardarIndice() error {
	raw, err := json.MarshalIndent(a.ind, "", "  ")
	if err != nil {
		return err
	}
	return a.fs().EscribirAtomico(a.rutaIndice(), append(raw, '\n'))
}

// ----- Identificadores y permisos -----

func idDe(ruta string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(filepath.ToSlash(ruta)))
}

// ruta resuelve un identificador (o su alias) en una ruta absoluta accesible.
func (a *Archivos) ruta(id string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rutaBloqueado(id)
}

func (a *Archivos) rutaBloqueado(id string) (string, error) {
	r, ok := a.ind.Alias[id]
	if !ok {
		raw, err := base64.RawURLEncoding.DecodeString(id)
		if err != nil || len(raw) == 0 {
			return "", noEsta(id)
		}
		r = filepath.FromSlash(string(raw))
	}
	if !filepath.IsAbs(r) || filepath.Clean(r) != r {
		return "", noEsta(id)
	}
	if !a.accesible(r) {
		return "", ErrFuera
	}
	return r, nil
}

func dentro(base, r string) bool {
	rel, err := filepath.Rel(base, r)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// accesible: bajo la raíz (fuera de `.sytue`), un reciente, la carpeta de un reciente o
// lo que hay directamente dentro de ella. Se llama con a.mu tomado.
func (a *Archivos) accesible(r string) bool {
	if dentro(a.meta(), r) {
		return false
	}
	if dentro(a.raiz(), r) {
		return true
	}
	for _, x := range a.ind.Recientes {
		d := filepath.Dir(x.Ruta)
		if x.Ruta == r || d == r || d == filepath.Dir(r) {
			return true
		}
	}
	return false
}

func noEsta(que string) error {
	return &api.Error{Status: http.StatusNotFound, URL: que, Body: `{"code":"not_found","message":"The file does not exist."}`}
}

func conflicto(que string) error {
	return errors.Join(ErrExiste, &api.Error{Status: http.StatusConflict, URL: que, Body: `{"code":"conflict","message":"An item with that name already exists."}`})
}

func oculto(nombre string) bool { return strings.HasPrefix(nombre, ".") }

func nombreValido(n string) (string, error) {
	n = strings.TrimSpace(n)
	if n == "" || n == "." || n == ".." || strings.ContainsAny(n, "/\\\x00") || n == Meta {
		return "", ErrNombre
	}
	return n, nil
}

// ----- Convertir a los tipos del API -----

func tipoDe(nombre string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(nombre))); t != "" {
		return t
	}
	return "application/octet-stream"
}

func (a *Archivos) fichero(r string, fi fs.FileInfo) api.File {
	return api.File{
		ID: idDe(r), FolderID: idDe(filepath.Dir(r)), Name: fi.Name(), SizeBytes: fi.Size(),
		ContentType: tipoDe(fi.Name()), Version: a.historial(r).Actual,
		OwnerSub: a.scope.Subject, ModifiedBy: a.scope.Subject,
		CreatedAt: fi.ModTime(), UpdatedAt: fi.ModTime(), Path: r,
	}
}

// ficheroConSuma es fichero con el SHA-256 del contenido: lo que ReplaceFile compara para
// saber que nadie lo cambió desde que se abrió.
func (a *Archivos) ficheroConSuma(r string) (api.File, error) {
	fi, err := a.fs().Stat(r)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.File{}, noEsta(r)
		}
		return api.File{}, err
	}
	if !fi.Mode().IsRegular() {
		return api.File{}, noEsta(r)
	}
	f := a.fichero(r, fi)
	if f.Checksum, err = sumaEn(a.fs(), r); err != nil {
		return api.File{}, err
	}
	return f, nil
}

// suma es el SHA-256 de un fichero del disco.
func suma(r string) (string, error) { return sumaEn(DelDisco, r) }

func sumaEn(s Sistema, r string) (string, error) {
	f, err := s.Open(r)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (a *Archivos) carpeta(r string, fi fs.FileInfo) api.Folder {
	c := api.Folder{
		ID: idDe(r), Name: fi.Name(), IsRoot: r == a.raiz(), OwnerSub: a.scope.Subject,
		CreatedAt: fi.ModTime(), UpdatedAt: fi.ModTime(), Path: r,
	}
	if r != a.raiz() {
		padre := filepath.Dir(r)
		a.mu.Lock()
		ok := a.accesible(padre)
		a.mu.Unlock()
		if ok {
			id := idDe(padre)
			c.ParentID = &id
		}
	}
	return c
}

// ----- api.DriveData -----

// PrivateStorageScope es el alcance local: inquilino «local», sujeto la instalación.
func (a *Archivos) PrivateStorageScope() storage.Scope { return a.scope }

// PDFAuthorName es el nombre del perfil (P-09).
func (a *Archivos) PDFAuthorName() string { return a.autor }

// Root es la carpeta raíz.
func (a *Archivos) Root(ctx context.Context) (api.Folder, error) {
	if err := ctx.Err(); err != nil {
		return api.Folder{}, err
	}
	fi, err := a.fs().Stat(a.raiz())
	if err != nil {
		return api.Folder{}, err
	}
	return a.carpeta(a.raiz(), fi), nil
}

// Folder es una carpeta accesible.
func (a *Archivos) Folder(ctx context.Context, id string) (api.Folder, error) {
	r, err := a.ruta(id)
	if err != nil {
		return api.Folder{}, err
	}
	fi, err := a.fs().Stat(r)
	if err != nil || !fi.IsDir() {
		return api.Folder{}, noEsta(id)
	}
	return a.carpeta(r, fi), ctx.Err()
}

// File es un fichero accesible, con su suma.
func (a *Archivos) File(ctx context.Context, id string) (api.File, error) {
	r, err := a.ruta(id)
	if err != nil {
		return api.File{}, err
	}
	if err := ctx.Err(); err != nil {
		return api.File{}, err
	}
	return a.ficheroConSuma(r)
}

// Children lista una carpeta entera (sin páginas), sin lo oculto.
func (a *Archivos) Children(ctx context.Context, id, _ string) (api.Children, error) {
	r, err := a.ruta(id)
	if err != nil {
		return api.Children{}, err
	}
	entradas, err := a.fs().ReadDir(r)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.Children{}, noEsta(id)
		}
		return api.Children{}, err
	}
	var out api.Children
	for _, e := range entradas {
		if err := ctx.Err(); err != nil {
			return api.Children{}, err
		}
		if oculto(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue // se borró mientras se listaba
		}
		hijo := filepath.Join(r, e.Name())
		switch {
		case fi.IsDir():
			out.Folders = append(out.Folders, a.carpeta(hijo, fi))
		case fi.Mode().IsRegular():
			out.Files = append(out.Files, a.fichero(hijo, fi))
		}
	}
	return out, nil
}

// CreateFolder crea una carpeta dentro de otra.
func (a *Archivos) CreateFolder(ctx context.Context, padre, nombre string) (api.Folder, error) {
	r, err := a.ruta(padre)
	if err != nil {
		return api.Folder{}, err
	}
	if nombre, err = nombreValido(nombre); err != nil {
		return api.Folder{}, err
	}
	nueva := filepath.Join(r, nombre)
	if err := a.fs().Mkdir(nueva, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return api.Folder{}, conflicto(nombre)
		}
		return api.Folder{}, err
	}
	fi, err := a.fs().Stat(nueva)
	if err != nil {
		return api.Folder{}, err
	}
	return a.carpeta(nueva, fi), ctx.Err()
}

// Download abre el contenido de un fichero.
func (a *Archivos) Download(_ context.Context, id string) (io.ReadCloser, error) {
	r, err := a.ruta(id)
	if err != nil {
		return nil, err
	}
	f, err := a.fs().Open(r)
	if errors.Is(err, os.ErrNotExist) {
		return nil, noEsta(id)
	}
	return f, err
}

// Upload escribe un fichero en una carpeta. Con un nombre que ya existe lo SUSTITUYE
// (como Drive), guardando antes la versión anterior.
func (a *Archivos) Upload(ctx context.Context, carpeta, nombre, _ string, cuerpo io.Reader, tamano int64) (api.File, error) {
	d, err := a.ruta(carpeta)
	if err != nil {
		return api.File{}, err
	}
	if nombre, err = nombreValido(nombre); err != nil {
		return api.File{}, err
	}
	if fi, err := a.fs().Stat(d); err != nil || !fi.IsDir() {
		return api.File{}, noEsta(carpeta)
	}
	r := filepath.Join(d, nombre)
	if err := a.escribir(ctx, r, cuerpo, tamano); err != nil {
		return api.File{}, err
	}
	return a.ficheroConSuma(r)
}

// ReplaceFile guarda en su sitio un fichero abierto (P-02): falla con ErrDesfasado si
// cambió desde que se abrió, guarda la versión anterior y escribe a temporal + renombrado.
func (a *Archivos) ReplaceFile(ctx context.Context, original api.File, _ string, cuerpo io.Reader, tamano int64) (api.File, error) {
	r, err := a.ruta(original.ID)
	if err != nil {
		return api.File{}, err
	}
	actual, err := a.ficheroConSuma(r)
	if err != nil {
		return api.File{}, err
	}
	switch {
	case original.Checksum != "" && original.Checksum != actual.Checksum,
		original.Version != 0 && original.Version != actual.Version,
		original.Checksum == "" && !original.UpdatedAt.IsZero() && !original.UpdatedAt.Equal(actual.UpdatedAt):
		return api.File{}, ErrDesfasado
	}
	if err := a.escribir(ctx, r, cuerpo, tamano); err != nil {
		return api.File{}, err
	}
	return a.ficheroConSuma(r)
}

// escribir deja cuerpo en r sin que se vea nunca un fichero a medias: primero a un
// temporal de la misma carpeta, con su tamaño comprobado; si r ya existe, su contenido
// pasa a ser una versión; y después el renombrado, que es atómico.
func (a *Archivos) escribir(ctx context.Context, r string, cuerpo io.Reader, tamano int64) error {
	tmp, err := a.fs().CreateTemp(filepath.Dir(r), "."+filepath.Base(r)+".*.tmp")
	if err != nil {
		return err
	}
	nombreTmp := tmp.Name()
	defer func() { _ = a.fs().Remove(nombreTmp) }() // no hace nada tras el renombrado
	n, err := io.Copy(tmp, cuerpo)
	if err == nil && tamano >= 0 && n != tamano {
		err = fmt.Errorf("archivos: se esperaban %d bytes y llegaron %d", tamano, n)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return err
	}
	if fi, err := a.fs().Stat(r); err == nil {
		if !fi.Mode().IsRegular() {
			return conflicto(filepath.Base(r))
		}
		_ = a.fs().Chmod(nombreTmp, fi.Mode().Perm())
		if err := a.archivar(r, fi); err != nil {
			return fmt.Errorf("archivos: no se pudo guardar la versión anterior: %w", err)
		}
	}
	return a.fs().Rename(nombreTmp, r)
}

// Rename cambia el nombre en su misma carpeta.
func (a *Archivos) Rename(_ context.Context, k api.DriveKind, id, nombre string) error {
	r, err := a.ruta(id)
	if err != nil {
		return err
	}
	if r == a.raiz() {
		return ErrNombre
	}
	if nombre, err = nombreValido(nombre); err != nil {
		return err
	}
	return a.mover(k, id, r, filepath.Join(filepath.Dir(r), nombre))
}

// Move lleva un elemento a otra carpeta accesible.
func (a *Archivos) Move(_ context.Context, k api.DriveKind, id, destino string) error {
	r, err := a.ruta(id)
	if err != nil {
		return err
	}
	d, err := a.ruta(destino)
	if err != nil {
		return err
	}
	if r == a.raiz() || (k == api.DriveFolder && dentro(r, d)) {
		return ErrNombre
	}
	return a.mover(k, id, r, filepath.Join(d, filepath.Base(r)))
}

func (a *Archivos) mover(k api.DriveKind, id, de, a2 string) error {
	if de == a2 {
		return nil
	}
	fi, err := a.fs().Stat(de)
	if err != nil {
		return noEsta(id)
	}
	if (k == api.DriveFolder) != fi.IsDir() {
		return noEsta(id)
	}
	if _, err := a.fs().Lstat(a2); err == nil {
		return conflicto(filepath.Base(a2))
	}
	if err := a.fs().Rename(de, a2); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.trasMover(de, a2, fi.IsDir())
	return a.guardarIndice()
}

// trasMover lleva a a2 lo que el índice sabe de la ruta de (y de lo que cuelga, si es carpeta):
// alias del identificador viejo, recientes, favoritos y versiones. Con a.mu tomado.
func (a *Archivos) trasMover(de, a2 string, carpeta bool) {
	cambiar := func(r string) (string, bool) {
		if r == de {
			return a2, true
		}
		if carpeta && dentro(de, r) {
			rel, _ := filepath.Rel(de, r)
			return filepath.Join(a2, rel), true
		}
		return r, false
	}
	if a.ind.Alias == nil {
		a.ind.Alias = map[string]string{}
	}
	for k, v := range a.ind.Alias {
		if n, ok := cambiar(v); ok {
			a.ind.Alias[k] = n
		}
	}
	a.ind.Alias[idDe(de)] = a2
	for i := range a.ind.Recientes {
		if n, ok := cambiar(a.ind.Recientes[i].Ruta); ok {
			a.ind.Recientes[i].Ruta = n
		}
	}
	for i := range a.ind.Favoritos {
		if n, ok := cambiar(a.ind.Favoritos[i]); ok {
			a.ind.Favoritos[i] = n
		}
	}
	a.moverEtiquetas(cambiar)
	if !carpeta {
		viejo, nuevo := a.dirVersiones(de), a.dirVersiones(a2)
		if _, err := a.fs().Stat(viejo); err == nil {
			_ = a.fs().RemoveAll(nuevo)
			if a.fs().Rename(viejo, nuevo) == nil {
				h := a.historial(a2)
				h.Ruta = a2
				_ = a.guardarHistorial(a2, h)
			}
		}
	}
}

// CopyFile deja una copia a su lado: «nombre (2).ext».
func (a *Archivos) CopyFile(ctx context.Context, id string) (api.File, error) {
	r, err := a.ruta(id)
	if err != nil {
		return api.File{}, err
	}
	f, err := a.fs().Open(r)
	if err != nil {
		return api.File{}, noEsta(id)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return api.File{}, noEsta(id)
	}
	destino := nombreLibreEn(a.fs(), filepath.Dir(r), fi.Name())
	if err := a.escribir(ctx, destino, f, fi.Size()); err != nil {
		return api.File{}, err
	}
	return a.ficheroConSuma(destino)
}

// NombreLibre es el primer «base (n).ext» que no existe en dir, en el disco.
func NombreLibre(dir, nombre string) string { return nombreLibreEn(DelDisco, dir, nombre) }

// nombreLibreEn es NombreLibre en el sistema s. Un error que no sea «no existe» (sin
// permiso, un proveedor que no contesta) también se queda con ese nombre: lo que escriba
// después dirá por qué no se puede, en vez de buscar otro sin fin.
func nombreLibreEn(s Sistema, dir, nombre string) string {
	ext := filepath.Ext(nombre)
	base := strings.TrimSuffix(nombre, ext)
	for n := 2; ; n++ {
		c := filepath.Join(dir, base+" ("+strconv.Itoa(n)+")"+ext)
		if _, err := s.Lstat(c); err != nil {
			return c
		}
	}
}

// Search busca por nombre bajo la raíz, sin lo oculto; también encuentra lo que lleva una
// etiqueta libre que coincide (DR-09).
func (a *Archivos) Search(ctx context.Context, q string) (api.Children, error) {
	q = buscar.Plegar(strings.TrimSpace(q))
	var out api.Children
	if q == "" {
		return out, nil
	}
	a.mu.Lock()
	etiquetadas := map[string]bool{}
	for r := range a.ind.Etiquetas {
		if a.conEtiqueta(r, q) {
			etiquetadas[r] = true
		}
	}
	a.mu.Unlock()
	fin := errors.New("fin")
	err := a.fs().WalkDir(a.raiz(), func(r string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // una carpeta ilegible no corta la búsqueda: se sigue con el resto
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if r == a.raiz() {
			return nil
		}
		if oculto(e.Name()) {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.Contains(buscar.Plegar(e.Name()), q) && !etiquetadas[r] {
			return nil
		}
		fi, err := e.Info()
		if err != nil {
			return nil //nolint:nilerr // lo que desaparece mientras se busca no se encuentra, sin más
		}
		if e.IsDir() {
			out.Folders = append(out.Folders, a.carpeta(r, fi))
		} else if fi.Mode().IsRegular() {
			out.Files = append(out.Files, a.fichero(r, fi))
		}
		if len(out.Folders)+len(out.Files) >= maxBusqueda {
			return fin
		}
		return nil
	})
	if err != nil && !errors.Is(err, fin) {
		return api.Children{}, err
	}
	return out, nil
}

// ----- Recientes y favoritos -----

// FileByPath resuelve una ruta (api.DrivePathResolver). Primero, relativa a la raíz
// («/PDF/contrato.pdf», la de Drive); si ahí no hay nada y es absoluta, la del disco: un
// fichero suelto que se abre sin copiarlo (D-24). Lo que se resuelve pasa a recientes.
func (a *Archivos) FileByPath(ctx context.Context, ruta string) (api.File, error) {
	if err := ctx.Err(); err != nil {
		return api.File{}, err
	}
	var candidatos []string
	rel := filepath.Clean(filepath.FromSlash("/" + strings.TrimLeft(filepath.ToSlash(ruta), "/")))
	if enRaiz := filepath.Join(a.raiz(), rel); dentro(a.raiz(), enRaiz) && enRaiz != a.raiz() && !dentro(a.meta(), enRaiz) {
		candidatos = append(candidatos, enRaiz)
	}
	if filepath.IsAbs(ruta) {
		if abs := filepath.Clean(ruta); !dentro(a.meta(), abs) {
			candidatos = append(candidatos, abs)
		}
	}
	for _, r := range candidatos {
		fi, err := a.fs().Stat(r)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if err := a.Recordar(r); err != nil {
			return api.File{}, err
		}
		return a.ficheroConSuma(r)
	}
	return api.File{}, noEsta(ruta)
}

// Recordar apunta un fichero como abierto ahora: sale el primero en recientes y él y su
// carpeta pasan a ser accesibles.
func (a *Archivos) Recordar(ruta string) error {
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	if dentro(a.meta(), abs) {
		return ErrFuera
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ind.Recientes = slices.DeleteFunc(a.ind.Recientes, func(x reciente) bool { return x.Ruta == abs })
	a.ind.Recientes = slices.Insert(a.ind.Recientes, 0, reciente{Ruta: abs, Abierto: a.ahora()})
	if len(a.ind.Recientes) > MaxRecientes {
		a.ind.Recientes = a.ind.Recientes[:MaxRecientes]
	}
	return a.guardarIndice()
}

// Olvidar quita un fichero de recientes (no lo borra del disco).
func (a *Archivos) Olvidar(ruta string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ind.Recientes = slices.DeleteFunc(a.ind.Recientes, func(x reciente) bool { return x.Ruta == ruta })
	return a.guardarIndice()
}

// Recent son los ficheros abiertos hace poco que siguen existiendo.
func (a *Archivos) Recent(ctx context.Context, _ string) (api.Compartidos, string, error) {
	a.mu.Lock()
	rs := slices.Clone(a.ind.Recientes)
	a.mu.Unlock()
	var out api.Compartidos
	for _, x := range rs {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		if fi, err := a.fs().Stat(x.Ruta); err == nil && fi.Mode().IsRegular() {
			out = append(out, api.Compartido{File: a.fichero(x.Ruta, fi)})
		}
	}
	return out, "", nil
}

// ListPDFs es lo que lista la portada de CertiPDF (api.PDFLister): los PDF recientes,
// estén donde estén, y los de la carpeta «PDF» de la raíz; el más reciente primero.
func (a *Archivos) ListPDFs(ctx context.Context) ([]api.File, error) {
	cuando := map[string]time.Time{}
	var out []api.File
	poner := func(r string, t time.Time) {
		if _, ya := cuando[r]; ya || !strings.EqualFold(filepath.Ext(r), ".pdf") {
			return
		}
		fi, err := a.fs().Stat(r)
		if err != nil || !fi.Mode().IsRegular() {
			return
		}
		f := a.fichero(r, fi)
		if m := fi.ModTime(); m.After(t) {
			t = m
		}
		cuando[r] = t
		out = append(out, f)
	}
	a.mu.Lock()
	rs := slices.Clone(a.ind.Recientes)
	a.mu.Unlock()
	for _, x := range rs {
		poner(x.Ruta, x.Abierto)
	}
	carpeta := filepath.Join(a.raiz(), api.PDFCarpeta)
	if es, err := a.fs().ReadDir(carpeta); err == nil {
		for _, e := range es {
			if !oculto(e.Name()) {
				poner(filepath.Join(carpeta, e.Name()), time.Time{})
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	slices.SortStableFunc(out, func(x, y api.File) int { return cuando[y.Path].Compare(cuando[x.Path]) })
	return out, nil
}

// Favorites son los marcados con estrella que siguen existiendo.
func (a *Archivos) Favorites(context.Context) (api.Compartidos, error) {
	a.mu.Lock()
	favoritos := slices.Clone(a.ind.Favoritos)
	a.mu.Unlock()
	var out api.Compartidos
	for _, r := range favoritos {
		fi, err := a.fs().Stat(r)
		switch {
		case err != nil:
		case fi.IsDir():
			c := a.carpeta(r, fi)
			out = append(out, api.Compartido{File: api.File{ID: c.ID, Name: c.Name, UpdatedAt: c.UpdatedAt, Path: r}, IsDir: true})
		case fi.Mode().IsRegular():
			out = append(out, api.Compartido{File: a.fichero(r, fi)})
		}
	}
	return out, nil
}

// SetFavorite pone o quita la estrella.
func (a *Archivos) SetFavorite(_ context.Context, _ api.DriveKind, id string, si bool) error {
	r, err := a.ruta(id)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ind.Favoritos = slices.DeleteFunc(a.ind.Favoritos, func(x string) bool { return x == r })
	if si {
		a.ind.Favoritos = append(a.ind.Favoritos, r)
	}
	return a.guardarIndice()
}

// ----- Papelera -----

// Delete manda a la papelera propia (`.sytue/papelera`). Se recupera con Restore.
func (a *Archivos) Delete(_ context.Context, k api.DriveKind, id string) error {
	r, err := a.ruta(id)
	if err != nil {
		return err
	}
	if r == a.raiz() {
		return ErrNombre
	}
	fi, err := a.fs().Stat(r)
	if err != nil || (k == api.DriveFolder) != fi.IsDir() {
		return noEsta(id)
	}
	clave := uuid() + filepath.Ext(r)
	destino := filepath.Join(a.meta(), "papelera", clave)
	if err := moverFichero(a.fs(), r, destino, fi); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ind.Papelera = append(a.ind.Papelera, enPapelera{
		Clave: clave, Ruta: r, Carpeta: fi.IsDir(), Borrado: a.ahora(),
		Etiquetas: a.sacarEtiquetas(r, fi.IsDir()),
	})
	a.ind.Recientes = slices.DeleteFunc(a.ind.Recientes, func(x reciente) bool { return x.Ruta == r })
	return a.guardarIndice()
}

// Restore devuelve algo de la papelera a donde estaba. El identificador es el que tenía.
func (a *Archivos) Restore(_ context.Context, _ api.DriveKind, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	raw, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return noEsta(id)
	}
	ruta := filepath.FromSlash(string(raw))
	i := slices.IndexFunc(a.ind.Papelera, func(x enPapelera) bool { return x.Ruta == ruta })
	if i < 0 {
		return noEsta(id)
	}
	x := a.ind.Papelera[i]
	if _, err := a.fs().Lstat(x.Ruta); err == nil {
		return conflicto(filepath.Base(x.Ruta))
	}
	origen := filepath.Join(a.meta(), "papelera", x.Clave)
	fi, err := a.fs().Stat(origen)
	if err != nil {
		return noEsta(id)
	}
	if err := a.fs().MkdirAll(filepath.Dir(x.Ruta), 0o755); err != nil {
		return err
	}
	if err := moverFichero(a.fs(), origen, x.Ruta, fi); err != nil {
		return err
	}
	a.ind.Papelera = slices.Delete(a.ind.Papelera, i, i+1)
	if len(x.Etiquetas) > 0 {
		if a.ind.Etiquetas == nil {
			a.ind.Etiquetas = map[string][]string{}
		}
		for k, ls := range x.Etiquetas {
			a.ind.Etiquetas[k] = ls
		}
	}
	return a.guardarIndice()
}

// Trash lista la papelera.
func (a *Archivos) Trash(context.Context) (api.DriveTrash, error) {
	a.mu.Lock()
	ps := slices.Clone(a.ind.Papelera)
	a.mu.Unlock()
	out := api.DriveTrash{RetentionSeconds: int64(RetencionPapelera / time.Second)}
	for _, x := range ps {
		fi, err := a.fs().Stat(filepath.Join(a.meta(), "papelera", x.Clave))
		if err != nil {
			continue
		}
		borrado := x.Borrado
		padre := idDe(filepath.Dir(x.Ruta))
		if x.Carpeta {
			out.Folders = append(out.Folders, api.Folder{ID: idDe(x.Ruta), ParentID: &padre, Name: filepath.Base(x.Ruta), UpdatedAt: fi.ModTime(), DeletedAt: &borrado, Path: x.Ruta})
			continue
		}
		out.Files = append(out.Files, api.File{
			ID: idDe(x.Ruta), FolderID: padre, Name: filepath.Base(x.Ruta), SizeBytes: fi.Size(),
			ContentType: tipoDe(x.Ruta), UpdatedAt: fi.ModTime(), DeletedAt: &borrado, Path: x.Ruta,
		})
	}
	return out, nil
}

// purgarPapelera borra lo que lleva más de RetencionPapelera. Con a.mu tomado.
func (a *Archivos) purgarPapelera() {
	limite := a.ahora().Add(-RetencionPapelera)
	n := len(a.ind.Papelera)
	a.ind.Papelera = slices.DeleteFunc(a.ind.Papelera, func(x enPapelera) bool {
		if x.Borrado.Before(limite) {
			_ = a.fs().RemoveAll(filepath.Join(a.meta(), "papelera", x.Clave))
			return true
		}
		return false
	})
	if len(a.ind.Papelera) != n {
		_ = a.guardarIndice()
	}
}

// moverFichero renombra y, entre discos distintos (o entre un fichero suelto del disco y
// un árbol de documentos), copia y borra (solo ficheros).
func moverFichero(s Sistema, de, a2 string, fi fs.FileInfo) error {
	err := s.Rename(de, a2)
	if err == nil || fi.IsDir() {
		return err
	}
	datos, rerr := s.ReadFile(de)
	if rerr != nil {
		return err
	}
	if werr := s.EscribirAtomico(a2, datos); werr != nil {
		return werr
	}
	return s.Remove(de)
}

func uuid() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ----- Lo que no hay en local -----

// SensitivityLabels: sin organización no hay etiquetas.
func (a *Archivos) SensitivityLabels(context.Context) ([]api.EtiquetaDrive, error) { return nil, nil }

// SetLabel: sin organización no hay etiquetas.
func (a *Archivos) SetLabel(context.Context, string, string) error { return ErrNoDisponible }
