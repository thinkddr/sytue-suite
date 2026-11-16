// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos

package drive

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"golang.org/x/text/collate"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En qué orden se ve una carpeta de Drive, y qué se recuerda entre sesiones: la
// paridad de `lib/orden.ts` de la web, con la MISMA regla y las mismas claves.
//
// Las CARPETAS van delante en todos los órdenes, como en cualquier gestor de ficheros:
// ordenar por tamaño no debería esconder las carpetas entre los ficheros. El desempate
// es siempre por nombre, para que dos ficheros con la misma fecha no bailen entre
// recargas.
//
// «Mi unidad» pagina, así que la ordena el SERVICIO (`?orden=&sentido=`, keyset por
// columna): ordenar aquí solo ordenaría lo cargado y rompería la continuidad al
// encadenar páginas. `Ordenar` queda para lo que llega entero y para fijar en
// `testdata/orden.json` que los dos lados ordenan igual.

// CampoOrden es la columna por la que se ordena. Los valores son los de la web
// (`files.col.<campo>` en los catálogos), no los del servicio.
type CampoOrden string

const (
	OrdenNombre     CampoOrden = "name"
	OrdenModificado CampoOrden = "modified"
	OrdenTamano     CampoOrden = "size"
)

// CamposOrden son las tres, en el orden en que las ofrece el selector «Ordenar».
var CamposOrden = []CampoOrden{OrdenNombre, OrdenModificado, OrdenTamano}

// Orden es la columna y el sentido.
type Orden struct {
	Campo CampoOrden
	Desc  bool
}

// OrdenDelServicio es el de siempre, y el que el servicio aplica si no se le pide otro.
var OrdenDelServicio = Orden{Campo: OrdenNombre}

// Las preferencias guardan los mismos valores que `localStorage` en la web: quien lee
// una sabe leer la otra. La clave va con guiones y no con puntos: con forma de clave
// i18n, el guardián de catálogos la tomaría por un texto que traducir.
const (
	clavePrefOrden = "sytue-drive-orden"
	clavePrefVista = "sytue-drive-vista"
)

// SiguienteOrden es la pulsación sobre una columna: la misma invierte el sentido; otra
// empieza por lo que se busca casi siempre (nombre de la A a la Z; fecha y tamaño, lo
// más reciente y lo más grande primero).
func SiguienteOrden(actual Orden, campo CampoOrden) Orden {
	if actual.Campo == campo {
		return Orden{Campo: campo, Desc: !actual.Desc}
	}
	return Orden{Campo: campo, Desc: campo != OrdenNombre}
}

// sentido es `asc` o `desc`, como lo escriben la web y el servicio.
func (o Orden) sentido() string {
	if o.Desc {
		return "desc"
	}
	return "asc"
}

// campoDelServicio es cómo llama el servicio a cada campo.
var campoDelServicio = map[CampoOrden]string{
	OrdenNombre:     "nombre",
	OrdenModificado: "modificado",
	OrdenTamano:     "tamano",
}

// Listado es lo que se le pide al servicio. El orden de siempre no lleva parámetros:
// así la URL de la primera página es la de antes y siguen valiendo la caché sin
// conexión (`offline.Transport`) y los cursores ya emitidos.
func (o Orden) Listado() api.OrdenListado {
	if o == OrdenDelServicio {
		return api.OrdenListado{}
	}
	return api.OrdenListado{Campo: campoDelServicio[o.Campo], Sentido: o.sentido()}
}

// Ordenar ordena en su sitio: carpetas delante, luego el campo y, a igualdad, el
// nombre. Compara como `Intl.Collator(locale, {numeric: true, sensitivity: 'base'})`
// de la web: «informe 2» antes que «informe 10», sin distinguir tildes ni mayúsculas, y
// con las letras del idioma de quien mira (la «ö» sueca va detrás de la «z»).
func Ordenar(els []Elemento, o Orden, l i18n.Locale) {
	col := collate.New(l.Tag(), collate.Loose, collate.Numeric)
	porNombre := func(a, b Elemento) int { return col.CompareString(a.Nombre, b.Nombre) }
	var campo func(a, b Elemento) int
	switch o.Campo {
	case OrdenTamano:
		// Una carpeta no tiene tamaño (-1); entre carpetas da igual, que van aparte.
		campo = func(a, b Elemento) int { return cmpInt(max(a.Tamano, 0), max(b.Tamano, 0)) }
	case OrdenModificado:
		campo = func(a, b Elemento) int { return a.Modificado.Compare(b.Modificado) }
	default:
		campo = porNombre
	}
	signo := 1
	if o.Desc {
		signo = -1
	}
	slices.SortStableFunc(els, func(a, b Elemento) int {
		if a.Carpeta != b.Carpeta {
			if a.Carpeta {
				return -1
			}
			return 1
		}
		if c := signo * campo(a, b); c != 0 {
			return c
		}
		return porNombre(a, b)
	})
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Preferencias es lo que se necesita de `fyne.Preferences`: así las pruebas no
// dependen del almacén de la app.
type Preferencias interface {
	String(clave string) string
	SetString(clave, valor string)
}

// prefs da el almacén donde se recuerdan vista y orden. Es una variable para que las
// pruebas no se dejen recordado nada entre ellas: comparten una sola app.
var prefs = prefsDeLaApp

// prefsDeLaApp son las de la app, o nil si no hay app.
func prefsDeLaApp() Preferencias {
	if a := fyne.CurrentApp(); a != nil {
		return a.Preferences()
	}
	return nil
}

// LeerOrden devuelve el orden recordado; uno que no se entiende es el de siempre.
func LeerOrden(p Preferencias) Orden {
	if p == nil {
		return OrdenDelServicio
	}
	campo, sentido, _ := strings.Cut(p.String(clavePrefOrden), ":")
	if !slices.Contains(CamposOrden, CampoOrden(campo)) || (sentido != "asc" && sentido != "desc") {
		return OrdenDelServicio
	}
	return Orden{Campo: CampoOrden(campo), Desc: sentido == "desc"}
}

// GuardarOrden lo recuerda con el formato de la web (`modified:desc`).
func GuardarOrden(p Preferencias, o Orden) {
	if p != nil {
		p.SetString(clavePrefOrden, string(o.Campo)+":"+o.sentido())
	}
}

// LeerVista dice si se recuerda la cuadrícula; lo demás es la lista, como en la web.
func LeerVista(p Preferencias) bool {
	return p != nil && p.String(clavePrefVista) == "grid"
}

// GuardarVista la recuerda con los valores de la web (`list` o `grid`).
func GuardarVista(p Preferencias, rejilla bool) {
	if p == nil {
		return
	}
	v := "list"
	if rejilla {
		v = "grid"
	}
	p.SetString(clavePrefVista, v)
}
