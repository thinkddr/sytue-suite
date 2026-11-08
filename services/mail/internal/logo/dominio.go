// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package logo busca el logo de la web de un remitente para el avatar de la bandeja
// (como Proton o Apple Mail): el icono que declara su portada, o el
// `apple-touch-icon.png` o el `favicon.ico` de siempre.
//
// Lo busca el SERVICIO y no el cliente, y por DOMINIO: la web del remitente no sabe
// quién lee ni cuándo, solo que alguien de Sytue recibió correo suyo alguna vez. Las
// peticiones salen por la barrera de platform/pkg/ssrfhttp.
//
// La decisión de qué dominio se mira es la misma que la de la web
// (apps/frontend/apps/mail/src/lib/dominioRemitente.ts): las dos recorren
// testdata/dominios.json.
package logo

import (
	"regexp"
	"strings"
)

// buzonesPersonales: detrás de `ana@gmail.com` hay una PERSONA, no Google. Poner el
// logo de Gmail en su fila sería mentir sobre quién escribe.
var buzonesPersonales = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "outlook.com": true, "outlook.es": true,
	"hotmail.com": true, "hotmail.es": true, "live.com": true, "msn.com": true,
	"yahoo.com": true, "yahoo.es": true, "ymail.com": true, "icloud.com": true,
	"me.com": true, "mac.com": true, "aol.com": true, "proton.me": true,
	"protonmail.com": true, "pm.me": true, "gmx.com": true, "gmx.es": true,
	"gmx.de": true, "gmx.net": true, "web.de": true, "mail.com": true, "zoho.com": true,
	"yandex.com": true, "yandex.ru": true, "mail.ru": true, "tutanota.com": true,
	"tuta.io": true, "telefonica.net": true, "sytue.eu": true,
}

// sufijosDobles son los sufijos públicos de dos niveles más comunes. No es la Public
// Suffix List entera: si uno raro se cuela, no hay logo y salen las iniciales.
var sufijosDobles = map[string]bool{
	"co.uk": true, "org.uk": true, "ac.uk": true, "gov.uk": true, "com.es": true,
	"org.es": true, "gob.es": true, "edu.es": true, "nom.es": true, "com.mx": true,
	"com.ar": true, "com.br": true, "com.co": true, "com.pe": true, "com.au": true,
	"co.nz": true, "co.jp": true, "co.za": true, "com.tr": true, "com.cn": true,
	"co.in": true,
}

var etiqueta = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Registrable normaliza un host a su parte registrable (`corp.vodafone.es` →
// `vodafone.es`), o "" si no es un dominio público válido. Los subdominios de envío
// casi nunca tienen web; la marca vive en el registrable.
func Registrable(raw string) string {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if host == "" || len(host) > 253 {
		return ""
	}
	partes := strings.Split(host, ".")
	if len(partes) < 2 {
		return ""
	}
	for _, p := range partes {
		if !etiqueta.MatchString(p) {
			return ""
		}
	}
	// Un TLD numérico es una IP escrita como dominio.
	if strings.Trim(partes[len(partes)-1], "0123456789") == "" {
		return ""
	}
	n := 2
	if sufijosDobles[strings.Join(partes[len(partes)-2:], ".")] {
		n = 3
	}
	if len(partes) < n {
		return ""
	}
	return strings.Join(partes[len(partes)-n:], ".")
}

// DeCorreo es el dominio del que buscar el logo de una dirección, o "" si la fila se
// queda con las iniciales: dirección mal formada o buzón personal gratuito.
func DeCorreo(correo string) string {
	i := strings.LastIndex(correo, "@")
	if i < 1 {
		return ""
	}
	return Valido(correo[i+1:])
}

// Valido es Registrable sin los buzones personales: lo que el endpoint acepta.
func Valido(dominio string) string {
	d := Registrable(dominio)
	if d == "" || buzonesPersonales[d] {
		return ""
	}
	return d
}
