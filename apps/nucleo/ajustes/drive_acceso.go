// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// FicheroAccesoDrive es la única marca no secreta que lee File Provider. Nunca
// contiene tokens, nombres, correos ni identificadores originales de la cuenta.
const FicheroAccesoDrive = "drive-acceso.json"

// AccesoDrive autoriza una identidad opaca y una revisión estable de su sesión.
type AccesoDrive = AccesoSesion

// LeerAccesoDrive falla cerrado: no se autoriza con una marca ausente o ilegible.
func LeerAccesoDrive() (AccesoDrive, error) {
	dir, err := Carpeta()
	if err != nil {
		return AccesoDrive{}, err
	}
	return leerAccesoDrive(filepath.Join(dir, FicheroAccesoDrive))
}

func leerAccesoDrive(ruta string) (AccesoDrive, error) { return leerAcceso(ruta) }

// PonerAccesoDrive conserva la revisión al renovar la misma identidad. Cerrar,
// cambiar de cuenta y volver a entrar la cambian, incluso A→B→A. La extensión
// cancela transferencias que ya no pertenecen a la autorización con que empezaron.
func PonerAccesoDrive(identidad string, activo bool) error {
	ctx, cancelar := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancelar()
	return PonerAccesoDriveCon(ctx, identidad, activo)
}

// PonerAccesoDriveCon usa un cerrojo propio: AlmacenSesion ya puede tener el de
// sesión. El plazo limita también cambios desde la interfaz cuando otro proceso sale.
func PonerAccesoDriveCon(ctx context.Context, identidad string, activo bool) (err error) {
	dir, err := Carpeta()
	if err != nil {
		return err
	}
	ruta := filepath.Join(dir, FicheroAccesoDrive)
	defer func() {
		if err != nil {
			err = retirarAccesoDriveTrasError(ruta, activo, err, os.Remove)
		}
	}()
	f, err := os.OpenFile(filepath.Join(dir, "drive-acceso.cerrojo"), os.O_CREATE|os.O_RDWR, permisoFichero)
	if err != nil {
		return err
	}
	soltar, err := CerrojoFichero(ctx, f)
	if err != nil {
		return err
	}
	defer soltar()
	return escribirAccesoDrive(ruta, identidad, activo, EscribirEntero, os.Remove)
}

func escribirAccesoDrive(ruta, identidad string, activo bool, escribir func(string, []byte) error, quitar func(string) error) error {
	return escribirAcceso(ruta, identidad, activo, escribir, quitar)
}

// Los escritores activos mantienen CerrojoSesion durante su publicación. Un
// cierre bajo ese mismo cerrojo puede retirar el archivo aunque la extensión
// retenga el cerrojo de la marca: ningún escritor activo podrá recrearlo después.
// Al desautorizar, que no quede archivo ya cumple el cierre; si tampoco se puede
// borrar, el error debe llegar a la persona para no afirmar un cierre inexistente.
func retirarAccesoDriveTrasError(ruta string, activo bool, causa error, quitar func(string) error) error {
	borrar := quitar(ruta)
	if errors.Is(borrar, os.ErrNotExist) {
		borrar = nil
	}
	if !activo && borrar == nil {
		return nil
	}
	return errors.Join(causa, borrar)
}
