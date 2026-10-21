// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

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

// AImportar convierte lo leído de un `.onex` en lo que las funciones de creación
// esperan.
//
// Va aquí y no en el store porque es una traducción entre dos formas del mismo
// dato, sin base de datos de por medio: así se puede probar sin Postgres, que es
// lo que hace que se pruebe.
//
// Todo lo que decide el SERVIDOR se deja fuera —identificadores, dueño, fechas—:
// el fichero describe un cuaderno, no reclama un sitio concreto en la base de
// datos de nadie.
func (nb Notebook) AImportar() NotebookInput {
	return NotebookInput{
		Title:      nb.Title,
		Emoji:      nb.Emoji,
		Color:      nb.Color,
		Layout:     nb.Layout,
		Visibility: nb.Visibility,
		Indexable:  &nb.Indexable,
		Encrypted:  nb.Encrypted,
		Crypto:     nb.Crypto,
		// ParentID y RoomID NO se restauran: apuntan a un cuaderno padre y a una
		// sala de ESTA instalación, y un fichero traído de otra parte los tendría
		// colgando de identificadores que aquí no existen. El cuaderno entra a la
		// raíz y quien lo importa lo coloca.
	}
}

// AImportar hace lo propio con una nota, conservando lo que la sitúa en el
// lienzo: sin `Col`, `Position` y `Width` la restauración recoloca todo, y eso no
// es restaurar.
func (n Note) AImportar() NoteInput {
	data := n.Data
	if n.ContentCipher != "" {
		// Task execution controls refer to the original objects, not the new IDs
		// allocated by an import. They are not writable archive payload.
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) == nil && fields != nil {
			delete(fields, "repeated_into")
			delete(fields, "carried_from")
			delete(fields, "carry_count")
			data, _ = json.Marshal(fields)
		}
	}
	x, y, col, pos := n.X, n.Y, n.Col, n.Position
	// UN ANCHO A CERO NO ES UN ANCHO: es una nota que el fichero trae sin él, y se
	// deja a nil para que la creación ponga el de siempre. Con `&w` a cero, la
	// validación (80..1200) tiraba la nota entera —y con ella cualquier `.onex`
	// escrito por un script que no supiera de anchos— contándola como
	// «descartada» sin más explicación.
	var w *int
	if n.Width > 0 {
		ancho := n.Width
		w = &ancho
	}
	cancelada := n.CancelledAt != nil
	return NoteInput{
		CipherVersion: EncryptedNoteWriteVersion,
		Cancelled:     &cancelada,
		Kind:          n.Kind,
		Title:         n.Title,
		Content:       n.Content,
		ContentCipher: n.ContentCipher,
		Data:          data,
		X:             &x,
		Y:             &y,
		Col:           &col,
		Position:      &pos,
		Width:         w,
		Collapsed:     &n.Collapsed,
		Checked:       &n.Checked,
		Pinned:        &n.Pinned,
		Visibility:    n.Visibility,
		Indexable:     n.Indexable,
		Links:         n.Links,
		// GroupID se deja fuera por lo mismo que ParentID: agrupa notas por su
		// identificador, y los de un fichero ajeno no existen aquí.
	}
}
