// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestElAvisoDeDescargaNoRemiteAlNavegador: la app de escritorio guarda en la carpeta de
// descargas y no hay navegador al que remitir, así que su aviso es
// `gallery.downloadingNative` y no el `gallery.downloading` de la web («…Comprueba las
// descargas del navegador»). Un catálogo regenerado a destiempo ya los igualó una vez
// (63c44a1, arreglado en 2b0ac0d): esto vigila los 26 idiomas y el aviso que se ve, para
// que revertir o colar uno de esos commits suelto se ponga en rojo. VISTO EN ROJO con
// `descargarSeleccion` pidiendo `gallery.downloading`.
func TestElAvisoDeDescargaNoRemiteAlNavegador(t *testing.T) {
	for _, l := range i18n.Locales {
		tr, err := Traductor(l)
		if err != nil {
			t.Fatalf("%s: %v", l, err)
		}
		v := i18n.Vars{"count": 2}
		nativo, web := tr.T("gallery.downloadingNative", v), tr.T("gallery.downloading", v)
		if nativo == "" || nativo == "gallery.downloadingNative" || nativo == web {
			t.Errorf("%s: el aviso de Fyne es %q, el mismo que el de la web o sin traducir", l, nativo)
		}
	}

	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.Guardar = func(string, []byte) (string, error) { return "", nil }
	a.lanzar = func(func()) {} // la descarga no llega a correr: se mira el aviso de salida
	a.entrarEnSeleccion()
	a.alternar(a.g.fotos[0])
	a.alternar(a.g.fotos[1])
	a.descargarSeleccion()
	if quiero := a.v.T.T("gallery.downloadingNative", i18n.Vars{"count": 2}); a.g.aviso != quiero {
		t.Errorf("al descargar el aviso dice %q y quiero %q", a.g.aviso, quiero)
	}
}
