// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package efactura

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"math/big"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
)

// La factura para el cliente, lista para imprimir o guardar en PDF desde el
// navegador. Con VERI*FACTU lleva arriba el QR tributario con sus dos leyendas,
// como pide la especificación del QR: «QR tributario:» encima y «VERI*FACTU»
// debajo, a entre 30 y 40 mm.

// LeyendasIVA dicen en la factura por qué no lleva IVA, con el precepto de la Ley
// 37/1992: es una mención obligatoria (Reglamento de facturación, art. 6.1).
var LeyendasIVA = map[string]string{
	"iva_exenta":      "Operación exenta de IVA (artículo 20 de la Ley 37/1992)",
	"iva_exportacion": "Exportación exenta de IVA (artículo 21 de la Ley 37/1992)",
	"iva_entrega_ue":  "Entrega intracomunitaria exenta de IVA (artículo 25 de la Ley 37/1992)",
	"iva_no_sujeta":   "Operación no sujeta a IVA por las reglas de localización (artículos 69 y 70 de la Ley 37/1992)",
	"iva_inversion":   "Inversión del sujeto pasivo (artículo 84.Uno.2.º de la Ley 37/1992)",
}

// ParteImprimible son las cuatro líneas de una parte en el impreso.
type ParteImprimible struct {
	Nombre, NIF, Direccion, Ciudad string
}

// Imprimible es lo que lleva el impreso de la factura: lo comparten el HTML para
// imprimir, el PDF y el envío por correo, para que el cliente reciba exactamente lo
// que se ve al imprimir.
type Imprimible struct {
	Idioma, Titulo, Numero, Aviso   string
	Emisor, Cliente                 ParteImprimible
	Datos                           [][2]string
	Concepto, Base, IVA, Total      string
	RotuloBase, RotuloIVA, RotTotal string
	Leyenda                         string
	QR                              template.URL
	QRTexto                         string
	// QRPNG es la imagen del QR, para el PDF (el HTML la lleva en QR como data:).
	QRPNG []byte
	// Con líneas: sus columnas, las líneas y el desglose por tipo de IVA.
	Columnas [5]string
	Lineas   [][5]string
	Grupos   [][3]string
}

// Imprimir arma el impreso de la factura. cliente nil (o una simplificada) no
// identifica al comprador. rot traduce las claves del catálogo (`doc.factura`,
// `campo.base`…) al idioma del impreso.
func Imprimir(f Factura, emisor Parte, cliente *Parte, idioma string, rot func(string) string) Imprimible {
	fecha := func(s string) string {
		t, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
		if err != nil {
			return ""
		}
		return t.Format("02/01/2006")
	}
	simplificada := f.Simplificada()
	out := Imprimible{
		Idioma: idioma, Titulo: rot("doc.factura"), Numero: f.Numero,
		Emisor: ParteImprimible{
			Nombre: emisor.Nombre, NIF: emisor.NIF, Direccion: emisor.Direccion,
			Ciudad: strings.TrimSpace(emisor.CP + " " + emisor.Ciudad),
		},
		Concepto:   strings.TrimSpace(f.Notas),
		RotuloBase: rot("campo.base"), RotuloIVA: rot("campo.iva"), RotTotal: rot("campo.total"),
		Leyenda: LeyendasIVA[f.RegimenIVA],
	}
	if simplificada {
		out.Titulo = rot("doc.factura_simplificada")
	}
	if cliente != nil && !simplificada {
		c := *cliente
		out.Cliente = ParteImprimible{
			Nombre: strings.TrimSpace(c.Nombre), NIF: strings.TrimSpace(c.NIF), Direccion: strings.TrimSpace(c.Direccion),
			Ciudad: strings.TrimSpace(strings.Join([]string{strings.TrimSpace(c.CP), strings.TrimSpace(c.Ciudad), strings.TrimSpace(c.Pais)}, " ")),
		}
	}
	out.Datos = append(out.Datos, [2]string{rot("campo.fecha"), fecha(f.Fecha)})
	if v := fecha(f.Vencimiento); v != "" {
		out.Datos = append(out.Datos, [2]string{rot("campo.vencimiento"), v})
	}
	if f.Rectificativa {
		out.Titulo = rot("doc.factura_rectificativa")
		if f.Rectifica != "" {
			out.Datos = append(out.Datos, [2]string{rot("campo.rectifica"), f.Rectifica})
		}
	}

	if len(f.Lineas) > 0 {
		out.conLineas(f.Lineas, simplificada, rot)
	} else {
		out.Base, out.IVA, out.Total = Euros(f.Base), Euros(f.IVA), Euros(f.Base+f.IVA)
		if tipo, ok := dominio.TipoDeCuota(f.Base, f.IVA); ok && f.IVA != 0 {
			out.RotuloIVA = fmt.Sprintf("%s %s %%", out.RotuloIVA, strings.ReplaceAll(dominio.Porcentaje(tipo), ".", ","))
		}
	}

	switch f.Estado {
	case "borrador":
		out.Aviso = rot("valor.borrador")
	case "anulado":
		out.Aviso = rot("valor.anulado")
	}
	if f.QR != "" {
		out.QRTexto = f.QR
	}
	return out
}

// PonerQR pone la imagen del QR tributario (un PNG) en el impreso: como data: en el
// HTML y tal cual en el PDF. Sin ella, el impreso lleva la URL en texto.
func (f *Imprimible) PonerQR(png []byte) {
	if len(png) == 0 {
		return
	}
	f.QRPNG = png
	f.QR = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(png)) //nolint:gosec // lo genera quien llama
}

// HTML escribe el impreso como una página lista para imprimir.
func (f Imprimible) HTML(w io.Writer) error { return plantillaFactura.Execute(w, f) }

// conLineas pinta las líneas y el desglose por tipo de IVA, con los totales que
// salen de él. La completa da el precio unitario SIN IVA y la base de cada línea
// (RD 1619/2012, art. 6.1.f); la simplificada, el precio con IVA y lo cobrado,
// como un tique. Las dos, la base y la cuota de cada tipo (arts. 6.1 y 7.1.f).
func (f *Imprimible) conLineas(lineas []dominio.LineaFactura, simplificada bool, rot func(string) string) {
	f.Columnas = [5]string{rot("campo.descripcion"), rot("campo.cantidad"), rot("doc.precio_sin_iva"), rot("campo.tipo_iva"), rot("campo.base")}
	if simplificada {
		f.Columnas[2], f.Columnas[4] = rot("campo.precio_con_iva"), rot("doc.importe_con_iva")
	}
	for _, l := range lineas {
		precio, importe := PrecioSinIVA(l.Precio, l.Tipo), Euros(dominio.SinIVA(l.Importe, l.Tipo))
		if simplificada {
			precio, importe = Euros(l.Precio), Euros(l.Importe)
		}
		f.Lineas = append(f.Lineas, [5]string{l.Descripcion, strings.ReplaceAll(l.Cantidad, ".", ","), precio, fmt.Sprintf("%d %%", l.Tipo), importe})
	}
	var base, iva int64
	for _, g := range dominio.Desglose(lineas) {
		f.Grupos = append(f.Grupos, [3]string{fmt.Sprintf("%d %%", g.Tipo), Euros(g.Base), Euros(g.Cuota)})
		base, iva = base+g.Base, iva+g.Cuota
	}
	f.Base, f.IVA, f.Total = Euros(base), Euros(iva), Euros(base+iva)
}

// PrecioSinIVA es el precio unitario sin IVA: al céntimo si es exacto y, si no,
// con cuatro decimales, para que cantidad × precio dé la base de la línea
// (2,50 € al 4 % son 2,4038 €, y tres, 7,21 €; con 2,40 € saldrían 7,20).
func PrecioSinIVA(precio, tipo int64) string {
	centimos := new(big.Rat).Mul(new(big.Rat).SetInt64(precio), big.NewRat(100, 100+tipo))
	if centimos.IsInt() {
		return Euros(centimos.Num().Int64())
	}
	n, _ := strconv.ParseInt(centimos.Mul(centimos, big.NewRat(100, 1)).FloatString(0), 10, 64)
	signo := ""
	if n < 0 {
		signo, n = "-", -n
	}
	return fmt.Sprintf("%s%s%02d €", signo, strings.TrimSuffix(Euros(n/100), " €"), n%100)
}

// Euros escribe céntimos como se leen en una factura española: «1.234,56 €».
func Euros(c int64) string {
	signo := ""
	if c < 0 {
		signo, c = "-", -c
	}
	entero := fmt.Sprintf("%d", c/100)
	for i := len(entero) - 3; i > 0; i -= 3 {
		entero = entero[:i] + "." + entero[i:]
	}
	return fmt.Sprintf("%s%s,%02d €", signo, entero, c%100)
}

var plantillaFactura = template.Must(template.New("factura").Parse(`<!doctype html>
<html lang="{{.Idioma}}"><head><meta charset="utf-8"><title>{{.Titulo}} {{.Numero}}</title>
<style>
@page{size:A4;margin:16mm}
*{box-sizing:border-box}
body{font:13px/1.45 system-ui,-apple-system,"Segoe UI",sans-serif;color:#111;max-width:190mm;margin:1.5rem auto;padding:0 1rem}
header{display:flex;justify-content:space-between;align-items:flex-start;gap:1.5rem}
.emisor strong{font-size:1.25rem;display:block}
.qr{text-align:center;font-size:11px;font-weight:600}
.qr img{width:35mm;height:35mm;display:block;margin:1mm auto;image-rendering:pixelated}
.qr .url{font-weight:400;font-size:9px;max-width:40mm;word-break:break-all}
h1{font-size:1.5rem;margin:1.6rem 0 .2rem}
.aviso{display:inline-block;border:2px solid #b91c1c;color:#b91c1c;padding:.1rem .6rem;font-weight:700;text-transform:uppercase;margin-left:.6rem;font-size:.9rem;vertical-align:middle}
.bloques{display:flex;justify-content:space-between;gap:2rem;margin:1rem 0 1.4rem}
dl{display:grid;grid-template-columns:auto auto;gap:.15rem 1rem;margin:0}
dt{color:#555}dd{margin:0;text-align:right}
table{width:100%;border-collapse:collapse;margin-top:.6rem}
th,td{padding:.45rem .3rem;border-bottom:1px solid #ddd;text-align:left;vertical-align:top}
td.n,th.n{text-align:right;white-space:nowrap}
tfoot td{border:0}tfoot tr:last-child td{font-weight:700;font-size:1.05rem;border-top:2px solid #111}
.leyenda{margin-top:1.2rem;font-size:12px}
@media print{body{margin:0;padding:0;max-width:none}}
</style></head><body>
<header>
  <div class="emisor"><strong>{{.Emisor.Nombre}}</strong>{{.Emisor.NIF}}<br>{{.Emisor.Direccion}}<br>{{.Emisor.Ciudad}}</div>
  {{if .QRTexto}}<div class="qr">QR tributario:{{if .QR}}<img src="{{.QR}}" alt="QR tributario">{{else}}<div class="url">{{.QRTexto}}</div>{{end}}VERI*FACTU</div>{{end}}
</header>
<h1>{{.Titulo}} {{.Numero}}{{if .Aviso}}<span class="aviso">{{.Aviso}}</span>{{end}}</h1>
<div class="bloques">
  <div>{{if .Cliente.Nombre}}<strong>{{.Cliente.Nombre}}</strong><br>{{.Cliente.NIF}}<br>{{.Cliente.Direccion}}<br>{{.Cliente.Ciudad}}{{end}}</div>
  <dl>{{range .Datos}}<dt>{{index . 0}}</dt><dd>{{index . 1}}</dd>{{end}}</dl>
</div>
{{if .Lineas}}{{if .Concepto}}<p>{{.Concepto}}</p>
{{end}}<table>
  <thead><tr>{{range $i, $c := .Columnas}}<th{{if $i}} class="n"{{end}}>{{$c}}</th>{{end}}</tr></thead>
  <tbody>{{range .Lineas}}
    <tr>{{range $i, $c := .}}<td{{if $i}} class="n"{{end}}>{{$c}}</td>{{end}}</tr>{{end}}
  </tbody>
</table>
<table>
  <thead><tr><th>{{index .Columnas 3}}</th><th class="n">{{.RotuloBase}}</th><th class="n">{{.RotuloIVA}}</th></tr></thead>
  <tbody>{{range .Grupos}}
    <tr><td>{{index . 0}}</td><td class="n">{{index . 1}}</td><td class="n">{{index . 2}}</td></tr>{{end}}
  </tbody>
  <tfoot>
    <tr><td></td><td class="n">{{.Base}}</td><td class="n">{{.IVA}}</td></tr>
    <tr><td class="n" colspan="2">{{.RotTotal}}</td><td class="n">{{.Total}}</td></tr>
  </tfoot>
</table>{{else}}<table>
  <thead><tr><th></th><th class="n">{{.RotuloBase}}</th></tr></thead>
  <tbody><tr><td>{{.Concepto}}</td><td class="n">{{.Base}}</td></tr></tbody>
  <tfoot>
    <tr><td class="n">{{.RotuloBase}}</td><td class="n">{{.Base}}</td></tr>
    <tr><td class="n">{{.RotuloIVA}}</td><td class="n">{{.IVA}}</td></tr>
    <tr><td class="n">{{.RotTotal}}</td><td class="n">{{.Total}}</td></tr>
  </tfoot>
</table>{{end}}
{{if .Leyenda}}<p class="leyenda">{{.Leyenda}}</p>{{end}}
</body></html>`))
