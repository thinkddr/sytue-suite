// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import "git.sytue.eu/sytue/one/apps/fyne/anfitrion"

// El selector de ficheros del móvil (`ficheros.EnElMovil` con el del toolkit, las copias
// en la carpeta temporal y el filtro por tipo que abre la galería) vive en `anfitrion`
// (elegir.go): no es sesión, y Sytue Office, que no enlaza `sesion`, también lo necesita.
// Aquí solo se delega, sin cambiar lo que hace.

// ponerSelector hace que `ficheros.Elegir` abra en el móvil el selector del toolkit; en
// escritorio no hace nada (anfitrion.PonerSelector).
func (s *Suite) ponerSelector() { anfitrion.PonerSelector(s.app) }

// SoltarElegido borra la copia que dejó el selector del teléfono, si `ruta` es una
// (anfitrion.SoltarElegido).
func SoltarElegido(ruta string) { anfitrion.SoltarElegido(ruta) }
