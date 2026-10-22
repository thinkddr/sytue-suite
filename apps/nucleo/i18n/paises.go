// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

import (
	_ "embed"
	"encoding/json"
)

// paises.json lo escribe gen-i18n.ts con `Intl.DisplayNames(…, { type: 'region' })`: los
// 31 países del ERP (la UE, el EEE y Suiza) en los 26 idiomas, como los pinta la web. No
// salen de golang.org/x/text: su CLDR es otro y no coincide letra a letra.
//
//go:embed paises.json
var paisesJSON []byte

var paises = func() map[Locale]map[string]string {
	var m map[Locale]map[string]string
	if err := json.Unmarshal(paisesJSON, &m); err != nil {
		panic("i18n: paises.json: " + err.Error())
	}
	return m
}()

// CountryName es el nombre de un país del ERP en el idioma: «España», «Spain»,
// «Hispania». Un código que no es de esos 31 sale tal cual, que es lo que devuelve
// `Intl.DisplayNames` cuando no lo conoce.
func CountryName(l Locale, codigo string) string {
	if n, hay := paises[l][codigo]; hay {
		return n
	}
	return codigo
}
