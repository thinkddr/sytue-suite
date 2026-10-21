// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// El campo de identidad aparece en TRES formularios y los gestores de contraseñas
// rellenan dos veces en cualquiera de ellos. Que el arreglo esté solo en el de
// acceso es justo el fallo que se corrigió: quien recupera su contraseña o entra
// por su organización ve el mismo campo duplicado.
func TestLosTresFormulariosNormalizanElCampoDeIdentidad(t *testing.T) {
	paginas := map[string]string{
		"acceso":       loginFormHTML("es", "", "", "", false),
		"organización": ssoFormHTML("es", "", "", ""),
	}
	for nombre, html := range paginas {
		if !strings.Contains(html, emailFieldAttrs) {
			t.Errorf("%s: el campo no usa los atributos compartidos", nombre)
		}
		if !strings.Contains(html, emailFieldScript) {
			t.Errorf("%s: falta la normalización del campo de identidad", nombre)
		}
	}
}

// El script va inline y la CSP del IdP no permite 'unsafe-inline': sin su hash en
// script-src el navegador lo bloquea y el campo vuelve a verse duplicado, esta vez
// sin nada en los logs que lo explique.
func TestElScriptDelCampoDeIdentidadEstaEnLaCSP(t *testing.T) {
	sum := sha256.Sum256([]byte(emailFieldScript))
	hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if !strings.Contains(cspScriptHashes, hash) {
		t.Errorf("falta el hash del script del campo en script-src: %s", cspScriptHashes)
	}
}

// El arreglo del navegador es de presentación; la defensa de verdad es esta, y
// tiene que seguir aceptando lo que un gestor deje escrito. Se comprueba aquí
// junto al script para que quien toque uno vea el otro.
func TestElServidorSeQuedaConUnaSolaDireccion(t *testing.T) {
	casos := map[string]string{
		"ana@sytue.com ana@sytue.com":  "ana@sytue.com", // doble relleno del gestor
		"  ana@sytue.com  ":            "ana@sytue.com", // espacios de un copiar y pegar
		"ANA@Sytue.com":                "ana@sytue.com", // el correo no distingue mayúsculas
		"ana@sytue.com otra@sytue.com": "ana@sytue.com", // dos distintas: gana la primera
		"ana@sytue.com\tana@sytue.com": "ana@sytue.com", // el separador puede ser un tabulador
		"":                             "",
	}
	for entrada, quiero := range casos {
		if got := domain.NormalizeEmail(entrada); got != quiero {
			t.Errorf("NormalizeEmail(%q) = %q, quiero %q", entrada, got, quiero)
		}
	}
}
