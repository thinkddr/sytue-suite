// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// contactoVIES es el contacto del catálogo con lo que se compara con VIES.
func contactoVIES() Modelo {
	return Modelo{
		Clave: ModeloContacto, Modulo: "contactos", Etiqueta: "Contactos", Lista: []string{"nombre"},
		Campos: []Campo{
			{Clave: "nombre", Tipo: "texto", Etiqueta: "Nombre"},
			{Clave: "nif", Tipo: "texto", Etiqueta: "NIF"},
			{Clave: "direccion", Tipo: "texto", Etiqueta: "Dirección"},
			{Clave: "cp", Tipo: "texto", Etiqueta: "Código postal"},
			{Clave: "ciudad", Tipo: "texto", Etiqueta: "Ciudad"},
			{Clave: "pais", Tipo: "texto", Etiqueta: "País"},
		},
	}
}

// viesDePrueba contesta como VIES y apunta qué se le preguntó.
type viesDePrueba struct {
	preguntas [][2]string
	r         ComprobacionVIES
	err       error
}

func (v *viesDePrueba) comprobar(_ context.Context, nif, pais string) (ComprobacionVIES, error) {
	v.preguntas = append(v.preguntas, [2]string{nif, pais})
	return v.r, v.err
}

func pantallaVIES(t *testing.T) *Pantalla {
	t.Helper()
	p, _ := pantallaFacturas(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{}`) })
	p.Organizacion = &api.ERPOrganizacion{Pais: "ES"}
	p.lista.Modelo = contactoVIES()
	return p
}

var exempleSARL = ComprobacionVIES{Pais: "FR", Numero: "12345678901", Valido: true, Nombre: "EXEMPLE SARL", Direccion: "1 RUE DE L’EXEMPLE, 75002 PARIS"}

// «Comprobar en VIES» sale debajo del NIF de un contacto solo si la pantalla tiene con
// qué preguntar (la conectada no: lo de siempre), no pregunta nada hasta pulsarlo, y lo
// contestado va al lado de lo que hay en la ficha SIN cambiar ningún campo.
func TestComprobarEnVIESEnLaFichaSoloAlPulsar(t *testing.T) {
	p := pantallaVIES(t)
	comprobar := textoVIES("comprobar", p.tr)
	contacto := &Registro{ID: "c1", Modelo: ModeloContacto, Data: map[string]any{
		"nombre": "Exemple", "nif": "12345678901", "pais": "FR", "direccion": "1 rue de l’Exemple", "cp": "75002", "ciudad": "Paris",
	}}

	p.abrirFicha(contacto, nil)
	if p.ficha.ComprobarVIES != nil || botonDe(p.ficha.Objeto(tema.Claro, 420), comprobar) != nil {
		t.Fatal("sin Pantalla.ComprobarVIES (la conectada) la ficha ofrece VIES")
	}

	v := &viesDePrueba{r: exempleSARL}
	p.ComprobarVIES = v.comprobar
	p.abrirFicha(contacto, nil)
	f := p.ficha
	b := botonDe(f.Objeto(tema.Claro, 420), comprobar)
	if b == nil {
		t.Fatal("la ficha del contacto no ofrece «Comprobar en VIES»")
	}
	if len(v.preguntas) != 0 {
		t.Fatalf("se preguntó a VIES sin pulsar: %v", v.preguntas)
	}
	test.Tap(b)
	if len(v.preguntas) != 1 || v.preguntas[0] != [2]string{"12345678901", "FR"} {
		t.Fatalf("preguntado: %v", v.preguntas)
	}
	// Las líneas de un párrafo largo, juntas: lo que se lee.
	textos := strings.Join(textosDe(f.Objeto(tema.Claro, 420)), " ")
	for _, s := range []string{
		textoVIES("valido", p.tr), "FR 12345678901", textoVIES("enVIES", p.tr) + " " + textoVIES("enFicha", p.tr),
		"Nombre EXEMPLE SARL Exemple", "Dirección 1 RUE DE L’EXEMPLE, 75002 PARIS 1 rue de l’Exemple, 75002 Paris",
	} {
		if !strings.Contains(textos, s) {
			t.Errorf("la ficha no dice «%s»: %q", s, textos)
		}
	}
	if f.Cambiada() || f.Valores["nombre"] != "Exemple" {
		t.Errorf("VIES cambió la ficha: %v", f.Valores)
	}

	// Sin país en la ficha, el de la organización; y los fallos, con su texto.
	delete(contacto.Data, "pais")
	v.preguntas, v.err = nil, VIESSinRed
	p.abrirFicha(contacto, nil)
	p.ficha.ComprobarVIES()
	if len(v.preguntas) != 1 || v.preguntas[0][1] != "ES" {
		t.Fatalf("sin país en la ficha: %v", v.preguntas)
	}
	if !contiene(textosDe(p.ficha.Objeto(tema.Claro, 420)), textoVIES("falloSinRed", p.tr)) {
		t.Error("sin red, no lo dice")
	}
	v.err = errors.New("algo que no se sabe")
	p.ficha.ComprobarVIES()
	if !contiene(textosDe(p.ficha.Objeto(tema.Claro, 420)), textoVIES("falloNoResponde", p.tr)) {
		t.Error("un fallo cualquiera es que VIES no responde")
	}

	// Sin NIF no se pregunta: se dice.
	v.preguntas, v.err = nil, nil
	p.abrirFicha(&Registro{ID: "c2", Modelo: ModeloContacto, Data: map[string]any{"nombre": "Sin NIF"}}, nil)
	p.ficha.ComprobarVIES()
	if len(v.preguntas) != 0 || !contiene(textosDe(p.ficha.Objeto(tema.Claro, 420)), textoVIES("falloNif", p.tr)) {
		t.Errorf("sin NIF: %v", v.preguntas)
	}

	// Solo los contactos: una factura no lo ofrece.
	p.lista.Modelo = facturaArnes()
	p.abrirFicha(&Registro{ID: "f1", Modelo: api.ERPFacturaVenta}, nil)
	if p.ficha.ComprobarVIES != nil {
		t.Error("la ficha de una factura ofrece VIES")
	}
}

// En el plan de Sytue AI, cada contacto NUEVO con NIF lleva «Comprobar en VIES» (el que ya
// existía, no); pulsarlo pregunta por lo PROPUESTO y lo contestado va a su lado, sin tocar
// el plan.
func TestComprobarEnVIESLoQuePoneLaIA(t *testing.T) {
	p := pantallaVIES(t)
	plan := &api.ERPPlan{Resumen: "Doy de alta a Exemple como cliente.", Pasos: []api.ERPPasoPlan{
		{Ref: "c1", Modelo: ModeloContacto, Data: map[string]any{"nombre": "Exemple", "nif": "FR12345678901", "ciudad": "Paris"}},
		{Ref: "c2", Modelo: ModeloContacto, Existente: &api.ERPRegistroCorto{ID: "c9", Etiqueta: "Otra"}, Data: map[string]any{"nif": "ESB12345678"}},
	}}
	comprobar := textoVIES("comprobar", p.tr)

	sin := NuevoPanelIA(p.tr, nil, []Modelo{contactoVIES()})
	sin.Plan = plan
	if botonDe(sin.Cuerpo(tema.Claro, 670), comprobar) != nil {
		t.Fatal("sin Pantalla.ComprobarVIES (la conectada) el plan ofrece VIES")
	}

	v := &viesDePrueba{r: exempleSARL}
	p.ComprobarVIES = v.comprobar
	p.AbrirIA()
	x := p.panelIA
	x.Modelos = []Modelo{contactoVIES()}
	x.Plan = plan
	var botones []*ui.Boton
	recorrer(x.Cuerpo(tema.Claro, 670), func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && b.Texto == comprobar {
			botones = append(botones, b)
		}
	})
	if len(botones) != 1 {
		t.Fatalf("botones de VIES en el plan: %d; quiero uno (el contacto nuevo)", len(botones))
	}
	if len(v.preguntas) != 0 {
		t.Fatalf("se preguntó a VIES sin pulsar: %v", v.preguntas)
	}
	test.Tap(botones[0])
	if len(v.preguntas) != 1 || v.preguntas[0] != [2]string{"FR12345678901", "ES"} {
		t.Fatalf("preguntado: %v", v.preguntas)
	}
	textos := strings.Join(textosDe(x.Cuerpo(tema.Claro, 670)), " ")
	for _, s := range []string{
		textoVIES("valido", p.tr), textoVIES("enVIES", p.tr) + " " + textoVIES("propuesto", p.tr),
		"Nombre EXEMPLE SARL Exemple", "Dirección 1 RUE DE L’EXEMPLE, 75002 PARIS Paris",
	} {
		if !strings.Contains(textos, s) {
			t.Errorf("el plan no dice «%s»: %q", s, textos)
		}
	}
	if x.Plan != plan || len(plan.Pasos[0].Data) != 3 || plan.Pasos[0].Data["nombre"] != "Exemple" {
		t.Errorf("VIES cambió el plan: %+v", plan.Pasos[0].Data)
	}
}
