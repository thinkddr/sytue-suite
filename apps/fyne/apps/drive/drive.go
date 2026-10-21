// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package drive es Sytue Drive en Fyne: el explorador de ficheros y la papelera, con
// los widgets propios de `apps/fyne/ui` y sin una sola medida inventada.
//
// # Qué es la referencia
//
// `apps/frontend/apps/drive/src/components/FileBrowser.tsx`, y los números salen del
// barrido de producción (`test/e2e/medidas-web.mjs drive`, 1280×800, es-ES, DSF 1,
// corporate.sytue.eu v3.3.9). Cada constante de aquí lleva de dónde salió: «medido» es
// del volcado del DOM, «hoja» es de la clase de Tailwind convertida con el rem de 14.
//
// # La app es SUYA
//
// Un binario por producto, como manda `docs/CONVENTIONS.md` §13.6: aquí dentro solo hay
// destinos de Drive. Abrir un documento en Sytue Docs no es una pantalla más de este
// proceso, es una llamada al sistema por el esquema de URL de la otra app.
//
// # Sin conexión
//
// El contrato está en `docs/roadmap.md` §«Contrato offline de las apps nativas»: Drive
// navega por lo ya consultado y abre lo descargado, y TODA mutación —subir, mover,
// renombrar, papelera, favoritos, enlaces— se desactiva. No se encola nada: una subida
// a medias que se reintenta sola es como se duplica un fichero.
package drive

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	temafyne "fyne.io/fyne/v2/theme"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// paridad-web: apps/frontend/apps/drive/src/pages/index.astro

// Fase es en cuál de los estados está una pantalla: la de `api`, con alias para que
// las firmas de este paquete se lean sin `api.` delante.
//
// Estaba escrita aquí entera, y también en `notes` —idéntica hasta la coma— y en `erp`.
// La cuenta que la produce mira códigos HTTP, así que vive donde viven los códigos
// (`docs/CONVENTIONS.md` §13.2).
type Fase = api.Fase

// Las fases, con el cero en «cargando»: una pantalla recién construida todavía no ha
// pedido nada, y ese es el estado honesto.
const (
	Cargando    = api.FaseCargando
	Exito       = api.FaseExito
	Vacia       = api.FaseVacia
	Fallo       = api.FaseFallo
	SinSesion   = api.FaseSinSesion
	Prohibida   = api.FaseProhibida
	SinConexion = api.FaseSinConexion
)

// FaseDe traduce el resultado de una carga a la fase que toca pintar. Es `api.FaseDe`:
// el orden de las comprobaciones es el de la web y se decide una sola vez.
var FaseDe = api.FaseDe

// Elemento es una fila del explorador: lo que la pantalla necesita de una carpeta o de
// un fichero, ya unificado.
//
// La web tiene un `DriveResource` con ruta porque navega por URL; una app nativa navega
// por carpetas, así que aquí manda el ID y la ruta solo viene de lo que CRUZA el árbol
// (una búsqueda), donde sin ella dos ficheros con el mismo nombre son la misma fila.
type Elemento struct {
	ID      string
	Nombre  string
	Carpeta bool
	// Tamano es -1 en una carpeta: el servicio no lo da y la web escribe una raya.
	Tamano     int64
	Modificado time.Time
	// Ruta solo viene en una búsqueda y en la papelera.
	Ruta string
	// Purga es cuándo lo borrará solo el servicio. Cero: no hay cuenta atrás.
	Purga time.Time
	// Etiqueta es la de sensibilidad del fichero, si lleva: si prohíbe compartir fuera,
	// el diálogo de compartir lo dice al abrirse y no al fallar.
	Etiqueta *api.DriveLabel
	// Tipo es el `content_type` del fichero: «Subir a Cast» solo se ofrece en vídeos.
	Tipo string
}

// DeCarpeta convierte una carpeta del cliente en un Elemento.
func DeCarpeta(f api.Folder) Elemento {
	return Elemento{ID: f.ID, Nombre: f.Name, Carpeta: true, Tamano: -1, Modificado: f.UpdatedAt, Ruta: f.Path}
}

// DeFichero convierte un fichero del cliente en un Elemento.
func DeFichero(f api.File) Elemento {
	return Elemento{ID: f.ID, Nombre: f.Name, Tamano: f.SizeBytes, Modificado: f.UpdatedAt, Ruta: f.Path, Etiqueta: f.Label, Tipo: f.ContentType}
}

// DeHijos aplana una página de contenido en la lista que pinta la pantalla: primero las
// carpetas y luego los ficheros, que es el orden en que las devuelve el servicio y el
// que la web respeta.
func DeHijos(c api.Children) []Elemento {
	out := make([]Elemento, 0, len(c.Folders)+len(c.Files))
	for _, f := range c.Folders {
		out = append(out, DeCarpeta(f))
	}
	for _, f := range c.Files {
		out = append(out, DeFichero(f))
	}
	return out
}

// DePapelera aplana la papelera y le pone a cada elemento su cuenta atrás.
//
// La retención la da el servicio en segundos y se cuenta desde que se borró, no desde
// ahora: dos elementos borrados con una semana de diferencia se purgan con una semana de
// diferencia.
func DePapelera(p api.DriveTrash) []Elemento {
	out := make([]Elemento, 0, len(p.Folders)+len(p.Files))
	retencion := time.Duration(p.RetentionSeconds) * time.Second
	purga := func(borrado *time.Time) time.Time {
		if retencion <= 0 || borrado == nil {
			return time.Time{}
		}
		return borrado.Add(retencion)
	}
	for _, f := range p.Folders {
		e := DeCarpeta(f)
		e.Purga = purga(f.DeletedAt)
		out = append(out, e)
	}
	for _, f := range p.Files {
		e := DeFichero(f)
		e.Purga = purga(f.DeletedAt)
		out = append(out, e)
	}
	return out
}

// Extension es el `fileExt()` de la web: lo que va detrás del último punto, en
// minúsculas y como mucho cuatro letras, y nada si el nombre no tiene punto o la cola no
// parece una extensión.
func Extension(nombre string) string {
	i := strings.LastIndexByte(nombre, '.')
	if i <= 0 || i == len(nombre)-1 {
		return ""
	}
	ext := strings.ToLower(nombre[i+1:])
	if len(ext) > 8 {
		return "" // «archivo.copia de seguridad» no tiene extensión
	}
	for _, r := range ext {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return ext
}

// Tamano es el `formatSize` de la web: el número con el separador decimal del idioma y
// la unidad como SÍMBOLO, que no se traduce. Una carpeta escribe una raya.
func Tamano(l i18n.Locale, bytes int64) string {
	if bytes < 0 {
		return "—"
	}
	return i18n.FormatBytes(l, float64(bytes))
}

// Fecha es el `formatDate` de FileBrowser.tsx: `{ day: '2-digit', month: 'short', year:
// 'numeric' }`, «06 sept 2026» —con el día a dos cifras, que alinea la columna—, y no el
// `dateStyle: 'medium'` de la suite («6 sept 2026»). Medido en producción. Vacía si no hay.
func Fecha(l i18n.Locale, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return i18n.FormatDateShortMonth(l, t)
}

// ---- Piezas de composición de las pantallas ----

// Pila es `ui.Pila`: los hijos uno debajo de otro y uno de ellos elástico.
//
// Se movió a `apps/fyne/ui` el 2026-09-22 porque la necesitaba también Notepads y ya
// había una segunda versión —horizontal— dentro de `apps/photos`. El nombre se queda
// aquí como alias para no tocar las llamadas ni los guardianes de este paquete.
type Pila = ui.Pila

// SinRelleno es el tema de la suite con el relleno del TEMA a cero.
//
// Existe por una trampa medida: `widget.List` separa sus filas con
// `theme.Size(SizeNamePadding)` —7 px con este tema— y lo hace TAMBIÉN con
// `HideSeparators`. En una lista de 5.000 ficheros son 35.000 px de desvío. El cuerpo va
// envuelto en un `container.ThemeOverride` con esto, que es la vía que Fyne ofrece para
// cambiarle el tema a una rama del árbol sin tocar el de la aplicación.
type SinRelleno struct{ tema.Sytue }

var _ fyne.Theme = SinRelleno{}

// Size devuelve 0 para el relleno y lo de la suite para todo lo demás.
func (s SinRelleno) Size(n fyne.ThemeSizeName) float32 {
	if n == temafyne.SizeNamePadding {
		return 0
	}
	return s.Sytue.Size(n)
}
