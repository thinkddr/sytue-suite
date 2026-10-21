// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import "strings"

/*
Dónde está la aplicación OFICIAL de firma de cada país del Espacio Económico Europeo.

PORTADO VERBATIM de `apps/frontend/packages/office/src/pdf/firmaNacional.ts`, y verbatim
quiere decir que la lista NO se amplía aquí: si hace falta otro país, se añade en la web
—que es donde está verificado quién es la autoridad— y se copia. Un agente que «mejore»
esta lista por su cuenta está publicando recomendaciones sin haberlas comprobado.

POR QUÉ ESTA LISTA EXISTE, y por qué sigue existiendo en nativo. En el navegador no se
puede firmar con el almacén de certificados del sistema: no hay interfaz web que lo
permita. En una app de escritorio SÍ se podría —Llavero, el almacén de Windows, NSS—,
pero eso es criptografía nueva escrita por nosotros, que es exactamente lo que el
comentario de paquete prohíbe. Así que en nativo se hace lo mismo que en la web: quien
tiene su certificado en el sistema, en una tarjeta o en el DNI electrónico firma con la
aplicación de su país y vuelve con el PDF ya firmado.

Y LO QUE NO ESTÁ, NO SE INVENTA: para el resto del EEE se enlaza el navegador de listas
de confianza de la Comisión Europea, que es la fuente oficial.
*/

// ListaDeConfianzaUE es el navegador de listas de confianza de la Comisión Europea.
// Cubre todo el Espacio Económico Europeo y es la salida para cualquier país sin
// entrada propia en [HerramientasNacionales].
const ListaDeConfianzaUE = "https://eidas.ec.europa.eu/efda/tl-browser/"

// HerramientaNacional es una aplicación oficial de firma, con su país.
type HerramientaNacional struct {
	// Pais es ISO 3166-1 alfa-2, que es lo que trae la etiqueta de idioma.
	Pais string
	// Nombre es un nombre propio: es una marca y no se traduce en ningún idioma.
	Nombre string
	URL    string
}

// HerramientasNacionales son las verificadas el 2026-09-10 en la web. Cada una es de
// una administración o de una entidad pública del país, y ninguna cobra por firmar.
//
// Entran solo herramientas OFICIALES y GRATUITAS. No entran las de los prestadores
// privados aunque tengan versión gratuita, porque entonces esto sería una lista de
// recomendaciones comerciales y no una ayuda.
var HerramientasNacionales = []HerramientaNacional{
	// Gobierno de España. La de referencia para el DNI electrónico y la FNMT.
	{Pais: "ES", Nombre: "AutoFirma", URL: "https://firmaelectronica.gob.es/descargas"},
	// Agência para a Modernização Administrativa. Cartão de Cidadão y Chave Móvel.
	{Pais: "PT", Nombre: "Autenticação.gov", URL: "https://www.autenticacao.gov.pt/"},
	// RIA, la agencia estatal estonia de sistemas de información.
	{Pais: "EE", Nombre: "DigiDoc4", URL: "https://www.id.ee/en/rubriik/digidoc4-client/"},
	// A-SIT, el centro austriaco de tecnología segura. Firma PDF de escritorio.
	{Pais: "AT", Nombre: "PDF-Over", URL: "https://technology.a-sit.at/en/pdf-over-3/"},
	// LVRTC, empresa estatal letona.
	{Pais: "LV", Nombre: "eParaksts", URL: "https://www.eparaksts.lv/"},
	// BOSA, el servicio federal belga de política y apoyo.
	{Pais: "BE", Nombre: "eID", URL: "https://eid.belgium.be/"},
	// Servicio del Estado polaco: firma en el navegador con el perfil de confianza,
	// sin instalar nada. Va aquí porque es la vía oficial y gratuita del país.
	{
		Pais:   "PL",
		Nombre: "Podpis zaufany",
		URL:    "https://www.gov.pl/web/gov/podpisz-dokument-elektronicznie-wykorzystaj-podpis-zaufany",
	},
}

// HerramientaDe es la herramienta del país de quien está delante, si la hay.
//
// Se mira la REGIÓN de la etiqueta de idioma (`es-ES` → `ES`) y NO el idioma suelto: un
// austriaco y un alemán escriben los dos `de`, y sus herramientas no son la misma. Sin
// región no se adivina nada y se ofrece la lista entera.
func HerramientaDe(etiqueta string) (HerramientaNacional, bool) {
	partes := strings.Split(etiqueta, "-")
	if len(partes) < 2 || partes[1] == "" {
		return HerramientaNacional{}, false
	}
	region := strings.ToUpper(partes[1])
	for _, h := range HerramientasNacionales {
		if h.Pais == region {
			return h, true
		}
	}
	return HerramientaNacional{}, false
}
