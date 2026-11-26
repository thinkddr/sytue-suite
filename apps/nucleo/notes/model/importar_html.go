// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"bytes"
	"encoding/json"
)

// LA VUELTA DEL HTML NATIVO. El cuaderno exportado en HTML (exportar_html.go) lleva
// dentro, en un `<script type="application/json">`, el volcado íntegro con el MISMO sobre
// que el `.onex` (formato, versión, cuaderno y notas). Es lo que entrega «Tus datos» de
// cada cuaderno propio (`notes/notebooks/<título>.html`), SEA CUAL SEA el formato de las
// tablas que eligió la persona: en CSV y XML las tablas no se pueden volver a leer sin
// pérdida, y el HTML sí.
//
// [VolcadoDeHTML] saca ese JSON tal cual: lo que devuelve se lee como un `.onex` (por
// ejemplo con `LeerImportable` o `LeerNativoTransferencia`), con su misma validación.

// aperturaVolcado es la etiqueta que escribe volcadoEnScript.
const aperturaVolcado = `<script type="application/json" id="sytue-notepads-volcado">`

// VolcadoDeHTML saca el volcado de un cuaderno exportado en HTML. ok es falso si el
// fichero no lo lleva o lo que lleva no es JSON.
//
// No hace falta deshacer nada: al escribirlo, cada `</` del JSON se pone como `<\/`
// (para que una nota que hable de `</script>` no cierre el bloque antes de tiempo), y
// `\/` es el escape de `/` en JSON. Por eso el primer `</script>` es el final.
func VolcadoDeHTML(datos []byte) (volcado []byte, ok bool) {
	i := bytes.Index(datos, []byte(aperturaVolcado))
	if i < 0 {
		return nil, false
	}
	resto := datos[i+len(aperturaVolcado):]
	j := bytes.Index(resto, []byte("</script>"))
	if j < 0 {
		return nil, false
	}
	volcado = bytes.TrimSpace(resto[:j])
	if !json.Valid(volcado) {
		return nil, false
	}
	return volcado, true
}
