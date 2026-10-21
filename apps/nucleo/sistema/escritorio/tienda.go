// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build tienda

package escritorio

// DeTienda dice que este binario es la variante de la Mac App Store: se compila con la
// etiqueta `tienda` (`empaquetar.sh macappstore`), corre dentro del App Sandbox y se
// actualiza SOLO por la tienda. Con ella no hay actualizador propio ([Plataforma] es
// vacía y el montaje del DMG no se compila), la sesión va al llavero de protección de
// datos (apps/nucleo/auth), lo transversal a un contenedor de grupo (apps/nucleo/ajustes)
// y los ficheros se eligen con el panel del sistema dentro del proceso
// (apps/nucleo/ficheros). Ver «Mac App Store» en docs/operations.md.
const DeTienda = true
