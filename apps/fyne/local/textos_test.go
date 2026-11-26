// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"fmt"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/asistente"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// TestLosTextosEstanEnTodosLosIdiomas: los textos propios de la cáscara tienen las MISMAS
// claves en los 26 idiomas de la suite (PLAN §5.14): español e inglés en cada tabla de
// `tablas`, los otros veinticuatro en sus traducciones, sin uno de más ni de menos, ninguno
// vacío y con los huecos del inglés (`Faltan`).
func TestLosTextosEstanEnTodosLosIdiomas(t *testing.T) {
	if len(tablas) < 3 {
		t.Fatalf("solo hay %d tablas en la cáscara", len(tablas))
	}
	for _, p := range tablas {
		for _, f := range p.Faltan() {
			t.Error(f)
		}
	}
}

// TestCadaClaveEstaEnUnaTabla: una clave en dos tablas saldría con el texto de la primera
// y la otra sería texto muerto, traducido en veinticuatro idiomas para nada.
func TestCadaClaveEstaEnUnaTabla(t *testing.T) {
	donde := map[string]int{}
	for i, p := range tablas {
		for clave := range p.Base {
			if j, ok := donde[clave]; ok {
				t.Errorf("%q está en las tablas %d y %d", clave, j, i)
			}
			donde[clave] = i
		}
	}
}

// Cada idioma sale en el suyo, y lo que no se conoce cae al inglés.
func TestTextoPorIdioma(t *testing.T) {
	for loc, quiero := range map[i18n.Locale]string{"es": "Red", "en": "Network", "de": "Netzwerk", "pt-BR": "Rede", "xx": "Network"} {
		if got := Texto(loc, "red"); got != quiero {
			t.Errorf("%s: %q; quiero %q", loc, got, quiero)
		}
	}
	if got := Texto("fr", "no.existe"); got != "no.existe" {
		t.Errorf("clave desconocida: %q", got)
	}
	// Las claves de las otras tablas también salen en su idioma.
	if got := Texto("de", "cuentas.volver"); got != traduccionesAjustesApps["de"]["cuentas.volver"] || got == "" {
		t.Errorf("de, cuentas.volver: %q", got)
	}
	if got := Texto("es", "agenda.otro"); got != textosAgenda["agenda.otro"][0] {
		t.Errorf("es, agenda.otro: %q", got)
	}
}

func TestPropiosDiceLoQueFalta(t *testing.T) {
	p := Propios{Base: map[string][2]string{"a": {"%d cosas", "%d things"}}, Traducciones: map[i18n.Locale]map[string]string{"fr": {"a": "choses", "b": "x"}}}
	f := strings.Join(p.Faltan(), "\n")
	for _, quiero := range []string{`fr: "a" no lleva los huecos`, `fr: sobra "b"`, "de: no hay traducciones"} {
		if !strings.Contains(f, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, f)
		}
	}
	if p.En("fr", "a") != "choses" || p.En("de", "a") != "%d things" || p.En("es-MX", "a") != "%d cosas" || p.En("fr", "z") != "z" {
		t.Error("En")
	}
}

// Los proveedores de IA de Ajustes se nombran en el idioma de la app: los genéricos y la
// IA del sistema (también la del móvil) no salen con el nombre en español del núcleo.
func TestLosProveedoresDeIASalenEnSuIdioma(t *testing.T) {
	t.Setenv("SYTUE_IDIOMA", "de")
	l := &App{Espacio: &espacio.Espacio{}}
	for _, p := range asistente.Preajustes() {
		n := l.NombreIA(p)
		switch p.ID {
		case asistente.Anthropic, asistente.OpenAI, asistente.Gemini:
			if n != p.Nombre {
				t.Errorf("%s: la marca se dice tal cual, no %q", p.ID, n)
			}
		case asistente.Sistema:
			if n == p.Nombre {
				t.Errorf("%s: sale en español: %q", p.ID, n)
			}
		default:
			if n == p.Nombre || n != traducciones["de"][map[string]string{asistente.Ninguno: "ia.ninguna", asistente.Local: "ia.local", asistente.Otro: "ia.otro"}[p.ID]] {
				t.Errorf("%s: %q no es el nombre alemán", p.ID, n)
			}
		}
	}
}

// En iOS y Android, el nombre de la IA del sistema y lo que le falta salen en el idioma de
// la app (C-12): el núcleo da el estado (`asistente.EstadoDelSistema`) y la pantalla pone
// el texto. Antes salían del núcleo, en español en todos los idiomas. Se prueba en
// cualquier sistema con nombreIAMovil, que es lo que llama NombreIA en el móvil.
func TestLaIADelMovilSaleEnSuIdioma(t *testing.T) {
	for _, e := range asistente.Estados {
		if _, ok := textoEstadoIA[e]; !ok {
			t.Errorf("el estado %d no tiene texto en la pantalla", e)
		}
	}
	nombres := map[string]string{"ios": "ia.nombre.ios", "android": "ia.nombre.android"}
	for _, idioma := range []i18n.Locale{"es", "en", "de", "pl", "pt", "fr"} {
		t.Setenv("SYTUE_IDIOMA", string(idioma))
		l := &App{Espacio: &espacio.Espacio{}}
		for goos, claveNombre := range nombres {
			nombre := Texto(idioma, claveNombre)
			for _, e := range asistente.Estados {
				got := l.nombreIAMovil(goos, e)
				clave := textoEstadoIA[e]
				if clave == "" {
					if got != nombre {
						t.Errorf("%s, %s, estado %d: %q; lista o preguntando es el nombre solo, %q", idioma, goos, e, got, nombre)
					}
					continue
				}
				if quiero := fmt.Sprintf(Texto(idioma, "ia.noDisponible"), nombre, Texto(idioma, clave)); got != quiero {
					t.Errorf("%s, %s, estado %d: %q; quiero %q", idioma, goos, e, got, quiero)
				}
				if idioma != "es" && (strings.Contains(got, textosIA[clave][0]) || strings.Contains(got, "no disponible")) {
					t.Errorf("%s, %s, estado %d: sale en español: %q", idioma, goos, e, got)
				}
			}
		}
	}
	// Lo que antes decía el núcleo: el alemán ya no es el español.
	t.Setenv("SYTUE_IDIOMA", "de")
	l := &App{Espacio: &espacio.Espacio{}}
	if got := l.nombreIAMovil("android", asistente.SistemaApagada); got != "Gemini Nano (auf dem Telefon) — nicht verfügbar: aktivieren Sie Apple Intelligence in den Systemeinstellungen" {
		t.Errorf("de: %q", got)
	}
}
