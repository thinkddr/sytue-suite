// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"time"
)

// EL VOLCADO REIMPORTABLE (.onex): lo que hace falta para RESTAURAR un cuaderno.
//
// NO es el formato nativo —ese es el HTML, que se abre en cualquier navegador y
// también lleva el volcado dentro— sino el mismo dato SIN la página alrededor.
//
// Markdown, texto plano y PDF se llevan el contenido y pierden lo demás: dónde
// estaba cada nota en el lienzo, su ancho, el color de cada estado, los
// identificadores que sostienen los enlaces. Sirven para mirar un cuaderno en
// otro programa; no para devolverlo a su sitio.
//
// Es lo que se le pasa a un script, a una copia de seguridad o a otra
// instalación: el HTML nativo es para una persona; esto es para una máquina.
//
// Y hay una diferencia que no es de comodidad. En los formatos de lectura una
// nota CIFRADA no puede salir —el servidor no tiene la clave, y volcar el
// criptograma sería dar bytes que nadie abre—, así que una copia de seguridad de
// un cuaderno cifrado simplemente NO EXISTÍA. Aquí el criptograma viaja tal cual
// y lo descifra el navegador de su dueño al reimportarlo, que es quien tiene la
// clave. Ese es el motivo de que este formato exista.

// versionNativo va DENTRO del fichero, no en el nombre. Un fichero sin versión
// es un fichero que dentro de dos cambios de esquema nadie sabe si puede leer, y
// para entonces ya está en el disco de alguien.
const versionNativo = 1

// exportNativo es el sobre del volcado. Los nombres son los mismos que la API
// pone en el JSON de una nota, a propósito: quien ya trabaja con la API no
// aprende un vocabulario nuevo para leer el fichero.
type exportNativo struct {
	Formato     string    `json:"formato"`
	Version     int       `json:"version"`
	ExportadoEn time.Time `json:"exportado_en"`
	Notebook    Notebook  `json:"notebook"`
	Notas       []Note    `json:"notas"`
}

// NotebookANativo vuelca el cuaderno entero como JSON reimportable.
func NotebookANativo(nb Notebook, notas []Note) string {
	// El orden es el de pantalla, igual que en los otros formatos: un volcado que
	// al reimportarse coloca las notas en otro sitio no es una restauración.
	ordenadas := enOrdenDePantalla(notas)
	if ordenadas == nil {
		ordenadas = []Note{}
	}
	// EL TOKEN DEL ENLACE PÚBLICO NO SALE. Es el secreto que deja leer el cuaderno
	// SIN sesión —una credencial, no un dato del cuaderno— y su propio comentario
	// dice que solo se sirve a su dueño. Un volcado se guarda en un disco, se
	// manda por correo y se sube a una copia de seguridad: ahí dentro ese token
	// seguiría abriendo la puerta, y quien exporta no tiene por qué saberlo.
	//
	// Al reimportar se genera uno nuevo si hace falta, que además es lo correcto:
	// una copia restaurada no debería heredar los enlaces que ya circulaban.
	nb.PublicToken = ""

	doc := exportNativo{
		Formato: "sytue.notepads",
		Version: versionNativo,
		// UTC y no la hora local del servidor: el fichero se abre en cualquier
		// sitio y una marca sin zona no dice nada.
		ExportadoEn: time.Now().UTC(),
		Notebook:    nb,
		Notas:       ordenadas,
	}
	// Con sangría porque esto lo abre gente: un volcado de datos personales que
	// solo se puede leer pasándolo por otra herramienta es peor portabilidad.
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		// Marshal de estructuras sin canales ni funciones no falla; si algún día
		// lo hiciera, se prefiere un fichero vacío y visible a uno a medias que
		// parezca bueno.
		return ""
	}
	return string(b) + "\n"
}

// LeerNativo interpreta un fichero `.onex` y devuelve lo que hay que crear.
//
// SE VALIDA EL SOBRE ANTES QUE EL CONTENIDO. Un `.onex` puede venir de OneNote
// —que usa esa misma extensión para el índice de un cuaderno, y es binario— o de
// una versión futura de esto. En los dos casos hay que decirlo con claridad en
// vez de intentar leerlo y fallar por dentro con un error de JSON que no explica
// nada a quien solo arrastró un fichero.
func LeerNativo(datos []byte) (Notebook, []Note, error) {
	var doc exportNativo
	if err := json.Unmarshal(datos, &doc); err != nil {
		return Notebook{}, nil, ErrFormatoNoReconocido
	}
	if doc.Formato != "sytue.notepads" {
		return Notebook{}, nil, ErrFormatoNoReconocido
	}
	// Se acepta lo IGUAL O ANTERIOR, no lo posterior: un fichero de una versión
	// futura puede traer campos cuyo significado aquí se desconoce, y leerlo «a
	// medias» produciría un cuaderno que parece bien y no lo está.
	if doc.Version <= 0 || doc.Version > versionNativo {
		return Notebook{}, nil, ErrVersionNoSoportada
	}

	// El token del enlace público no se restaura NUNCA, venga o no en el fichero:
	// una copia no hereda los enlaces que ya circulaban. Y el dueño lo pone quien
	// importa, no el fichero: si no, cualquiera podría fabricar un `.onex` que
	// dijera pertenecer a otra persona.
	nb := doc.Notebook
	nb.PublicToken = ""
	nb.Tenant = ""
	nb.OwnerSub = ""

	notas := make([]Note, 0, len(doc.Notas))
	for _, n := range doc.Notas {
		n.Tenant = ""
		n.OwnerSub = ""
		notas = append(notas, n)
	}
	return nb, notas, nil
}
