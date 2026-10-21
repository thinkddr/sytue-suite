// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	_ "embed"
	"net/http"
)

// La marca de Sytue ID, vendorizada desde branding/marks/id.svg por
// scripts/sync-branding.sh. No se dibuja a mano aquí: los favicons escritos a
// mano ya acabaron una vez con siete colores distintos entre superficies.
//
// POR QUÉ EXISTE ESTE FICHERO. Sytue ID no declaraba NINGÚN icono —comprobado
// con navegador el 2026-09-08: `document.querySelectorAll('link[rel~=icon]')`
// vacío— y en la raíz de su host tampoco había respaldo (`/favicon.ico` y
// `/favicon.svg`, 404 los dos). O sea que la pantalla que más se ve de la suite
// salía con el icono en blanco del navegador: entrar en cualquier app sin
// sesión —`corporate.sytue.eu/mail`, por ejemplo— termina exactamente aquí.
//
//go:embed marca.svg
var marcaSVG []byte

// handleFavicon sirve la marca. `image/svg+xml` y no `image/x-icon`: lo que
// decide qué pinta el navegador es el tipo, no la extensión de la URL.
func handleFavicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	// Un icono no es dato de sesión. El `no-store` general del IdP está para las
	// páginas con credenciales dentro; aplicarlo aquí haría que cada pestaña se
	// lo volviera a bajar sin ganar nada.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(marcaSVG)
}

// La misma marca en formato ICO multi-resolución (256…16), generada por
// scripts/sync-branding.sh desde branding/marks/id.svg con ImageMagick.
//
//go:embed marca.ico
var marcaICO []byte

// handleFaviconICO atiende la petición que el navegador hace SOLO. Cuando una
// página no declara icono, el navegador pide `/favicon.ico` a la raíz del host
// por su cuenta: es de los tiempos en que `.ico` era el único formato. Aquí ya
// se declara el SVG, así que esto es para lo que no parsea HTML —desplegables de
// enlaces, lectores, atajos del sistema—.
//
// SIRVE UN ICO DE VERDAD, y antes redirigía al SVG. La redirección cubría a casi
// todos esos consumidores, pero no al único que exige el formato: Windows no
// acepta un SVG para anclar a la barra de tareas, así que ahí el icono seguía
// saliendo en blanco. Es el mismo motivo por el que las once superficies de
// Astro llevan su `favicon.ico` en `public/` en vez de una redirección.
func handleFaviconICO(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(marcaICO)
}

// La misma marca como PNG de 180×180 para Safari e iOS, generada por
// scripts/sync-branding.sh desde branding/marks/id-maskable.svg (color a
// sangre: iOS recorta con su máscara y rellena de negro lo transparente).
//
//go:embed marca-apple.png
var marcaApple []byte

// handleAppleTouchIcon — Safari no rasteriza SVG ni para la pestaña ni para
// «añadir a inicio», y pide `/apple-touch-icon.png` a la raíz del host aunque
// la página no lo declare. Sin esto el IdP salía sin icono en Safari, que es
// justo el navegador de la mitad de los móviles.
func handleAppleTouchIcon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(marcaApple)
}
