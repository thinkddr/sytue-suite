// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"cmp"
	"net/url"
	"regexp"
	"strings"
)

var esUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// DestinoDeEnlace lee un enlace que llega a la app —`sytue-bases:/bases/base/<id>` o la
// misma página de la web, con o sin la organización en la ruta— y dice qué base abre
// ("" es «Mis bases») y si la app sabe enseñarlo. Lo demás de la web (el formulario
// público `/f/<token>`, un banco de pruebas) no existe aquí.
// La tabla y el registro del aviso de un comentario (`?table=&record=`) los da
// [RegistroDeEnlace].
func DestinoDeEnlace(u *url.URL) (base string, ok bool) {
	partes := strings.FieldsFunc(cmp.Or(u.Path, u.Opaque), func(r rune) bool { return r == '/' })
	// `[bases/][<org>/]base/<id>`: se mira por el final, que la organización puede llamarse
	// como quiera (también «base»).
	if n := len(partes); n >= 2 && n <= 4 && partes[n-2] == "base" && esUUID.MatchString(partes[n-1]) {
		return strings.ToLower(partes[n-1]), true
	}
	// El prefijo `/bases` y, como mucho, la organización.
	return "", len(partes) <= 2
}

// RegistroDeEnlace es la tabla y el registro que pide un enlace de aviso
// (`?table=…&record=…`, `registroDelEnlace` de la web), o dos vacíos si falta alguno o no
// es un id.
func RegistroDeEnlace(u *url.URL) (tabla, registro string) {
	q := u.Query()
	tabla, registro = q.Get("table"), q.Get("record")
	if !esUUID.MatchString(tabla) || !esUUID.MatchString(registro) {
		return "", ""
	}
	return strings.ToLower(tabla), strings.ToLower(registro)
}
