// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"net/url"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// fichaDeMuestra es un contacto con todo lo que la ficha sabe enseñar.
func fichaDeMuestra() api.Contact {
	cumple := time.Date(1990, 5, 4, 0, 0, 0, 0, time.UTC)
	return api.Contact{
		ID: "ada", UID: "u-ada", FormattedName: "Ada Lovelace", Organization: "Analytical Engines",
		Birthday: &cumple, Note: "Primera programadora.\n\nLlamar por la tarde.",
		Emails: []api.ContactEmail{
			{Email: "ada@trabajo.test", Type: "work"},
			{Email: "ada@casa.test", Type: "home", IsPrimary: true},
		},
		Phones:    []api.ContactPhone{{Phone: "+34 600 11 22 33", Type: "cell"}, {Phone: "+44 20 7946 0000", Type: "iPhone"}},
		Addresses: []api.ContactAddress{{Street: "12 St James's Square", PostalCode: "SW1Y 4LB", Locality: "London", Country: "UK", Type: "other"}},
	}
}

// textosDe recoge lo que se lee en un árbol: rótulos, textos y enlaces. Sirve para
// comprobar la COPIA de una pantalla sin depender de dónde cae cada píxel.
func textosDe(o fyne.CanvasObject) []string {
	var fuera []string
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			fuera = append(fuera, textosDe(h)...)
		}
	case *container.Scroll:
		fuera = append(fuera, textosDe(x.Content)...)
	case *ui.Rotulo:
		fuera = append(fuera, x.Texto)
	case *ui.Parrafo:
		fuera = append(fuera, x.Texto)
	case *canvas.Text:
		fuera = append(fuera, x.Text)
	case *ui.Enlace:
		fuera = append(fuera, x.Texto)
	case *ui.Tarjeta:
		fuera = append(fuera, x.Titulo)
		fuera = append(fuera, textosDe(x.Cuerpo)...)
	case *ui.Boton:
		fuera = append(fuera, x.Texto)
	}
	return fuera
}

func contieneTexto(textos []string, s string) bool {
	for _, t := range textos {
		if t == s {
			return true
		}
	}
	return false
}

// TestLaFichaDiceLoQueDiceLaWeb: el principal delante, cada valor con su etiqueta (y un
// TYPE que no es del editor, tal cual), la dirección en líneas postales con «Otra», el
// cumpleaños en fecha LARGA y la nota partida por sus saltos de línea.
func TestLaFichaDiceLoQueDiceLaWeb(t *testing.T) {
	p := pantallaDeMuestra(t)
	c := fichaDeMuestra()
	p.Seleccionado = &c
	textos := textosDe(p.ficha())

	for _, quiero := range []string{
		"Ada Lovelace", "Analytical Engines", "Información de contacto",
		"ada@casa.test", "Casa", "ada@trabajo.test", "Trabajo",
		"+34 600 11 22 33", "Móvil", "+44 20 7946 0000", "iPhone",
		"Otra", "12 St James's Square", "SW1Y 4LB London", "UK",
		i18n.FormatDateLong("es", *c.Birthday), "Sin etiquetas",
	} {
		if !contieneTexto(textos, quiero) {
			t.Errorf("la ficha no dice %q", quiero)
		}
	}
	if got := i18n.FormatDateLong("es", *c.Birthday); got != "4 de mayo de 1990" {
		t.Errorf("el cumpleaños sale %q y la web dice «4 de mayo de 1990»", got)
	}
	if vs := correosTipados(c); vs[0].valor != "ada@casa.test" {
		t.Errorf("el primer correo es %q y va delante el principal", vs[0].valor)
	}
	if got := correoPrincipal(c); got != "ada@casa.test" {
		t.Errorf("«Enviar correo» iría a %q y el principal es ada@casa.test", got)
	}
}

// TestElValorDelDlEmpiezaDondeLaWeb: ContactDetail.tsx usa px-6 (24) + columna
// de 8rem (128) + gap-4 (16) = 168 px; bajo sm: px-4 + 6.5rem + gap-4 = 136.
func TestElValorDelDlEmpiezaDondeLaWeb(t *testing.T) {
	for _, c := range []struct {
		ventana float32
		quiero  float32
	}{{0, 168}, {1280, 168}, {600, 136}} {
		dt, dd := ui.NuevoRotulo("Correo", tema.TextoSM, ui.Medio, "text-muted"), ui.NuevoRotulo("x", tema.TextoBase, ui.Normal, "text")
		fila := container.New(&filaCampo{rotulo: ColumnaRotulo(c.ventana), relleno: RellenoHCampo(c.ventana)}, dt, dd)
		fila.Resize(fyne.NewSize(672, 50))
		if x := dd.Position().X; x != c.quiero {
			t.Errorf("con ventana %v el valor empieza en %v y la web en %v", c.ventana, x, c.quiero)
		}
		if y := dd.Position().Y; y != RellenoVCampo {
			t.Errorf("el valor empieza en y = %v y el `py-3` son %v", y, RellenoVCampo)
		}
	}
}

// TestLaEstrellaDeLaFichaNoTieneFondo: el `FavoriteToggle` no es un botón secundario;
// no lleva `bg-surface`, así que dentro de su borde se ve el papel de la ficha.
func TestLaEstrellaDeLaFichaNoTieneFondo(t *testing.T) {
	// En oscuro bg (#131313) y surface (#222222) son distintos: pintar surface
	// en el botón falla aunque la paleta clara tenga ambos fondos blancos.
	tema.Elegir("oscuro")
	t.Cleanup(func() { tema.Elegir("") })
	b := nuevoBotonEstrella(false, "Destacar", nil)
	img := pintarObjeto(t, b, fyne.NewSize(LadoEstrellaFicha, LadoEstrellaFicha))
	papel, superficie := tema.Color("bg", tema.Oscuro), tema.Color("surface", tema.Oscuro)
	var lado float32 = LadoEstrellaFicha
	mitad := int(lado / 2)
	got := px(img, 4, mitad)
	if !mismoColor(got, papel) || mismoColor(got, superficie) {
		t.Errorf("dentro del borde se pinta %v; la web deja ver el papel %v", got, papel)
	}
	// El `rgba(tinta, .12)` del borde se ve sobre el papel, que es lo que hay detrás.
	if borde, quiero := px(img, mitad, 0), tokens.SobreSRGB(tokens.Crudo("border", tema.Oscuro), papel); !mismoColor(borde, quiero) {
		t.Errorf("el borde de arriba es %v y es `border` sobre el papel (%v)", borde, quiero)
	}
}

// TestElNombreDeExportacionEsElDeLaWeb es `exportFilename`: cualquier alfabeto, lo
// demás a guiones, y «contacto» si no queda nada.
func TestElNombreDeExportacionEsElDeLaWeb(t *testing.T) {
	for entrada, quiero := range map[string]string{
		"  Ana María O'Neill ": "Ana-María-O-Neill.vcf",
		"日本 語":                 "日本-語.vcf",
		"--- !!":               "contacto.vcf",
		"":                     "contacto.vcf",
	} {
		if got := NombreExportacion(entrada); got != quiero {
			t.Errorf("NombreExportacion(%q) = %q, la web da %q", entrada, got, quiero)
		}
	}
}

// TestReunionInvocaACalendar: «Crear una reunión» abre Calendar con la persona invitada
// y el asunto escrito, el intent que Calendar ya lee.
func TestReunionInvocaACalendar(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{})
	ab := &abridorFalso{}
	a.Abridor = ab
	a.Pantalla.AlReunion(persona("ada", "Ada"), "ada@casa.test")
	q := url.Values{"action": {"create"}, "attendees": {"ada@casa.test"}, "title": {"Reunión con Ada"}}
	if quiero := "sytue-calendar:/?" + q.Encode(); len(ab.urls) != 1 || ab.urls[0] != quiero {
		t.Errorf("se abrió %v y la web pide %s", ab.urls, quiero)
	}
}

// TestEtiquetarEsOptimistaYVuelveAtras: la píldora aparece al pulsar y, si el servicio
// dice que no, desaparece y se avisa.
func TestEtiquetarEsOptimistaYVuelveAtras(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}, fallarGrupo: true}
	a := appDePrueba(t, s)
	a.Cargar()
	var visto []string
	a.enFondo = func(f func()) {
		visto = a.Pantalla.Etiquetas[0].Members
		f()
	}
	a.etiquetar(a.Pantalla.Etiquetas[0], a.Pantalla.Contactos[0])
	if !contieneTexto(visto, "u-ana") {
		t.Errorf("mientras la petición va, la etiqueta tiene %v y debería llevar a Ana", visto)
	}
	if contieneTexto(a.Pantalla.Etiquetas[0].Members, "u-ana") {
		t.Error("el servicio dijo que no y Ana se ha quedado en la etiqueta")
	}
	if len(a.avisos.Avisos()) != 1 {
		t.Errorf("hay %d avisos y debería haber uno", len(a.avisos.Avisos()))
	}
}
