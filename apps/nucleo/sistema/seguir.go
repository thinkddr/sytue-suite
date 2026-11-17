// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

// SeguirEnFondo pide al sistema que deje terminar lo que la app tiene en marcha aunque
// pase a segundo plano: en iOS, `beginBackgroundTask` (unos 30 s, lo que iOS decida);
// en los demás, nada. Devuelve con qué avisar de que ya ha terminado, que se puede llamar
// más de una vez. Es para acabar UNA subida a medias, no para trabajar en segundo plano:
// eso es la tarea de refresco (apps/fyne/sesion).
func SeguirEnFondo(nombre string) (terminar func()) { return seguirEnFondo(nombre) }
