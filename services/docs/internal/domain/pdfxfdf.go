// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Datos de formulario en XFDF (ISO 19444-1), que es como se intercambian.
//
// Sirve para dos cosas que hoy no se podían hacer: sacar lo que alguien
// rellenó para volcarlo a otro sitio, y meter en un impreso los datos que
// vienen de fuera. Es el formato que usan Acrobat y Foxit, así que un
// formulario relleno en Sytue se abre con sus datos en cualquiera de los dos y
// al revés.
//
// FDF —el hermano binario y antiguo— no se implementa: es un PDF con otra
// cabecera, nadie lo genera ya desde hace años y XFDF lo sustituye en el
// estándar.
package domain

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// maxXFDFBytes acota lo que se acepta importar. Un fichero de datos de
// formulario son kilobytes; megas significan que alguien mandó otra cosa.
const maxXFDFBytes = 4 << 20

// maxCamposXFDF acota cuántos campos se leen de un fichero ajeno.
const maxCamposXFDF = 5000

// ExportarXFDF devuelve los valores del formulario en XFDF.
//
// `nombreFichero` va en el `<f href>` para que un lector sepa a qué documento
// pertenecen los datos; es informativo y puede ir vacío.
func (d *PDFDoc) ExportarXFDF(nombreFichero string) ([]byte, error) {
	campos := d.CamposDelFormulario()
	if len(campos) == 0 {
		return nil, fmt.Errorf("pdf: el documento no tiene campos de formulario")
	}

	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString(`<xfdf xmlns="http://ns.adobe.com/xfdf/" xml:space="preserve">` + "\n")
	b.WriteString("  <fields>\n")
	for _, c := range campos {
		// Los botones de acción no llevan dato: exportarlos mete ruido en la hoja
		// de cálculo de quien recoge las respuestas.
		if c.Tipo == CampoBoton {
			continue
		}
		b.WriteString(`    <field name="`)
		xmlEscapar(&b, c.Nombre)
		b.WriteString("\">\n      <value>")
		xmlEscapar(&b, c.Valor)
		b.WriteString("</value>\n    </field>\n")
	}
	b.WriteString("  </fields>\n")
	if nombreFichero != "" {
		b.WriteString(`  <f href="`)
		xmlEscapar(&b, nombreFichero)
		b.WriteString("\"/>\n")
	}
	b.WriteString("</xfdf>\n")
	return []byte(b.String()), nil
}

// ImportarXFDF lee los valores de un fichero XFDF.
//
// Devuelve lo que se puede aplicar con RellenarFormularioPDF. NO se comprueba
// aquí que los campos existan en el documento: eso lo hace el relleno, que es
// quien sabe del documento, y así el mismo XFDF sirve para varios impresos con
// campos comunes.
func ImportarXFDF(datos []byte) (map[string]string, error) {
	if len(datos) == 0 {
		return nil, fmt.Errorf("pdf: el fichero de datos está vacío")
	}
	if len(datos) > maxXFDFBytes {
		return nil, fmt.Errorf("pdf: el fichero de datos es demasiado grande")
	}

	var doc struct {
		XMLName xml.Name `xml:"xfdf"`
		Fields  struct {
			Field []campoXFDF `xml:"field"`
		} `xml:"fields"`
	}
	// `xml.Unmarshal` de la stdlib NO resuelve entidades externas ni sigue
	// DOCTYPE, así que XXE y «billion laughs» no aplican: una entidad
	// desconocida es un error de parseo, no una expansión. Por eso se usa esto y
	// no un lector propio.
	if err := xml.Unmarshal(datos, &doc); err != nil {
		return nil, fmt.Errorf("pdf: el fichero no es un XFDF válido: %w", err)
	}

	out := make(map[string]string, len(doc.Fields.Field))
	aplanar(doc.Fields.Field, "", out, 0)
	if len(out) == 0 {
		return nil, fmt.Errorf("pdf: el fichero no trae ningún campo con valor")
	}
	return out, nil
}

// campoXFDF es un campo, que puede anidar otros: el estándar permite agrupar
// («cliente.direccion.calle») y el nombre completo se forma con puntos.
type campoXFDF struct {
	Name  string      `xml:"name,attr"`
	Value []string    `xml:"value"`
	Field []campoXFDF `xml:"field"`
}

// aplanar recorre el árbol componiendo el nombre completo.
func aplanar(campos []campoXFDF, prefijo string, out map[string]string, prof int) {
	if prof > maxProfundidadCampos || len(out) >= maxCamposXFDF {
		return
	}
	for _, c := range campos {
		nombre := strings.TrimSpace(c.Name)
		if nombre == "" {
			continue
		}
		if prefijo != "" {
			nombre = prefijo + "." + nombre
		}
		// Un campo puede traer varios <value> si es una lista de selección
		// múltiple. Se juntan por coma, que es como lo entrega el propio PDF.
		if len(c.Value) > 0 {
			out[nombre] = strings.Join(c.Value, ",")
		}
		if len(c.Field) > 0 {
			aplanar(c.Field, nombre, out, prof+1)
		}
		if len(out) >= maxCamposXFDF {
			return
		}
	}
}

// xmlEscapar escribe el texto escapando lo que rompería el documento.
//
// No se usa `xml.EscapeText` de la stdlib porque convierte los saltos de línea
// en `&#xA;`: es correcto, pero un XFDF con `xml:space="preserve"` los quiere
// literales, y Acrobat enseña la entidad tal cual en un campo de varias líneas.
func xmlEscapar(b *strings.Builder, s string) {
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		case '\n', '\t':
			b.WriteRune(r)
		default:
			// Los demás controles no son válidos en XML 1.0 ni con entidad: se
			// tiran, porque un fichero que no abre es peor que un dato recortado.
			if r >= 0x20 && r != 0x7f {
				b.WriteRune(r)
			}
		}
	}
}
