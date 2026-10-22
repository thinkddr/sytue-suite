// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

// # Imagen dentro de imagen en el teléfono
//
// La del escritorio es una ventana aparte (apps/cast/pip.go); en el teléfono no hay otra
// ventana, y lo que hay es la del sistema: en Android, la app entera encogida en una
// esquina, con un botón de reproducir o pausar. ImagenEnImagen es esa pieza; fuera de
// Android no está disponible y nadie ofrece el botón.
type imagenEnImagen interface {
	// Disponible dice si este teléfono sabe hacerlo.
	Disponible() bool
	// Activa dice si la app está ahora en la esquina.
	Activa() bool
	// Preparar deja dicha la proporción del vídeo y si se reproduce: el botón de la
	// ventana y, en Android 12+, entrar sola al salir de la app con el vídeo en marcha.
	Preparar(ancho, alto int, reproduciendo bool)
	// Entrar encoge la app a la esquina ahora.
	Entrar(reproduciendo bool) bool
}

// Los sucesos de la ventana del sistema (los números del Java).
const (
	PiPAlternar = 1 // el botón de reproducir o pausar
	PiPCerrada  = 2 // la cerraron: la app ya no se ve
)
