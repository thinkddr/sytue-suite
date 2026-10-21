// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

import (
	"fmt"
	"time"
)

// meses son los nombres abreviados de mes por idioma. Go no localiza fechas: su
// paquete time solo formatea en inglés, así que o se traen los nombres a mano o
// se arrastra golang.org/x/text entero para doce palabras por idioma.
//
// El alemán los abrevia con punto (Jan., Feb.) y el italiano y el francés en
// minúscula, como es su norma; no se uniforman por comodidad nuestra.
var meses = map[string][12]string{
	"en": {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	"es": {"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"},
	"fr": {"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."},
	"de": {"Jan.", "Feb.", "März", "Apr.", "Mai", "Juni", "Juli", "Aug.", "Sept.", "Okt.", "Nov.", "Dez."},
	"it": {"gen", "feb", "mar", "apr", "mag", "giu", "lug", "ago", "set", "ott", "nov", "dic"},
	"bg": {"яну", "фев", "март", "апр", "май", "юни", "юли", "авг", "сеп", "окт", "ное", "дек"},
	"cs": {"led", "úno", "bře", "dub", "kvě", "čvn", "čvc", "srp", "zář", "říj", "lis", "pro"},
	"da": {"jan.", "feb.", "mar.", "apr.", "maj", "jun.", "jul.", "aug.", "sep.", "okt.", "nov.", "dec."},
	"el": {"Ιαν", "Φεβ", "Μαρ", "Απρ", "Μαΐ", "Ιουν", "Ιουλ", "Αυγ", "Σεπ", "Οκτ", "Νοε", "Δεκ"},
	"et": {"jaan", "veebr", "märts", "apr", "mai", "juuni", "juuli", "aug", "sept", "okt", "nov", "dets"},
	"fi": {"tammi", "helmi", "maalis", "huhti", "touko", "kesä", "heinä", "elo", "syys", "loka", "marras", "joulu"},
	"ga": {"Ean", "Feabh", "Márta", "Aib", "Beal", "Meith", "Iúil", "Lún", "MFómh", "DFómh", "Samh", "Noll"},
	"hr": {"sij", "velj", "ožu", "tra", "svi", "lip", "srp", "kol", "ruj", "lis", "stu", "pro"},
	"hu": {"jan.", "febr.", "márc.", "ápr.", "máj.", "jún.", "júl.", "aug.", "szept.", "okt.", "nov.", "dec."},
	"is": {"jan.", "feb.", "mar.", "apr.", "maí", "jún.", "júl.", "ágú.", "sep.", "okt.", "nóv.", "des."},
	"lt": {"saus.", "vas.", "kov.", "bal.", "geg.", "birž.", "liep.", "rugp.", "rugs.", "spal.", "lapkr.", "gruod."},
	"lv": {"janv.", "febr.", "marts", "apr.", "maijs", "jūn.", "jūl.", "aug.", "sept.", "okt.", "nov.", "dec."},
	"mt": {"Jan", "Fra", "Mar", "Apr", "Mej", "Ġun", "Lul", "Aww", "Set", "Ott", "Nov", "Diċ"},
	"nb": {"jan.", "feb.", "mars", "apr.", "mai", "juni", "juli", "aug.", "sep.", "okt.", "nov.", "des."},
	"nl": {"jan", "feb", "mrt", "apr", "mei", "jun", "jul", "aug", "sep", "okt", "nov", "dec"},
	"pl": {"sty", "lut", "mar", "kwi", "maj", "cze", "lip", "sie", "wrz", "paź", "lis", "gru"},
	"pt": {"jan.", "fev.", "mar.", "abr.", "mai.", "jun.", "jul.", "ago.", "set.", "out.", "nov.", "dez."},
	"ro": {"ian.", "feb.", "mar.", "apr.", "mai", "iun.", "iul.", "aug.", "sept.", "oct.", "nov.", "dec."},
	"sk": {"jan", "feb", "mar", "apr", "máj", "jún", "júl", "aug", "sep", "okt", "nov", "dec"},
	"sl": {"jan.", "feb.", "mar.", "apr.", "maj", "jun.", "jul.", "avg.", "sep.", "okt.", "nov.", "dec."},
	"sv": {"jan.", "feb.", "mars", "apr.", "maj", "juni", "juli", "aug.", "sep.", "okt.", "nov.", "dec."},
}

// Fecha formatea una fecha con el mes abreviado del idioma.
//
// El orden es día-mes-año en los cinco, incluido el inglés: somos una suite
// europea y el formato estadounidense (mes primero) confunde justo donde más
// importa —en un listado de accesos, «04/08» y «08/04» son días distintos y
// ninguna pista en pantalla dice cuál es cuál—.
func Fecha(locale string, t time.Time, conHora bool) string {
	m := meses[locale]
	if m[0] == "" {
		m = meses[Default]
	}
	// El alemán escribe el día como ordinal, con punto: «28. Juli 2026». Sin él la
	// fecha se lee mal en alemán aunque las palabras estén traducidas.
	dia := fmt.Sprintf("%d", t.Day())
	if locale == "de" {
		dia += "."
	}
	if conHora {
		// Hora de 24 h en los cinco idiomas: es la convención europea y evita la
		// ambigüedad del AM/PM en una lista de sesiones.
		return fmt.Sprintf("%s %s, %02d:%02d", dia, m[t.Month()-1], t.Hour(), t.Minute())
	}
	return fmt.Sprintf("%s %s %d", dia, m[t.Month()-1], t.Year())
}
