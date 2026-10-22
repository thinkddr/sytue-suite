// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"io"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// enlaceDe busca un enlace de texto de la pantalla por su rótulo.
func enlaceDe(w fyne.Window, texto string) *ui.Enlace {
	var hallado *ui.Enlace
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if e, ok := o.(*ui.Enlace); ok && e.Texto == texto {
			hallado = e
		}
	})
	return hallado
}

// TestCompartirLaBase: el propietario concede a toda la organización, a una persona por
// su correo, a un grupo y a alguien de otra organización; retira un acceso, y transfiere
// la propiedad con una confirmación, tras la que deja de ver lo del propietario.
func TestCompartirLaBase(t *testing.T) {
	f := nuevoFalso(1)
	p, w := abrir(t, f)
	p.Org = func() string { return "acme" }
	pulsar(t, w, "Compartir")
	capturar(t, w, "dialogo-compartir")
	if got := textos(w); !strings.Contains(got, "Todavía no se comparte con nadie.") || !strings.Contains(got, "Transferir la propiedad") {
		t.Fatalf("el diálogo de compartir no enseña la lista vacía ni la transferencia:\n%s", got)
	}

	pulsar(t, w, "Compartir") // toda la organización, como lector: no pide a quién
	if !f.vio(`POST /bases/b1/permissions {"principal_kind":"tenant","role":"viewer","principal_id":"acme"}`) {
		t.Fatalf("compartir con la organización: %q", f.peticiones("POST /bases/b1/permissions"))
	}
	elegir(t, w, "Compartir con", "Una persona")
	if b := boton(w, "Compartir"); !b.Inerte {
		t.Error("a una persona no se le comparte sin su correo")
	}
	campoDeDialogo(t, w, "Correo de la persona").PonerTexto("nadie@acme.example")
	pulsar(t, w, "Compartir")
	if got := textos(w); !strings.Contains(got, "Nadie de tu organización tiene esa dirección.") {
		t.Errorf("el correo de alguien de fuera no se explica:\n%s", got)
	}
	campoDeDialogo(t, w, "Correo de la persona").PonerTexto(" eva@acme.example ")
	elegir(t, w, "Permiso", "Editor")
	pulsar(t, w, "Compartir")
	if !f.vio(`POST /bases/b1/permissions {"principal_kind":"user","role":"editor","email":"eva@acme.example"}`) {
		t.Fatalf("compartir con una persona: %q", f.peticiones("POST /bases/b1/permissions"))
	}
	if campoDeDialogo(t, w, "Correo de la persona").Texto() != "" {
		t.Error("tras compartir, el correo se queda escrito")
	}
	// Un grupo: los propios se ofrecen a un clic.
	elegir(t, w, "Compartir con", "Un grupo")
	pulsar(t, w, "ventas")
	pulsar(t, w, "Compartir")
	if !f.vio(`POST /bases/b1/permissions {"principal_kind":"group","role":"editor","principal_id":"ventas"}`) {
		t.Fatalf("compartir con un grupo: %q", f.peticiones("POST /bases/b1/permissions"))
	}
	elegir(t, w, "Compartir con", "Alguien de otra organización")
	campoDeDialogo(t, w, "@usuario o correo de la persona").PonerTexto("@ana")
	pulsar(t, w, "Compartir")
	if !f.vio(`POST /bases/b1/permissions {"principal_kind":"external","role":"editor","query":"@ana"}`) {
		t.Fatalf("compartir con alguien de fuera: %q", f.peticiones("POST /bases/b1/permissions"))
	}
	capturar(t, w, "dialogo-compartir-con-accesos")
	got := textos(w)
	for _, quiero := range []string{
		"Toda la organización · Lector", "eva@acme.example · Editor", "Grupo ventas · Editor", "@ana · otra organización · Editor",
	} {
		if !strings.Contains(got, quiero) {
			t.Errorf("falta %q en la lista de accesos:\n%s", quiero, got)
		}
	}

	// Retirar el primero.
	primero := f.permisos[0]["id"].(string)
	pulsar(t, w, "Dejar de compartir") // el último de la lista: el de fuera
	if len(f.permisos) != 3 || f.permisos[0]["id"] != primero {
		t.Fatalf("retirar un acceso quita ese y no otro: %d", len(f.permisos))
	}
	if strings.Contains(textos(w), "@ana · otra organización") {
		t.Error("el acceso retirado sigue en la lista")
	}

	// Transferir: pregunta antes, y quien transfiere se queda de editor.
	if b := boton(w, "Transferir"); !b.Inerte {
		t.Error("no se transfiere sin el correo del nuevo propietario")
	}
	campoDeDialogo(t, w, "Correo del nuevo propietario").PonerTexto("eva@acme.example")
	pulsar(t, w, "Transferir")
	if f.vio(`POST /bases/b1/transfer {"email":"eva@acme.example"}`) {
		t.Fatal("se ha transferido sin confirmar")
	}
	if got := textos(w); !strings.Contains(got, "¿Transferir la base Clientes a eva@acme.example? No podrás deshacerlo por tu cuenta.") {
		t.Errorf("la confirmación no dice qué se transfiere ni a quién:\n%s", got)
	}
	pulsar(t, w, "Transferir")
	if !f.vio(`POST /bases/b1/transfer {"email":"eva@acme.example"}`) {
		t.Fatalf("no se ha transferido: %q", f.peticiones("POST /bases/b1/transfer"))
	}
	if got := textos(w); strings.Contains(got, "[Compartir]") || strings.Contains(got, "[Borrar base]") || !strings.Contains(got, "[Añadir registro]") {
		t.Errorf("tras transferir se queda de editor: sin lo del propietario y con la rejilla editable:\n%s", got)
	}
}

// TestLosEnlacesPublicosDeUnFormulario: el propietario crea un enlace con su cupo, lo
// copia la única vez que se enseña, ve los que hay con su estado y los revoca.
func TestLosEnlacesPublicosDeUnFormulario(t *testing.T) {
	f := nuevoFalso(1)
	f.vistas["t1"] = []m{{"id": "vf", "table_id": "t1", "name": "Alta", "kind": "form", "config": m{"fields": []string{"nom"}}}}
	p, w := abrir(t, f)
	p.URLPublica = func(ruta string) string { return api.AppURL(api.ApexProd, "bases", "", ruta) }
	pulsar(t, w, "Alta · Formulario")
	capturar(t, w, "formulario-sin-enlaces")
	if got := textos(w); !strings.Contains(got, "Enlaces públicos") || !strings.Contains(got, "Todavía no hay enlaces.") {
		t.Fatalf("la vista de formulario no enseña sus enlaces públicos:\n%s", got)
	}
	p.abierta.cupo.PonerTexto("0")
	pulsar(t, w, "Crear enlace")
	if got := textos(w); !strings.Contains(got, "Escribe un número entre 1 y 10.000.") || len(f.peticiones("POST /views/")) != 0 {
		t.Fatalf("un cupo que no vale no se envía y se explica:\n%s", got)
	}
	p.abierta.cupo.PonerTexto("25")
	pulsar(t, w, "Crear enlace")
	if !f.vio(`POST /views/vf/form-links {"max_responses":25}`) {
		t.Fatalf("el enlace no se crea con su cupo: %q", f.peticiones("POST /views/"))
	}
	capturar(t, w, "formulario-enlace-recien-creado")
	const direccion = "https://bases.sytue.eu/bases/f/tok-secreto"
	if p.abierta.enlaceNuevo != direccion {
		t.Fatalf("la dirección del enlace es %q, quiero %q", p.abierta.enlaceNuevo, direccion)
	}
	got := textos(w)
	for _, quiero := range []string{"Copia el enlace ahora: no se volverá a mostrar.", "0 de 25 respuestas", "5 oct 2026", "[Revocar]"} {
		if !strings.Contains(got, quiero) {
			t.Errorf("falta %q:\n%s", quiero, got)
		}
	}
	pulsar(t, w, "Copiar")
	if copiado := fyne.CurrentApp().Clipboard().Content(); copiado != direccion || !strings.Contains(textos(w), "[Copiado]") {
		t.Errorf("copiar no deja la dirección en el portapapeles: %q", copiado)
	}
	pulsar(t, w, "Revocar")
	if !f.vio("DELETE /form-links/"+f.enlaces[0]["id"].(string)) || p.abierta.enlaceNuevo != "" {
		t.Fatalf("el enlace no se ha revocado: %q", f.peticiones("DELETE"))
	}
	if got := textos(w); strings.Contains(got, "[Revocar]") || strings.Contains(got, "no se volverá a mostrar") {
		t.Errorf("un enlace revocado no se puede volver a revocar ni se sigue enseñando:\n%s", got)
	}
	capturar(t, w, "formulario-enlace-revocado")

	// Quien no es propietario ve los campos del formulario, no sus enlaces.
	f.rol = "editor"
	p.Abrir("b1")
	pulsar(t, w, "Alta · Formulario")
	if got := textos(w); strings.Contains(got, "Enlaces públicos") || !strings.Contains(got, "•  Nombre (Texto)") {
		t.Errorf("un editor no gestiona los enlaces públicos:\n%s", got)
	}
}

// TestElEstadoDeUnEnlacePublico: revocado gana a caducado, y caducado a completo.
func TestElEstadoDeUnEnlacePublico(t *testing.T) {
	ahora := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	ayer, manana := ahora.Add(-24*time.Hour), ahora.Add(24*time.Hour)
	casos := []struct {
		e      api.BasesFormLink
		quiero string
	}{
		{api.BasesFormLink{MaxResponses: 10}, "active"},
		{api.BasesFormLink{MaxResponses: 10, ExpiresAt: &manana}, "active"},
		{api.BasesFormLink{MaxResponses: 10, Responses: 10}, "full"},
		{api.BasesFormLink{MaxResponses: 10, Responses: 10, ExpiresAt: &ayer}, "expired"},
		{api.BasesFormLink{MaxResponses: 10, ExpiresAt: &ayer, RevokedAt: &ayer}, "revoked"},
	}
	for _, c := range casos {
		if got := estadoDeEnlace(c.e, ahora); got != c.quiero {
			t.Errorf("%+v: %q, quiero %q", c.e, got, c.quiero)
		}
	}
}

// TestExportarGuardaLaTabla: «Exportar CSV» baja la tabla del servicio y la deja donde
// se guardan las descargas, con el nombre de la tabla. Sin dónde guardar, no se ofrece.
func TestExportarGuardaLaTabla(t *testing.T) {
	f := nuevoFalso(1)
	p, w := abrir(t, f)
	if enlaceDe(w, "Exportar CSV") != nil {
		t.Fatal("sin dónde guardar no se ofrece exportar")
	}
	var nombre, contenido string
	p.Guardar = func(n string, r io.Reader) (string, error) {
		datos, err := io.ReadAll(r)
		nombre, contenido = n, string(datos)
		return "/descargas/" + n, err
	}
	p.Repintar()
	capturar(t, w, "rejilla-con-exportar")
	csv := enlaceDe(w, "Exportar CSV")
	if csv == nil || enlaceDe(w, "Exportar JSON") == nil {
		t.Fatal("no se ofrece exportar en CSV y en JSON")
	}
	test.Tap(csv)
	capturar(t, w, "exportar-aviso")
	if !f.vio("GET /tables/t1/export?format=csv") || nombre != "Personas.csv" || contenido != "Nombre,Edad\nPersona 1,21\n" {
		t.Fatalf("la exportación no se ha guardado: %q, %q", nombre, contenido)
	}
	if got := textos(w); !strings.Contains(got, "Personas.csv — /descargas/Personas.csv") {
		t.Errorf("no se dice dónde ha quedado el fichero:\n%s", got)
	}
}
