// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Valores de celda: de lo que viaja (JSON) a lo que se escribe en la rejilla y de vuelta.
// Es el espejo de `lib/valores.ts` y, como allí, COMODIDAD de la interfaz: el servicio
// vuelve a validar todo y es el que manda.

// puedeEscribir dice si el rol deja escribir registros (editor o propietario).
func puedeEscribir(rol string) bool { return rol == api.BasesOwner || rol == api.BasesEditor }

// textoDeValor es el valor de una celda como texto para mostrar o para editar.
func textoDeValor(valor any) string {
	switch v := valor.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "✓"
		}
	case []any:
		partes := make([]string, len(v))
		for i, x := range v {
			partes[i] = textoDeValor(x)
		}
		return strings.Join(partes, ", ")
	case []string:
		return strings.Join(v, ", ")
	}
	return ""
}

// valorDeTexto es lo escrito en un campo → el valor a enviar. nil es vaciar. `ok` falso:
// no vale para ese tipo (la interfaz avisa y no envía).
func valorDeTexto(campo api.BasesField, texto string) (valor any, ok bool) {
	t := strings.TrimSpace(texto)
	if t == "" {
		return nil, true
	}
	switch campo.Kind {
	case api.BasesKindText:
		return texto, true
	case api.BasesKindNumber:
		if n, ok := leerNumero(t); ok {
			return n, true
		}
		return nil, false
	case api.BasesKindDate:
		if _, err := time.Parse("2006-01-02", t); err != nil {
			return nil, false
		}
		return t, true
	case api.BasesKindSelect:
		return t, slices.Contains(campo.Choices, t)
	}
	return valorConFormato(campo.Kind, t)
}

// leerNumero admite la coma decimal, que es como se escribe en gran parte de la UE.
func leerNumero(t string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.Replace(t, ",", ".", 1), 64)
	return n, err == nil && !math.IsInf(n, 0) && !math.IsNaN(n) && math.Abs(n) <= 1e15
}

// valorConFormato es valorDeTexto para los tipos con formato (URL, correo, teléfono,
// moneda y valoración); t ya viene recortado y no vacío.
func valorConFormato(kind, t string) (any, bool) {
	switch kind {
	case api.BasesKindCurrency:
		n, ok := leerNumero(t)
		if !ok {
			return nil, false
		}
		// Céntimos, como el servicio: más decimales son un error de redondeo, no un dato.
		return math.Round(n*100) / 100, true
	case api.BasesKindRating:
		n, err := strconv.Atoi(t)
		if err != nil || n < 0 || n > maxValoracion {
			return nil, false
		}
		if n == 0 { // cero estrellas es «sin valorar»
			return nil, true
		}
		return float64(n), true
	case api.BasesKindURL:
		return siVale(normalizarURL(t))
	case api.BasesKindEmail:
		return siVale(normalizarCorreo(t))
	case api.BasesKindPhone:
		return siVale(normalizarTelefono(t))
	}
	return nil, false
}

func siVale(s string, ok bool) (any, bool) {
	if !ok {
		return nil, false
	}
	return s, true
}

// valorEscrito es lo tecleado en un campo de texto, número o fecha → el valor a enviar.
// La fecha se teclea como se escribe (05/10/2026) y viaja en ISO. Es la cuenta de la
// celda de la rejilla y de la ficha del registro: las dos aceptan lo mismo.
func valorEscrito(campo api.BasesField, texto string) (valor any, ok bool) {
	if campo.Kind == api.BasesKindDate {
		if f, vacia, vale := ui.LeerFecha(texto, time.UTC); vale && !vacia {
			texto = f.Format("2006-01-02")
		} else if vacia {
			texto = ""
		} else {
			texto = "?"
		}
	}
	return valorDeTexto(campo, texto)
}

// textoParaEditar es lo que se pone en el campo al empezar a editar un valor: la fecha,
// como se escribe.
func textoParaEditar(campo api.BasesField, valor any) string {
	texto := textoDeValor(valor)
	if campo.Kind == api.BasesKindDate {
		if f, err := time.Parse("2006-01-02", texto); err == nil {
			texto = ui.EscribirFecha(f)
		}
	}
	return texto
}

// etiquetaDeRegistro es cómo se llama un registro en una lista de enlaces: su primer
// campo con valor.
func etiquetaDeRegistro(campos []api.BasesField, r api.BasesRecord) string {
	for _, c := range campos {
		if c.Kind == api.BasesKindLink || c.Kind == api.BasesKindCheckbox || c.Kind == api.BasesKindAttachment {
			continue
		}
		if t := textoDeValor(r.Data[c.ID]); t != "" {
			return t
		}
	}
	return r.ID[:min(len(r.ID), 8)]
}

// cadenasDe son los valores de una celda de selección múltiple, o nada.
func cadenasDe(valor any) []string {
	if ya, ok := valor.([]string); ok { // armado aquí y no llegado en JSON
		return ya
	}
	lista, _ := valor.([]any)
	out := make([]string, 0, len(lista))
	for _, x := range lista {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// efectivo es el campo tal como se filtra, se ordena y se agrega: una fórmula se comporta
// como el tipo de su resultado, y un campo con formato como el número o el texto que
// guarda (`lib/formulas.ts`, y `Campo.Efectivo` en el servicio).
func efectivo(c api.BasesField) api.BasesField {
	switch c.Kind {
	case api.BasesKindFormula:
		c.Kind = c.Result
		if c.Kind == "" {
			c.Kind = api.BasesKindNumber
		}
	case api.BasesKindCurrency, api.BasesKindRating:
		c.Kind = api.BasesKindNumber
	case api.BasesKindURL, api.BasesKindEmail, api.BasesKindPhone:
		c.Kind = api.BasesKindText
	}
	return c
}

// maxValoracion es el tope de un campo de valoración (`MAX_VALORACION`).
const maxValoracion = 5

// monedas son las que se ofrecen al crear un campo de moneda (`MONEDAS` en la web).
var monedas = []string{"EUR", "USD", "GBP", "CHF", "SEK", "DKK", "NOK", "ISK", "PLN", "CZK", "HUF", "RON", "BGN", "JPY"}

// textoVisible es el valor como se LEE en la rejilla, el tablero o la ficha: un importe
// con su moneda y el formato del idioma, y una valoración en estrellas. Para editar,
// textoParaEditar.
func textoVisible(campo api.BasesField, valor any, idioma i18n.Locale) string {
	n, esNumero := valor.(float64)
	switch {
	case campo.Kind == api.BasesKindCurrency && esNumero:
		return i18n.FormatCurrencyIn(idioma, int64(math.Round(n*100)), campo.Currency)
	case campo.Kind == api.BasesKindRating && esNumero:
		return estrellas(int(math.Round(n)))
	}
	return textoDeValor(valor)
}

// estrellas es «★★★☆☆»: las llenas y las vacías, para que se lea el tope.
func estrellas(n int) string {
	n = max(0, min(maxValoracion, n))
	return strings.Repeat("★", n) + strings.Repeat("☆", maxValoracion-n)
}

// Las tres normalizaciones repiten las de `lib/valores.ts` (y las del servicio, en
// `internal/domain/formatos.go`) para avisar antes de enviar.

func normalizarURL(t string) (string, bool) {
	if len(t) > 2048 || strings.ContainsAny(t, " \t\n\\") {
		return "", false
	}
	if !strings.Contains(t, "://") {
		if host, _, _ := strings.Cut(t, "/"); strings.Contains(host, ":") && !strings.Contains(t, ".") {
			return "", false
		}
		t = "https://" + t
	}
	u, err := url.Parse(t)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || strings.Contains(u.Host, "..") {
		return "", false
	}
	u.Host = strings.ToLower(u.Host)
	return u.String(), true
}

func normalizarCorreo(t string) (string, bool) {
	if len(t) > 254 || strings.ContainsAny(t, " <>,;\"") {
		return "", false
	}
	i := strings.LastIndexByte(t, '@')
	if i <= 0 {
		return "", false
	}
	dominio := strings.ToLower(t[i+1:])
	partes := strings.Split(dominio, ".")
	if len(partes) < 2 {
		return "", false
	}
	for _, p := range partes {
		if p == "" || strings.Trim(p, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
			return "", false
		}
	}
	return t[:i] + "@" + dominio, true
}

func normalizarTelefono(t string) (string, bool) {
	if len([]rune(t)) > 32 {
		return "", false
	}
	digitos := 0
	for i, r := range t {
		switch {
		case r >= '0' && r <= '9':
			digitos++
		case r == '+' && i == 0:
		case strings.ContainsRune(" -.()/", r):
		default:
			return "", false
		}
	}
	if digitos < 3 || digitos > 20 {
		return "", false
	}
	return strings.Join(strings.Fields(t), " "), true
}
