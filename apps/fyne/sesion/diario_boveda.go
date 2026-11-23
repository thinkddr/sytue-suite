// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

// El diario de dispositivo propio de la bóveda directa (ADR 0086, «Drive nativo
// sobre la bóveda»). Las subidas pendientes y las intenciones de la bóveda no
// viven en el diario de Matrix: los dos usan los mismos espacios de nombres
// (`storage-file-uploads-v1`, `drive-private-migration-v1`…) con su destino dentro,
// y en el mismo almacén una intención de un backend bloquearía la del otro. Cada
// app instalada tiene su diario de la bóveda en un directorio aparte, junto al
// suyo de Matrix:
//
//	<ajustes>/private-work/<app>/pq-vault
//
// Esos directorios se cuentan al salir y se borran con el descarte explícito
// igual que los de Matrix (directoriosAlmacenPrivado), y la limpieza de la salida
// los vacía de todo lo que no es trabajo duradero (vaciarAlmacenCon).

import (
	"context"
	"path/filepath"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// directorioDiarioBoveda es el diario de la bóveda de una app instalada.
func directorioDiarioBoveda(app string) (string, error) {
	if !storage.Product(app) {
		return "", auth.ErrTrabajoSesion
	}
	base, err := ajustes.Carpeta()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "private-work", app, "pq-vault"), nil
}

// productosDeApp son los productos cuyo trabajo puede guardar una app: el suyo y
// aquellos de los que es consumidora (photos y certipdf guardan el de Drive).
func productosDeApp(app string) []string {
	products := []string{app}
	for _, product := range storage.Products() {
		if product != app && storage.NativeReturnApp(product, app) != "" {
			products = append(products, product)
		}
	}
	return products
}

// directoriosDiarioBoveda son los diarios de la bóveda de todas las apps. Un
// directorio que no existe se cuenta como vacío.
func directoriosDiarioBoveda() ([]almacenPrivado, error) {
	dirs := make([]almacenPrivado, 0, len(storage.Products()))
	for _, app := range storage.Products() {
		dir, err := directorioDiarioBoveda(app)
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, almacenPrivado{ruta: dir, productos: productosDeApp(app), boveda: true})
	}
	return dirs, nil
}

// PendientesPrivadosDe cuenta el trabajo duradero sin terminar de un producto de
// una cuenta, solo en los diarios de Matrix (boveda=false) o solo en los de la
// bóveda (boveda=true), en todas las apps del dispositivo. Lee solo rutas
// cifradas: no necesita ninguna clave. Un error es un error, nunca un cero.
func (s *Suite) PendientesPrivadosDe(ctx context.Context, identity auth.IdentitySnapshot, product string, boveda bool) (int, error) {
	return s.PendientesPrivadosEn(ctx, identity, product, boveda, append(storage.PrivateJournalNamespaces(product), storage.PendingUploadsNamespace)...)
}

// PendientesPrivadosEn es PendientesPrivadosDe solo en `namespaces` (por ejemplo,
// la copia de Drive de Matrix a la bóveda, `drive-vault-migration-v1`).
func (s *Suite) PendientesPrivadosEn(ctx context.Context, identity auth.IdentitySnapshot, product string, boveda bool, namespaces ...string) (int, error) {
	if !storage.Product(product) || identity.Subject == "" || identity.Organization == "" || len(namespaces) == 0 {
		return 0, auth.ErrTrabajoSesion
	}
	dirs, err := s.directoriosAlmacenPrivado(ctx)
	if err != nil {
		return 0, err
	}
	owner := identity.Organization + "\x00" + identity.Subject + "\x00" + product
	total := 0
	for _, dir := range dirs {
		if dir.boveda != boveda {
			continue
		}
		count, err := offline.CountDirectoryOwner(dir.ruta, owner, namespaces...)
		if err != nil {
			return total, err
		}
		total += count
	}
	return total, nil
}
