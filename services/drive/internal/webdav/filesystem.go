// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package webdav

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/webdav"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

// servicio es lo que WebDAV usa de Drive (lo cumple *service.Service). Está para
// poder probar el protocolo con el manejador de verdad y un Drive de mentira.
type servicio interface {
	ResolvePath(ctx context.Context, p string) (service.Node, error)
	ListChildren(ctx context.Context, id uuid.UUID) ([]domain.Folder, []domain.File, error)
	OpenContentByFile(ctx context.Context, f domain.File) (io.ReadCloser, error)
	WritePath(ctx context.Context, p, contentType string, body io.Reader) (domain.File, error)
	MkdirPath(ctx context.Context, p string) error
	RemovePath(ctx context.Context, p string) error
	RenamePath(ctx context.Context, oldPath, newPath string) error
}

// FileSystem implementa webdav.FileSystem sobre el servicio de Drive. El tenant
// se toma del contexto de cada llamada (inyectado por el middleware authn), de
// modo que el mismo FileSystem sirve a todos los tenants con aislamiento RLS.
type FileSystem struct {
	svc servicio
}

// NewFileSystem construye el FileSystem WebDAV de Drive.
func NewFileSystem(svc *service.Service) *FileSystem { return &FileSystem{svc: svc} }

var _ webdav.FileSystem = (*FileSystem)(nil)

// toOSError traduce errores de dominio a los errores que webdav espera (os.*).
func toOSError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return os.ErrNotExist
	case errors.Is(err, domain.ErrConflict):
		return os.ErrExist
	case errors.Is(err, domain.ErrInvalidName), errors.Is(err, domain.ErrInvalidParent):
		return os.ErrInvalid
	case errors.Is(err, domain.ErrPermission), errors.Is(err, domain.ErrRootImmutable):
		return os.ErrPermission
	default:
		return err
	}
}

// Mkdir crea una carpeta (MKCOL).
func (fs *FileSystem) Mkdir(ctx context.Context, name string, _ os.FileMode) error {
	ctx = ctxWithActor(ctx)
	return toOSError(fs.svc.MkdirPath(ctx, name))
}

// RemoveAll borra un recurso y su subárbol (DELETE).
func (fs *FileSystem) RemoveAll(ctx context.Context, name string) error {
	ctx = ctxWithActor(ctx)
	if path.Clean("/"+name) == "/" {
		return os.ErrPermission // no borrar la raíz
	}
	return toOSError(fs.svc.RemovePath(ctx, name))
}

// Rename mueve/renombra un recurso (MOVE).
func (fs *FileSystem) Rename(ctx context.Context, oldName, newName string) error {
	ctx = ctxWithActor(ctx)
	return toOSError(fs.svc.RenamePath(ctx, oldName, newName))
}

// Stat devuelve la información de un recurso (PROPFIND de profundidad 0).
func (fs *FileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	if moveSrc := origenMoveDe(ctx); moveSrc != "" {
		cleanDst := path.Clean("/" + name)
		cleanSrc := path.Clean("/" + moveSrc)
		if cleanDst != cleanSrc && strings.EqualFold(cleanDst, cleanSrc) {
			return nil, os.ErrNotExist
		}
	}
	node, err := fs.svc.ResolvePath(ctx, name)
	if err != nil {
		return nil, toOSError(err)
	}
	return fileInfoFromNode(node), nil
}

// OpenFile abre un recurso para lectura o escritura según los flags.
func (fs *FileSystem) OpenFile(ctx context.Context, name string, flag int, _ os.FileMode) (webdav.File, error) {
	ctx = ctxWithActor(ctx)
	// PROPPATCH abre con os.O_RDWR sin O_CREATE ni O_TRUNC solo para leer/parchear
	// propiedades muertas; toda escritura de contenido (PUT, COPY, LOCK sobre
	// recurso nuevo) incluye O_WRONLY, O_CREATE u O_TRUNC.
	writing := flag&(os.O_WRONLY|os.O_CREATE|os.O_TRUNC) != 0

	if writing {
		// Apertura para escritura: el contenido sube a S3 mientras se escribe (writeFile).
		return newWriteFile(ctx, fs.svc, name), nil
	}

	node, err := fs.svc.ResolvePath(ctx, name)
	if err != nil {
		return nil, toOSError(err)
	}
	if node.IsDir() {
		// Directorio: GET lista hijos; Readdir los enumera.
		return fs.newDirFile(ctx, node), nil
	}
	// El blob NO se abre aquí, sino en la primera lectura: la biblioteca abre cada
	// entrada de un listado (PROPFIND) solo para preguntarle sus propiedades, y
	// abrirlo de verdad era una petición al almacén por fichero y por listado.
	return &readFile{info: fileInfoFromFile(*node.File), abrir: func() (io.ReadCloser, error) {
		rc, err := fs.svc.OpenContentByFile(ctx, *node.File)
		return rc, toOSError(err)
	}}, nil
}

// newDirFile crea un webdav.File de solo lectura que enumera los hijos. Como el
// blob de un fichero, los hijos se piden cuando alguien los lee (Readdir), no al
// abrir: en un listado cada subcarpeta se abre solo para sus propiedades.
func (fs *FileSystem) newDirFile(ctx context.Context, node service.Node) *dirFile {
	return &dirFile{info: fileInfoFromNode(node), listar: func() ([]os.FileInfo, error) {
		folders, files, err := fs.svc.ListChildren(ctx, node.Folder.ID)
		if err != nil {
			return nil, err
		}
		hijos := make([]os.FileInfo, 0, len(folders)+len(files))
		for _, f := range folders {
			hijos = append(hijos, fileInfoFromFolder(f))
		}
		for _, f := range files {
			hijos = append(hijos, fileInfoFromFile(f))
		}
		return hijos, nil
	}}
}

// ---------------------------------------------------------------------------
// os.FileInfo
// ---------------------------------------------------------------------------

type fileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	tipo    string
}

// ContentType da a PROPFIND el tipo que Drive ya tiene guardado. Sin esto la
// biblioteca ABRE cada fichero del listado para deducirlo, y con una extensión que
// no conoce lee su principio y rebobina: una descarga del almacén por fichero y
// por listado.
func (fi fileInfo) ContentType(context.Context) (string, error) {
	if fi.tipo == "" {
		return "application/octet-stream", nil
	}
	return fi.tipo, nil
}

var _ webdav.ContentTyper = fileInfo{}

func (fi fileInfo) Name() string       { return fi.name }
func (fi fileInfo) Size() int64        { return fi.size }
func (fi fileInfo) Mode() os.FileMode  { return fi.mode }
func (fi fileInfo) ModTime() time.Time { return fi.modTime }
func (fi fileInfo) IsDir() bool        { return fi.isDir }
func (fi fileInfo) Sys() any           { return nil }

func fileInfoFromNode(n service.Node) fileInfo {
	if n.IsDir() {
		return fileInfoFromFolder(*n.Folder)
	}
	return fileInfoFromFile(*n.File)
}

func fileInfoFromFolder(f domain.Folder) fileInfo {
	name := f.Name
	if f.IsRoot {
		name = "/"
	}
	return fileInfo{name: name, mode: os.ModeDir | 0o755, modTime: f.UpdatedAt, isDir: true}
}

func fileInfoFromFile(f domain.File) fileInfo {
	return fileInfo{name: f.Name, size: f.SizeBytes, mode: 0o644, modTime: f.UpdatedAt, tipo: f.ContentType}
}

// ---------------------------------------------------------------------------
// webdav.File: lectura de fichero
// ---------------------------------------------------------------------------

// readFile lee el blob en streaming y deja buscar en él sin tenerlo en memoria.
//
// La biblioteca sirve GET con http.ServeContent, que antes de mandar nada pregunta
// el tamaño yendo al final y volviendo (Seek), y con una cabecera Range salta al
// principio del tramo. Seek solo apunta dónde se quiere leer; es Read quien mueve
// el flujo hasta allí.
//
// ponytail: avanzar descarta bytes y retroceder reabre el blob. Vale para lo que
// pide un cliente WebDAV (el fichero entero, o reanudar una descarga); si algún
// día se sirve vídeo por aquí, pedir el tramo al almacén (objectstore.GetRange).
type readFile struct {
	info  fileInfo
	abrir func() (io.ReadCloser, error)
	rc    io.ReadCloser // nil hasta la primera lectura
	pos   int64         // dónde quiere leer quien llama
	en    int64         // por dónde va rc
}

func (f *readFile) Close() error {
	if f.rc == nil {
		return nil
	}
	return f.rc.Close()
}

func (f *readFile) Read(p []byte) (int, error) {
	if f.pos >= f.info.size {
		return 0, io.EOF
	}
	if f.rc == nil || f.en > f.pos {
		_ = f.Close()
		rc, err := f.abrir()
		if err != nil {
			return 0, err
		}
		f.rc, f.en = rc, 0
	}
	if f.en < f.pos {
		n, err := io.CopyN(io.Discard, f.rc, f.pos-f.en)
		f.en += n
		if err != nil {
			return 0, err
		}
	}
	n, err := f.rc.Read(p)
	f.pos += int64(n)
	f.en += int64(n)
	return n, err
}
func (f *readFile) Write([]byte) (int, error) { return 0, os.ErrPermission }
func (f *readFile) Readdir(int) ([]os.FileInfo, error) {
	return nil, errors.New("webdav: no es un directorio")
}
func (f *readFile) Stat() (os.FileInfo, error) { return f.info, nil }

func (f *readFile) DeadProps() (map[xml.Name]webdav.Property, error) {
	return map[xml.Name]webdav.Property{}, nil
}

func (f *readFile) Patch(p []webdav.Proppatch) ([]webdav.Propstat, error) {
	return aceptarProppatch(p), nil
}

func (f *readFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += f.pos
	case io.SeekEnd:
		offset += f.info.size
	default:
		return f.pos, errors.New("webdav: seek desde un origen desconocido")
	}
	if offset < 0 {
		return f.pos, errors.New("webdav: seek antes del principio")
	}
	f.pos = offset
	return offset, nil
}

// ---------------------------------------------------------------------------
// webdav.File: directorio
// ---------------------------------------------------------------------------

type dirFile struct {
	info     fileInfo
	listar   func() ([]os.FileInfo, error)
	children []os.FileInfo
	listado  bool
	offset   int
}

func (d *dirFile) Close() error              { return nil }
func (d *dirFile) Read([]byte) (int, error)  { return 0, errors.New("webdav: es un directorio") }
func (d *dirFile) Write([]byte) (int, error) { return 0, os.ErrPermission }
func (d *dirFile) Seek(int64, int) (int64, error) {
	return 0, errors.New("webdav: seek en directorio")
}
func (d *dirFile) Stat() (os.FileInfo, error) { return d.info, nil }

func (d *dirFile) DeadProps() (map[xml.Name]webdav.Property, error) {
	return map[xml.Name]webdav.Property{}, nil
}

func (d *dirFile) Patch(p []webdav.Proppatch) ([]webdav.Propstat, error) {
	return aceptarProppatch(p), nil
}

func aceptarProppatch(patches []webdav.Proppatch) []webdav.Propstat {
	ps := webdav.Propstat{Status: http.StatusOK}
	for _, patch := range patches {
		for _, prop := range patch.Props {
			ps.Props = append(ps.Props, webdav.Property{XMLName: prop.XMLName})
		}
	}
	return []webdav.Propstat{ps}
}

func (d *dirFile) Readdir(count int) ([]os.FileInfo, error) {
	if !d.listado {
		hijos, err := d.listar()
		if err != nil {
			return nil, err
		}
		d.children, d.listado = hijos, true
	}
	if count <= 0 {
		rest := d.children[d.offset:]
		d.offset = len(d.children)
		return rest, nil
	}
	if d.offset >= len(d.children) {
		return nil, io.EOF
	}
	end := d.offset + count
	if end > len(d.children) {
		end = len(d.children)
	}
	batch := d.children[d.offset:end]
	d.offset = end
	return batch, nil
}

// ---------------------------------------------------------------------------
// webdav.File: escritura (PUT). Bufferiza y sube a S3 al cerrar.
// ---------------------------------------------------------------------------

// writeFile sube lo que escribe el cliente MIENTRAS lo escribe: WritePath lee del otro
// extremo de una tubería y sube en partes (putBlob). Antes se juntaba el fichero entero
// en memoria y se subía al cerrar, y el tope de cuerpo es de gigas en un pod de 512 MiB
// (FILE-06). COPY pasa por aquí igual: la biblioteca abre el destino y copia en él.
type writeFile struct {
	ctx  context.Context
	svc  servicio
	name string
	pw   *io.PipeWriter // nil hasta que empieza la subida
	res  chan error     // lo que devolvió WritePath
	n    int64          // bytes escritos, para Stat
	done bool
}

func newWriteFile(ctx context.Context, svc servicio, name string) *writeFile {
	return &writeFile{ctx: ctx, svc: svc, name: name}
}

// subir arranca WritePath sobre la tubería.
func (f *writeFile) subir() {
	pr, pw := io.Pipe()
	f.pw, f.res = pw, make(chan error, 1)
	go func() {
		_, err := f.svc.WritePath(f.ctx, f.name, contentTypeForName(f.name), pr)
		// Si WritePath acaba sin leerlo todo (un error), lo que se siga escribiendo
		// falla en vez de esperar a un lector que ya no está.
		if err == nil {
			_ = pr.CloseWithError(io.ErrClosedPipe)
		} else {
			_ = pr.CloseWithError(err)
		}
		f.res <- err
	}()
}

func (f *writeFile) Write(p []byte) (int, error) {
	if f.done {
		return 0, os.ErrClosed
	}
	if f.pw == nil {
		f.subir()
	}
	n, err := f.pw.Write(p)
	f.n += int64(n)
	return n, err
}

func (f *writeFile) Read([]byte) (int, error) { return 0, os.ErrPermission }
func (f *writeFile) Readdir(int) ([]os.FileInfo, error) {
	return nil, errors.New("webdav: no es un directorio")
}

// Seek en escritura: solo se acepta el reset inicial (offset 0) que algunos
// clientes envían antes de empezar a escribir.
func (f *writeFile) Seek(offset int64, whence int) (int64, error) {
	if offset == 0 && (whence == io.SeekStart || whence == io.SeekCurrent) && f.n == 0 {
		return 0, nil
	}
	return 0, errors.New("webdav: seek no soportado en escritura")
}

func (f *writeFile) Stat() (os.FileInfo, error) {
	// Antes de cerrar, lo escrito hasta ahora (la biblioteca lo pide antes de Close).
	return fileInfo{name: path.Base(f.name), size: f.n, mode: 0o644, modTime: time.Now()}, nil
}

func (f *writeFile) Close() error {
	if f.done {
		return nil
	}
	f.done = true
	if f.pw == nil {
		f.subir() // un fichero vacío también se crea
	}
	_ = f.pw.Close()
	return toOSError(<-f.res)
}

// contentTypeForName deriva un Content-Type de la extensión (best effort).
func contentTypeForName(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
