// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"regexp"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Borrador es lo que rellena el redactor: `ComposeDraft` de `components/utils.ts`,
// más lo que en la web lleva el propio compositor (el id del borrador guardado y los
// ficheros).
type Borrador struct {
	// De es la dirección «De» elegida; vacía, la principal.
	De string
	// Para, Cc y Cco son lo que la persona escribe: direcciones separadas por comas,
	// con o sin nombre (`Nombre <a@b>`).
	Para, Cc, Cco string
	Asunto        string
	// Cuerpo es texto plano: la cita de la respuesta y la firma incluidas. Es también
	// la parte de texto del mensaje cuando hay HTML.
	Cuerpo string
	// HTML es el cuerpo con formato del editor; vacío mientras no se toca, y entonces el
	// mensaje sale solo con el texto.
	HTML     string
	Adjuntos []eml.Adjunto
	// Drive son los adjuntos de Drive: van al cuerpo como enlaces (TextoConDrive).
	Drive []EnlaceDrive
	// ID es el borrador guardado en Borradores que se está editando, o vacío.
	ID string
	// EnRespuestaA y Referencias son los Message-ID del hilo al que se contesta: con
	// ellos el mensaje se agrupa bien en cualquier otro cliente.
	EnRespuestaA string
	Referencias  []string
}

// ConContenido dice si hay algo que perder: sin nada escrito, cerrar no guarda ni
// pregunta (`hasContent`).
func (b Borrador) ConContenido() bool {
	return strings.TrimSpace(b.Para+b.Cc+b.Cco+b.Asunto+b.Cuerpo) != ""
}

// Los prefijos NO se traducen: son los que entienden todos los clientes de correo, y
// con los que agrupan el hilo.
var (
	prefijoRe  = regexp.MustCompile(`(?i)^\s*re:`)
	prefijoFwd = regexp.MustCompile(`(?i)^\s*(fwd?|rv):`)
)

// direccionDeCampo es `addressToField`: «Nombre <a@b>» o «a@b».
func direccionDeCampo(d Direccion) string {
	if n := espaciosFuera(d.Nombre); n != "" {
		return n + " <" + d.Correo + ">"
	}
	return d.Correo
}

// cita es `quotedBody`: la cabecera de atribución —UNA cadena del catálogo, porque el
// orden «fecha / quién / escribió» cambia entre lenguas— y el cuerpo con «> » por
// línea. Sin el cuerpo completo, el extracto del sobre.
func cita(t *i18n.Translator, l i18n.Locale, m Mensaje, completo string) string {
	cabecera := t.T("reader.quoteHeader", i18n.Vars{
		"date": FechaLarga(l, m.Recibido), "name": NombreVisible(t, m.De),
	})
	texto := completo
	if texto == "" {
		texto = m.Extracto
	}
	lineas := strings.Split(texto, "\n")
	for i, x := range lineas {
		lineas[i] = "> " + x
	}
	return "\n\n" + cabecera + "\n" + strings.Join(lineas, "\n")
}

// BorradorDeRespuesta es `buildReplyDraft`: destinatarios, asunto con su prefijo sin
// duplicarlo y el cuerpo citado. Responder a todos pone en copia al resto de
// destinatarios originales, sin la persona ni el remitente y sin repetir.
func BorradorDeRespuesta(t *i18n.Translator, l i18n.Locale, m Mensaje, modo Respuesta, yo, completo string) Borrador {
	asunto := m.Asunto
	b := Borrador{Cuerpo: cita(t, l, m, completo), EnRespuestaA: m.MessageID}
	if m.MessageID != "" {
		b.Referencias = []string{m.MessageID}
	}
	if modo == Reenviar {
		b.Asunto = asunto
		if !prefijoFwd.MatchString(asunto) {
			b.Asunto = "Fwd: " + asunto
		}
		b.EnRespuestaA, b.Referencias = "", nil // un reenvío empieza hilo
		return b
	}
	b.Asunto = asunto
	if !prefijoRe.MatchString(asunto) {
		b.Asunto = "Re: " + asunto
	}
	var para []string
	for _, d := range m.De {
		para = append(para, direccionDeCampo(d))
	}
	b.Para = strings.Join(para, ", ")
	if modo == ResponderATodos {
		yo = strings.ToLower(strings.TrimSpace(yo))
		fuera := map[string]bool{yo: true}
		for _, d := range m.De {
			fuera[strings.ToLower(d.Correo)] = true
		}
		var cc []string
		for _, d := range append(append([]Direccion{}, m.Para...), m.CC...) {
			e := strings.ToLower(d.Correo)
			if fuera[e] {
				continue
			}
			fuera[e] = true
			cc = append(cc, direccionDeCampo(d))
		}
		b.Cc = strings.Join(cc, ", ")
	}
	return b
}

// ConFirma es `seedHtml` en texto: la firma va detrás, separada por el «-- » que los
// clientes de correo reconocen, salvo en un borrador reabierto —ya la lleva— o si no
// hay firma.
func ConFirma(cuerpo, firma string, esBorrador bool) string {
	if esBorrador || strings.TrimSpace(firma) == "" {
		return cuerpo
	}
	return cuerpo + "\n\n-- \n" + firma
}

// TituloRedactor es `composeTitle`: «Responder» o «Reenviar» según el prefijo del
// asunto, «Editar borrador» si se reabre uno, y si no, «Nuevo mensaje».
func TituloRedactor(t *i18n.Translator, asunto string, borrador bool) string {
	switch {
	case borrador:
		return t.T("compose.editDraftTitle", nil)
	case prefijoRe.MatchString(asunto):
		return t.T("compose.replyTitle", nil)
	case prefijoFwd.MatchString(asunto):
		return t.T("compose.forwardTitle", nil)
	}
	return t.T("compose.newTitle", nil)
}

// patronAdjunto es el de `client/adjuntos.ts`: «te adjunto», «attached», «pièce
// jointe», «Anhang»… en los idiomas que la web vigila.
var patronAdjunto = regexp.MustCompile(`(?i)\b(adjunt[oa]s?|adjuntad[oa]s?|attach(?:ed|ment|ments|ing)?|pi[eè]ces? jointes?|ci-joint[es]?|anh[aä]ng(?:e|end)?|angeh[aä]ngt|beigef[uü]gt|anbei|allegat[oaie]|allego)\b`)

// MencionaAdjunto es `mencionaAdjunto(sinCitas(…))`: el texto dice que lleva algo
// adjunto. Las líneas citadas («> …») no cuentan: el «te adjunto» de quien escribió
// antes no es de quien envía ahora.
func MencionaAdjunto(cuerpo string) bool {
	var propias []string
	for _, l := range strings.Split(cuerpo, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), ">") {
			propias = append(propias, l)
		}
	}
	return patronAdjunto.MatchString(strings.Join(propias, "\n"))
}
