// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// textosAviso es lo que la app dice de VERI*FACTU (D-29), en español e inglés; los otros
// veinticuatro idiomas de la suite (C-12) están en textos_idiomas.go. Va aquí y no en el
// catálogo de la pantalla, que es el de la edición conectada y no se toca.
var textosAviso = map[string][2]string{ // clave: {es, en}
	"verifactu": {
		"Sytue ERP de Sytue Office emite las facturas SIN VERI*FACTU: no genera registros ni los envía a la AEAT. Si tu actividad lo exige, usa un programa que lo lleve.",
		"Sytue ERP in Sytue Office issues invoices WITHOUT VERI*FACTU: it neither creates records nor sends them to the Spanish Tax Agency (AEAT). If your business needs it, use software that supports it.",
	},
}

// aviso son los textos de la franja, en los 26 idiomas.
var aviso = i18n.Propios{Base: textosAviso, Traducciones: traduccionesAviso}

// AvisoVerifactu es el aviso en el idioma dado (código BCP 47 o su base); uno que no es
// de la suite, en inglés.
func AvisoVerifactu(idioma string) string {
	return aviso.En(i18n.Locale(strings.ToLower(idioma)), "verifactu")
}

// DatosEnvio es lo que dice el correo de una factura: su número, el importe a pagar, el
// vencimiento (dd/mm/aaaa; vacío si no tiene) y quién la emite.
type DatosEnvio struct {
	Numero, Importe, Vence, Emisor string
}

// plantillasEnvio son el asunto y el texto del correo de una factura, propios de Sytue
// Office (la web de la suite propone «Factura {ref}» y una frase): con el número, el
// importe y el vencimiento, y firmado por la organización. En español e inglés; los otros
// veinticuatro idiomas, en textos_idiomas.go.
var plantillasEnvio = map[string][2]string{
	"asunto":       {"Factura {numero}", "Invoice {numero}"},
	"asuntoEmisor": {"Factura {numero} de {emisor}", "Invoice {numero} from {emisor}"},
	"saludo":       {"Hola:", "Hello,"},
	"cuerpo":       {"Te enviamos adjunta la factura {numero}, por un importe de {importe}.", "Please find attached invoice {numero}, for {importe}."},
	"cuerpoVence": {
		"Te enviamos adjunta la factura {numero}, por un importe de {importe}, con vencimiento el {vence}.",
		"Please find attached invoice {numero}, for {importe}, due on {vence}.",
	},
	"despedida": {"Un saludo,", "Kind regards,"},
}

// envio son las plantillas del correo, en los 26 idiomas.
var envio = i18n.Propios{Base: plantillasEnvio, Traducciones: traduccionesEnvio}

// PlantillaEnvio son el asunto y el texto propuestos para el correo de la factura en ese
// idioma (código BCP 47 o su base); en uno que no es de la suite, en inglés.
func PlantillaEnvio(idioma string, d DatosEnvio) (asunto, mensaje string) {
	loc := i18n.Locale(strings.ToLower(idioma))
	r := strings.NewReplacer("{numero}", d.Numero, "{importe}", d.Importe, "{vence}", d.Vence, "{emisor}", d.Emisor)
	poner := func(clave string) string { return r.Replace(envio.En(loc, clave)) }
	asunto = poner("asunto")
	if d.Emisor != "" {
		asunto = poner("asuntoEmisor")
	}
	cuerpo := poner("cuerpo")
	if d.Vence != "" {
		cuerpo = poner("cuerpoVence")
	}
	mensaje = poner("saludo") + "\n\n" + cuerpo + "\n\n" + poner("despedida")
	if d.Emisor != "" {
		mensaje += "\n" + d.Emisor
	}
	return asunto, mensaje
}
