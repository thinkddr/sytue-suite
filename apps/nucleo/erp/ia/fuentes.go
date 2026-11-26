// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/exportar"
)

// FuenteLibro es la clave de la fuente de los apuntes del libro diario.
const FuenteLibro = "apuntes"

// FuenteDelLibro describe los apuntes del libro como fuente de informes.
func FuenteDelLibro(cat *catalogo.Catalogo, idioma string) Fuente {
	r := func(clave string) string { return cat.Rotulo(clave, idioma) }
	return Fuente{Clave: FuenteLibro, Etiqueta: r("fuente.apuntes"), Campos: []Campo{
		{Clave: "fecha", Tipo: TipoFecha, Etiqueta: r("campo.fecha")},
		{Clave: "diario", Tipo: TipoTexto, Etiqueta: r("campo.diario")},
		{Clave: "ref", Tipo: TipoTexto, Etiqueta: r("campo.ref")},
		{Clave: "concepto", Tipo: TipoTexto, Etiqueta: r("campo.descripcion")},
		{Clave: "cuenta", Tipo: TipoTexto, Etiqueta: r("campo.cuenta")},
		{Clave: "debe", Tipo: TipoDinero, Etiqueta: r("campo.debe")},
		{Clave: "haber", Tipo: TipoDinero, Etiqueta: r("campo.haber")},
	}}
}

// FuenteDeModelo describe un modelo del catálogo como fuente de informes.
func FuenteDeModelo(cat *catalogo.Catalogo, m *catalogo.Modelo, idioma string) Fuente {
	v := cat.VistaModelo(m, idioma)
	f := Fuente{Clave: m.Clave, Etiqueta: v.Etiqueta}
	if v.Numerado {
		f.Campos = append(f.Campos, Campo{Clave: "ref", Tipo: TipoTexto, Etiqueta: cat.Rotulo("campo.ref", idioma)})
	}
	for _, c := range v.Campos {
		campo := Campo{Clave: c.Clave, Tipo: TipoDeInforme(c.Tipo), Etiqueta: c.Etiqueta}
		for _, o := range c.Opciones {
			if campo.Valores == nil {
				campo.Valores = map[string]string{}
			}
			campo.Valores[o.Valor] = o.Etiqueta
		}
		f.Campos = append(f.Campos, campo)
	}
	if len(v.Estados) > 0 {
		estado := Campo{Clave: "estado", Tipo: TipoTexto, Etiqueta: cat.Rotulo("campo.estado", idioma), Valores: map[string]string{}}
		for _, e := range v.Estados {
			estado.Valores[e.Valor] = e.Etiqueta
		}
		f.Campos = append(f.Campos, estado)
	}
	return f
}

// TipoDeInforme es el tipo de informe de un tipo de campo del catálogo.
func TipoDeInforme(tipo string) string {
	switch tipo {
	case catalogo.TipoEntero, catalogo.TipoDecimal:
		return TipoNumero
	case catalogo.TipoDinero:
		return TipoDinero
	case catalogo.TipoFecha, catalogo.TipoFechaHora:
		return TipoFecha
	case catalogo.TipoBooleano:
		return TipoBooleano
	}
	return TipoTexto
}

// FilaDe pasa un registro a una fila de informe, con cada valor en su tipo: el dinero
// en céntimos, la relación por su nombre.
func FilaDe(m *catalogo.Modelo, ref, estado string, data map[string]any, etiquetas map[string]string) Fila {
	fila := Fila{"ref": ref, "estado": estado}
	for _, c := range m.Campos {
		v := data[c.Clave]
		switch c.Tipo {
		case catalogo.TipoDinero:
			if cents, ok := exportar.Centimos(v); ok {
				fila[c.Clave] = cents
			}
		case catalogo.TipoEntero, catalogo.TipoDecimal:
			if x, err := strconv.ParseFloat(exportar.Texto(v), 64); err == nil {
				fila[c.Clave] = x
			}
		case catalogo.TipoBooleano:
			if b, ok := v.(bool); ok {
				fila[c.Clave] = b
			}
		case catalogo.TipoRelacion:
			id, _ := v.(string)
			fila[c.Clave] = exportar.Legible(etiquetas, id)
		default:
			if v != nil {
				fila[c.Clave] = exportar.Texto(v)
			}
		}
	}
	return fila
}
