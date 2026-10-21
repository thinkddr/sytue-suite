// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Un título de documento y un nombre de fichero NO son lo mismo, y esa distancia
// es la que se salva aquí.
//
// Un título es texto libre: puede llevar barras, saltos de línea pegados de otro
// sitio, ser vacío o medir un párrafo entero. Un nombre de fichero de Drive no
// admite nada de eso (`domain.ValidName`: sin `/ \ \x00`, sin caracteres de
// control, 255 bytes como mucho). Mandar el título tal cual habría dado un 400 de
// Drive justo en el momento de crear el documento —o sea, no poder crear un
// documento titulado «Ingresos 2026/2027»—, así que se sanea aquí y no se le
// pregunta a la persona por un nombre que ella no ha pedido poner.

// maxNombre es el tope de Drive, en BYTES (su comprobación es sobre `len`). Se
// deja hueco para la extensión y para el sufijo de desambiguación, porque el
// recorte se hace ANTES de añadirlos: recortar después partiría el «.odt».
const maxNombre = 200

// Saneado convierte un título de documento en un nombre de fichero admisible.
// No es reversible ni pretende serlo: el título de verdad sigue en Docs, esto
// solo es la etiqueta con la que el fichero se ve en la unidad.
func Saneado(titulo string) string {
	limpio := strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == 0:
			// El separador de ruta se sustituye por un guion en vez de borrarse:
			// «2026/2027» se lee mejor como «2026-2027» que como «20262027».
			return '-'
		case r < 0x20 || r == 0x7f:
			return ' '
		default:
			return r
		}
	}, titulo)
	// Espacios colapsados y extremos limpios: un nombre que empieza por espacio se
	// ve como si estuviera vacío en cualquier listado.
	limpio = strings.Join(strings.Fields(limpio), " ")
	// Se recortan también los guiones de los extremos, que casi nunca los puso
	// nadie: son los separadores de ruta que acaba de sustituir el paso anterior.
	// Un título «/» quedaría si no como un fichero llamado «-».
	limpio = strings.Trim(limpio, " .-")
	if limpio == "" {
		// Mismo texto que usa el dominio cuando no hay título (domain.Service.Create).
		limpio = "Sin título"
	}
	if len(limpio) > maxNombre {
		limpio = limpio[:maxNombre]
		// El corte por bytes puede caer en medio de un carácter multibyte y dejar
		// una secuencia UTF-8 inválida; se retrocede hasta el último válido.
		for limpio != "" && !utf8.ValidString(limpio) {
			limpio = limpio[:len(limpio)-1]
		}
		limpio = strings.TrimSpace(limpio)
	}
	return limpio
}

// NombreLibre devuelve «<título>.<ext>», o «<título> (2).<ext>» y siguientes si
// ese nombre ya está ocupado en la carpeta.
//
// POR QUÉ HACE FALTA: en Drive, subir con un nombre existente SUSTITUYE el
// fichero (nueva versión del mismo id). Sin esto, el segundo documento «Sin
// título» del día habría heredado el id del primero y —vía el índice único de
// `source_ref`, migración 0006— habría acabado siendo el primero.
//
// La comparación es sin distinguir mayúsculas porque así es como Drive resuelve
// el conflicto de nombres; comparar sensible aquí dejaría pasar «INFORME.odt»
// contra «informe.odt» y volveríamos a la sustitución silenciosa.
//
// ponytail: se recorre la lista que ya se pidió para localizar la carpeta, sin
// llamada extra. La lista de Drive viene paginada (500 por página) y aquí solo
// llega la primera: con más de 500 ficheros en la carpeta, un nombre repetido
// más allá de esa página no se ve y Drive lo trataría como sustitución. Si la
// carpeta de alguien crece tanto, lo que toca es preguntarle a Drive por el
// nombre concreto en vez de listar.
func NombreLibre(existentes []Fichero, titulo, ext string) string {
	base := Saneado(titulo)
	ocupados := make(map[string]struct{}, len(existentes))
	for _, f := range existentes {
		ocupados[strings.ToLower(f.Name)] = struct{}{}
	}
	candidato := base + "." + ext
	for n := 2; ; n++ {
		if _, ocupado := ocupados[strings.ToLower(candidato)]; !ocupado {
			return candidato
		}
		candidato = fmt.Sprintf("%s (%d).%s", base, n, ext)
	}
}

// SufijoDesambiguador devuelve el « (N)» que NombreLibre tuvo que añadir para no
// pisar un fichero existente, o cadena vacía si el nombre entró tal cual.
//
// PARA QUE EL TÍTULO Y EL FICHERO DIGAN LO MISMO. Sin esto, crear tres
// documentos en blanco seguidos deja tres ficheros bien distinguidos en la unidad
// —«Documento sin título», «… (2)», «… (3)»— y tres documentos con el MISMO
// título en la lista de Sytue Docs, que es donde la persona los busca. Se ve como
// que «no se creó nada»: están, pero no se distinguen.
//
// SE DEVUELVE EL SUFIJO Y NO EL NOMBRE ENTERO a propósito. El nombre del fichero
// pasó por `Saneado`, que cambia lo que no vale en un nombre de fichero: usarlo
// como título convertiría «Informe 2026/2027» en «Informe 2026-2027» a espaldas
// de quien lo escribió. El sufijo, en cambio, es información nueva —«este es el
// segundo»— y se puede añadir al título original sin tocar el resto.
func SufijoDesambiguador(titulo, ext, nombreFinal string) string {
	base := Saneado(titulo)
	sinExt := strings.TrimSuffix(nombreFinal, "."+ext)
	if sinExt == base || !strings.HasPrefix(sinExt, base+" (") || !strings.HasSuffix(sinExt, ")") {
		return ""
	}
	return sinExt[len(base):]
}
