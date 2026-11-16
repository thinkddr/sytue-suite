// SPDX-License-Identifier: Apache-2.0
package ajustes

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	// ErrPersistenciaNoSoportada indicates that no directory barrier is
	// implemented for this platform; it is returned before any mutation.
	ErrPersistenciaNoSoportada = errors.New("ajustes: barrera de directorio no soportada")
	// ErrPersistenciaIncierta means an operation may already have changed the
	// filesystem, but its complete directory barrier was not confirmed.
	ErrPersistenciaIncierta = errors.New("ajustes: resultado de persistencia incierto")
	ErrRaizDurableCambiada  = errors.New("ajustes: la ruta de la raiz ha cambiado")
)

type operacionesDurables struct {
	syncDir   func(*os.File) error
	syncFile  func(*os.File) error
	closeDir  func(*os.File) error
	closeFile func(*os.File) error
	rename    func(*os.Root, string, string) error
	remove    func(*os.Root, string) error
}

func operacionesReales() operacionesDurables {
	return operacionesDurables{
		syncDir:   sincronizarDirectorio,
		syncFile:  (*os.File).Sync,
		closeDir:  (*os.File).Close,
		closeFile: (*os.File).Close,
		rename:    (*os.Root).Rename,
		remove:    (*os.Root).Remove,
	}
}

type enlaceDurable struct {
	padre  *os.Root
	hijo   *os.Root
	nombre string
}

// RaizDurable keeps the anchored directory descriptors alive. It does not own
// the caller's anchor, coordinate processes, or establish that anchor's own
// parent entry is durable. The caller must keep the anchor and its ancestors
// stable, prevent concurrent renames/unlinks of the path, and use a shared
// record lock across writes, reads, deletes and recovery. Identity checks below
// detect some path changes; they are not a substitute for that exclusion.
//
// Success means the required OS barriers returned successfully. It is not a
// guarantee against power loss on every filesystem or device.
type RaizDurable struct {
	mu      sync.Mutex
	raices  []*os.Root
	enlaces []enlaceDurable
	op      operacionesDurables
	cerrada bool
}

// AbrirRaizDurable opens or creates ruta beneath an already durable, trusted
// anchor. It syncs each directory and its parent, including already existing
// components. The anchor's own parent must have been made durable separately.
// Unsupported platforms fail before creating directories or files.
func AbrirRaizDurable(ancla *os.Root, ruta string) (*RaizDurable, error) {
	if !plataformaDirectorioDurable {
		return nil, ErrPersistenciaNoSoportada
	}
	return abrirRaizDurable(ancla, ruta, operacionesReales())
}

func (op operacionesDurables) valida() bool {
	return op.syncDir != nil && op.syncFile != nil && op.closeDir != nil && op.closeFile != nil && op.rename != nil && op.remove != nil
}

func validarRutaDurable(ruta string) bool {
	if !filepath.IsLocal(ruta) || filepath.Clean(ruta) != ruta || strings.ContainsRune(ruta, '\\') {
		return false
	}
	if ruta == "." {
		return true
	}
	componentes := strings.Split(ruta, string(filepath.Separator))
	// Keep held descriptors and repeated path checks bounded before mutation.
	if len(componentes) > 64 {
		return false
	}
	for _, nombre := range componentes {
		if !nombreDurable(nombre) {
			return false
		}
	}
	return true
}

func abrirRaizDurable(ancla *os.Root, ruta string, op operacionesDurables) (*RaizDurable, error) {
	if ancla == nil || !op.valida() || !validarRutaDurable(ruta) {
		return nil, fs.ErrInvalid
	}
	actual, err := ancla.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	r := &RaizDurable{raices: []*os.Root{actual}, op: op}
	cambiado := false
	fail := func(cause error) (*RaizDurable, error) {
		closeErr := r.cerrar()
		if cambiado {
			return nil, errors.Join(ErrPersistenciaIncierta, cause, closeErr)
		}
		return nil, errors.Join(cause, closeErr)
	}
	if err := r.syncRaiz(actual); err != nil {
		return fail(err)
	}
	limpia := filepath.Clean(ruta)
	if limpia == "." {
		return r, nil
	}
	for _, nombre := range strings.Split(limpia, string(filepath.Separator)) {
		hijo, pasoCambiado, err := r.avanzarComponente(actual, nombre)
		if pasoCambiado {
			cambiado = true
		}
		if err != nil {
			return fail(err)
		}
		actual = hijo
	}
	return r, nil
}

func (r *RaizDurable) avanzarComponente(actual *os.Root, nombre string) (*os.Root, bool, error) {
	cambiado := false
	if err := actual.Mkdir(nombre, permisoCarpeta); err == nil {
		cambiado = true
	} else if !errors.Is(err, fs.ErrExist) {
		// A filesystem may report an error after making a directory visible.
		return nil, true, err
	}
	info, err := actual.Lstat(nombre)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, cambiado, errors.Join(ErrRaizDurableCambiada, err)
	}
	hijo, err := actual.OpenRoot(nombre)
	if err != nil {
		return nil, cambiado, err
	}
	r.raices = append(r.raices, hijo)
	r.enlaces = append(r.enlaces, enlaceDurable{padre: actual, hijo: hijo, nombre: nombre})
	if err := r.syncRaiz(hijo); err != nil {
		return nil, cambiado, err
	}
	if err := r.syncRaiz(actual); err != nil {
		return nil, cambiado, err
	}
	if err := r.verificar(); err != nil {
		return nil, cambiado, err
	}
	return hijo, cambiado, nil
}

func nombreDurable(nombre string) bool {
	return nombre != "" && nombre != "." && nombre != ".." && filepath.IsLocal(nombre) && !strings.ContainsAny(nombre, "/\\\x00")
}

func (r *RaizDurable) syncRaiz(root *os.Root) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(r.op.syncDir(dir), r.op.closeDir(dir))
}

func (r *RaizDurable) verificar() error {
	for _, enlace := range r.enlaces {
		antes, err := enlace.padre.Lstat(enlace.nombre)
		if err != nil {
			return errors.Join(ErrRaizDurableCambiada, err)
		}
		if !antes.IsDir() || antes.Mode()&os.ModeSymlink != 0 {
			return ErrRaizDurableCambiada
		}
		actual, err := enlace.hijo.Stat(".")
		if err != nil || !os.SameFile(antes, actual) {
			return errors.Join(ErrRaizDurableCambiada, err)
		}
	}
	return nil
}

// Escribir replaces one immediate child. The caller supplies a process-shared
// record lock and must preserve datos until the return value is resolved. An
// uncertain rename keeps its temporary file if it still exists. Cleanup of a
// pre-rename failure is not itself synced, so crash recovery must also account
// for orphaned temporary names. This API provides no journal recovery policy.
func (r *RaizDurable) Escribir(nombre string, datos []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cerrada || !nombreDurable(nombre) {
		return fs.ErrInvalid
	}
	if err := r.verificar(); err != nil {
		return err
	}
	padre := r.raices[len(r.raices)-1]
	if err := r.syncRaiz(padre); err != nil {
		return err
	}
	intentado, err := escribirEnRaizDetalle(padre, nombre, datos, func(a, b string) error {
		return r.op.rename(padre, a, b)
	}, r.op.syncFile, r.op.closeFile, true)
	if err != nil {
		if intentado {
			return errors.Join(ErrPersistenciaIncierta, err)
		}
		return err
	}
	if err := r.syncRaiz(padre); err != nil {
		return errors.Join(ErrPersistenciaIncierta, err)
	}
	if err := r.verificar(); err != nil {
		return errors.Join(ErrPersistenciaIncierta, err)
	}
	return nil
}

// Borrar syncs the parent even if the name is already absent, so callers can
// resolve an uncertain earlier unlink. The caller must ensure that no newer
// generation can be removed by the retry.
func (r *RaizDurable) Borrar(nombre string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cerrada || !nombreDurable(nombre) {
		return fs.ErrInvalid
	}
	if err := r.verificar(); err != nil {
		return err
	}
	padre := r.raices[len(r.raices)-1]
	if err := r.syncRaiz(padre); err != nil {
		return err
	}
	if err := r.op.remove(padre, nombre); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.Join(ErrPersistenciaIncierta, err)
	}
	if err := r.syncRaiz(padre); err != nil {
		return errors.Join(ErrPersistenciaIncierta, err)
	}
	if err := r.verificar(); err != nil {
		return errors.Join(ErrPersistenciaIncierta, err)
	}
	return nil
}

func (r *RaizDurable) cerrar() error {
	var errs []error
	for i := len(r.raices) - 1; i >= 0; i-- {
		errs = append(errs, r.raices[i].Close())
	}
	r.raices = nil
	return errors.Join(errs...)
}

func (r *RaizDurable) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cerrada {
		return nil
	}
	r.cerrada = true
	return r.cerrar()
}
