// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestMailNoReiniciaPorCambiosDeApariencia(t *testing.T) {
	s := servicioDeEjemplo()
	b, _ := buzonDePrueba(t, s)
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-a")
	ctx := b.ctx
	antes := len(s.llamadas)
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-a")
	if len(s.llamadas) != antes || ctx.Err() != nil {
		t.Fatal("la misma identidad abrió otra vida de correo")
	}
	b.Parar()
	if ctx.Err() == nil {
		t.Fatal("cerrar la ventana deja vivo el flujo de correo")
	}
}

func TestMailNoEjecutaPeticionesEncoladasDeOtraCuenta(t *testing.T) {
	s := servicioDeEjemplo()
	b, _ := buzonDePrueba(t, s)
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-a")
	anterior := b.ctx
	var trabajo []func()
	b.Lanzar = func(f func()) { trabajo = append(trabajo, f) }
	b.cargarCarpetas()
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-b")
	antes := s.pidio("Mailboxes")
	trabajo[0]()
	if anterior.Err() == nil || s.pidio("Mailboxes") != antes {
		t.Fatal("la petición encolada usó la siguiente cuenta")
	}
	for i := 1; i < len(trabajo); i++ {
		trabajo[i]()
	}
	if s.pidio("Mailboxes") != antes+1 || b.P.Menu.EstadoCarpetas != Listo {
		t.Fatal("la nueva cuenta no obtuvo su bandeja")
	}
}

func TestMailIgnoraRespuestaTardiaAunqueElTransporteNoCancele(t *testing.T) {
	b, _ := buzonDePrueba(t, servicioDeEjemplo())
	b.CambiarSesion(context.Background(), "emisor-a\x00persona-a\x00org")
	publicado := false
	pedir(b, func(ctx context.Context) (string, error) {
		b.CambiarSesion(context.Background(), "emisor-a\x00persona-b\x00org")
		if ctx.Err() == nil {
			t.Fatal("el cambio de cuenta no canceló la petición anterior")
		}
		return "dato de la otra persona", nil
	}, func(string, error) { publicado = true })
	if publicado {
		t.Fatal("una respuesta de otra cuenta llegó a la pantalla nueva")
	}
}

func TestMailSeparaBorradoresPorPersonaYOrganizacion(t *testing.T) {
	s := servicioDeEjemplo()
	b, _ := buzonDePrueba(t, s)
	w := test.NewTempWindow(t, b.P.Objeto())
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-a")
	b.Redactar()
	if b.redaccion == nil {
		t.Fatal("el fixture no abrió el redactor real")
	}
	b.redaccion.r.B.Asunto = "asunto sin terminar"
	b.redaccion.r.B.Cuerpo = "borrador de org-a"
	c := correoDePrueba(t, &correoFalso{})
	c.cancelar()
	c.b = b
	c.ctx, c.cancelar = context.WithCancel(b.ctx)
	c.redactando = true
	c.texto.PonerTexto("borrador externo de org-a")
	c.campos["password"].PonerTexto("synthetic-password")
	c.d.Mostrar(w.Canvas())
	b.correoAbierto, b.cerrarCorreoExterno = c, c.cerrar
	b.cuerpos["privado"] = Cuerpo{Texto: "mensaje anterior"}
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-b")
	if b.redaccion != nil || b.correoAbierto != nil || len(b.cuerpos) != 0 || len(w.Canvas().Overlays().List()) != 0 {
		t.Fatal("otra organización ve contenido o diálogos anteriores")
	}
	if c.texto.Texto() != "borrador externo de org-a" || c.campos["password"].Texto() != "" {
		t.Fatal("suspender perdió el borrador externo o retuvo su contraseña")
	}
	if s.pidio("SaveDraft") != 0 {
		t.Fatal("cambiar de cuenta intentó guardar con la nueva identidad")
	}
	b.CambiarSesion(context.Background(), "")
	if b.ctx.Err() == nil || b.P.Menu.EstadoCarpetas != SinPermiso || len(b.P.Bandeja.Conversaciones) != 0 {
		t.Fatal("salir deja datos o lecturas activos")
	}
	b.CambiarSesion(context.Background(), "emisor-a\x00otra-persona\x00org-a")
	if b.redaccion != nil || b.correoAbierto != nil {
		t.Fatal("otra persona recuperó borradores ajenos")
	}
	b.CambiarSesion(context.Background(), "emisor-b\x00persona\x00org-a")
	if b.redaccion != nil || b.correoAbierto != nil {
		t.Fatal("otro emisor recuperó borradores ajenos")
	}
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-a")
	if b.redaccion == nil || b.redaccion.r.B.Cuerpo != "borrador de org-a" || b.correoAbierto != c || c.texto.Texto() != "borrador externo de org-a" || !c.redactando || c.ctx.Err() != nil {
		t.Fatal("volver a la misma identidad no recuperó sus borradores")
	}
	if s.pidio("SaveDraft") != 0 {
		t.Fatal("recuperar un borrador reintentó un guardado automáticamente")
	}
}
