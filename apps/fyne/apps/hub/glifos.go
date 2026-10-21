// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

// Los glifos PROPIOS del Hub: los siete trazados que `Launcher.tsx` declara él mismo
// porque no están en el catálogo compartido de `Icon.tsx`. Se copian verbatim —misma
// rejilla de 24, mismo trazo de 2, mismos remates— y `TestLosGlifosSonLosDeLaWeb` los
// compara con el fichero de la web, así que uno que cambie allí pone esto en rojo.
//
// La lupa NO está aquí: esa sí es del catálogo (`Icon name="search"`), y copiarla
// sería la segunda copia del mismo dibujo. Lo dice el propio comentario de la web.
const (
	GlifoCorreo   = "M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 2 9 6 9-6"
	GlifoFichero  = "M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6"
	GlifoSubir    = "M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 8l5-5 5 5M12 3v12"
	GlifoAgenda   = "M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z"
	GlifoDescarga = "M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"
	GlifoTelefono = "M7 2h10a1 1 0 0 1 1 1v18a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1Zm4 16h2"
	GlifoArchivo  = "M21 8v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V8M2 3h20v5H2zM10 12h4"
)

// Glifos es el mapa clave → trazado, en el orden de `GLYPHS`. Lo usa el guardián.
var Glifos = map[string]string{
	"mail":     GlifoCorreo,
	"file":     GlifoFichero,
	"upload":   GlifoSubir,
	"calendar": GlifoAgenda,
	"download": GlifoDescarga,
	"phone":    GlifoTelefono,
	"archive":  GlifoArchivo,
}
