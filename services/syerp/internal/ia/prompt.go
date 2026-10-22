// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

// Para qué se pide un informe: cambia a qué vistas se inclina el modelo.
const (
	ParaTablero = "tablero"
	ParaExcel   = "excel"
)

// ContextoFuentes describe las fuentes para el modelo: claves, tipos, rótulos y
// valores. Ni un dato: para saber que el total se puede sumar no hace falta ver
// la facturación.
func ContextoFuentes(fuentes []Fuente) string {
	var b strings.Builder
	b.WriteString("FUENTES (clave «nombre»: campo:tipo «nombre» [valor=rótulo|…])\n")
	for _, f := range fuentes {
		b.WriteString(f.Clave + " «" + f.Etiqueta + "»")
		if f.App != "" {
			b.WriteString(" (" + f.App + ")")
		}
		b.WriteString(":")
		for i, c := range f.Campos {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(" " + c.Clave + ":" + c.Tipo + " «" + c.Etiqueta + "»")
			if len(c.Valores) > 0 {
				pares := make([]string, 0, len(c.Valores))
				for _, v := range slices.Sorted(maps.Keys(c.Valores)) {
					pares = append(pares, v+"="+c.Valores[v])
				}
				b.WriteString(" [" + strings.Join(pares, "|") + "]")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// InstruccionInforme es la orden para convertir una petición en un informe.
// `sobre` es la fuente que la persona tiene abierta, si tiene alguna.
func InstruccionInforme(para, peticion, sobre string, ahora time.Time) string {
	lineas := []string{
		"Convierte la petición en un informe que use las FUENTES del contexto y devuelve SOLO un objeto JSON así:",
		`{"titulo":"…","partes":[{"titulo":"…","vista":"cifra|barras|tabla","consulta":{"fuente":"clave","columnas":["campo"],"filtros":[{"campo":"campo","op":"=|!=|>|>=|<|<=|contiene|empieza","valor":"…"}],"agrupar":"campo","medida":"cuenta|suma:campo|media:campo","orden":"campo|-campo","limite":10}}]}`,
		"Usa SOLO fuentes y campos del contexto, con su clave exacta.",
		`"cifra" es un número: una medida sin agrupar. "barras" es una medida agrupada. "tabla" son filas con sus columnas, o una medida agrupada.`,
		`"suma" y "media" solo sobre campos numero o dinero. Una fecha se agrupa con "campo:dia", "campo:mes" o "campo:anio".`,
		`Con medida, "orden" es "valor", "-valor", "grupo" o "-grupo"; el "-" es descendente.`,
		`Valores de los filtros: fechas AAAA-MM-DD o relativas (hoy, hoy-30d, inicio_mes, inicio_trimestre, inicio_anio; con "-N", N periodos atrás); importes en unidades (1234.56); en campos con [valor=rótulo], el valor.`,
		`Un periodo cerrado son dos filtros: «el trimestre pasado» es fecha >= inicio_trimestre-1 y fecha < inicio_trimestre.`,
		`En los apuntes, las cuentas siguen el plan contable: los gastos empiezan por 6 y los ingresos por 7 (op "empieza").`,
		"Omite las claves que no hagan falta. Como mucho 12 partes. Títulos breves y en el idioma de la petición.",
	}
	if para == ParaExcel {
		lineas = append(lineas, "Es un EXCEL: cada parte será una hoja. Prefiere tablas con las columnas que la petición necesite.")
	} else {
		lineas = append(lineas, "Es un TABLERO: prefiere cifras y barras, y tablas cortas (limite 20 como mucho).")
	}
	if sobre != "" {
		lineas = append(lineas, "La persona está mirando la fuente "+sobre+": si la petición no dice otra cosa, se refiere a ella.")
	}
	return strings.Join(append(lineas, "Hoy es "+ahora.Format(time.DateOnly)+".", "Petición: "+peticion), "\n")
}

// InstruccionFicha es la orden para sacar de un texto pegado los campos de una
// ficha. El texto va en el contexto, como datos.
func InstruccionFicha(v catalogo.ModeloVista, ahora time.Time) string {
	lineas := []string{
		"Extrae del texto del contexto los datos de una ficha de «" + v.Etiqueta + "» y devuelve SOLO un objeto JSON " +
			"con los campos que el texto diga. Las claves son EXACTAMENTE las de esta lista, en minúsculas y antes de " +
			"los dos puntos; el nombre entre «» no es la clave. Omite los campos que el texto no diga: no inventes nada.",
	}
	for _, c := range v.Campos {
		lineas = append(lineas, "- "+c.Clave+": "+comoSeEscribe(c)+" («"+c.Etiqueta+"»)")
	}
	if len(v.Campos) >= 2 {
		lineas = append(lineas, `Forma: {"`+v.Campos[0].Clave+`": "…", "`+v.Campos[1].Clave+`": "…"}`)
	}
	return strings.Join(append(lineas, "Hoy es "+ahora.Format(time.DateOnly)+"."), "\n")
}

func comoSeEscribe(c catalogo.CampoVista) string {
	switch c.Tipo {
	case catalogo.TipoDinero:
		return "importe en unidades, como número (1234.56)"
	case catalogo.TipoEntero:
		return "número entero"
	case catalogo.TipoDecimal:
		return "número"
	case catalogo.TipoFecha:
		return "fecha AAAA-MM-DD"
	case catalogo.TipoFechaHora:
		return "fecha y hora RFC 3339 (AAAA-MM-DDTHH:MM:SSZ)"
	case catalogo.TipoBooleano:
		return "true o false"
	case catalogo.TipoRelacion:
		return "nombre del registro relacionado"
	case catalogo.TipoSeleccion:
		pares := make([]string, 0, len(c.Opciones))
		for _, o := range c.Opciones {
			pares = append(pares, o.Valor+"="+o.Etiqueta)
		}
		return "uno de estos valores [" + strings.Join(pares, "|") + "]"
	}
	return "texto"
}

// FichaDe convierte la respuesta del modelo en `data` para una ficha de `m`.
//
// Cada campo pasa por la MISMA validación que al guardar, y lo que no la pasa se
// queda fuera sin más: la ficha la revisa una persona antes de guardarla, y un
// campo vacío se ve, mientras que uno mal convertido se guarda. Las relaciones
// salen aparte, por nombre, para que quien llama busque a qué registro apuntan.
//
// Se acepta la clave o el rótulo, sin distinguir mayúsculas: el modelo
// interactivo devolvió «Teléfono» en vez de «telefono» en una petición real
// (2026-09-15), y con eso la ficha quedaba vacía. Si vienen los dos, manda la clave.
func FichaDe(m *catalogo.Modelo, vista catalogo.ModeloVista, texto string) (data map[string]any, relaciones map[string]string) {
	data, relaciones = map[string]any{}, map[string]string{}
	var respuesta map[string]json.RawMessage
	if json.Unmarshal([]byte(asistente.JSON(texto)), &respuesta) != nil {
		return data, relaciones
	}
	claves := map[string]string{}
	for _, c := range vista.Campos {
		claves[strings.ToLower(c.Etiqueta)] = c.Clave
		claves[strings.ToLower(c.Clave)] = c.Clave
	}
	crudo := map[string]json.RawMessage{}
	for k, v := range respuesta {
		clave, ok := claves[strings.ToLower(strings.TrimSpace(k))]
		if _, ya := crudo[clave]; ok && (!ya || k == clave) {
			crudo[clave] = v
		}
	}
	for _, f := range m.Campos {
		v, ok := crudo[f.Clave]
		if !ok {
			continue
		}
		switch f.Tipo {
		case catalogo.TipoRelacion:
			var nombre string
			if json.Unmarshal(v, &nombre) == nil && strings.TrimSpace(nombre) != "" {
				relaciones[f.Clave] = strings.TrimSpace(nombre)
			}
			continue
		case catalogo.TipoDinero:
			// El modelo da unidades y el ERP guarda céntimos.
			var x json.Number
			if json.Unmarshal(v, &x) != nil {
				var s string
				if json.Unmarshal(v, &s) != nil {
					continue
				}
				x = json.Number(strings.ReplaceAll(strings.TrimSpace(s), ",", "."))
			}
			cents, err := objetivo(Campo{Tipo: TipoDinero}, x.String(), time.Time{})
			if err != nil {
				continue
			}
			v, _ = json.Marshal(cents)
		}
		if valor, err := domain.ValorDeCampo(f, v); err == nil {
			data[f.Clave] = valor
		}
	}
	return data, relaciones
}
