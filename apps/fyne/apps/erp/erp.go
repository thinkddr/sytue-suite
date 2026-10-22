// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package erp es Sytue ERP en Fyne: el menú de áreas, la lista de un modelo y la
// portada, con paridad a píxel con `apps/frontend/apps/erp`.
//
// # De dónde salen los números
//
// De medir el DOM de producción (`corporate.sytue.eu/erp`, 1280×800, rem 14) con
// `test/e2e/medidas-web.mjs erp` el 2026-09-22. Cada constante lleva escrito si es
// **medida** (salió de `getBoundingClientRect`) o de **hoja** (se deriva de la clase de
// Tailwind con el rem de 14 porque ninguna pantalla de producción la montaba).
//
// La cuenta del barrido: portada 567 elementos, lista 163. Y el aviso que va con ella:
// **la cuenta de auditoría no tiene permisos en los módulos del ERP**, así que las tres
// pantallas medidas traen respuestas 403 del servicio. Eso NO invalida las medidas de
// cromo —menú, cabecera, filtros, estado vacío, que es lo que se pinta igual—, pero
// quiere decir que **no hay ni una fila de tabla real medida**: el alto de fila sale de
// `ui.AltoFilaTabla()`, que es de hoja y ya tiene su propio guardián.
//
// # NO HAY UNA PANTALLA POR MÓDULO
//
// Es la decisión de producto del ERP y se conserva tal cual: hay UNA lista y UNA ficha,
// y las dos se generan del catálogo que publica el servicio. Encender Fabricación no
// añade un fichero aquí. Por eso este paquete no sabe el nombre de ningún módulo.
//
// # Esta ola es de SOLO LECTURA, y es deliberado
//
// Se navega, se lista, se filtra, se ordena y se busca. No se escribe NADA: ni alta, ni
// edición, ni importación, ni acciones en lote, ni encender un módulo. Las razones, por
// orden de peso:
//
//  1. **Son facturas.** El encargo dice que los documentos de la gente no se corrompen
//     nunca y que lo que no se entienda entero va en solo lectura. Una ficha generada
//     del catálogo que guarda `data` completo (el PUT REEMPLAZA el registro entero) es
//     exactamente lo que no se hace a medias.
//  2. **VERI\*FACTU.** Numerar, encadenar y firmar es del servicio y es obligación legal
//     desde el 1-1-2027. Nada de aquí lo toca, y lo que se enseñe de él será lo que
//     conteste el servicio.
//  3. Lo que falta está escrito en el informe de la ola, no adivinado aquí.
//
// # Lo que este paquete NO construye
//
// Ni un widget genérico. La tabla, el armazón, el botón, el campo, la insignia y el
// estado vacío son de `apps/fyne/ui` y se importan. Lo propio es lo que en la web
// también es propio del ERP: la ENTRADA del menú —que no es el `SidebarItem` de la
// suite, mide 26,25 contra 33,25 y no lleva icono—, la píldora de filtro y la
// composición de la portada.
package erp

import (
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// paridad-web: apps/frontend/apps/erp/src/pages/index.astro

// Traductor es el de la suite con el catálogo común y el del ERP encima, que es el
// orden de `createAppTranslator` en la web.
func Traductor(l i18n.Locale) (*i18n.Translator, error) {
	return i18n.New(l, i18n.Common(), catalogos.ERP())
}

// Fase es en cuál de sus estados está una vista: la de `api`, con alias para que las
// firmas de este paquete se lean sin `api.` delante.
//
// En el ERP el estado de permiso denegado **no es teórico**: los permisos son POR
// MÓDULO y el servicio contesta 403 a quien mira un área que no tiene concedida.
//
// Y el 401 **no tiene pantalla propia aquí**, que es la diferencia con Drive: la web
// del ERP tampoco la distingue —su `catch` escribe `error.offline` para todo—, así que
// `FaseSinSesion` se pinta como `Fallo`. Es una diferencia de dominio y se queda; lo
// que no se queda es la tercera copia de la enumeración y de su cuenta
// (`docs/CONVENTIONS.md` §13.2).
type Fase = api.Fase

// Las fases, con el cero en «cargando»: una vista recién construida todavía no ha
// pedido nada, y ese es el estado honesto.
const (
	Cargando    = api.FaseCargando
	Exito       = api.FaseExito
	Vacia       = api.FaseVacia
	Fallo       = api.FaseFallo
	SinSesion   = api.FaseSinSesion
	Prohibida   = api.FaseProhibida
	SinConexion = api.FaseSinConexion
)

// FaseDe traduce el resultado de una carga a la fase que toca pintar: `api.FaseDe`.
var FaseDe = api.FaseDe

// Modelo, Campo, Registro y Modulo son los del cliente: el catálogo es el contrato y no
// se reescribe aquí. Se alían para que las firmas de este paquete se lean sin `api.`
// delante, no para cambiarlos.
type (
	// Catalogo es lo que el ERP sabe hacer y qué áreas ha encendido la organización.
	Catalogo = api.ERPCatalogo
	// Modulo es un área de negocio: una sección del menú.
	Modulo = api.ERPModulo
	// Modelo es un tipo de registro, con sus campos y sus estados.
	Modelo = api.ERPModelo
	// Campo es un campo de un modelo, ya con su rótulo traducido por el servicio.
	Campo = api.ERPCampo
	// Registro es una fila de cualquier modelo.
	Registro = api.ERPRegistro
	// Pagina es una página de una lista, con los nombres de sus relaciones resueltos.
	Pagina = api.ERPPagina
	// Permisos es el nivel de quien mira en cada módulo.
	Permisos = api.ERPPermisos
)

// NombreDe es `nombreDe` de formato.ts: cómo se llama una fila en una lista y en la
// paleta.
func NombreDe(r Registro) string {
	switch {
	case r.Etiqueta != "":
		return r.Etiqueta
	case r.Ref != "":
		return r.Ref
	case len(r.ID) > 8:
		return r.ID[:8]
	}
	return r.ID
}

// AlineadoDerecha es `alineadoDerecha`: las cifras se alinean a la derecha, que es como
// se comparan.
func AlineadoDerecha(tipo string) bool {
	return tipo == api.ERPDinero || tipo == api.ERPEntero || tipo == api.ERPDecimal
}

// SinValor es lo que la web escribe en una celda vacía: una raya larga, no un hueco.
const SinValor = "—"

// Celda es `celda` de formato.ts: el valor de una celda listo para pintar.
//
// `etiquetas` trae el nombre legible de las relaciones de ESTA página, que el servicio
// resuelve de una vez; sin él, una lista de cincuenta pedidos enseñaría cincuenta
// identificadores. Que falte una significa que la fila apuntada está borrada, y
// entonces se dice con la raya en vez de enseñar un `id` que no significa nada.
func Celda(c Campo, valor any, l i18n.Locale, etiquetas map[string]string) string {
	if valor == nil || valor == "" {
		return SinValor
	}
	switch c.Tipo {
	case api.ERPDinero:
		// El dinero viaja en CÉNTIMOS enteros. Llega por JSON como float64, así que se
		// redondea al entero más cercano antes de formatear: convertirlo truncando
		// perdería un céntimo en cuanto el número no sea representable exacto.
		if n, ok := entero(valor); ok {
			return i18n.FormatCurrency(l, n)
		}
	case api.ERPEntero:
		if n, ok := entero(valor); ok {
			return i18n.FormatNumber(l, float64(n))
		}
	case api.ERPDecimal:
		// Los decimales viajan como CADENA canónica («2.5», con punto) para no perder
		// precisión. Aquí se pintan con el separador de quien mira, y los ceros de la
		// derecha se conservan: en un inventario son la precisión con la que se midió.
		return Decimal(texto(valor), l)
	case api.ERPFecha:
		if t, ok := fechaISO(texto(valor)); ok {
			return i18n.FormatDate(l, t)
		}
	case api.ERPFechaHora:
		if t, err := time.Parse(time.RFC3339, texto(valor)); err == nil {
			return i18n.FormatDateTime(l, t)
		}
	case api.ERPBooleano:
		if si, ok := valor.(bool); ok && si {
			return "✓"
		}
		return SinValor
	case api.ERPSeleccion:
		v := texto(valor)
		for _, o := range c.Opciones {
			if o.Valor == v {
				return o.Etiqueta
			}
		}
		return v
	case api.ERPRelacion:
		if n, hay := etiquetas[texto(valor)]; hay {
			return n
		}
		return SinValor
	}
	return texto(valor)
}

// Decimal es `formatearDecimal`: un decimal canónico («1234.5») al formato de quien
// mira («1.234,5»).
//
// Se parte por el punto y se formatea SOLO la parte entera, sin pasar la cadena por un
// flotante: aquí no hay nada que calcular —hace falta separar miles y cambiar el
// separador—, y convertir volvería a meter coma flotante en el único sitio del cliente
// que la evita a propósito.
func Decimal(canonico string, l i18n.Locale) string {
	negativo := strings.HasPrefix(canonico, "-")
	cuerpo := strings.TrimPrefix(canonico, "-")
	entera, decimales, hay := strings.Cut(cuerpo, ".")
	n, err := parsearEntero(entera)
	if err != nil {
		return canonico
	}
	out := i18n.FormatNumber(l, float64(n))
	if hay {
		_, sep := i18n.Separadores(l)
		out += sep + decimales
	}
	if negativo {
		out = "-" + out
	}
	return out
}

// Tono es el color de la insignia de un estado. Es `tonoDeEstado` de ErpApp.tsx, y los
// valores son los que nombra el servicio: no se deducen del rótulo, que está traducido.
func Tono(estado string) string {
	switch estado {
	case "pagado", "hecho", "ganada", "aceptado", "entregado", "aprobado", "contratado":
		return "success"
	case "confirmado", "en_curso", "activa", "publicado":
		return "info"
	case "cancelado", "rechazado", "perdida", "anulado":
		return "danger"
	}
	return "neutral"
}

// RotuloEstado es el rótulo traducido de un estado, tal y como lo declara el modelo. Un
// estado que el modelo no declare se enseña con su clave: es lo que hace la web, y es
// mejor que esconderlo.
func RotuloEstado(m Modelo, estado string) string {
	for _, e := range m.Estados {
		if e.Valor == estado {
			return e.Etiqueta
		}
	}
	return estado
}

// Coincide es el filtro de la paleta: sin distinguir mayúsculas NI TILDES, porque quien
// escribe «fabricacion» espera encontrar «Fabricación».
func Coincide(etiqueta, aguja string) bool {
	return buscar.Contiene(etiqueta, aguja)
}
