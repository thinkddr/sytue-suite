// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package card

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
)

// Sitios web (URL, RFC 6350 §6.7.8), personas relacionadas (RELATED, §6.6.6) y
// aniversario (ANNIVERSARY, §6.2.6). La web los leía de la vCard; la app nativa, que va
// por la API REST, no los veía y, peor, no podía reenviarlos. Ahora se proyectan
// (migración 0008) con la MISMA regla que aplica la migración a lo que ya estaba:
//
//   - URL: solo direcciones http(s) completas. Un «ftp://», un «www.ejemplo.com» sin
//     esquema… se conservan en la tarjeta tal cual, ocultos, como el BDAY de texto.
//   - RELATED: solo los de texto (un nombre: «Ana»). Una referencia a otra tarjeta
//     (`urn:uuid:`, `mailto:`) no es algo que una persona sepa leer ni escribir en un
//     campo, así que también se conserva oculta.
//   - ANNIVERSARY: solo la fecha completa; uno parcial se conserva.
//
// Lo proyectado se borra al no reenviarlo, que es como se quita; lo oculto se queda.

// URL es un sitio web del contacto. Type es «home», «work» u «other» (sin TYPE).
type URL struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

// Related es una persona relacionada con el contacto, por su nombre. Type es uno de los
// tipos de relación del registro de la IANA (RFC 6350 §6.6.6: «spouse», «child»…), o
// vacío si la tarjeta no dice cuál.
type Related struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

const (
	// maxURL y maxRelated acotan lo que se acepta por la API: un sitio web o un nombre,
	// no un documento.
	maxURL     = 2048
	maxRelated = 256
	// URLOther es el tipo de un sitio web sin TYPE.
	URLOther = "other"
)

// tiposURL son los TYPE de URL que se proyectan; cualquier otro es «other».
var tiposURL = []string{"home", "work"}

// TiposRelacion son los tipos de RELATED del registro de la IANA (RFC 6350 §6.6.6).
var TiposRelacion = []string{
	"contact", "acquaintance", "friend", "met", "co-worker", "colleague", "co-resident",
	"neighbor", "child", "parent", "sibling", "spouse", "kin", "muse", "crush", "date",
	"sweetheart", "me", "agent", "emergency",
}

// conEsquema es un valor que ya es una URI («urn:uuid:…», «mailto:…», «https://…»):
// un esquema y nada de espacios. «Ana: mi hermana» no lo es.
var conEsquema = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:\S*$`)

// otroEsquema es un valor que empieza por un esquema sin «//» («mailto:», «tel:»).
var otroEsquema = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:\D`)

// NormalizeURL valida un sitio web escrito por una persona y lo devuelve completo: sin
// esquema se le pone «https://» («ejemplo.com» → «https://ejemplo.com»), y solo valen
// http y https —un «javascript:» en una ficha que se abre con un clic sería una
// trampa—.
func NormalizeURL(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > maxURL || strings.ContainsAny(v, " \t\r\n") {
		return "", fmt.Errorf("%w: sitio web no válido %q", ErrInvalidContact, v)
	}
	// «mailto:ana@x.es» trae su esquema y no es un sitio web; «ejemplo.com:8080» no lo
	// trae (lo que sigue a los dos puntos es un puerto).
	if !strings.Contains(v, "://") && !otroEsquema.MatchString(v) {
		v = "https://" + v
	}
	u, err := url.Parse(v)
	// Sin usuario: en «https://banco.example@otro.example» se lee un sitio y se abre otro.
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return "", fmt.Errorf("%w: sitio web no válido %q", ErrInvalidContact, v)
	}
	return v, nil
}

// NormalizeURLType es el tipo de un sitio web: «home», «work» u «other».
func NormalizeURLType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if slices.Contains(tiposURL, t) {
		return t
	}
	return URLOther
}

// NormalizeRelated valida el nombre de una persona relacionada.
func NormalizeRelated(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxRelated || strings.ContainsAny(name, "\r\n") {
		return "", fmt.Errorf("%w: persona relacionada no válida", ErrInvalidContact)
	}
	return name, nil
}

// NormalizeRelatedType es el tipo de relación si es uno del registro, o vacío.
func NormalizeRelatedType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if slices.Contains(TiposRelacion, t) {
		return t
	}
	return ""
}

// urlProyectable dice si un URL de la tarjeta se proyecta: una dirección http(s)
// completa, la que NormalizeURL dejaría igual.
func urlProyectable(f *vcard.Field) (string, bool) {
	v := strings.TrimSpace(f.Value)
	if !strings.HasPrefix(strings.ToLower(v), "http://") && !strings.HasPrefix(strings.ToLower(v), "https://") {
		return "", false
	}
	n, err := NormalizeURL(v)
	return n, err == nil && n == v
}

// relatedProyectable dice si un RELATED se proyecta: uno de texto, con un nombre.
func relatedProyectable(f *vcard.Field) (string, bool) {
	v := strings.TrimSpace(f.Value)
	esTexto := strings.EqualFold(f.Params.Get(vcard.ParamValue), "text")
	if v == "" || (!esTexto && conEsquema.MatchString(v)) {
		return "", false
	}
	n, err := NormalizeRelated(v)
	return n, err == nil
}

func decodeURLs(card vcard.Card) []URL {
	var out []URL
	for _, f := range card[vcard.FieldURL] {
		if v, ok := urlProyectable(f); ok {
			out = append(out, URL{URL: v, Type: NormalizeURLType(tipoDe(f, tiposURL...))})
		}
	}
	return out
}

func decodeRelated(card vcard.Card) []Related {
	var out []Related
	for _, f := range card[vcard.FieldRelated] {
		if v, ok := relatedProyectable(f); ok {
			out = append(out, Related{Name: v, Type: NormalizeRelatedType(tipoDe(f, TiposRelacion...))})
		}
	}
	return out
}

// leerAniversario es el ANNIVERSARY si es una fecha completa.
func leerAniversario(f *vcard.Field) *time.Time {
	if f == nil {
		return nil
	}
	if t, err := parseBirthday(f.Value); err == nil {
		return &t
	}
	return nil
}

// camposURL son los URL que se guardan: los proyectados del contacto y, detrás, los de
// la tarjeta de base que la proyección no enseña.
func camposURL(c Contact, base []*vcard.Field) []*vcard.Field {
	out := make([]*vcard.Field, 0, len(c.URLs)+len(base))
	for _, u := range c.URLs {
		f := &vcard.Field{Value: u.URL, Params: vcard.Params{}}
		if t := NormalizeURLType(u.Type); t != URLOther {
			f.Params.Set(vcard.ParamType, t)
		}
		out = append(out, f)
	}
	for _, f := range base {
		if _, ok := urlProyectable(f); !ok {
			out = append(out, f)
		}
	}
	return out
}

// camposRelated son los RELATED que se guardan, con la misma regla que camposURL. Van
// con VALUE=text: sin él, un lector de vCard 4 toma el valor por una URI.
func camposRelated(c Contact, base []*vcard.Field) []*vcard.Field {
	out := make([]*vcard.Field, 0, len(c.Related)+len(base))
	for _, r := range c.Related {
		f := &vcard.Field{Value: r.Name, Params: vcard.Params{}}
		f.Params.Set(vcard.ParamValue, "text")
		if t := NormalizeRelatedType(r.Type); t != "" {
			f.Params.Set(vcard.ParamType, t)
		}
		out = append(out, f)
	}
	for _, f := range base {
		if _, ok := relatedProyectable(f); !ok {
			out = append(out, f)
		}
	}
	return out
}

// campoAniversario es el ANNIVERSARY que se guarda: la fecha del contacto o, si no
// trae, el de la base que no es una fecha completa (oculto). Nil, sin ANNIVERSARY.
func campoAniversario(c Contact, base *vcard.Field) *vcard.Field {
	switch {
	case c.Anniversary != nil:
		return &vcard.Field{Value: c.Anniversary.Format(bdayLayout)}
	case base != nil && strings.TrimSpace(base.Value) != "" && leerAniversario(base) == nil:
		return base
	}
	return nil
}
