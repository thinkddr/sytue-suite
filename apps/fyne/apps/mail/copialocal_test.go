// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// errSinRed es lo que devuelve el cliente cuando no hay respuesta: no es un *api.Error.
var errSinRed = errors.New("dial tcp: lookup mail.sytue.eu: no such host")

const dueñoDePrueba = "ada/acme"

func almacenDePrueba(t *testing.T) *offline.Store {
	t.Helper()
	st, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// buzonConCopia es el buzón con almacén y con alguien dentro (persona «ada» de
// «acme»), que es de quien son las copias.
func buzonConCopia(t *testing.T, s *servicioFalso, st *offline.Store) (*Buzon, *[]func()) {
	t.Helper()
	b, _, pendientes := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	b.Almacen = st
	b.CambiarSesion(context.Background(), "emisor\x00ada\x00acme")
	*pendientes = nil
	return b, pendientes
}

func avisoDeCopia(b *Buzon) *ui.Aviso {
	av := b.P.Avisos.Avisos()
	if len(av) == 0 {
		return nil
	}
	return av[len(av)-1]
}

func copiasDe(st *offline.Store) []correo.Guardada { return correo.Copias(st, dueñoDePrueba) }

// TestRedactorSinConexionCerrarDejaLaCopiaYRedactarLaRecupera es el «Hecho cuando» de
// la pieza: con el servicio caído, cerrar deja la copia (y lo dice); «Redactar» la
// recupera con su adjunto y avisa; guardar con red la borra.
func TestRedactorSinConexionCerrarDejaLaCopiaYRedactarLaRecupera(t *testing.T) {
	s := servicioConTodas()
	s.errGuardar = errSinRed
	st := almacenDePrueba(t)
	b, _ := buzonConCopia(t, s, st)
	tr := b.P.T

	pdf := eml.Adjunto{Nombre: "presupuesto.pdf", Tipo: "application/pdf", Datos: []byte("%PDF-1.7 presupuesto")}
	b.abrirRedactor(Borrador{Para: "grace@example.org", Asunto: "Sin red", Cuerpo: "Te paso el presupuesto.", Adjuntos: []eml.Adjunto{pdf}})
	b.redaccion.d.AlCerrar() // clic fuera o Escape: guardar y cerrar

	if b.redaccion != nil {
		t.Fatal("el redactor sigue abierto tras cerrarlo")
	}
	if cs := copiasDe(st); len(cs) != 1 {
		t.Fatalf("sin red, cerrar dejó %d copias en el equipo; quiero 1", len(cs))
	}
	if a := avisoDeCopia(b); a == nil || a.Tono != ui.AvisoInfo || a.Titulo != tr.T("compose.draftSavedOffline", nil) {
		t.Fatalf("cerrar sin red no dice que el borrador quedó en el equipo: %+v", a)
	}

	b.Redactar()
	rd := b.redaccion
	if rd == nil || rd.r.B.Asunto != "Sin red" || rd.r.B.Para != "grace@example.org" || rd.r.B.Cuerpo != "Te paso el presupuesto." {
		t.Fatalf("«Redactar» no recuperó lo escrito: %+v", rd)
	}
	if len(rd.r.B.Adjuntos) != 1 || rd.r.B.Adjuntos[0].Nombre != "presupuesto.pdf" || !bytes.Equal(rd.r.B.Adjuntos[0].Datos, pdf.Datos) {
		t.Fatalf("el adjunto no volvió con la copia: %+v", rd.r.B.Adjuntos)
	}
	if a := avisoDeCopia(b); a == nil || a.Titulo != tr.T("compose.draftRestoredOffline", nil) || a.Descripcion != "" {
		t.Fatalf("recuperar la copia no se dice: %+v", a)
	}

	// Vuelve la red: guardar llega y la copia sobra.
	s.errGuardar = nil
	rd.guardar.AlPulsar()
	if cs := copiasDe(st); len(cs) != 0 {
		t.Fatalf("tras guardar con red quedan %d copias en el equipo", len(cs))
	}
	if b.redaccion != nil {
		t.Fatal("«Guardar borrador» con red no cerró el redactor")
	}
}

// TestElAutoguardadoDejaLaCopiaAntesDeUnCierreForzado: la copia se escribe en cuanto
// salta el autoguardado, sin esperar a que el servidor conteste; otra ventana (la app
// vuelta a abrir tras matarla) la encuentra.
func TestElAutoguardadoDejaLaCopiaAntesDeUnCierreForzado(t *testing.T) {
	s := servicioConTodas()
	s.errGuardar = errSinRed
	st := almacenDePrueba(t)
	b, pendientes := buzonConCopia(t, s, st)
	b.abrirRedactor(Borrador{Asunto: "A medias"})
	rd := b.redaccion
	rd.r.B.Cuerpo = "Lo que llevo escrito"
	rd.r.AlCambiar()
	if len(*pendientes) == 0 {
		t.Fatal("cambiar el redactor no programó el autoguardado")
	}
	(*pendientes)[len(*pendientes)-1]()
	if cs := copiasDe(st); len(cs) != 1 {
		t.Fatalf("el autoguardado sin red dejó %d copias; quiero 1", len(cs))
	}
	if a := avisoDeCopia(b); a != nil && a.Titulo == b.P.T.T("compose.draftFailed", nil) {
		t.Fatal("el autoguardado tiene que ser silencioso")
	}

	// Cierre forzado: nada más se ejecuta. La app vuelve a abrirse con el mismo almacén.
	otro, _ := buzonConCopia(t, servicioConTodas(), st)
	otro.Redactar()
	if otro.redaccion == nil || otro.redaccion.r.B.Cuerpo != "Lo que llevo escrito" || otro.redaccion.r.B.Asunto != "A medias" {
		t.Fatalf("tras el cierre forzado no se recuperó lo escrito: %+v", otro.redaccion)
	}
}

// TestEnviarSinConexionGuardaLaCopiaYDejaElRedactorAbierto es `compose.sendOffline`:
// el mensaje no sale, se queda en el equipo y el redactor sigue ahí para enviarlo
// con red; al salir, la copia se borra.
func TestEnviarSinConexionGuardaLaCopiaYDejaElRedactorAbierto(t *testing.T) {
	s := servicioConTodas()
	s.errEnvio = errSinRed
	st := almacenDePrueba(t)
	b, _ := buzonConCopia(t, s, st)
	b.abrirRedactor(Borrador{Para: "grace@example.org", Asunto: "Hola", Cuerpo: "Nos vemos el lunes."})
	rd := b.redaccion
	b.enviar(rd, false)
	if b.redaccion != rd {
		t.Fatal("enviar sin red cerró el redactor")
	}
	if cs := copiasDe(st); len(cs) != 1 {
		t.Fatalf("enviar sin red dejó %d copias; quiero 1", len(cs))
	}
	if a := avisoDeCopia(b); a == nil || a.Tono != ui.AvisoInfo || a.Titulo != b.P.T.T("compose.sendOffline", nil) {
		t.Fatalf("enviar sin red no lo dice: %+v", a)
	}

	s.errEnvio = nil
	b.enviar(rd, false)
	if s.pidio("Send") != 2 || b.redaccion != nil {
		t.Fatalf("con red no salió: %v", s.llamadas)
	}
	if cs := copiasDe(st); len(cs) != 0 {
		t.Fatalf("tras enviar quedan %d copias en el equipo", len(cs))
	}
}

// TestUnRechazoDelServicioNoEsSinConexion: lo que el servicio contesta no se arregla
// guardándolo en el equipo; el aviso es el de siempre.
func TestUnRechazoDelServicioNoEsSinConexion(t *testing.T) {
	s := servicioConTodas()
	s.errGuardar = &api.Error{Status: http.StatusInternalServerError}
	st := almacenDePrueba(t)
	b, _ := buzonConCopia(t, s, st)
	b.abrirRedactor(Borrador{Asunto: "Rechazado"})
	b.redaccion.guardar.AlPulsar()
	if a := avisoDeCopia(b); a == nil || a.Titulo != b.P.T.T("compose.draftFailed", nil) {
		t.Fatalf("un 500 se ha dado por guardado en el equipo: %+v", a)
	}
	if b.redaccion == nil {
		t.Fatal("si el guardado falla, el redactor se queda abierto")
	}
}

// TestDescartarBorraLaCopia: la papelera y «Descartar» del aspa no quieren lo escrito.
func TestDescartarBorraLaCopia(t *testing.T) {
	s := servicioConTodas()
	s.errGuardar = errSinRed
	st := almacenDePrueba(t)
	b, _ := buzonConCopia(t, s, st)
	b.abrirRedactor(Borrador{Asunto: "Para tirar"})
	b.redaccion.d.AlCerrar()
	if len(copiasDe(st)) != 1 {
		t.Fatal("no se dejó la copia")
	}
	b.Redactar()
	b.descartar(b.redaccion)
	if cs := copiasDe(st); len(cs) != 0 {
		t.Fatalf("descartar dejó %d copias", len(cs))
	}
	b.Redactar()
	if b.redaccion == nil || b.redaccion.r.B.Asunto == "Para tirar" {
		t.Fatal("tras descartar, «Redactar» volvió a sacar lo descartado")
	}
}

// TestLaCopiaEsDeSuDueño: lo que dejó otra persona en el mismo equipo no se le abre a
// quien está dentro ahora.
func TestLaCopiaEsDeSuDueño(t *testing.T) {
	st := almacenDePrueba(t)
	s := servicioConTodas()
	s.errGuardar = errSinRed
	b, _ := buzonConCopia(t, s, st)
	b.abrirRedactor(Borrador{Asunto: "De Ada"})
	b.redaccion.d.AlCerrar()

	b.CambiarSesion(context.Background(), "emisor\x00bob\x00acme")
	b.Redactar()
	if b.redaccion == nil || b.redaccion.r.B.Asunto == "De Ada" {
		t.Fatal("a Bob se le abrió la copia de Ada")
	}
}

// TestLosAdjuntosQueNoCabenSeDicen: la copia tiene tope; lo que no cabe no se guarda y
// se avisa al guardarla y al recuperarla.
func TestLosAdjuntosQueNoCabenSeDicen(t *testing.T) {
	s := servicioConTodas()
	s.errGuardar = errSinRed
	st := almacenDePrueba(t)
	b, _ := buzonConCopia(t, s, st)
	tr := b.P.T
	grande := eml.Adjunto{Nombre: "video.mp4", Tipo: "video/mp4", Datos: make([]byte, correo.TopeAdjuntos+1)}
	nota := eml.Adjunto{Nombre: "nota.txt", Tipo: "text/plain", Datos: []byte("cabe")}
	b.abrirRedactor(Borrador{Asunto: "Con vídeo", Adjuntos: []eml.Adjunto{grande, nota}})
	b.redaccion.guardar.AlPulsar()
	if a := avisoDeCopia(b); a == nil || a.Titulo != tr.T("compose.draftSavedOffline", nil) || a.Descripcion != tr.T("compose.draftAttachmentsNotKept", nil) {
		t.Fatalf("no se dice que el adjunto grande no se guardó: %+v", a)
	}
	b.Redactar()
	rd := b.redaccion
	if rd == nil || len(rd.r.B.Adjuntos) != 1 || rd.r.B.Adjuntos[0].Nombre != "nota.txt" {
		t.Fatalf("la copia trae %+v; solo cabía la nota", rd)
	}
	if a := avisoDeCopia(b); a == nil || a.Titulo != tr.T("compose.draftRestoredOffline", nil) || a.Descripcion != tr.T("compose.draftAttachmentsNotKept", nil) {
		t.Fatalf("al recuperarla no se recuerda lo que faltó: %+v", a)
	}
}

// TestLaCopiaSigueAlBorradorQueSustituyeElServicio: si se escribe mientras el guardado
// está en marcha, la copia se queda (lo nuevo solo está ahí) y apunta al borrador que
// el servicio acaba de crear, no al que sustituyó; cerrar con el guardado en marcha
// no deja que «Redactar» la saque antes de que conteste.
func TestLaCopiaSigueAlBorradorQueSustituyeElServicio(t *testing.T) {
	s := servicioConTodas()
	st := almacenDePrueba(t)
	b, pendientes := buzonConCopia(t, s, st)
	var colgadas []func()
	b.Lanzar = func(f func()) { colgadas = append(colgadas, f) }
	b.abrirRedactor(Borrador{Asunto: "Versión 1"})
	rd := b.redaccion
	rd.r.AlCambiar()
	(*pendientes)[len(*pendientes)-1]() // autoguardado: copia y guardado en marcha
	if len(colgadas) != 1 {
		t.Fatalf("el autoguardado no pidió guardar: %d llamadas", len(colgadas))
	}
	rd.r.B.Asunto = "Versión 2"
	rd.r.AlCambiar()
	rd.d.AlCerrar() // cerrar con el guardado aún sin contestar
	b.Redactar()
	if b.redaccion != nil && b.redaccion.r.B.Asunto == "Versión 2" {
		t.Fatal("«Redactar» recuperó una copia cuyo guardado aún no ha contestado")
	}
	if b.redaccion != nil {
		b.cerrar(b.redaccion)
	}
	colgadas[0]() // el servicio contesta con un id nuevo
	cs := copiasDe(st)
	if len(cs) != 1 {
		t.Fatalf("lo escrito tras el guardado debe seguir en el equipo; hay %d copias", len(cs))
	}
	if cs[0].Servidor != rd.r.B.ID || rd.r.B.ID == "" {
		t.Fatalf("la copia apunta al borrador %q; el servicio lo sustituyó por %q", cs[0].Servidor, rd.r.B.ID)
	}
	b.Redactar()
	if b.redaccion == nil || b.redaccion.r.B.Asunto != "Versión 2" || b.redaccion.r.B.ID != rd.r.B.ID {
		t.Fatalf("«Redactar» no recuperó lo último contra el borrador nuevo: %+v", b.redaccion)
	}
}
