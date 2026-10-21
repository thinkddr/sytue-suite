// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func bloqueadosDePrueba() []api.CastBlocked {
	return []api.CastBlocked{{Sub: "u2", Name: "Marta Ruiz", CreatedAt: ahoraFijo.Add(-48 * 3600e9)}}
}

func denunciasDePrueba() []api.CastReport {
	return []api.CastReport{
		{
			ID: "r1", Kind: api.CastReportComment, VideoID: "v01", TargetID: "k1", TargetSub: "u2", TargetName: "Marta Ruiz",
			Excerpt: "Un comentario fuera de tono sobre el ponente.", Reason: "harassment", Details: "Insulta al ponente.",
			Status: "open", CreatedAt: ahoraFijo.Add(-3600e9), Actions: []string{api.CastActionDismiss, api.CastActionDelete, api.CastActionBlock},
		},
		{
			ID: "r2", Kind: api.CastReportVideo, VideoID: "v02", TargetID: "v02", TargetSub: "u3", TargetName: "Jon Arrieta",
			Excerpt: "Demo de producto", Reason: "copyright", Anonymous: true,
			Status: "open", CreatedAt: ahoraFijo.Add(-5 * 3600e9), Actions: []string{api.CastActionDismiss, api.CastActionUnlist, api.CastActionUnpublish, api.CastActionBlock},
		},
	}
}

// recorrer visita un árbol de objetos, entrando en contenedores y widgets.
func recorrer(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
	if o == nil {
		return
	}
	f(o)
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			recorrer(h, f)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			recorrer(h, f)
		}
	}
}

// rotulos son los textos de los enlaces y los botones de un objeto: lo que se puede pulsar.
func rotulos(o fyne.CanvasObject) []string {
	var out []string
	recorrer(o, func(x fyne.CanvasObject) {
		switch p := x.(type) {
		case *ui.Enlace:
			out = append(out, p.Texto)
		case *ui.Boton:
			out = append(out, p.Texto)
		}
	})
	return out
}

func cuantos(lista []string, texto string) int {
	n := 0
	for _, x := range lista {
		if x == texto {
			n++
		}
	}
	return n
}

// TestSeDenunciaYSeBloqueaLoAjeno: en la página de un vídeo, cada comentario de otra
// persona lleva «Denunciar» y «Bloquear a …», y lo propio no. Sin sesión se puede
// denunciar y no bloquear. El vídeo propio no ofrece denunciarse.
func TestSeDenunciaYSeBloqueaLoAjeno(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.Yo = func() string { return "u-Ana Pérez" } // la autora de k2
	a.AbrirVideo("v30")                           // un vídeo de otra persona
	r := rotulos(a.pantallaVer())
	if n := cuantos(r, "Denunciar"); n != 3 {
		// Dos comentarios ajenos y el botón del vídeo.
		t.Errorf("hay %d «Denunciar» y deberían ser 3 (k1, k3 y el vídeo): %q", n, r)
	}
	if !slices.Contains(r, "Bloquear a Marta Ruiz") || !slices.Contains(r, "Bloquear a Jon Arrieta") {
		t.Errorf("faltan los «Bloquear a …» de los comentarios ajenos: %q", r)
	}
	if slices.Contains(r, "Bloquear a Ana Pérez") {
		t.Errorf("se ofrece bloquearse a una misma: %q", r)
	}

	// El vídeo propio: sin «Denunciar» del vídeo (quedan los de los comentarios).
	propio := a.ver.video
	propio.Owner.Sub = "u-Ana Pérez"
	a.ver.video = propio
	if n := cuantos(rotulos(a.pantallaVer()), "Denunciar"); n != 2 {
		t.Errorf("el vídeo propio ofrece denunciarse: %d «Denunciar»", n)
	}

	// Sin sesión: denunciar sí, bloquear no.
	b, _ := appDePrueba(t, servicioCompleto(), false)
	b.AbrirVideo("v30")
	r = rotulos(b.pantallaVer())
	if cuantos(r, "Denunciar") != 4 {
		t.Errorf("sin sesión hay %d «Denunciar» y deberían ser 4 (tres comentarios y el vídeo): %q", cuantos(r, "Denunciar"), r)
	}
	if slices.ContainsFunc(r, func(x string) bool { return strings.HasPrefix(x, "Bloquear a") }) {
		t.Errorf("sin sesión se ofrece bloquear: %q", r)
	}
}

// TestLaDenunciaSaleConSuMotivo: con sesión va a la ruta con sesión; sin ella, a la
// pública, después de pedir y resolver la prueba de trabajo. Y el formulario es un
// grupo de una sola opción, con el texto recortado al tope del servicio.
func TestLaDenunciaSaleConSuMotivo(t *testing.T) {
	denuncia := api.CastNewReport{Kind: api.CastReportComment, VideoID: "v01", Target: "k1"}
	enviar := func(a *App, motivo, detalle string) {
		t.Helper()
		f := &FormularioDenuncia{}
		_ = f.Objeto(a.v)
		f.Elegir("violence")
		f.Elegir(motivo)
		f.Detalle.PonerTexto(detalle)
		var fallo error
		a.enviarDenuncia(denuncia, f, func(err error) { fallo = err })
		if fallo != nil {
			t.Fatal(fallo)
		}
		if marcadas := slices.IndexFunc(f.Radios, func(r *ui.Eleccion) bool { return r.Marcada }); f.Radios[marcadas].Etiqueta != a.v.Traduce("report.reason."+motivo) {
			t.Errorf("la opción marcada no es la elegida")
		}
		n := 0
		for _, r := range f.Radios {
			if r.Marcada {
				n++
			}
		}
		if n != 1 || len(f.Radios) != len(api.CastMotivos) {
			t.Errorf("hay %d opciones marcadas de %d", n, len(f.Radios))
		}
	}

	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	enviar(a, "spam", "  repite el mismo enlace  ")
	if !contiene(s.vistas(), `POST /api/v1/videos/v01/reports? {"kind":"comment","target":"k1","reason":"spam","details":"repite el mismo enlace"}`) {
		t.Errorf("con sesión no fue a la ruta con sesión: %q", s.vistas())
	}
	if contiene(s.vistas(), "GET /api/v1/public/reports/altcha") {
		t.Errorf("con sesión se pidió la prueba de trabajo: %q", s.vistas())
	}

	s2 := servicioCompleto()
	b, _ := appDePrueba(t, s2, false)
	enviar(b, "sexual", strings.Repeat("a", maxDetalleDenuncia+50))
	v := s2.vistas()
	if !contiene(v, "GET /api/v1/public/reports/altcha") {
		t.Fatalf("sin sesión no se pidió la prueba de trabajo: %q", v)
	}
	i := slices.IndexFunc(v, func(x string) bool { return strings.HasPrefix(x, "POST /api/v1/public/videos/v01/reports") })
	if i < 0 || !strings.Contains(v[i], `"altcha":"`) || !strings.Contains(v[i], `"reason":"sexual"`) {
		t.Fatalf("sin sesión no fue a la pública con la prueba resuelta: %q", v)
	}
	if strings.Count(v[i], "a") > maxDetalleDenuncia+200 {
		t.Errorf("el detalle no se recortó al tope del servicio")
	}
}

// TestElDialogoNoEnviaSinMotivo: el botón de enviar está apagado hasta que se elige un
// motivo, y al enviarse el diálogo se cierra.
func TestElDialogoNoEnviaSinMotivo(t *testing.T) {
	s := servicioCompleto()
	a, w := appDePrueba(t, s, true)
	a.AbrirVideo("v02")
	a.AbrirDenuncia(api.CastNewReport{Kind: api.CastReportVideo, VideoID: "v02"}, "")
	capa := w.Canvas().Overlays().Top()
	if capa == nil {
		t.Fatal("el diálogo no se abrió")
	}
	var enviar *ui.Boton
	var radio *ui.Eleccion
	recorrer(capa, func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Boton:
			if x.Texto == "Enviar la denuncia" {
				enviar = x
			}
		case *ui.Eleccion:
			if x.Etiqueta == "Violencia" {
				radio = x
			}
		}
	})
	if enviar == nil || radio == nil {
		t.Fatalf("faltan el botón (%v) o el motivo (%v) en el diálogo", enviar != nil, radio != nil)
	}
	if !enviar.Inerte {
		t.Error("se puede enviar sin elegir motivo")
	}
	test.Tap(enviar)
	if contiene(s.vistas(), "POST /api/v1/videos/v02/reports") {
		t.Fatal("se envió sin motivo")
	}
	test.Tap(radio)
	if enviar.Inerte {
		t.Fatal("con el motivo elegido sigue sin poderse enviar")
	}
	test.Tap(enviar)
	if !contiene(s.vistas(), `POST /api/v1/videos/v02/reports? {"kind":"video","reason":"violence"}`) {
		t.Errorf("la denuncia no salió con su motivo: %q", s.vistas())
	}
	if w.Canvas().Overlays().Top() != nil {
		t.Error("el diálogo sigue abierto tras enviarse")
	}
}

// TestBloquearQuitaLoSuyoDeLaPantalla: bloquear manda el sub de la persona y, sin
// esperar a recargar, sus comentarios y las respuestas a ellos dejan de pintarse.
func TestBloquearQuitaLoSuyoDeLaPantalla(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.AbrirVideo("v01")
	antes := strings.Join(textosDePantalla(a.pantallaVer()), "\n")
	if !strings.Contains(antes, "Muy claro, gracias") || !strings.Contains(antes, "está en la carpeta del trimestre") {
		t.Fatalf("la pantalla no trae los comentarios de partida:\n%s", antes)
	}
	a.bloquear("u2", "Marta Ruiz")
	if !contiene(s.vistas(), `POST /api/v1/videos/v01/blocks? {"sub":"u2"}`) {
		t.Fatalf("el bloqueo no se mandó: %q", s.vistas())
	}
	despues := strings.Join(textosDePantalla(a.pantallaVer()), "\n")
	if strings.Contains(despues, "Muy claro, gracias") {
		t.Error("el comentario de la persona bloqueada sigue en pantalla")
	}
	if strings.Contains(despues, "está en la carpeta del trimestre") {
		t.Error("la respuesta al comentario bloqueado se queda sin hilo en pantalla")
	}
	if !strings.Contains(despues, "Las cifras de la región norte") {
		t.Error("bloquear se llevó un comentario de otra persona")
	}
	if len(a.ver.comentarios) != 3 {
		t.Error("bloquear borró comentarios: solo debe dejar de pintarlos")
	}
}

func textosDePantalla(o fyne.CanvasObject) []string {
	var out []string
	textosLeidos(o, &out)
	return out
}

// TestLaColaEsDeQuienAdministra: «Denuncias y bloqueos» pide los bloqueos propios y
// pregunta al servicio si se administra. Si sí, trae la cola, pone el contador en el
// menú y deja resolver; si no (403), solo enseña los bloqueos propios.
func TestLaColaEsDeQuienAdministra(t *testing.T) {
	s := servicioCompleto()
	s.denuncias, s.bloqueos = denunciasDePrueba(), bloqueadosDePrueba()
	a, _ := appDePrueba(t, s, true)
	a.Ir(RutaModeracion)
	v := s.vistas()
	for _, quiero := range []string{"GET /api/v1/blocks", "GET /api/v1/moderation/summary", "GET /api/v1/moderation/reports?status=open"} {
		if !contiene(v, quiero) {
			t.Errorf("no se pidió %q: %q", quiero, v)
		}
	}
	if !a.mod.administra || len(a.mod.denuncias) != 2 || len(a.mod.mios) != 1 || a.v.Denuncias != 2 {
		t.Fatalf("estado: administra=%v, %d denuncias, %d bloqueos, contador %d", a.mod.administra, len(a.mod.denuncias), len(a.mod.mios), a.v.Denuncias)
	}
	r := rotulos(a.pantallaModeracion())
	for _, quiero := range []string{"Descartar", "Borrarlo", "Retirar de la portada pública", "Despublicar", "Desbloquear a Marta Ruiz"} {
		if !slices.Contains(r, quiero) {
			t.Errorf("falta «%s» en la pantalla: %q", quiero, r)
		}
	}
	// La cola no dice quién denunció: solo que una llegó sin cuenta.
	textos := strings.Join(textosDePantalla(a.pantallaModeracion()), "\n")
	if !strings.Contains(textos, "Enviada sin cuenta") || !strings.Contains(textos, "Insulta al ponente.") {
		t.Errorf("a la cola le falta lo que el servicio manda:\n%s", textos)
	}

	// Borrar el comentario cierra esa denuncia y baja el contador.
	a.resolverDenuncia(a.mod.denuncias[0], api.CastActionDelete)
	if !contiene(s.vistas(), `POST /api/v1/moderation/reports/r1? {"action":"delete"}`) {
		t.Errorf("la acción no se mandó: %q", s.vistas())
	}
	if len(a.mod.denuncias) != 1 || a.mod.denuncias[0].ID != "r2" || a.v.Denuncias != 1 {
		t.Errorf("tras resolver quedan %d denuncias y el contador en %d", len(a.mod.denuncias), a.v.Denuncias)
	}
	// Desbloquear quita la fila.
	a.quitarBloqueo(a.mod.mios[0], false)
	if !contiene(s.vistas(), "DELETE /api/v1/blocks/u2") || len(a.mod.mios) != 0 {
		t.Errorf("desbloquear: %d filas, %q", len(a.mod.mios), s.vistas())
	}
	// Las otras dos pestañas piden lo suyo.
	a.mod.pestana = PestanaBloqueosOrg
	a.pedirCola()
	if !contiene(s.vistas(), "GET /api/v1/moderation/blocks") {
		t.Errorf("la pestaña de bloqueos de la organización no pidió los suyos: %q", s.vistas())
	}

	// Quien no administra: sin cola, sin pestañas y sin contador.
	s2 := servicioCompleto()
	s2.noAdministra, s2.bloqueos = true, bloqueadosDePrueba()
	b, _ := appDePrueba(t, s2, true)
	b.Ir(RutaModeracion)
	if b.mod.administra || b.v.Denuncias != 0 || len(b.mod.mios) != 1 {
		t.Fatalf("sin administrar: administra=%v, contador %d, %d bloqueos", b.mod.administra, b.v.Denuncias, len(b.mod.mios))
	}
	if contiene(s2.vistas(), "GET /api/v1/moderation/reports") {
		t.Errorf("quien no administra pidió la cola: %q", s2.vistas())
	}
	r = rotulos(b.pantallaModeracion())
	if slices.Contains(r, "Descartar") || !slices.Contains(r, "Desbloquear a Marta Ruiz") {
		t.Errorf("la pantalla de quien no administra: %q", r)
	}

	// Sin sesión, la pantalla pide entrar y no llama a nada.
	s3 := servicioCompleto()
	c, _ := appDePrueba(t, s3, false)
	c.Ir(RutaModeracion)
	if contiene(s3.vistas(), "GET /api/v1/blocks") {
		t.Errorf("sin sesión se pidieron los bloqueos: %q", s3.vistas())
	}
}

// TestElAvisoDeUnaDenunciaAbreLaCola: el enlace del aviso del Hub
// (`/cast/<org>/moderation`) lleva a «Denuncias y bloqueos», y el menú tiene su
// entrada con el contador de las abiertas.
func TestElAvisoDeUnaDenunciaAbreLaCola(t *testing.T) {
	s := servicioCompleto()
	s.denuncias = denunciasDePrueba()
	a, _ := appDePrueba(t, s, true)
	u, _ := url.Parse("https://cast.sytue.eu/cast/acme/moderation")
	if !a.AbrirEnlace(u) || a.Ruta() != RutaModeracion {
		t.Fatalf("el enlace del aviso no abre la cola: ruta %v", a.Ruta())
	}
	v := vistaDePrueba(t, tema.Claro)
	v.Denuncias = 2
	var contador string
	recorrer(Lateral(v, SecModeracion, nil), func(o fyne.CanvasObject) {
		if f, ok := o.(*ui.FilaLateral); ok && f.Rotulo == "Denuncias y bloqueos" {
			contador = f.Contador
		}
	})
	if contador != "2" {
		t.Errorf("el menú no lleva el contador de denuncias abiertas: %q", contador)
	}
}
