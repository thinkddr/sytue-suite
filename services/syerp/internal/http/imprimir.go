// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/qr"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
	"git.sytue.eu/sytue/one/services/syerp/internal/verifactu"
)

// La factura para el cliente, lista para imprimir o guardar en PDF desde el
// navegador. Con VERI*FACTU lleva arriba el QR tributario con sus dos leyendas,
// como pide la especificación del QR: «QR tributario:» encima y «VERI*FACTU»
// debajo, a entre 30 y 40 mm.

// leyendasIVA dicen en la factura por qué no lleva IVA, con el precepto de la Ley
// 37/1992: es una mención obligatoria (Reglamento de facturación, art. 6.1).
var leyendasIVA = map[string]string{
	"iva_exenta":      "Operación exenta de IVA (artículo 20 de la Ley 37/1992)",
	"iva_exportacion": "Exportación exenta de IVA (artículo 21 de la Ley 37/1992)",
	"iva_entrega_ue":  "Entrega intracomunitaria exenta de IVA (artículo 25 de la Ley 37/1992)",
	"iva_no_sujeta":   "Operación no sujeta a IVA por las reglas de localización (artículos 69 y 70 de la Ley 37/1992)",
	"iva_inversion":   "Inversión del sujeto pasivo (artículo 84.Uno.2.º de la Ley 37/1992)",
}

type parteImprimible struct {
	Nombre, NIF, Direccion, Ciudad string
}

type facturaImprimible struct {
	Idioma, Titulo, Numero, Aviso   string
	Emisor, Cliente                 parteImprimible
	Datos                           [][2]string
	Concepto, Base, IVA, Total      string
	RotuloBase, RotuloIVA, RotTotal string
	Leyenda                         string
	QR                              template.URL
	QRTexto                         string
	// Con líneas: sus columnas, las líneas y el desglose por tipo de IVA.
	Columnas [5]string
	Lineas   [][5]string
	Grupos   [][3]string
}

// Imprimir — GET /api/v1/{modelo}/{id}/imprimir.
func (h *Handlers) Imprimir(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if m.Clave != "factura_venta" {
		httpx.Fail(w, r, errNoContable.WithMessage("Only customer invoices can be printed."))
		return
	}
	ctx := r.Context()
	reg, err := h.store.Obtener(ctx, m.Clave, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	org, err := h.store.Organizacion(ctx)
	if errors.Is(err, store.ErrNoEncontrado) {
		httpx.Fail(w, r, errSinOrganizacion)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	_, urlQR, err := h.registrosDeFactura(ctx, reg)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	lineas, err := h.store.LineasDeFactura(ctx, reg.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	simplificada := cadena(reg.Data["tipo_factura"]) == "F2"
	idioma := httpx.LanguageFromRequest(r)
	rot := func(clave string) string { return h.cat.Rotulo(clave, idioma) }

	f := h.construirFactura(ctx, reg, org, lineas, simplificada, idioma, rot, urlQR)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := plantillaFactura.Execute(w, f); err != nil {
		httpx.Fail(w, r, err)
	}
}

// conLineas pinta las líneas y el desglose por tipo de IVA, con los totales que
// salen de él. La completa da el precio unitario SIN IVA y la base de cada línea
// (RD 1619/2012, art. 6.1.f); la simplificada, el precio con IVA y lo cobrado,
// como un tique. Las dos, la base y la cuota de cada tipo (arts. 6.1 y 7.1.f).
func (f *facturaImprimible) conLineas(lineas []domain.LineaFactura, simplificada bool, rot func(string) string) {
	f.Columnas = [5]string{rot("campo.descripcion"), rot("campo.cantidad"), rot("doc.precio_sin_iva"), rot("campo.tipo_iva"), rot("campo.base")}
	if simplificada {
		f.Columnas[2], f.Columnas[4] = rot("campo.precio_con_iva"), rot("doc.importe_con_iva")
	}
	for _, l := range lineas {
		precio, importe := precioSinIVA(l.Precio, l.Tipo), euros(domain.SinIVA(l.Importe, l.Tipo))
		if simplificada {
			precio, importe = euros(l.Precio), euros(l.Importe)
		}
		f.Lineas = append(f.Lineas, [5]string{l.Descripcion, strings.ReplaceAll(l.Cantidad, ".", ","), precio, fmt.Sprintf("%d %%", l.Tipo), importe})
	}
	var base, iva int64
	for _, g := range domain.Desglose(lineas) {
		f.Grupos = append(f.Grupos, [3]string{fmt.Sprintf("%d %%", g.Tipo), euros(g.Base), euros(g.Cuota)})
		base, iva = base+g.Base, iva+g.Cuota
	}
	f.Base, f.IVA, f.Total = euros(base), euros(iva), euros(base+iva)
}

// precioSinIVA es el precio unitario sin IVA: al céntimo si es exacto y, si no,
// con cuatro decimales, para que cantidad × precio dé la base de la línea
// (2,50 € al 4 % son 2,4038 €, y tres, 7,21 €; con 2,40 € saldrían 7,20).
func precioSinIVA(precio, tipo int64) string {
	centimos := new(big.Rat).Mul(new(big.Rat).SetInt64(precio), big.NewRat(100, 100+tipo))
	if centimos.IsInt() {
		return euros(centimos.Num().Int64())
	}
	n, _ := strconv.ParseInt(centimos.Mul(centimos, big.NewRat(100, 1)).FloatString(0), 10, 64)
	signo := ""
	if n < 0 {
		signo, n = "-", -n
	}
	return fmt.Sprintf("%s%s%02d €", signo, strings.TrimSuffix(euros(n/100), " €"), n%100)
}

// euros escribe céntimos como se leen en una factura española: «1.234,56 €».
func euros(c int64) string {
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

func (h *Handlers) construirFactura(ctx context.Context, reg store.Registro, org store.Organizacion, lineas []domain.LineaFactura, simplificada bool, idioma string, rot func(string) string, urlQR string) facturaImprimible {
	fecha := func(k string) string {
		t, err := time.Parse(time.DateOnly, strings.TrimSpace(cadena(reg.Data[k])))
		if err != nil {
			return ""
		}
		return t.Format("02/01/2006")
	}

	f := facturaImprimible{
		Idioma: idioma, Titulo: rot("doc.factura"), Numero: reg.Ref,
		Emisor: parteImprimible{
			Nombre: org.Nombre, NIF: org.NIF, Direccion: org.Direccion,
			Ciudad: strings.TrimSpace(org.CP + " " + org.Ciudad),
		},
		Concepto:   strings.TrimSpace(cadena(reg.Data["notas"])),
		RotuloBase: rot("campo.base"), RotuloIVA: rot("campo.iva"), RotTotal: rot("campo.total"),
		Leyenda: leyendasIVA[cadena(reg.Data["regimen_iva"])],
	}
	if simplificada {
		f.Titulo = rot("doc.factura_simplificada")
	}
	if id := cadena(reg.Data["cliente"]); id != "" && !simplificada {
		if c, err := h.store.Obtener(ctx, "contacto", id); err == nil {
			texto := func(k string) string { return strings.TrimSpace(cadena(c.Data[k])) }
			f.Cliente = parteImprimible{
				Nombre: primero(texto("nombre"), c.Etiqueta), NIF: texto("nif"), Direccion: texto("direccion"),
				Ciudad: strings.TrimSpace(strings.Join([]string{texto("cp"), texto("ciudad"), texto("pais")}, " ")),
			}
		}
	}
	f.Datos = append(f.Datos, [2]string{rot("campo.fecha"), fecha("fecha")})
	if v := fecha("vencimiento"); v != "" {
		f.Datos = append(f.Datos, [2]string{rot("campo.vencimiento"), v})
	}
	if id := cadena(reg.Data["rectifica"]); id != "" {
		f.Titulo = rot("doc.factura_rectificativa")
		if orig, err := h.store.Obtener(ctx, "factura_venta", id); err == nil {
			f.Datos = append(f.Datos, [2]string{rot("campo.rectifica"), orig.Ref})
		}
	}

	if len(lineas) > 0 {
		f.conLineas(lineas, simplificada, rot)
	} else {
		base, _ := domain.Centimos(reg.Data, "base")
		iva, _ := domain.Centimos(reg.Data, "iva")
		f.Base, f.IVA, f.Total = euros(base), euros(iva), euros(base+iva)
		if tipo, ok := verifactu.TipoIVA(base, iva); ok && iva != 0 {
			f.RotuloIVA = fmt.Sprintf("%s %s %%", f.RotuloIVA, strings.ReplaceAll(verifactu.Porcentaje(tipo), ".", ","))
		}
	}

	switch reg.Estado {
	case "borrador":
		f.Aviso = rot("valor.borrador")
	case "anulado":
		f.Aviso = rot("valor.anulado")
	}
	if urlQR != "" {
		f.QRTexto = urlQR
		if png, err := qr.PNG(urlQR); err == nil {
			f.QR = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(png)) //nolint:gosec // lo genera este servicio
		}
	}
	return f
}
