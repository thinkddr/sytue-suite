// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"slices"
	"strings"
	"testing"
)

// Una unidad de la nube se mira con el mismo explorador: su nombre es la primera miga,
// lo que se sube va a su raíz, el «⋯» de lo suyo no ofrece lo que el proveedor no
// expone, y elegir «Mi unidad» en el lateral vuelve al disco. Las etiquetas libres
// (DR-09) están en el «⋯» de lo local y no en el de la nube.
func TestUnidadDeLaNubeYEtiquetasEnElEquipo(t *testing.T) {
	tr := traductor(t)
	p := NuevaPantalla(tr, nil, nil, nil)
	p.EnEquipo = (&equipoDoble{}).enEquipo()
	p.EnEquipo.Etiquetas = &EtiquetasEnEquipo{}
	p.EnEquipo.Unidades = func() []UnidadEquipo { return []UnidadEquipo{{ID: "nube~c1~", Nombre: "Nextcloud de casa"}} }
	p.EnEquipo.EsRemoto = func(id string) bool { return strings.HasPrefix(id, "nube~") }
	p.Objeto()

	if len(p.lateral.Unidades) != 1 || p.lateral.Unidades[0].Nombre != "Nextcloud de casa" {
		t.Fatal("la unidad no salió en el lateral", p.lateral.Unidades)
	}
	local := Elemento{ID: "f1", Nombre: "notas.md"}
	if !slices.Contains(textos(p.AccionesDe(local, false, false)), "«etiquetas»") {
		t.Error("lo local debe ofrecer «Etiquetas»", textos(p.AccionesDe(local, false, false)))
	}

	p.AbrirUnidad(p.lateral.Unidades[0])
	if !p.EnUnidad() || p.carpetaActual() != "nube~c1~" || p.explor.RaizNombre != "Nextcloud de casa" || p.lateral.UnidadActiva != "nube~c1~" {
		t.Fatal("no se entró en la unidad", p.carpetaActual(), p.explor.RaizNombre)
	}
	remoto := Elemento{ID: "nube~c1~L0luZm9ybWUuZG9jeA", Nombre: "informe.docx"}
	// Lo que hace el proveedor (renombrar, mover, tirar a SU papelera), la estrella y las
	// etiquetas que se guardan en este equipo —la estrella lo dice—, copiarlo al lado y
	// traérselo; ni versiones ni detalles.
	quiero := []string{"«copiarA»", "Renombrar", "Mover a…", "Hacer una copia", "«etiquetas»", "«favoritoNube»", "Enviar a la papelera"}
	if got := textos(p.AccionesDe(remoto, false, false)); !slices.Equal(got, quiero) {
		t.Errorf("el «⋯» de la nube: %q, quiero %q", got, quiero)
	}
	if got := textos(p.AccionesDe(remoto, false, true)); !slices.Contains(got, "«favoritoNubeQuitar»") {
		t.Errorf("con estrella, quitarla: %q", got)
	}
	carpeta := Elemento{ID: "nube~c1~L0luZm9ybWVz", Nombre: "Informes", Carpeta: true}
	if got := textos(p.AccionesDe(carpeta, false, false)); slices.Contains(got, "«copiarA»") || slices.Contains(got, "Hacer una copia") || !slices.Contains(got, "Mover a…") {
		t.Errorf("una carpeta de la nube no se copia, pero se mueve: %q", got)
	}
	if n := len(p.elementosNuevoEnEquipo()); n != 2 {
		t.Errorf("«Nuevo» en la nube es una carpeta y subir: %d", n)
	}
	// Dentro de una carpeta de la unidad y vuelta a su raíz por las migas.
	p.explor.Ruta = []Elemento{{ID: "nube~c1~L0luZm9ybWVz", Nombre: "Informes", Carpeta: true}}
	if p.carpetaActual() != "nube~c1~L0luZm9ybWVz" {
		t.Fatal("carpeta de la unidad", p.carpetaActual())
	}
	p.explor.Ruta = nil
	if p.carpetaActual() != "nube~c1~" {
		t.Fatal("la raíz de las migas es la de la unidad", p.carpetaActual())
	}
	// Elegir «Mi unidad» en el lateral sale de la unidad.
	p.lateral.AlElegir(MiUnidad)
	if p.EnUnidad() || p.carpetaActual() != "" || p.explor.RaizNombre != "" || p.lateral.UnidadActiva != "" {
		t.Fatal("no se volvió al disco")
	}
	// Si la cuenta desaparece de Ajustes, se sale de su unidad.
	p.AbrirUnidad(UnidadEquipo{ID: "nube~c1~", Nombre: "Nextcloud de casa"})
	p.EnEquipo.Unidades = func() []UnidadEquipo { return nil }
	p.RefrescarUnidades()
	if p.EnUnidad() || len(p.lateral.Unidades) != 0 {
		t.Fatal("una cuenta quitada sigue en el lateral")
	}
}

// En el equipo la papelera no habla de servidores: las dos frases de la ayuda son las
// propias; sin texto propio, las del catálogo.
func TestPapeleraDelEquipoConTextosPropios(t *testing.T) {
	tr := traductor(t)
	p := NuevaPantalla(tr, nil, nil, nil)
	p.EnEquipo = (&equipoDoble{}).enEquipo()
	p.Objeto()
	if got := p.papelera.t("trash.helpCountdown"); got != "«papeleraCuenta»" {
		t.Error("cuenta atrás", got)
	}
	if got := p.papelera.t("trash.help"); got != "«papeleraAyuda»" {
		t.Error("ayuda", got)
	}
	if got := p.papelera.t("trash.title"); got != tr.T("trash.title", nil) {
		t.Error("lo demás es del catálogo", got)
	}
	c := NuevaPantalla(tr, nil, nil, nil)
	c.Objeto()
	if got := c.papelera.t("trash.helpCountdown"); got != tr.T("trash.helpCountdown", nil) {
		t.Error("la conectada no cambia", got)
	}
}
