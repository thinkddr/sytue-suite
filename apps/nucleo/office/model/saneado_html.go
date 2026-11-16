// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"bytes"
	"encoding/json"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/saneado"
)

// SANEADO DEL HTML DE UN DOCUMENTO (XSS-01, auditoría de seguridad del 2026-10-06).
//
// La regla —qué se quita con su contenido, qué etiquetas, atributos y esquemas de URL se
// conservan, y que para lo legítimo es una no-operación byte a byte— vive en
// apps/nucleo/saneado, porque Docus en Fyne la aplica también a lo que se pega. Aquí se
// aplica a cada registro de contenido de un documento antes de guardarlo.

// SanearHTML devuelve `in` con el contrato aplicado (saneado.HTML). Si no hay nada que
// quitar devuelve `in` tal cual, sin reservar ni copiar.
func SanearHTML(in string) string { return saneado.HTML(in) }

// urlAdmitida es la regla 4 del contrato (saneado.URLAdmitida).
func urlAdmitida(valor string, recurso bool) bool { return saneado.URLAdmitida(valor, recurso) }

// SanearContenido sanea el valor del registro de contenido `clave` de un documento: el HTML
// de un documento de texto, o el JSON de una hoja (el HTML de cada celda) o de una
// presentación (el `html` de cada caja). Lo demás de esos modelos no es HTML y no se toca.
// Si no hay nada que quitar devuelve `contenido` tal cual.
func SanearContenido(clave, contenido string) string {
	switch clave {
	case "doc:html":
		return SanearHTML(contenido)
	case claveHoja, claveDiapos:
	default:
		return contenido
	}
	// Sin un `<` —ni escrito como escape de JSON— no hay HTML en ninguna cadena del modelo:
	// lo normal en una hoja de números, que así no paga ni la lectura del JSON.
	if !strings.Contains(contenido, "<") && !strings.Contains(contenido, `\u003c`) && !strings.Contains(contenido, `\u003C`) {
		return contenido
	}
	dec := json.NewDecoder(strings.NewReader(contenido))
	dec.UseNumber() // los números vuelven a salir como se escribieron
	var modelo any
	if dec.Decode(&modelo) != nil {
		return contenido // no es un modelo: ningún cliente saca de él una celda ni una caja
	}
	var cambiado bool
	if clave == claveHoja {
		cambiado = sanearCeldas(modelo)
	} else {
		cambiado = sanearCampoHTML(modelo)
	}
	if !cambiado {
		return contenido
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if enc.Encode(modelo) != nil {
		return contenido
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// sanearCeldas sanea `sheets[].cells[ref]`, que es donde una hoja lleva HTML.
func sanearCeldas(modelo any) bool {
	m, _ := modelo.(map[string]any)
	hojas, _ := m["sheets"].([]any)
	cambiado := false
	for _, h := range hojas {
		hoja, _ := h.(map[string]any)
		celdas, _ := hoja["cells"].(map[string]any)
		for ref, v := range celdas {
			if txt, ok := v.(string); ok {
				if limpio := SanearHTML(txt); limpio != txt {
					celdas[ref], cambiado = limpio, true
				}
			}
		}
	}
	return cambiado
}

// sanearCampoHTML sanea todo campo `html` del modelo de una presentación: el de las cajas
// de las diapositivas, del patrón y de los diseños, sin la lista de sitios donde hoy hay cajas.
func sanearCampoHTML(v any) bool {
	cambiado := false
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			cambiado = sanearCampoHTML(e) || cambiado
		}
	case map[string]any:
		for k, e := range x {
			if txt, ok := e.(string); ok && k == "html" {
				if limpio := SanearHTML(txt); limpio != txt {
					x[k], cambiado = limpio, true
				}
				continue
			}
			cambiado = sanearCampoHTML(e) || cambiado
		}
	}
	return cambiado
}

// SanearOps sanea, en sitio, los `set` de los registros de contenido de un lote. Es lo que
// llama el almacén antes de guardar.
func SanearOps(ops []Op) {
	for i := range ops {
		if ops[i].Kind != OpSet {
			continue
		}
		switch ops[i].Key {
		case "doc:html", claveHoja, claveDiapos:
		default:
			continue
		}
		var contenido string
		if json.Unmarshal(ops[i].Value, &contenido) != nil {
			continue
		}
		if limpio := SanearContenido(ops[i].Key, contenido); limpio != contenido {
			ops[i].Value = jsonString(limpio)
		}
	}
}
