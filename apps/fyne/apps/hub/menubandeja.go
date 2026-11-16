// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Lo de hoy en la bandeja del sistema
//
// El Hub se queda en la bandeja al cerrar su ventana. Lo que la bandeja enseña antes de
// las pantallas del portal es lo de hoy, como las bandejas de Drive, Dropbox o Microsoft
// 365: cuántos avisos quedan por leer, las mismas acciones del día que las píldoras de la
// portada y lo último que se ha tocado en la suite. NO es el selector de aplicaciones
// que §13.6 deja fuera: los accesos son acciones («Escribir un correo»), no productos.
//
// Es un constructor puro para poder probarlo sin CGO; `cmd/hub` lo monta y lo vuelve a
// montar cuando cambia el contador o «Reciente».

// MaxRecientesBandeja son los recientes que caben en el submenú: un menú del sistema no
// se desplaza y lo de más atrás ya está en «Actividad reciente».
const MaxRecientesBandeja = 5

// LargoRotuloBandeja es lo más largo que se deja un título reciente: un asunto de correo
// entero ensancha el menú del sistema hasta media pantalla.
const LargoRotuloBandeja = 48

// saltoAcceso es a qué producto, y a qué ruta dentro de él, lleva un acceso rápido que
// sale del Hub. Es la tabla de `QUICK_ACTIONS` de la web para esos cuatro, y la usan la
// portada y la bandeja: una sola lista.
type saltoAcceso struct{ producto, ruta string }

var saltosAcceso = map[string]saltoAcceso{
	"quick.compose":  {"mail", "/compose"},
	"quick.newDoc":   {"docs", ""},
	"quick.upload":   {"drive", "/?upload=1"},
	"quick.newEvent": {"calendar", "/new"},
}

// AccesoBandeja es una acción del día tal y como la abre la bandeja: el producto de
// Sytue con su ruta o, si la organización se lo ha dado a otra aplicación, la dirección
// de ESA aplicación, sin la ruta de Sytue (pegarla daría un enlace roto).
type AccesoBandeja struct {
	// Clave es la del catálogo, la misma de la píldora.
	Clave string
	// Producto y Ruta son el destino en Sytue; vacíos si lo sirve otra aplicación.
	Producto string
	Ruta     string
	// URL es la de la aplicación de terceros que sustituye al producto.
	URL string
}

// AccesosBandeja son las acciones del día (`AccionesDelDia`), resueltas contra los
// sustitutos de la organización.
func AccesosBandeja(sustitutos map[string]api.Sustituto) []AccesoBandeja {
	out := make([]AccesoBandeja, 0, len(AccionesDelDia))
	for _, clave := range AccionesDelDia {
		s, ok := saltosAcceso[clave]
		if !ok {
			continue
		}
		if sub, ok := sustitutos[s.producto]; ok {
			out = append(out, AccesoBandeja{Clave: clave, URL: sub.Href})
			continue
		}
		out = append(out, AccesoBandeja{Clave: clave, Producto: s.producto, Ruta: s.ruta})
	}
	return out
}

// AccionesBandeja es lo que hacen las entradas. La bandeja no navega por su cuenta.
type AccionesBandeja struct {
	// Ir trae la ventana del Hub a una de sus pantallas.
	Ir func(Destino)
	// Saltar abre otro producto (`Vista.Saltar`).
	Saltar func(p Producto, ruta string)
	// AbrirURL abre una dirección: un reciente o la aplicación que sustituye a un producto.
	AbrirURL func(string)
}

// MenuBandeja es lo de hoy: «Avisos (3)», las acciones del día y, si hay, el submenú
// con los cinco últimos recientes. Quien lo monta le añade detrás las pantallas y los
// ajustes del sistema.
func MenuBandeja(t *i18n.Translator, sinLeer int, accesos []AccesoBandeja, recientes []api.HubHit, a AccionesBandeja) *fyne.Menu {
	rotuloAvisos := t.T("inbox.heading", nil)
	if sinLeer > 0 {
		rotuloAvisos = t.T("inbox.trayUnread", i18n.Vars{"count": sinLeer})
	}
	elementos := []*fyne.MenuItem{fyne.NewMenuItem(rotuloAvisos, func() {
		if a.Ir != nil {
			a.Ir(AAvisos)
		}
	})}

	if len(accesos) > 0 {
		elementos = append(elementos, fyne.NewMenuItemSeparator())
	}
	for _, acc := range accesos {
		elementos = append(elementos, fyne.NewMenuItem(t.T(acc.Clave, nil), pulsarAcceso(acc, a)))
	}

	if len(recientes) > 0 {
		recientes = recientes[:min(len(recientes), MaxRecientesBandeja)]
		sub := make([]*fyne.MenuItem, 0, len(recientes))
		for _, h := range recientes {
			url := h.URL
			sub = append(sub, fyne.NewMenuItem(rotuloReciente(h), func() {
				if a.AbrirURL != nil && url != "" {
					a.AbrirURL(url)
				}
			}))
		}
		r := fyne.NewMenuItem(t.T("recent.tray", nil), nil)
		r.ChildMenu = fyne.NewMenu("", sub...)
		elementos = append(elementos, fyne.NewMenuItemSeparator(), r)
	}
	return fyne.NewMenu(t.T("shell.apps", nil), elementos...)
}

func pulsarAcceso(acc AccesoBandeja, a AccionesBandeja) func() {
	return func() {
		if acc.URL != "" {
			if a.AbrirURL != nil {
				a.AbrirURL(acc.URL)
			}
			return
		}
		if p, ok := ProductoDe(acc.Producto); ok && a.Saltar != nil {
			a.Saltar(p, acc.Ruta)
		}
	}
}

// rotuloReciente es el título, recortado con «…» si no cabe, y sin título, el nombre del
// producto de donde viene: una entrada en blanco no se puede elegir.
func rotuloReciente(h api.HubHit) string {
	titulo := strings.Join(strings.Fields(h.Title), " ")
	if titulo == "" {
		if p, ok := ProductoDe(h.Source); ok {
			return p.Nombre
		}
		return h.Source
	}
	if r := []rune(titulo); len(r) > LargoRotuloBandeja {
		return strings.TrimSpace(string(r[:LargoRotuloBandeja-1])) + "…"
	}
	return titulo
}

// AccesosBandeja son los accesos con los sustitutos que la vista conoce ahora.
func (v *Vista) AccesosBandeja() []AccesoBandeja {
	v.mu.Lock()
	defer v.mu.Unlock()
	return AccesosBandeja(v.lanzador.Sustitutos)
}

// RecientesBandeja son los recientes ya cargados (nada mientras carga o si falló: la
// bandeja no enseña estados, solo lo que hay).
func (v *Vista) RecientesBandeja() []api.HubHit {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.lanzador.Reciente.Estado != Listo {
		return nil
	}
	return append([]api.HubHit(nil), v.lanzador.Reciente.Items...)
}

// AlCambiarHoy apunta a quien hay que avisar cuando cambia «Reciente» o los sustitutos:
// la bandeja se vuelve a montar. Se llama en el hilo de la interfaz.
func (v *Vista) AlCambiarHoy(f func()) {
	v.mu.Lock()
	v.alCambiarHoy = f
	v.mu.Unlock()
}

// avisarHoy lo llama quien cambia lo de hoy, fuera del cerrojo.
func (v *Vista) avisarHoy() {
	v.mu.Lock()
	f := v.alCambiarHoy
	v.mu.Unlock()
	if f != nil {
		fyne.Do(f)
	}
}
