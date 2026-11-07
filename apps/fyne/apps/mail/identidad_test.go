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
	c.campos["password"].PonerTexto("synthetic-password")
	c.d.Mostrar(w.Canvas())
	b.correoAbierto, b.cerrarCorreoExterno = c, c.cerrar
	b.cuerpos["privado"] = Cuerpo{Texto: "mensaje anterior"}
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-b")
	if b.redaccion != nil || b.correoAbierto != nil || len(b.cuerpos) != 0 || len(w.Canvas().Overlays().List()) != 0 {
		t.Fatal("otra organización ve contenido o diálogos anteriores")
	}
	if c.campos["password"].Texto() != "" {
		t.Fatal("suspender retuvo la contraseña de la cuenta externa")
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
	if b.redaccion == nil || b.redaccion.r.B.Cuerpo != "borrador de org-a" || b.correoAbierto != c || c.ctx.Err() != nil {
		t.Fatal("volver a la misma identidad no recuperó sus borradores")
	}
	if s.pidio("SaveDraft") != 0 {
		t.Fatal("recuperar un borrador reintentó un guardado automáticamente")
	}
}

// B1: un inicio de sesión que esperaba al navegador muere con la vida de la otra
// cuenta; al volver, «Cancelar» sale del asistente en vez de cortar una espera que ya no hay.
func TestVolverALaCuentaNoDejaUnInicioDeSesionColgado(t *testing.T) {
	b, _ := buzonDePrueba(t, servicioDeEjemplo())
	w := test.NewTempWindow(t, b.P.Objeto())
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-a")
	cuentas := cuentaGmailNueva()
	c := correoDePrueba(t, &correoFalso{cuentas: cuentas})
	c.cancelar()
	c.b = b
	b.ConCorreoExterno(&correoFalso{cuentas: cuentas}, func(ctx context.Context, _, _ string, _ ...string) (string, error) {
		<-ctx.Done()
		return "borrador", ctx.Err()
	})
	c.ctx, c.cancelar = context.WithCancel(b.ctx)
	c.proveedor, c.asistente, c.paso, c.instancia = "gmail", true, 1, ""
	c.cuentas = cuentas
	c.d.Mostrar(w.Canvas())
	b.correoAbierto, b.cerrarCorreoExterno = c, c.cerrar
	var trabajo func()
	b.Lanzar = func(f func()) {
		if trabajo == nil {
			trabajo = f // la espera al navegador; lo demás no se ejecuta
		}
	}
	c.autorizar()
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-b")
	trabajo() // la espera acaba con la vida anterior y su respuesta no se aplica
	b.Lanzar = func(f func()) { f() }
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-a")
	if b.correoAbierto != c || c.ocupado || c.autorizando != nil {
		t.Fatalf("el asistente vuelve esperando a un navegador que ya no contesta: ocupado %v", c.ocupado)
	}
	pulsarPieCorreo(t, c, "common.cancel")
	if c.asistente {
		t.Fatal("«Cancelar» no sale del asistente tras volver a la cuenta")
	}
}

// Cambiar de cuenta suelta la carpeta externa a la vista y las cuentas conocidas: eran
// de la otra persona, y si las carpetas nuevas no cargan, nada lo corregiría.
//
// VISTO EN ROJO (2026-11-06): sin `dejarExterna` en olvidarBuzon, la carpeta de Gmail
// seguía marcada en el lateral tras salir. Deshecho.
func TestCambiarDeCuentaSueltaLaCarpetaExterna(t *testing.T) {
	b, _ := buzonConGmail(t)
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-a")
	b.ElegirExterna("nueva", "INBOX")
	if b.externa == nil || len(b.P.Menu.Cuentas) == 0 {
		t.Fatal("el fixture no abrió la carpeta externa")
	}
	b.CambiarSesion(context.Background(), "")
	if b.externa != nil || b.P.Menu.Externa != nil || b.P.Franja != nil || len(b.P.Menu.Cuentas) != 0 {
		t.Fatal("salir deja a la vista la carpeta externa de la cuenta anterior")
	}
	if _, ok := b.cuentaExterna("nueva"); ok {
		t.Fatal("salir conserva las cuentas externas de la cuenta anterior")
	}
}
