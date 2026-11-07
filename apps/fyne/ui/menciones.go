// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// Las reglas son las de la web, `apps/frontend/packages/shell/src/menciones.ts`: es
// lógica y no una pantalla, así que no lleva marcador de paridad (§13.7).

import (
	"context"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// Mencionar a alguien de la organización: al escribir «@» se ofrece su gente y, al
// elegir a alguien, se escribe `@ana@empresa.es`, que es la forma que reconocen los
// servicios para avisar (`Mencionados` de `platform/pkg/hubnotify`).
//
// Lo usan los comentarios de Sytue Docs, que son un `AreaTexto` (editor/docus), y el
// editor de una nota de Notepads, que es el suyo propio (apps/notes). Cada uno pinta su
// lista; la regla de cuándo se menciona, a quién se ofrece y de dónde sale la gente es
// esta. Vivía dentro de docus y Notepads la habría copiado (docs/CONVENTIONS.md §13.2),
// que es lo mismo que hizo la web al subirla a `@sytue/shell`.

// blancoJS es el `\s` de JavaScript: el de RE2 se queda en el ASCII, y el espacio duro que
// el editor de notas pone tras un enlace no abría la mención que la web sí abre.
const blancoJS = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

var reMencion = regexp.MustCompile(`(?:^|[` + blancoJS + `])@([^` + blancoJS + `@]*)$`)

// ConsultaMencion es lo que se lleva escrito tras la última «@» suelta, en minúsculas;
// `false` si no se está mencionando (una arroba dentro de una dirección no menciona).
func ConsultaMencion(texto string) (string, bool) {
	m := reMencion.FindStringSubmatch(texto)
	if m == nil {
		return "", false
	}
	return strings.ToLower(m[1]), true
}

// PonerMencion sustituye la mención a medias del final por la dirección elegida.
func PonerMencion(texto, email string) string {
	i := strings.LastIndex(texto, "@")
	if i < 0 {
		return texto
	}
	return texto[:i] + "@" + email + " "
}

// CandidatosMencion son quienes casan con lo escrito, por nombre o por dirección y sin
// acentos: seis como mucho.
func CandidatosMencion(gente []api.Miembro, q string) []api.Miembro {
	var out []api.Miembro
	for _, p := range gente {
		if len(out) == 6 {
			break
		}
		if buscar.Contiene(p.Email, q) || buscar.Contiene(p.Nombre, q) {
			out = append(out, p)
		}
	}
	return out
}

// MoverActiva es adónde va la opción activa de una lista de sugerencias con una flecha:
// da la vuelta en los dos extremos. Otra tecla la deja donde estaba.
func MoverActiva(activa, total int, tecla fyne.KeyName) int {
	switch {
	case total == 0:
		return 0
	case tecla == fyne.KeyDown:
		return (activa + 1) % total
	case tecla == fyne.KeyUp:
		return (activa - 1 + total) % total
	}
	return activa
}

// AgendaMenciones es la gente a la que se puede mencionar (`genteDeLaOrganizacion`): se
// pide UNA vez, a la primera «@», y un fallo de red se reintenta en la siguiente. Vale
// su valor cero. Solo se toca desde el hilo de la interfaz.
type AgendaMenciones struct {
	gente  []api.Miembro
	pedida bool
}

// Gente es la que ya ha llegado (nada hasta entonces). Si aún no se ha pedido y hay
// directorio, la pide en una rutina de `lanzar` y, al llegar, llama a `listo` en el hilo
// de la interfaz.
func (a *AgendaMenciones) Gente(dir *api.Directorio, lanzar func(func()), listo func()) []api.Miembro {
	if a.pedida || dir == nil {
		return a.gente
	}
	a.pedida = true
	if lanzar == nil {
		lanzar = func(f func()) { go f() }
	}
	lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelar()
		// Una página de la agenda, 500, el tope de Sytue ID: como `contactos`.
		gente, err := dir.Agenda(ctx, 500)
		fyne.Do(func() {
			if err != nil {
				a.pedida = false
				return
			}
			a.gente = gente
			if listo != nil {
				listo()
			}
		})
	})
	return a.gente
}

// Poner la deja ya pedida con esa gente: para quien ya la tenga, y para las pruebas.
func (a *AgendaMenciones) Poner(gente []api.Miembro) { a.gente, a.pedida = gente, true }
