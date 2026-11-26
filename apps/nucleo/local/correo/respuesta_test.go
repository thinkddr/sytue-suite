// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/sieve/sieveprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/cuentas"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// La respuesta automática (M-10) contra el ManageSieve de mentira de `sieveprueba`, con
// las conexiones por el portero de red de verdad y su lista blanca sacada de las cuentas
// (`local/cuentas`), como en la app.

func sieveDePrueba(t *testing.T, ajustar func(*sieveprueba.Servidor)) *sieveprueba.Servidor {
	t.Helper()
	s := &sieveprueba.Servidor{Usuario: usuario, Contrasena: clave, Nombres: []string{"mail.ejemplo.eu"}}
	if ajustar != nil {
		ajustar(s)
	}
	if err := s.Escuchar(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Cerrar)
	return s
}

// buzonConPortero abre el correo con el portero de la app: solo sale lo que permiten las
// cuentas guardadas. dirigir manda cada conexión permitida al servidor de prueba (hace de
// DNS); marcadas cuenta las que salen.
func buzonConPortero(t *testing.T, dirigir string, raices *Red) (*Buzon, *atomic.Int32) {
	t.Helper()
	ajustes, datos := t.TempDir(), t.TempDir()
	reg := cuentas.Abrir(ajustes, datos)
	p := red.Abrir("")
	p.Fuente(reg.Destinos)
	var marcadas atomic.Int32
	// Es `red.Portero.Marcar` (la lista blanca con el host y el puerto de verdad) con la
	// llamada dirigida al servidor de prueba: estos nombres no existen en ningún DNS.
	rd := Red{Marcar: func(ctx context.Context, tipo, direccion string) (net.Conn, error) {
		host, puertoTxt, err := net.SplitHostPort(direccion)
		if err != nil {
			return nil, err
		}
		puerto, _ := strconv.Atoi(puertoTxt)
		if err := p.Permitido(red.Correo, host, puerto, true); err != nil {
			return nil, err
		}
		marcadas.Add(1)
		var d net.Dialer
		return d.DialContext(ctx, tipo, dirigir)
	}}
	if raices != nil {
		rd.Raices = raices.Raices
	}
	b, err := Abrir(filepath.Join(datos, "Correo"), &secretosEnMemoria{m: map[string]string{}}, rd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Cerrar)
	return b, &marcadas
}

func darDeAlta(t *testing.T, b *Buzon, c Cuenta) Cuenta {
	t.Helper()
	if c.SMTP.Host == "" {
		c.SMTP = Servidor{Host: c.IMAP.Host, Puerto: 587, Seguridad: SeguridadSTARTTLS}
	}
	c, err := b.Anadir(c, clave)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRespuestaAutomaticaPorElPortero(t *testing.T) {
	s := sieveDePrueba(t, nil)
	b, marcadas := buzonConPortero(t, s.Direccion(), &Red{Raices: s.Raices()})
	ctx := context.Background()
	imap := darDeAlta(t, b, Cuenta{
		Proveedor: ProveedorIMAP, Direccion: usuario,
		IMAP: Servidor{Host: "mail.ejemplo.eu", Puerto: 993, Seguridad: SeguridadTLS},
	})
	gmail := darDeAlta(t, b, Cuenta{
		Proveedor: ProveedorGmail, Direccion: "ana@gmail.com",
		IMAP: Servidor{Host: "imap.gmail.com", Puerto: 993, Seguridad: SeguridadTLS},
	})

	// Solo la de un IMAP cualquiera puede tenerla; a Gmail no se le pregunta nunca.
	cs := b.CuentasRespuesta()
	if len(cs) != 1 || cs[0].ID != imap.ID {
		t.Fatalf("cuentas con respuesta: %+v", cs)
	}
	if _, err := b.LeerRespuesta(ctx, gmail.ID); !errors.Is(err, api.ErrMailSinRespuesta) || marcadas.Load() != 0 {
		t.Fatalf("Gmail: %v, %d conexiones", err, marcadas.Load())
	}

	r, err := b.LeerRespuesta(ctx, imap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Activa || r.Dias != 7 || !r.ConFechas || r.Reglas != "" || r.Bloqueada != "" || r.Direccion != usuario {
		t.Fatalf("sin nada: %+v", r)
	}
	if cifrada, entro := s.EntroCifrada(usuario); !entro || !cifrada {
		t.Fatal("la contraseña solo viaja con TLS (STARTTLS)")
	}

	// La persona ya tiene sus reglas activas: siguen funcionando detrás de la respuesta.
	s.Poner("reglas", "require [\"fileinto\"];\r\nfileinto \"Archivo\";\r\n", true)
	r, err = b.LeerRespuesta(ctx, imap.ID)
	if err != nil || r.Reglas != "reglas" {
		t.Fatalf("%+v %v", r, err)
	}
	desde := time.Date(2026, 12, 1, 15, 30, 0, 0, time.Local)
	hasta := time.Date(2026, 12, 15, 23, 59, 59, 0, time.Local)
	r = api.MailRespuesta{Activa: true, Asunto: "De vacaciones", Mensaje: "Vuelvo el 16.\n.\nUn saludo", Dias: 4, Desde: &desde, Hasta: &hasta}
	if err := b.GuardarRespuesta(ctx, imap.ID, r); err != nil {
		t.Fatal(err)
	}
	texto, _ := s.Script(NombreScriptSieve)
	for _, quiero := range []string{`:from "ana@ejemplo.eu"`, `:addresses ["ana@ejemplo.eu"]`, `"2026-12-01"`, `"2026-12-15"`, `include :personal :optional "reglas"`} {
		if !strings.Contains(texto, quiero) {
			t.Errorf("falta %q:\n%s", quiero, texto)
		}
	}
	if s.Activo() != NombreScriptSieve {
		t.Fatalf("activo %q", s.Activo())
	}
	leida, err := b.LeerRespuesta(ctx, imap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !leida.Activa || leida.Asunto != "De vacaciones" || leida.Mensaje != r.Mensaje || leida.Dias != 4 || leida.Reglas != "reglas" ||
		leida.Desde == nil || !leida.Desde.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.Local)) || leida.Hasta == nil || leida.Hasta.Day() != 15 {
		t.Fatalf("releída del servidor: %+v", leida)
	}
	// Apagarla devuelve las reglas de la persona.
	leida.Activa = false
	if err := b.GuardarRespuesta(ctx, imap.ID, leida); err != nil {
		t.Fatal(err)
	}
	if s.Activo() != "reglas" {
		t.Fatalf("activo tras apagar: %q", s.Activo())
	}
	if _, err := b.LeerRespuesta(ctx, "no-existe"); !errors.Is(err, ErrSinCuenta) {
		t.Fatal(err)
	}
}

// El portero solo deja el servidor Sieve de la cuenta (el que salió de su configuración):
// un puerto que nadie configuró no sale.
func TestElPorteroSoloDejaElServidorSieveDeLaCuenta(t *testing.T) {
	s := sieveDePrueba(t, nil)
	b, marcadas := buzonConPortero(t, s.Direccion(), &Red{Raices: s.Raices()})
	c := darDeAlta(t, b, Cuenta{
		Proveedor: ProveedorIMAP, Direccion: usuario,
		IMAP: Servidor{Host: "mail.ejemplo.eu", Puerto: 993, Seguridad: SeguridadTLS},
	})
	marcar := b.red.Marcar
	_, err := marcar(context.Background(), "tcp", "mail.ejemplo.eu:4191")
	if !errors.Is(err, red.ErrDenegado) || marcadas.Load() != 0 {
		t.Fatalf("un puerto que no es de la cuenta: %v", err)
	}
	// El que escribió la persona sí, una vez guardado.
	c.Sieve = Servidor{Host: "sieve.ejemplo.eu", Puerto: 2000}
	if _, err := b.Anadir(c, ""); err != nil {
		t.Fatal(err)
	}
	c2, err := marcar(context.Background(), "tcp", "sieve.ejemplo.eu:2000")
	if err != nil {
		t.Fatalf("el servidor Sieve escrito por la persona: %v", err)
	}
	_ = c2.Close()
	if _, err := marcar(context.Background(), "tcp", "mail.ejemplo.eu:4190"); !errors.Is(err, red.ErrDenegado) {
		t.Fatalf("con uno escrito, el de serie ya no vale: %v", err)
	}
}

// Sin servidor Sieve (nada en el puerto, o algo que no es ManageSieve, o sin `vacation`),
// la cuenta no la ofrece: la pantalla no enseña la sección. Un servidor que no cifra, en
// cambio, es un error que se dice.
func TestSinSieveNoHaySeccion(t *testing.T) {
	ctx := context.Background()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cerrado := ln.Addr().String()
	_ = ln.Close()
	b, _ := buzonConPortero(t, cerrado, nil)
	c := darDeAlta(t, b, Cuenta{
		Proveedor: ProveedorIMAP, Direccion: usuario,
		IMAP: Servidor{Host: "mail.ejemplo.eu", Puerto: 993, Seguridad: SeguridadTLS},
	})
	inicio := time.Now()
	if _, err := b.LeerRespuesta(ctx, c.ID); !errors.Is(err, api.ErrMailSinRespuesta) {
		t.Fatalf("nada en el puerto: %v", err)
	}
	if time.Since(inicio) > PlazoSieve {
		t.Fatal("tardó más que el plazo")
	}

	sinVacation := sieveDePrueba(t, func(s *sieveprueba.Servidor) { s.Extensiones = []string{"fileinto"} })
	b2, _ := buzonConPortero(t, sinVacation.Direccion(), &Red{Raices: sinVacation.Raices()})
	c2 := darDeAlta(t, b2, Cuenta{
		Proveedor: ProveedorIMAP, Direccion: usuario,
		IMAP: Servidor{Host: "mail.ejemplo.eu", Puerto: 993, Seguridad: SeguridadTLS},
	})
	if _, err := b2.LeerRespuesta(ctx, c2.ID); !errors.Is(err, api.ErrMailSinRespuesta) {
		t.Fatalf("sin vacation: %v", err)
	}

	sinTLS := sieveDePrueba(t, func(s *sieveprueba.Servidor) { s.SinTLS = true })
	b3, _ := buzonConPortero(t, sinTLS.Direccion(), &Red{Raices: sinTLS.Raices()})
	c3 := darDeAlta(t, b3, Cuenta{
		Proveedor: ProveedorIMAP, Direccion: usuario,
		IMAP: Servidor{Host: "mail.ejemplo.eu", Puerto: 993, Seguridad: SeguridadTLS},
	})
	_, err = b3.LeerRespuesta(ctx, c3.ID)
	if !errors.Is(err, ErrSinCifrar) || errors.Is(err, api.ErrMailSinRespuesta) {
		t.Fatalf("sin STARTTLS: %v", err)
	}
	if _, entro := sinTLS.EntroCifrada(usuario); entro {
		t.Fatal("la contraseña no sale sin cifrar")
	}
}

// Una contraseña rechazada es ErrAcceso, como en IMAP.
func TestRespuestaConContrasenaRechazada(t *testing.T) {
	s := sieveDePrueba(t, func(s *sieveprueba.Servidor) { s.Contrasena = "otra" })
	b, _ := buzonConPortero(t, s.Direccion(), &Red{Raices: s.Raices()})
	c := darDeAlta(t, b, Cuenta{
		Proveedor: ProveedorIMAP, Direccion: usuario,
		IMAP: Servidor{Host: "mail.ejemplo.eu", Puerto: 993, Seguridad: SeguridadTLS},
	})
	if _, err := b.LeerRespuesta(context.Background(), c.ID); !errors.Is(err, ErrAcceso) {
		t.Fatalf("%v", err)
	}
}

// Con un servidor de este equipo se habla sin cifrar (la regla de Mail), y la sección
// funciona igual.
func TestRespuestaConUnServidorLocalSinCifrar(t *testing.T) {
	s := sieveDePrueba(t, func(s *sieveprueba.Servidor) { s.SinTLS = true })
	b, err := Abrir(t.TempDir(), &secretosEnMemoria{m: map[string]string{}}, Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Cerrar)
	c := darDeAlta(t, b, Cuenta{
		Proveedor: ProveedorIMAP, Direccion: usuario,
		IMAP:  Servidor{Host: "127.0.0.1", Puerto: 1143, Seguridad: SeguridadNinguna},
		SMTP:  Servidor{Host: "127.0.0.1", Puerto: 1025, Seguridad: SeguridadNinguna},
		Sieve: Servidor{Puerto: s.Puerto()},
	})
	if err := b.GuardarRespuesta(context.Background(), c.ID, api.MailRespuesta{Activa: true, Asunto: "Fuera", Mensaje: "Hola"}); err != nil {
		t.Fatal(err)
	}
	if s.Activo() != NombreScriptSieve {
		t.Fatalf("activo %q", s.Activo())
	}
}

// local/cuentas (que abre el portero) y ServidorSieve dicen lo mismo de cada cuenta.
func TestElServidorSieveCasaConElDelPortero(t *testing.T) {
	for _, c := range []Cuenta{
		{Proveedor: ProveedorIMAP, IMAP: Servidor{Host: "mail.casa.eu"}},
		{Proveedor: ProveedorIMAP, IMAP: Servidor{Host: "127.0.0.1"}},
		{Proveedor: ProveedorIMAP, IMAP: Servidor{Host: "mail.casa.eu"}, Sieve: Servidor{Puerto: 2000}},
		{Proveedor: ProveedorGmail, IMAP: Servidor{Host: "imap.gmail.com"}},
		{Proveedor: ProveedorMicrosoft, IMAP: Servidor{Host: "outlook.office365.com"}},
		{Proveedor: ProveedorICloud, IMAP: Servidor{Host: "imap.mail.me.com"}, Sieve: Servidor{Host: "sieve.casa.eu"}},
		{Proveedor: ProveedorSytue, IMAP: Servidor{Host: "imap.sytue.eu"}},
	} {
		raw, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		mc, err := cuentas.DeCorreo(raw)
		if err != nil {
			t.Fatal(err)
		}
		var delPortero *cuentas.Servidor
		for i, s := range mc.Servidores {
			if s.Protocolo == "managesieve" {
				delPortero = &mc.Servidores[i]
			}
		}
		s, ok := c.ServidorSieve()
		if ok != (delPortero != nil) {
			t.Fatalf("%s: ServidorSieve %v, portero %+v", c.Proveedor, ok, delPortero)
		}
		if ok && (delPortero.Host != s.Host || delPortero.Puerto != s.Puerto || delPortero.Seguridad != s.Seguridad) {
			t.Fatalf("%s: %+v frente a %+v", c.Proveedor, s, *delPortero)
		}
	}
	if PuertoSieve != cuentas.PuertoSieve || ProveedorIMAP != cuentas.ProveedorIMAPGenerico {
		t.Fatal("las constantes de los dos paquetes no casan")
	}
}

func TestValidarElServidorSieve(t *testing.T) {
	c := Cuenta{
		Direccion: usuario, IMAP: Servidor{Host: "a", Puerto: 1}, SMTP: Servidor{Host: "a", Puerto: 1},
		Sieve: Servidor{Host: " sieve.casa.eu ", Puerto: 4190, Seguridad: SeguridadNinguna, Usuario: "x"},
	}
	if err := validar(&c); err != nil || c.Sieve != (Servidor{Host: "sieve.casa.eu", Puerto: 4190}) {
		t.Fatalf("%+v %v", c.Sieve, err)
	}
	for _, mal := range []Servidor{{Host: "sieve.casa.eu:4190"}, {Host: "a b"}, {Puerto: 70000}} {
		c.Sieve = mal
		if err := validar(&c); err == nil {
			t.Errorf("%+v no vale", mal)
		}
	}
}
