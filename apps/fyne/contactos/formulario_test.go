// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"encoding/json"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// formularioDePrueba carga la app y abre el formulario de un contacto (nil: uno nuevo).
func formularioDePrueba(t *testing.T, s *servicioFalso, editar bool) (*App, *formulario) {
	t.Helper()
	a := appDePrueba(t, s)
	a.Cargar()
	var c *api.Contact
	if editar {
		c = &a.Pantalla.Contactos[0]
	}
	f := a.abrirFormulario(c)
	if f == nil {
		t.Fatal("el formulario no se abrió")
	}
	return a, f
}

func abierto(a *App) bool { return a.lienzo().Overlays().Top() != nil }

// cuerpoDe decodifica la única escritura que se espera.
func cuerpoDe(t *testing.T, s *servicioFalso, ruta string) api.ContactInput {
	t.Helper()
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != ruta {
		t.Fatalf("escrituras %v y se esperaba solo «%s»", s.escrituras, ruta)
	}
	var in api.ContactInput
	if err := json.Unmarshal([]byte(s.escrituras[0].cuerpo), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

// TestElFormularioCreaConLoQueSeEscribe: el nombre recortado, los correos con su
// etiqueta y el principal ELEGIDO (no el primero), la fila vacía de teléfono fuera y el
// cumpleaños como fecha ISO. Y al terminar, el diálogo se cierra y lo dice.
func TestElFormularioCreaConLoQueSeEscribe(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}}
	a, f := formularioDePrueba(t, s, false)

	f.nombre.PonerTexto("  Ada Lovelace ")
	f.correos[0].valor.PonerTexto("ada@casa.test")
	f.correos = append(f.correos, f.nuevaFila(tiposCorreo, 1, "ada@trabajo.test", "form.emailPlaceholder"))
	f.principalCorreo = 1
	f.org.PonerTexto("Analytical Engines")
	f.cumple.PonerTexto("10/12/1815")
	f.guardar()

	in := cuerpoDe(t, s, "POST /api/v1/address-books/l1/contacts")
	if in.FormattedName != "Ada Lovelace" || in.Organization != "Analytical Engines" || in.Birthday != "1815-12-10" {
		t.Errorf("nombre %q, organización %q, cumpleaños %q", in.FormattedName, in.Organization, in.Birthday)
	}
	quiero := []api.ContactEmail{{Email: "ada@casa.test", Type: "home"}, {Email: "ada@trabajo.test", Type: "work", IsPrimary: true}}
	if len(in.Emails) != 2 || in.Emails[0] != quiero[0] || in.Emails[1] != quiero[1] {
		t.Errorf("correos %+v y la web manda %+v", in.Emails, quiero)
	}
	if len(in.Phones) != 0 || len(in.Addresses) != 0 {
		t.Errorf("las filas vacías viajan: %+v %+v", in.Phones, in.Addresses)
	}
	if abierto(a) {
		t.Error("guardado, el diálogo sigue abierto")
	}
	if av := a.avisos.Avisos(); len(av) != 1 || av[0].Titulo != "Contacto creado" {
		t.Errorf("avisos %v", av)
	}
}

// TestSinNombreNoSeGuarda: sin nombre y con un correo mal escrito no sale ninguna
// petición; se marcan los dos campos y se dice por qué, con las frases de la web.
func TestSinNombreNoSeGuarda(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}}
	a, f := formularioDePrueba(t, s, false)
	f.correos[0].valor.PonerTexto("no-es-un-correo")
	f.guardar()

	if len(s.escrituras) != 0 {
		t.Fatalf("se mandó %v con el formulario mal", s.escrituras)
	}
	if !f.nombre.Invalida || !f.correos[0].valor.Invalida {
		t.Error("los campos con error no se marcan")
	}
	textos := textosDe(f.cuerpo)
	for _, frase := range []string{"El contacto necesita un nombre.", "Introduce un correo válido (nombre@dominio.eu)."} {
		if !contieneTexto(textos, frase) {
			t.Errorf("el formulario no dice «%s»", frase)
		}
	}
	if !abierto(a) {
		t.Error("con errores el diálogo se ha cerrado")
	}
	// Escribir quita el error, como `onInput` en la web.
	f.nombre.PonerTexto("Ada")
	if f.nombre.Invalida || contieneTexto(textosDe(f.cuerpo), "El contacto necesita un nombre.") {
		t.Error("tras escribir el nombre, el error sigue")
	}
}

// TestEditarConservaLoQueNoSeVe: el PUT sustituye el contacto entero, así que lo que el
// formulario no enseña —apellidos, cargo— tiene que viajar igual, con su etag.
func TestEditarConservaLoQueNoSeVe(t *testing.T) {
	c := fichaDeMuestra()
	c.FamilyName, c.Title, c.ETag = "Lovelace", "Condesa", "e1"
	s := &servicioFalso{contactos: []api.Contact{c}}
	_, f := formularioDePrueba(t, s, true)
	if f.nombre.Texto() != "Ada Lovelace" || len(f.correos) != 2 || f.principalCorreo != 1 || len(f.direcciones) != 1 {
		t.Fatalf("el formulario no se rellena con la ficha: %q, %d correos (principal %d), %d direcciones",
			f.nombre.Texto(), len(f.correos), f.principalCorreo, len(f.direcciones))
	}
	// Un TYPE que el editor no ofrece («iPhone») se edita como «Otro».
	if got := f.telefonos[1].tipo.Valor(); got != "Otro" {
		t.Errorf("el teléfono «iPhone» sale como %q", got)
	}
	f.guardar()
	in := cuerpoDe(t, s, "PUT /api/v1/contacts/ada")
	if in.FamilyName != "Lovelace" || in.Title != "Condesa" || in.Birthday != "1990-05-04" {
		t.Errorf("se perdió lo que no se ve: apellidos %q, cargo %q, cumpleaños %q", in.FamilyName, in.Title, in.Birthday)
	}
	if len(in.Addresses) != 1 || in.Addresses[0].Locality != "London" || in.Addresses[0].Type != "other" {
		t.Errorf("dirección %+v", in.Addresses)
	}
}

// TestEliminarPideConfirmacion: el primer clic solo cambia el rótulo; el segundo manda
// el contacto a la papelera y lo dice con la pista de que se puede restaurar.
func TestEliminarPideConfirmacion(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{fichaDeMuestra()}}
	a, f := formularioDePrueba(t, s, true)
	f.pulsarBorrar()
	if len(s.escrituras) != 0 || f.borrar.Texto != "Confirmar eliminación" {
		t.Fatalf("al primer clic: %v, rótulo %q", s.escrituras, f.borrar.Texto)
	}
	f.pulsarBorrar()
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != "DELETE /api/v1/contacts/ada" {
		t.Fatalf("escrituras %v", s.escrituras)
	}
	av := a.avisos.Avisos()
	if len(av) != 1 || av[0].Titulo != "Contacto movido a la papelera" || av[0].Descripcion != "Puedes restaurarlo desde la papelera." {
		t.Errorf("avisos %+v", av)
	}
	if abierto(a) {
		t.Error("eliminado, el diálogo sigue abierto")
	}
}

// TestElPieDelFormularioEsElDeLaWeb: a 1280 el diálogo va de 360 a 920; «Guardar»
// acaba a 1 de borde y 24 de px-6 del lado derecho (895); «Cancelar» queda a
// gap-2 (8 px) y al editar «Eliminar» empieza en 360 + 1 + 24 = 385.
//
// Los ANCHOS de los botones no se atan aquí: son de `ui.Boton`, y el volcado de este
// diálogo (2026-09-21) es anterior a `--font-render-hinting=none`, así que sus 79 y 71
// traen el redondeo por glifo del Chromium del piloto (ver REGLAS §3).
func TestElPieDelFormularioEsElDeLaWeb(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{fichaDeMuestra()}}
	a, f := formularioDePrueba(t, s, true)
	pos := func(o fyne.CanvasObject) fyne.Position {
		return fyne.CurrentApp().Driver().AbsolutePositionForObject(o)
	}
	derecha := func(b *ui.Boton) float32 { return pos(b).X + b.Size().Width }
	if got := derecha(f.guardarBoton); got != 895 {
		t.Errorf("«Guardar» acaba en x = %v y en la web en 895", got)
	}
	if got, quiero := derecha(f.cancelar), pos(f.guardarBoton).X-ui.HuecoPieDialogo; got != quiero {
		t.Errorf("«Cancelar» acaba en x = %v y va a `gap-2` de «Guardar» (%v)", got, quiero)
	}
	if got := pos(f.borrar).X; got != 385 {
		t.Errorf("«Eliminar» empieza en x = %v y en la web en 385", got)
	}
	guardar(t, a.lienzo().Capture(), "contactos-formulario")
}

// TestLaVistaPreviaSigueALoQueSeEscribe: sin nombre dice «Nuevo contacto» y «Vista
// previa» (en `text-subtle`); al escribir, el nombre y la organización (en
// `text-muted`).
func TestLaVistaPreviaSigueALoQueSeEscribe(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}}
	_, f := formularioDePrueba(t, s, false)
	if f.previaNombre.Text != "Nuevo contacto" || f.previaOrg.Text != "Vista previa" ||
		f.previaOrg.Color != tema.Color("text-subtle", tema.Claro) {
		t.Errorf("vacía, la previa dice %q / %q en %v", f.previaNombre.Text, f.previaOrg.Text, f.previaOrg.Color)
	}
	f.nombre.PonerTexto("Ada Lovelace")
	f.org.PonerTexto("Analytical Engines")
	if f.previaNombre.Text != "Ada Lovelace" || f.previaOrg.Text != "Analytical Engines" ||
		f.previaOrg.Color != tema.Color("text-muted", tema.Claro) || f.avatar.Nombre != "Ada Lovelace" {
		t.Errorf("escrita, la previa dice %q / %q en %v (avatar %q)",
			f.previaNombre.Text, f.previaOrg.Text, f.previaOrg.Color, f.avatar.Nombre)
	}
}

// TestLaFilaDelCorreoEnvuelveEnElMovil: TypedRows de ContactModal.tsx usa
// w-28 (112), grow basis-48 (192), aspa sm w-8 (32) y gap-2 (8), con controles
// h-9 (36). A 516 cabe todo; a 288, 112 + 8 + 192 > 288 y el selector queda
// en la primera línea. Valor y aspa comparten la segunda (248 + 8 + 32).
func TestLaFilaDelCorreoEnvuelveEnElMovil(t *testing.T) {
	tipo := ui.NuevoSelector([]string{"Casa"}, nil)
	tipo.Ancho = AnchoTipo
	valor := ui.NuevaEntrada("")
	aspa := botonQuitar("Quitar", nil)
	fila := container.New(&filaFlexible{Hueco: HuecoFieldset, Crece: 1, Base: BaseValor}, tipo, valor, aspa)

	fila.Resize(fyne.NewSize(516, 100))
	if got := valor.Size().Width; got != 516-112-32-16 {
		t.Errorf("a 516 el valor mide %v y en la web %v", got, 516-112-32-16)
	}
	if aspa.Position().Y != valor.Position().Y+(36-32)/2 {
		t.Errorf("a 516 el aspa está en y = %v, fuera de la línea del valor", aspa.Position().Y)
	}
	fila.Resize(fyne.NewSize(288, 100))
	if got := valor.Size().Width; got != 288-32-8 {
		t.Errorf("a 288 el valor mide %v y en la web %v", got, 288-32-8)
	}
	if p := aspa.Position(); p.X != 256 || p.Y != 36+8+(36-32)/2 {
		t.Errorf("a 288 el aspa está en %v y en la web baja a (256, 46)", p)
	}
	if h := fila.MinSize().Height; h != 36+8+36 {
		t.Errorf("a 288 la fila mide %v de alto y con dos líneas son %v", h, 36+8+36)
	}
}

// TestElAsistenteRellenaLaFichaYNoGuarda es AsistenteFicha: lo pegado va a Sytue AI como
// `ficha`/`contact` en modo calidad, y la propuesta RELLENA el formulario —el nombre y
// la organización se sustituyen, los correos y teléfonos se suman sin duplicar y sin
// las filas vacías— sin guardar nada. Si no hay ficha, lo dice debajo del área.
func TestElAsistenteRellenaLaFichaYNoGuarda(t *testing.T) {
	s := &servicioFalso{
		contactos: []api.Contact{persona("ana", "Ana")},
		ia: "```json\n{\"nombre\":\"Ana Ruiz\",\"organizacion\":\"Acme S.L.\",\"correos\":[\"ANA@casa.test\",\"ana.ruiz@acme.es\"]," +
			"\"telefonos\":[\"+34 600 11 22 33\"],\"nota\":\"Feria de Bilbao\"}\n```",
	}
	a, f := formularioDePrueba(t, s, false)
	if f.ia == nil || !f.rellenar.Inerte {
		t.Fatal("sin nada pegado, «Rellenar» tiene que salir apagado")
	}
	f.correos[0].valor.PonerTexto("ana@casa.test")
	firma := "Ana Ruiz — Directora Comercial, Acme S.L. · ana.ruiz@acme.es · +34 600 11 22 33"
	f.ia.PonerTexto(firma)
	if f.rellenar.Inerte {
		t.Fatal("con algo pegado, «Rellenar» sigue apagado")
	}
	f.pedirFicha()
	var pedido map[string]string
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != "POST /api/v1/assist" ||
		json.Unmarshal([]byte(s.escrituras[0].cuerpo), &pedido) != nil ||
		pedido["action"] != "ficha" || pedido["kind"] != "contact" || pedido["modo"] != "calidad" ||
		pedido["context"] != firma || pedido["instruction"] != api.InstruccionFicha {
		t.Fatalf("al asistente le llega %v", s.escrituras)
	}
	if f.nombre.Texto() != "Ana Ruiz" || f.org.Texto() != "Acme S.L." || f.nota.Texto() != "Feria de Bilbao" || f.ia.Texto() != "" {
		t.Errorf("nombre %q, organización %q, nota %q, área %q", f.nombre.Texto(), f.org.Texto(), f.nota.Texto(), f.ia.Texto())
	}
	if len(f.correos) != 2 || f.correos[1].valor.Texto() != "ana.ruiz@acme.es" || tiposCorreo[f.correos[1].tipo.Elegida].valor != "work" {
		t.Errorf("correos %d", len(f.correos))
	}
	if len(f.telefonos) != 1 || f.telefonos[0].valor.Texto() != "+34 600 11 22 33" || tiposTel[f.telefonos[0].tipo.Elegida].valor != "cell" {
		t.Errorf("teléfonos %d: la fila vacía se tenía que ir", len(f.telefonos))
	}
	if !abierto(a) {
		t.Error("el asistente no guarda: el formulario sigue abierto")
	}

	s.ia = "no sé"
	f.ia.PonerTexto("hola")
	f.pedirFicha()
	if !f.errIA || f.nombre.Texto() != "Ana Ruiz" {
		t.Errorf("sin ficha: error %v, nombre %q", f.errIA, f.nombre.Texto())
	}
}
