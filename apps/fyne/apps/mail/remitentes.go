// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"encoding/base64"
	"net/mail"
	"regexp"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # El «De» del redactor (fase B, punto 6)
//
// Como el de Apple Mail: las direcciones de Sytue —la principal, los alias y los buzones
// compartidos que dejan enviar— y después cada cuenta externa que puede enviar. Elegir
// una externa manda el correo por su proveedor (`MailSend`, con `ExpectedAddress`) en
// vez de por Sytue. Es `remitentes.ts` de la web, con los mismos casos
// (testdata/remitentes.json).

// prefijoExterna va delante de la instancia en el valor de una externa.
const prefijoExterna = "externa:"

// limiteAdjuntosExternos es lo que caben los adjuntos: con la sobrecarga de MIME y
// base64 entra en los 25 MiB por mensaje del proveedor (`LIMITE_ADJUNTOS_EXTERNOS`).
const limiteAdjuntosExternos = 18 << 20

// OpcionDe es una opción del «De» (`Remitente` en la web).
type OpcionDe struct {
	// Valor es la dirección de Sytue, o `externa:<instancia>`.
	Valor string
	// Direccion es lo que se lee en el selector.
	Direccion string
	// Cuenta es la instancia de la cuenta externa; vacía en las de Sytue.
	Cuenta string
}

// valorExterno es el valor del «De» que escribe desde esa cuenta externa.
func valorExterno(instancia string) string { return prefijoExterna + instancia }

// valorDe es el valor del «De» de un borrador.
func valorDe(b Borrador) string {
	if b.Cuenta != "" {
		return valorExterno(b.Cuenta)
	}
	return b.De
}

// remitentes son las opciones del «De»: las de Sytue, ya sin repetir, y detrás las
// externas que envían, en el orden de la lista (la primera vez que sale una instancia
// es la que cuenta). Una externa sin dirección, o con la de una opción de Sytue, no se
// ofrece: dos «ana@ejemplo.eu» iguales no dirían por dónde sale el correo.
func remitentes(sytue []string, externas []api.ExternalMailAccount) []OpcionDe {
	out := make([]OpcionDe, 0, len(sytue)+len(externas))
	vistas := map[string]bool{}
	for _, d := range sytue {
		out = append(out, OpcionDe{Valor: d, Direccion: d})
		vistas[strings.ToLower(d)] = true
	}
	cuentas := map[string]bool{}
	for _, c := range externas {
		if cuentas[c.InstanceID] {
			continue
		}
		cuentas[c.InstanceID] = true
		if !c.Capabilities.Send || c.Address == "" || vistas[strings.ToLower(c.Address)] {
			continue
		}
		vistas[strings.ToLower(c.Address)] = true
		out = append(out, OpcionDe{Valor: valorExterno(c.InstanceID), Direccion: c.Address, Cuenta: c.InstanceID})
	}
	return out
}

// remitenteElegido es la opción de ese valor. La que ya no está (la cuenta se
// desconectó) se queda en la primera, que es lo que enseña el selector: nunca se envía
// desde algo que no se ve. `false` sin opciones.
func remitenteElegido(valor string, lista []OpcionDe) (int, bool) {
	for i, r := range lista {
		if r.Valor == valor {
			return i, true
		}
	}
	return 0, len(lista) > 0
}

// imagenCid es un `<img src="cid:…">`: una imagen metida en el texto.
var imagenCid = regexp.MustCompile(`(?i)<img\b[^>]*?\bsrc\s*=\s*["']?cid:[^>]*>`)

// sinImagenesCid quita del HTML las imágenes metidas en el texto: el envío de una
// cuenta externa no lleva Content-ID, así que esas imágenes salen como adjuntos.
func sinImagenesCid(html string) string { return imagenCid.ReplaceAllString(html, "") }

// direccionesExternas lee «Para», «CC» o «CCO»: direcciones separadas por comas (o
// puntos y comas), con o sin nombre. Vacío no es un error.
func direccionesExternas(text string) ([]api.ExternalMailAddress, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	addresses, err := mail.ParseAddressList(strings.ReplaceAll(text, ";", ","))
	if err != nil {
		return nil, err
	}
	out := make([]api.ExternalMailAddress, len(addresses))
	for i, a := range addresses {
		out[i] = api.ExternalMailAddress{Name: a.Name, Email: a.Address}
	}
	return out, nil
}

// salidaExterna es el correo tal y como lo envía el proveedor de esa cuenta: `texto` y
// `html` ya con los enlaces de Drive, y todos los ficheros como adjuntos. Un error de
// dirección es errDireccion; uno de tamaño, errDemasiadoGrande.
func salidaExterna(direccion string, br Borrador, texto, html string) (api.ExternalMailOutgoing, error) {
	var listas [3][]api.ExternalMailAddress
	for i, campo := range []string{br.Para, br.Cc, br.Cco} {
		d, err := direccionesExternas(campo)
		if err != nil {
			return api.ExternalMailOutgoing{}, errDireccion
		}
		listas[i] = d
	}
	total := 0
	adjuntos := make([]api.ExternalMailAttachment, 0, len(br.Adjuntos))
	for _, a := range br.Adjuntos {
		total += len(a.Datos)
		adjuntos = append(adjuntos, api.ExternalMailAttachment{Name: a.Nombre, Type: a.Tipo, Size: int64(len(a.Datos)), ContentBase64: base64.StdEncoding.EncodeToString(a.Datos)})
	}
	if total > limiteAdjuntosExternos {
		return api.ExternalMailOutgoing{}, errDemasiadoGrande
	}
	out := api.ExternalMailOutgoing{
		// El servidor comprueba que la cuenta sigue siendo esa dirección.
		ExpectedAddress: direccion, To: listas[0], Cc: listas[1], Bcc: listas[2],
		Subject: br.Asunto, Text: texto, Attachments: adjuntos,
	}
	if h := sinImagenesCid(html); strings.TrimSpace(h) != "" {
		out.HTML = h
	}
	return out, nil
}
