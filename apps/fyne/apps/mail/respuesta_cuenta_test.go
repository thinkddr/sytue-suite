// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// servicioConSieve es el servicio falso con la respuesta automática por cuenta (como el
// correo local): cada cuenta contesta lo que diga su entrada.
type servicioConSieve struct {
	*servicioFalso
	cuentas    []api.MailAlias
	leidas     map[string]api.MailRespuesta
	errores    map[string]error
	guardadas  []api.MailRespuesta
	preguntas  []string
	errGuardar error
}

func (s *servicioConSieve) CuentasRespuesta() []api.MailAlias { return s.cuentas }

func (s *servicioConSieve) LeerRespuesta(_ context.Context, cuenta string) (api.MailRespuesta, error) {
	s.preguntas = append(s.preguntas, cuenta)
	if err := s.errores[cuenta]; err != nil {
		return api.MailRespuesta{}, err
	}
	return s.leidas[cuenta], nil
}

func (s *servicioConSieve) GuardarRespuesta(_ context.Context, cuenta string, r api.MailRespuesta) error {
	if s.errGuardar != nil {
		return s.errGuardar
	}
	r.Cuenta = cuenta
	s.guardadas = append(s.guardadas, r)
	anterior := s.leidas[cuenta]
	r.ConFechas, r.Reglas, r.Bloqueada = anterior.ConFechas, anterior.Reglas, anterior.Bloqueada
	s.leidas[cuenta] = r
	return nil
}

var _ api.MailRespuestas = (*servicioConSieve)(nil)

func conSieve() *servicioConSieve {
	return &servicioConSieve{
		servicioFalso: servicioConTodas(),
		cuentas:       []api.MailAlias{{ID: "casa", Address: "ana@casa.eu"}, {ID: "trabajo", Address: "ana@trabajo.eu"}},
		leidas: map[string]api.MailRespuesta{
			"casa":    {Cuenta: "casa", Asunto: "Fuera", Mensaje: "Vuelvo pronto", Dias: 3, ConFechas: true, Reglas: "reglas"},
			"trabajo": {Cuenta: "trabajo", Dias: 7, Bloqueada: "filtros"},
		},
		errores: map[string]error{},
	}
}

// Sin ninguna cuenta con servidor Sieve, «Vacaciones» no aparece y no se le pregunta
// al servicio de Sytue (no lo hay).
func TestSinSieveLaFilaNoAparece(t *testing.T) {
	s := conSieve()
	s.errores["casa"] = api.ErrMailSinRespuesta
	s.errores["trabajo"] = api.ErrMailSinRespuesta
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	b.verAjustes()
	if b.P.Ajustes.Boton("vacation") != nil {
		t.Fatal("sin servidor Sieve la fila «Vacaciones» no sale")
	}
	if s.pidio("Vacation") != 0 {
		t.Fatal("con la respuesta por cuenta no se pregunta al servicio")
	}
	if !slices.Equal(s.preguntas, []string{"casa", "trabajo"}) {
		t.Fatalf("se pregunta a cada cuenta al abrir los Ajustes: %v", s.preguntas)
	}
	if b.verVacaciones() != nil {
		t.Fatal("sin cuentas con Sieve no hay diálogo")
	}
	// Sin cuentas que puedan tenerla (Gmail, Outlook), ni se pregunta.
	s2 := conSieve()
	s2.cuentas = nil
	b2, _, _ := buzonConVentana(t, s2, fyne.NewSize(1280, 800))
	b2.verAjustes()
	if b2.P.Ajustes.Boton("vacation") != nil || len(s2.preguntas) != 0 {
		t.Fatalf("sin cuentas que puedan tenerla: fila %v, preguntas %v", b2.P.Ajustes.Boton("vacation") != nil, s2.preguntas)
	}
}

// La conectada no cambia: con *api.Mail (aquí, el servicio falso sin la interfaz) la
// fila sale siempre y se lee del servicio.
func TestSinLaInterfazTodoComoSiempre(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	b.AjustesOcultos = map[string]bool{"aliases": true}
	b.verAjustes()
	if b.P.Ajustes.Boton("vacation") == nil || s.pidio("Vacation") == 0 {
		t.Fatal("la conectada pierde su fila o deja de leer el servicio")
	}
	if got := b.ajustesOcultos(); len(got) != 1 || !got["aliases"] {
		t.Fatalf("ocultas: %v", got)
	}
	g := b.verVacaciones()
	if g.porCuenta != nil || g.grupoCuenta != nil || g.dias != nil || g.nota != nil {
		t.Fatal("el diálogo de la conectada no lleva nada de la respuesta por cuenta")
	}
}

func TestLaRespuestaPorCuentaSeLeeYSeGuarda(t *testing.T) {
	s := conSieve()
	s.errores["trabajo"] = errors.New("el servidor rechazó la contraseña") // tiene Sieve, falla otra cosa
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	tr := traductor(t)
	b.verAjustes()
	if b.P.Ajustes.Boton("vacation") == nil {
		t.Fatal("con una cuenta con Sieve la fila sale")
	}
	if e := b.P.Ajustes.estado("vacation"); e == nil || e.Rotulo != tr.T("vacation.statusOffLabel", nil) {
		t.Fatalf("apagada: %+v", e)
	}
	test.Tap(b.P.Ajustes.Boton("vacation"))
	if !hayDialogo(w.Canvas(), tr.T("vacation.title", nil)) {
		t.Fatal("«Editar» no abre el diálogo")
	}
	for capas := w.Canvas().Overlays(); capas.Top() != nil; {
		capas.Remove(capas.Top())
	}
	g := b.verVacaciones()
	if g.estado != Listo || g.selCuenta == nil || len(g.selCuenta.Opciones) != 2 {
		t.Fatalf("estado %v, selector %v", g.estado, g.selCuenta)
	}
	if g.asunto.Texto() != "Fuera" || g.dias.Texto() != "3" || g.conmutador.Encendido || !g.cajaFechas.Visible() {
		t.Fatalf("formulario: %q %q %v", g.asunto.Texto(), g.dias.Texto(), g.conmutador.Encendido)
	}
	// Sus reglas siguen detrás: se dice.
	if !hayAyuda(g.nota, "reglas") {
		t.Fatal("no dice que sus reglas siguen funcionando")
	}
	// Días fuera de margen: rojo y sin guardar.
	g.dias.PonerTexto("0")
	if !g.guardar.Inerte || !g.dias.Invalida {
		t.Fatal("0 días se deja guardar")
	}
	g.salvar()
	if len(s.guardadas) != 0 {
		t.Fatal("se guardó con 0 días")
	}
	g.dias.PonerTexto("5")
	g.conmutador.Encendido = true
	g.asunto.PonerTexto("")
	g.desde.PonerTexto("01/12/2026")
	g.hasta.PonerTexto("15/12/2026")
	g.pintar()
	g.salvar()
	if len(s.guardadas) != 1 {
		t.Fatalf("guardadas: %+v", s.guardadas)
	}
	r := s.guardadas[0]
	if r.Cuenta != "casa" || !r.Activa || r.Dias != 5 || r.Asunto != tr.T("vacation.subjectPlaceholder", nil) ||
		r.Desde == nil || r.Desde.Day() != 1 || r.Hasta == nil || r.Hasta.Day() != 15 || r.Hasta.Hour() != 23 {
		t.Fatalf("guardada: %+v", r)
	}
	if hayDialogo(w.Canvas(), tr.T("vacation.title", nil)) {
		t.Error("tras guardar el diálogo sigue abierto")
	}
	// Lo recién guardado es lo que dice la fila: se vuelve a leer del servidor.
	if e := b.P.Ajustes.estado("vacation"); e == nil || e.Tono == ui.InsigniaNeutra {
		t.Fatalf("tras encenderla la fila dice %+v", e)
	}

	// La otra cuenta falla al leer: se dice, sin formulario que guardar en blanco.
	g = b.verVacaciones()
	g.selCuenta.Elegida = 1
	g.selCuenta.AlElegir(1)
	if g.estado != Error || !g.guardar.Inerte || g.cuenta != "trabajo" {
		t.Fatalf("estado %v, cuenta %q", g.estado, g.cuenta)
	}
	if g.caja.Objects[0] != fyne.CanvasObject(g.grupoCuenta) {
		t.Fatal("con el error, el selector de cuenta sigue a mano")
	}
}

// Con otras reglas activas que no se pueden combinar, no se puede encender, y se dice.
func TestBloqueadaNoSeEnciende(t *testing.T) {
	s := conSieve()
	s.cuentas = s.cuentas[1:]
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	b.verAjustes()
	g := b.verVacaciones()
	if g.selCuenta != nil || g.grupoCuenta == nil {
		t.Fatal("con una sola cuenta se enseña su dirección, sin selector")
	}
	if !hayAyuda(g.nota, "filtros") {
		t.Fatal("no explica qué reglas lo impiden")
	}
	if !g.conmutador.Disabled() {
		t.Fatal("bloqueada, el conmutador se deja encender")
	}
	if g.cajaFechas.Visible() || !hayAyuda(g.nota, "") {
		t.Fatal("sin fechas en el servidor, las fechas no salen y se dice")
	}
	g.conmutador.Encendido = true // aunque se forzara
	g.pintar()
	g.salvar()
	if !g.guardar.Inerte || len(s.guardadas) != 0 {
		t.Fatal("bloqueada, se deja encender")
	}
	// Apagada sí se guarda el texto (para la próxima vez).
	g.conmutador.Encendido = false
	g.asunto.PonerTexto("Luego")
	g.salvar()
	if len(s.guardadas) != 1 || s.guardadas[0].Activa {
		t.Fatalf("%+v", s.guardadas)
	}
	if s.guardadas[0].Desde != nil {
		t.Fatal("sin fechas en el servidor no se mandan fechas")
	}
}

// hayAyuda dice si en c hay una ayuda en línea con ese texto dentro ("" = cualquiera
// distinta de la de las reglas).
func hayAyuda(c *fyne.Container, dentro string) bool {
	for _, o := range c.Objects {
		a, ok := o.(*ui.AyudaEnLinea)
		if !ok {
			continue
		}
		if dentro == "" || strings.Contains(a.Cuerpo, dentro) {
			return true
		}
	}
	return false
}

// Los textos propios de la respuesta por cuenta están en los 26 idiomas, con los mismos
// huecos que el inglés.
func TestLosTextosDeLaRespuestaCuadran(t *testing.T) {
	p := i18n.Propios{Base: TextosRespuesta, Traducciones: traduccionesRespuesta}
	for _, f := range p.Faltan() {
		t.Error(f)
	}
	huecos := regexp.MustCompile(`\{[a-zA-Z]+\}`)
	conjunto := func(s string) []string {
		v := huecos.FindAllString(s, -1)
		slices.Sort(v)
		return v
	}
	for clave, v := range TextosRespuesta {
		if a, b := conjunto(v[0]), conjunto(v[1]); !slices.Equal(a, b) {
			t.Errorf("%q: el español lleva %v y el inglés %v", clave, a, b)
		}
	}
	for loc, tr := range traduccionesRespuesta {
		for clave, v := range tr {
			if a, b := conjunto(v), conjunto(TextosRespuesta[clave][1]); !slices.Equal(a, b) {
				t.Errorf("%s: %q lleva %v y el inglés %v", loc, clave, a, b)
			}
		}
	}
	if got := p.En("de", "respuestaDias"); got != "Tage zwischen Antworten an dieselbe Person" {
		t.Errorf("alemán: %q", got)
	}
	if got := p.En("pt-BR", "respuestaDias"); got != "Dias entre respostas à mesma pessoa" {
		t.Errorf("variante regional: %q", got)
	}
}
