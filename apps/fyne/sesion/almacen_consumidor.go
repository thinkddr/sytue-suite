// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ConsumidorAlmacenamiento derives the recipient from this installed app's
// registered return scheme. It does not grant a foreign Android UID authority.
func (s *Suite) ConsumidorAlmacenamiento(product string) (string, error) {
	if s == nil || s.app == nil || !storage.Product(product) {
		return "", auth.ErrTrabajoSesion
	}
	app, err := productoAplicacionNativa(s.app)
	if err != nil {
		return "", err
	}
	if app == product {
		return "", nil
	}
	if storage.NativeReturnApp(product, app) == "" || runtime.GOOS == "android" {
		return "", ajustes.ErrTrabajoCompartidoNoDisponible
	}
	return app, nil
}

func productoAplicacionNativa(app fyne.App) (string, error) {
	if app == nil {
		return "", auth.ErrTrabajoSesion
	}
	scheme := app.Metadata().Custom["esquema"]
	for _, product := range storage.Products() {
		if scheme == api.Esquema(storage.NativeApp(product)) {
			return product, nil
		}
	}
	return "", auth.ErrTrabajoSesion
}

// AlmacenConsumidor opens only this consumer's encrypted journal. iOS requires
// the approved suite App Group; a per-app preference fallback cannot represent
// closed apps during global logout. Android delegation remains unsupported.
func AlmacenConsumidor(ctx context.Context, consumer string) (*offline.Store, error) {
	if runtime.GOOS == "android" {
		return nil, ajustes.ErrTrabajoCompartidoNoDisponible
	}
	available, err := ajustes.EstadoTrabajoSesionCompartido()
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, ajustes.ErrTrabajoCompartidoNoDisponible
	}
	dir, err := directorioAlmacenConsumidor(consumer)
	if err != nil {
		return nil, err
	}
	return offline.OpenContext(ctx, dir)
}

func directorioAlmacenConsumidor(consumer string) (string, error) {
	if storage.NativeReturnApp("drive", consumer) == "" || consumer == "" {
		return "", auth.ErrTrabajoSesion
	}
	return directorioAlmacenAplicacion(consumer)
}

// Every native app owns one journal directory and its private OS key. This
// registry grants metadata enumeration only, never another app/UID's key access.
func directorioAlmacenAplicacion(app string) (string, error) {
	if !storage.Product(app) {
		return "", auth.ErrTrabajoSesion
	}
	base, err := ajustes.Carpeta()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "private-work", app, "offline"), nil
}

// This catalog enumerates only authorized suite-owned directories. Callers use
// offline metadata operations, without loading another app's encrypted store.

// The process may follow an app-wide physical relocation without keeping old
// HTTP transports pointed at the retired inode. This contains no SDK identity.
type almacenCache struct {
	store       *offline.Store
	dir, source string
}

type almacenPrivado struct {
	ruta      string
	productos []string
	// boveda: es el diario propio de la bóveda directa (diario_boveda.go), no
	// uno de los de Matrix.
	boveda bool
}

// directoriosAlmacenPrivado son todos los diarios privados del dispositivo: los
// de Matrix y, detrás, los de la bóveda directa. Contar pendientes antes de salir
// y descartar con permiso explícito pasan por aquí, así que el diario de la
// bóveda bloquea la salida y se borra con el descarte igual que los de Matrix.
func (s *Suite) directoriosAlmacenPrivado(ctx context.Context) ([]almacenPrivado, error) {
	dirs, err := s.directoriosDiarioMatrix(ctx)
	if err != nil {
		return nil, err
	}
	boveda, err := directoriosDiarioBoveda()
	if err != nil {
		return nil, err
	}
	return append(dirs, boveda...), nil
}

func (s *Suite) directoriosDiarioMatrix(ctx context.Context) ([]almacenPrivado, error) {
	legacy, err := s.directorioCacheActual(ctx)
	if err != nil {
		return nil, err
	}
	dirs := []almacenPrivado{{ruta: legacy, productos: storage.Products()}}
	if runtime.GOOS == "android" {
		return dirs, nil
	}
	available, err := ajustes.EstadoTrabajoSesionCompartido()
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, ajustes.ErrTrabajoCompartidoNoDisponible
	}
	for _, app := range storage.Products() {
		dir, err := directorioAlmacenAplicacion(app)
		if err != nil {
			return nil, err
		}
		if dir == legacy {
			continue
		}
		if _, err := offline.JournalRelocationVisible(ctx, dir); err != nil {
			return nil, err
		}
		products := []string{app}
		for _, product := range storage.Products() {
			if storage.NativeReturnApp(product, app) != "" {
				products = append(products, product)
			}
		}
		if dir != legacy {
			dirs = append(dirs, almacenPrivado{ruta: dir, productos: products})
		}
	}
	return dirs, nil
}

func (s *Suite) contarAlmacenPrivado(ctx context.Context, namespaces ...string) (int, error) {
	dirs, err := s.directoriosAlmacenPrivado(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, dir := range dirs {
		count, err := offline.CountDirectory(dir.ruta, namespaces...)
		if err != nil {
			return total, err
		}
		total += count
	}
	return total, nil
}

func (s *Suite) directorioCacheActual(ctx context.Context) (string, error) {
	if cached := s.cacheAlmacen.Load(); cached != nil {
		retired, err := offline.JournalRelocationRetired(ctx, cached.source, cached.dir)
		if err != nil {
			return "", err
		}
		if !retired {
			return "", offline.ErrPendingJournal
		}
		return cached.dir, nil
	}
	return directorioAlmacenActivo(ctx)
}

func (s *Suite) cacheReubicada(ctx context.Context) (bool, error) {
	if s.cacheAlmacen.Load() == nil && !almacenAplicacionSeparado {
		return false, nil
	}
	source, err := directorioAlmacen()
	if err != nil {
		return false, err
	}
	active, err := s.directorioCacheActual(ctx)
	return active != source, err
}

// After relocation, legacy Notes queues follow the same account boundary as
// native journals. Another person's retained drafts are never a logout wipe.
func (s *Suite) pendientesLegados(ctx context.Context, identity auth.IdentitySnapshot) (int, error) {
	moved, err := s.cacheReubicada(ctx)
	if err != nil {
		return 0, err
	}
	dirs, err := s.directoriosAlmacenPrivado(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for index, dir := range dirs {
		var count int
		if (index == 0 && !moved) || identity.Subject == "" || identity.Organization == "" {
			count, err = offline.CountDirectory(dir.ruta, notas.EspacioCapturas, notas.EspacioBorradores)
		} else {
			count, err = offline.CountDirectoryOwner(dir.ruta, identity.Subject+"/"+identity.Organization, notas.EspacioCapturas, notas.EspacioBorradores)
		}
		if err != nil {
			return total, err
		}
		total += count
	}
	return total, nil
}
