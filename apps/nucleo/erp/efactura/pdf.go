// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package efactura

import (
	"errors"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/pdfdoc"
)

// La factura en PDF: lo que se adjunta al correo que va al cliente, y lo que se
// descarga para archivarla. Sale del MISMO Imprimible que el impreso HTML,
// así que dice lo mismo, con las mismas menciones obligatorias; solo cambia el
// papel. Se escribe con apps/nucleo/pdfdoc (fuentes incrustadas, PDF etiquetado)
// y no con un navegador sin cabeza: el servicio no tiene uno, ni debe tenerlo
// para producir una página con una tabla.

// Tamaños en puntos del PDF de la factura.
const (
	pdfFacturaEmisor = 14.0
	pdfFacturaTitulo = 18.0
	pdfFacturaCuerpo = 10.0
	pdfFacturaMenor  = 8.0
	// 35 mm: el QR tributario va entre 30 y 40 mm (especificación del QR).
	pdfFacturaQR = 35 * 72 / 25.4
)

// colorAviso es el rojo del sello «Borrador»/«Anulada», el mismo del impreso.
const colorAviso = "b91c1c"

// PDF escribe el PDF de la factura.
func (f Imprimible) PDF() ([]byte, error) {
	d := pdfdoc.Nuevo(pdfdoc.Opciones{
		Titulo: strings.TrimSpace(f.Titulo + " " + f.Numero), Idioma: f.Idioma, Productor: "Sytue ERP",
	})
	pdfCabecera(d, f)
	d.Hueco(12)

	titulo := pdfdoc.Parrafo{
		Rol: "H1", Tam: pdfFacturaTitulo, Despues: 4,
		Fragmentos: []pdfdoc.Fragmento{{Texto: strings.TrimSpace(f.Titulo + " " + f.Numero), Negrita: true}},
	}
	if f.Aviso != "" {
		titulo.Fragmentos = append(titulo.Fragmentos,
			pdfdoc.Fragmento{Texto: "  " + strings.ToUpper(f.Aviso), Negrita: true, Color: colorAviso})
	}
	d.Parrafo(titulo)
	d.Hueco(6)

	// El cliente a la izquierda y las fechas (y la factura que rectifica) a la derecha.
	var datos []pdfdoc.Bloque
	for _, par := range f.Datos {
		if par[1] != "" {
			p := pdfDerecha(par[0]+": "+par[1], false)
			datos = append(datos, pdfdoc.Bloque{Parrafo: &p})
		}
	}
	d.Tabla(pdfdoc.Tabla{Filas: [][]pdfdoc.Celda{{
		{Bloques: pdfParte(f.Cliente, pdfFacturaCuerpo+1)}, {Bloques: pdfNoVacio(datos)},
	}}})
	d.Hueco(10)

	if len(f.Lineas) > 0 {
		if f.Concepto != "" {
			d.Parrafo(pdfTexto(f.Concepto))
			d.Hueco(4)
		}
		d.Tabla(pdfLineas(f))
		d.Hueco(10)
		d.Tabla(pdfDesglose(f))
	} else {
		d.Tabla(pdfdoc.Tabla{Filas: [][]pdfdoc.Celda{
			{pdfCelda(pdfTexto(f.Concepto+" "), false), pdfCelda(pdfDerecha(f.Base, false), false)},
			{pdfCelda(pdfDerecha(f.RotuloBase, false), false), pdfCelda(pdfDerecha(f.Base, false), false)},
			{pdfCelda(pdfDerecha(f.RotuloIVA, false), false), pdfCelda(pdfDerecha(f.IVA, false), false)},
			{pdfCelda(pdfDerecha(f.RotTotal, true), false), pdfCelda(pdfDerecha(f.Total, true), false)},
		}})
	}

	if f.Leyenda != "" {
		d.Hueco(12)
		p := pdfTexto(f.Leyenda)
		p.Tam = pdfFacturaMenor + 1
		d.Parrafo(p)
	}
	pdf, err := d.Bytes()
	if err != nil {
		return nil, errors.Join(errors.New("PDF de la factura"), err)
	}
	return pdf, nil
}

func pdfTexto(s string) pdfdoc.Parrafo {
	return pdfdoc.Parrafo{Fragmentos: []pdfdoc.Fragmento{{Texto: s}}, Tam: pdfFacturaCuerpo}
}

func pdfDerecha(s string, negrita bool) pdfdoc.Parrafo {
	p := pdfTexto(s)
	p.Alinear, p.Fragmentos[0].Negrita = pdfdoc.Derecha, negrita
	return p
}

func pdfCelda(p pdfdoc.Parrafo, encabezado bool) pdfdoc.Celda {
	return pdfdoc.Celda{Bloques: []pdfdoc.Bloque{{Parrafo: &p}}, Encabezado: encabezado}
}

// pdfNoVacio: una celda sin bloques no tiene altura; lleva un espacio.
func pdfNoVacio(bs []pdfdoc.Bloque) []pdfdoc.Bloque {
	if len(bs) == 0 {
		vacio := pdfTexto(" ")
		bs = append(bs, pdfdoc.Bloque{Parrafo: &vacio})
	}
	return bs
}

// pdfParte son las líneas de una parte (emisor o cliente), el nombre en negrita.
func pdfParte(p ParteImprimible, tam float64) []pdfdoc.Bloque {
	var bs []pdfdoc.Bloque
	for i, l := range []string{p.Nombre, p.NIF, p.Direccion, p.Ciudad} {
		if strings.TrimSpace(l) == "" {
			continue
		}
		par := pdfTexto(l)
		if i == 0 {
			par.Tam, par.Fragmentos[0].Negrita = tam, true
		}
		bs = append(bs, pdfdoc.Bloque{Parrafo: &par})
	}
	return pdfNoVacio(bs)
}

// pdfCabecera: el emisor a la izquierda y el QR tributario a la derecha, con sus
// dos leyendas («QR tributario:» encima, «VERI*FACTU» debajo).
func pdfCabecera(d *pdfdoc.Doc, f Imprimible) {
	emisor := pdfParte(f.Emisor, pdfFacturaEmisor)
	if f.QRTexto == "" {
		for _, b := range emisor {
			d.Parrafo(*b.Parrafo)
		}
		return
	}
	arriba, abajo := pdfDerecha("QR tributario:", true), pdfDerecha("VERI*FACTU", true)
	qr := []pdfdoc.Bloque{{Parrafo: &arriba}}
	if len(f.QRPNG) > 0 {
		qr = append(qr, pdfdoc.Bloque{Imagen: &pdfdoc.Imagen{
			Datos: f.QRPNG, Ancho: pdfFacturaQR, Alto: pdfFacturaQR, Alt: "QR tributario", Alinear: pdfdoc.Derecha,
		}})
	} else {
		url := pdfDerecha(f.QRTexto, false)
		url.Tam = pdfFacturaMenor
		qr = append(qr, pdfdoc.Bloque{Parrafo: &url})
	}
	qr = append(qr, pdfdoc.Bloque{Parrafo: &abajo})
	d.Tabla(pdfdoc.Tabla{Filas: [][]pdfdoc.Celda{{{Bloques: emisor}, {Bloques: qr}}}})
}

// pdfLineas es la tabla de líneas: la descripción a la izquierda y las cifras a la derecha.
func pdfLineas(f Imprimible) pdfdoc.Tabla {
	fila := func(cs [5]string, encabezado bool) []pdfdoc.Celda {
		out := make([]pdfdoc.Celda, 0, len(cs))
		for i, c := range cs {
			p := pdfTexto(c)
			p.Fragmentos[0].Negrita = encabezado
			if i > 0 {
				p.Alinear = pdfdoc.Derecha
			}
			out = append(out, pdfCelda(p, encabezado))
		}
		return out
	}
	filas := make([][]pdfdoc.Celda, 0, 1+len(f.Lineas))
	filas = append(filas, fila(f.Columnas, true))
	for _, l := range f.Lineas {
		filas = append(filas, fila(l, false))
	}
	return pdfdoc.Tabla{Filas: filas}
}

// pdfDesglose es el desglose por tipo de IVA, con los totales que salen de él.
func pdfDesglose(f Imprimible) pdfdoc.Tabla {
	filas := make([][]pdfdoc.Celda, 0, len(f.Grupos)+3)
	filas = append(filas, []pdfdoc.Celda{
		pdfCelda(pdfDerecha(f.Columnas[3], true), true), pdfCelda(pdfDerecha(f.RotuloBase, true), true), pdfCelda(pdfDerecha(f.RotuloIVA, true), true),
	})
	for _, g := range f.Grupos {
		filas = append(filas, []pdfdoc.Celda{pdfCelda(pdfDerecha(g[0], false), false), pdfCelda(pdfDerecha(g[1], false), false), pdfCelda(pdfDerecha(g[2], false), false)})
	}
	total := pdfCelda(pdfDerecha(f.RotTotal, true), false)
	total.ColSpan = 2
	filas = append(filas,
		[]pdfdoc.Celda{pdfCelda(pdfDerecha("", false), false), pdfCelda(pdfDerecha(f.Base, true), false), pdfCelda(pdfDerecha(f.IVA, true), false)},
		[]pdfdoc.Celda{total, {Cubierta: true}, pdfCelda(pdfDerecha(f.Total, true), false)})
	return pdfdoc.Tabla{Filas: filas}
}

// NombrePDF es el nombre del fichero de la factura: su número, sin lo que un
// sistema de ficheros no admite.
func (f Imprimible) NombrePDF() string {
	n := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 0x20 {
			return '-'
		}
		return r
	}, strings.TrimSpace(f.Numero))
	if n == "" {
		n = "factura"
	}
	return n + ".pdf"
}
