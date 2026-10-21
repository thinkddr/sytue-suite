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
//
// ponytail: solo la base. La tabla y el registro del aviso de un comentario
// (`?table=&record=`) no se siguen: la base se abre en su primera tabla.
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
