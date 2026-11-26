// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen

// CerrojoExclusivo toma el cerrojo EXCLUSIVO del almacén de dir, sin esperar: falla con
// [ErrEnUso] si alguna instancia lo tiene abierto (su cerrojo compartido) o lo está
// compactando. Mientras se tiene, nadie lo compacta ni lo abre ([AbrirPuerto] espera
// [EsperaCerrojo] y luego falla). Es para quien se lleva el almacén entero a otro sitio
// —mover la carpeta de datos—, que necesita las apps cerradas. Donde el sistema no da
// cerrojos también falla con ErrEnUso: no se puede saber si alguien lo tiene. Hay que
// llamar a soltar al acabar.
func CerrojoExclusivo(dir string) (soltar func(), err error) {
	return tomarCerrojo(dir, false, esperaExclusivo)
}

// FicheroCerrojo es el nombre del fichero del cerrojo dentro de la carpeta del almacén:
// quien copia el almacén entero no lo lee (en Windows está bloqueado), lo crea vacío.
const FicheroCerrojo = ficheroCerrojo
