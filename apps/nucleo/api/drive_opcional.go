// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import "context"

// # Lo que un DriveData puede añadir
//
// Interfaces OPCIONALES de la fachada privada (PLAN-EDICION-LOCAL §5.7 y Anexo A, DR-03):
// con `WithPrivateData`, Purge, CopyFolder, BatchMove, BatchTrash y Quota fallan cerrados
// porque `DriveData` no los tiene. Un repositorio que además cumpla una de estas se usa
// para ese método; uno que no, sigue fallando cerrado exactamente como antes. La cumple
// el Drive de Sytue Office (`local/archivos`); el repositorio cifrado, todavía no.

// DrivePurger borra de verdad lo que está en la papelera.
type DrivePurger interface {
	Purge(context.Context, DriveKind, string) error
}

// DriveFolderCopier copia una carpeta con su subárbol junto a la original.
type DriveFolderCopier interface {
	CopyFolder(context.Context, string) (Folder, error)
}

// DriveBatcher mueve y tira en lote. `destino` vacío es la raíz, como en el servicio.
type DriveBatcher interface {
	BatchMove(ctx context.Context, ficheros, carpetas []string, destino string) (DriveLote, error)
	BatchTrash(ctx context.Context, ficheros, carpetas []string) (DriveLote, error)
}

// DriveQuotaReader dice cuánto espacio hay; en el dispositivo, el del disco.
type DriveQuotaReader interface {
	Quota(context.Context) (DriveQuota, error)
}
