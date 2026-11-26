// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sieve

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/sieve/sieveprueba"
)

// Todo en proceso: el servidor de mentira de sieveprueba en 127.0.0.1, con su propio
// certificado para STARTTLS.

const (
	usuario = "ana@ejemplo.eu"
	clave   = "contraseña-de-app"
)

func servidor(t *testing.T, ajustar func(*sieveprueba.Servidor)) *sieveprueba.Servidor {
	t.Helper()
	s := &sieveprueba.Servidor{Usuario: usuario, Contrasena: clave, Token: "token-bueno"}
	if ajustar != nil {
		ajustar(s)
	}
	if err := s.Escuchar(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Cerrar)
	return s
}

func abrir(t *testing.T, direccion string, o Opciones) (*Cliente, error) {
	t.Helper()
	conn, err := net.Dial("tcp", direccion)
	if err != nil {
		t.Fatal(err)
	}
	ctx, parar := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(parar)
	return Conectar(ctx, conn, o)
}

// conectado es un cliente ya dentro, por STARTTLS.
func conectado(t *testing.T, s *sieveprueba.Servidor) *Cliente {
	t.Helper()
	c, err := abrir(t, s.Direccion(), Opciones{Host: "127.0.0.1", Raices: s.Raices()})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Autenticar(usuario, clave); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Cerrar() })
	return c
}

func TestEntraPorSTARTTLSConPLAIN(t *testing.T) {
	s := servidor(t, nil)
	c := conectado(t, s)
	if !c.Cifrada() {
		t.Fatal("la conexión no subió a TLS")
	}
	if cifrada, entro := s.EntroCifrada(usuario); !entro || !cifrada {
		t.Fatalf("entró=%v cifrada=%v: la contraseña solo puede viajar con TLS", entro, cifrada)
	}
	caps := c.Capacidades()
	if !caps.Extension("vacation") || !caps.Extension("INCLUDE") || caps.Version != "1.0" || caps.STARTTLS || caps.Owner != usuario {
		t.Fatalf("capacidades tras entrar: %+v", caps)
	}
	if err := c.Salir(); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(s.Ordenes(), "LOGOUT") {
		t.Fatalf("no se despidió: %v", s.Ordenes())
	}
}

// Un servidor que no ofrece STARTTLS no se usa: nunca se baja a claro, y la contraseña
// no sale.
func TestSinSTARTTLSNoSeHablaEnClaro(t *testing.T) {
	s := servidor(t, func(s *sieveprueba.Servidor) { s.SinTLS = true })
	_, err := abrir(t, s.Direccion(), Opciones{Host: "127.0.0.1", Raices: s.Raices()})
	if !errors.Is(err, ErrSinTLS) {
		t.Fatalf("quiero ErrSinTLS, no %v", err)
	}
	if slices.Contains(s.Ordenes(), "AUTHENTICATE") {
		t.Fatal("mandó la contraseña sin cifrar")
	}
}

// Sin cifrar solo con un servidor de este equipo: la misma regla que el correo.
func TestSinCifrarSoloConLocalhost(t *testing.T) {
	s := servidor(t, func(s *sieveprueba.Servidor) { s.SinTLS = true })
	if _, err := abrir(t, s.Direccion(), Opciones{Host: "sieve.ejemplo.eu", SinCifrar: true}); !errors.Is(err, ErrSinCifrar) {
		t.Fatalf("a un servidor de fuera sin cifrar: %v", err)
	}
	c, err := abrir(t, s.Direccion(), Opciones{Host: "localhost", SinCifrar: true})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Cerrar()
	if err := c.Autenticar(usuario, clave); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"localhost", "127.0.0.1", "::1", "[::1]", "LOCALHOST.", "a.localhost"} {
		if !EsLocal(h) {
			t.Errorf("%s es de este equipo", h)
		}
	}
	for _, h := range []string{"", "ejemplo.eu", "192.168.1.2", "localhost.ejemplo.eu", "10.0.0.1"} {
		if EsLocal(h) {
			t.Errorf("%s no es de este equipo", h)
		}
	}
}

// Un certificado en que no se confía no vale: la comprobación no se apaga nunca.
func TestCertificadoNoConfiadoNoEntra(t *testing.T) {
	s := servidor(t, nil)
	if _, err := abrir(t, s.Direccion(), Opciones{Host: "127.0.0.1"}); err == nil || errors.Is(err, ErrNoEsSieve) {
		t.Fatalf("con las raíces del sistema el certificado propio no vale: %v", err)
	}
	if _, err := abrir(t, s.Direccion(), Opciones{Host: "otro.ejemplo.eu", Raices: s.Raices()}); err == nil {
		t.Fatal("un certificado de otro nombre no vale")
	}
}

func TestContrasenaRechazada(t *testing.T) {
	s := servidor(t, nil)
	c, err := abrir(t, s.Direccion(), Opciones{Host: "127.0.0.1", Raices: s.Raices()})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Cerrar()
	err = c.Autenticar(usuario, "otra")
	var es *ErrorServidor
	if !errors.Is(err, ErrAcceso) || !errors.As(err, &es) || es.Respuesta != "NO" {
		t.Fatalf("quiero ErrAcceso con el NO del servidor: %v", err)
	}
	if err := c.Autenticar(usuario, "con\x00nul"); !errors.Is(err, ErrAcceso) {
		t.Fatalf("un NUL partiría el PLAIN: %v", err)
	}
}

func TestEntraConToken(t *testing.T) {
	for _, tc := range []struct {
		sasl []string
		usa  string
	}{
		{[]string{"PLAIN", "OAUTHBEARER", "XOAUTH2"}, "OAUTHBEARER"},
		{[]string{"PLAIN", "XOAUTH2"}, "XOAUTH2"},
	} {
		s := servidor(t, func(s *sieveprueba.Servidor) { s.SASL = tc.sasl })
		c, err := abrir(t, s.Direccion(), Opciones{Host: "127.0.0.1", Raices: s.Raices()})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.AutenticarToken(usuario, "token-malo"); !errors.Is(err, ErrAcceso) {
			t.Fatalf("%s con un token malo: %v", tc.usa, err)
		}
		// Tras el reto de error y su NO, la conexión sigue sirviendo.
		if err := c.AutenticarToken(usuario, "token-bueno"); err != nil {
			t.Fatalf("%s: %v", tc.usa, err)
		}
		if _, err := c.Scripts(); err != nil {
			t.Fatal(err)
		}
		_ = c.Cerrar()
	}
	s := servidor(t, nil)
	c, err := abrir(t, s.Direccion(), Opciones{Host: "127.0.0.1", Raices: s.Raices()})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Cerrar()
	if err := c.AutenticarToken(usuario, "token-bueno"); !errors.Is(err, ErrSinMecanismo) {
		t.Fatalf("sin OAUTHBEARER ni XOAUTH2: %v", err)
	}
}

// Literales en las dos direcciones: los scripts van como `{n+}` y vuelven como `{n}`,
// y los nombres pueden llegar como literales.
func TestLiterales(t *testing.T) {
	s := servidor(t, func(s *sieveprueba.Servidor) { s.NombresLiteral = true })
	raro := `mis "reglas" \ de casa`
	texto := "require [\"fileinto\"];\r\n# \"comillas\" y \\barras\\\r\nif true { fileinto \"INBOX\"; }\r\n"
	s.Poner(raro, texto, true)
	c := conectado(t, s)
	ss, err := c.Scripts()
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 1 || ss[0].Nombre != raro || !ss[0].Activo {
		t.Fatalf("LISTSCRIPTS: %+v", ss)
	}
	got, err := c.Leer(raro)
	if err != nil || got != texto {
		t.Fatalf("GETSCRIPT: %q, %v", got, err)
	}
	largo := strings.Repeat("x", 5000)
	nuevo := "# " + largo + "\r\nkeep;\r\n"
	if err := c.Subir("largo", nuevo); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Script("largo"); got != nuevo {
		t.Fatalf("PUTSCRIPT con literal: %d bytes", len(got))
	}
	if err := c.Activar(""); err != nil || s.Activo() != "" {
		t.Fatalf("SETACTIVE \"\" desactiva: %v, activo %q", err, s.Activo())
	}
	if got := cadena("con\r\nsalto"); !strings.HasPrefix(got, "{10+}\r\n") {
		t.Fatalf("una cadena con saltos va como literal: %q", got)
	}
	if got := cadena(`a "b" \c`); got != `"a \"b\" \\c"` {
		t.Fatalf("entre comillas: %q", got)
	}
	if err := c.Subir("mal\nnombre", "keep;"); !errors.Is(err, ErrNombre) {
		t.Fatalf("un nombre con salto de línea: %v", err)
	}
}

func TestErroresDelServidor(t *testing.T) {
	s := servidor(t, func(s *sieveprueba.Servidor) {
		s.Fallos = map[string]string{"PUTSCRIPT": `NO (QUOTA/MAXSIZE) "Script demasiado grande"`}
	})
	c := conectado(t, s)
	err := c.Subir("x", "keep;")
	var es *ErrorServidor
	if !errors.As(err, &es) || es.Respuesta != "NO" || es.Codigo != "QUOTA/MAXSIZE" || es.Texto != "Script demasiado grande" || es.Orden != "PUTSCRIPT" {
		t.Fatalf("el NO con su código: %#v", err)
	}
	// Un NO no rompe la conexión.
	if _, err := c.Scripts(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Leer("no-existe"); !errors.As(err, &es) || es.Codigo != "NONEXISTENT" {
		t.Fatalf("GETSCRIPT de algo que no está: %v", err)
	}
	// El texto del servidor llega limpio y recortado.
	s2 := servidor(t, func(s *sieveprueba.Servidor) {
		s.Fallos = map[string]string{"SETACTIVE": "NO \"" + strings.Repeat("á", 400) + "\x07\""}
	})
	c2 := conectado(t, s2)
	err = c2.Activar("x")
	if !errors.As(err, &es) || len(es.Texto) > topeTextoError+len("…") || strings.ContainsRune(es.Texto, 7) {
		t.Fatalf("texto sin recortar o con control: %d bytes", len(es.Texto))
	}
	// Un BYE sí: la conexión queda rota.
	s3 := servidor(t, func(s *sieveprueba.Servidor) { s.Fallos = map[string]string{"LISTSCRIPTS": `BYE "me voy"`} })
	c3 := conectado(t, s3)
	if _, err := c3.Scripts(); !errors.As(err, &es) || es.Respuesta != "BYE" {
		t.Fatalf("BYE: %v", err)
	}
	if _, err := c3.Leer("x"); err == nil {
		t.Fatal("tras un BYE la conexión no sirve")
	}
}

// Lo que llega del servidor tiene tope: un literal o una línea enormes rompen la
// conexión sin leerlos enteros.
func TestTopesDeTamano(t *testing.T) {
	s := servidor(t, func(s *sieveprueba.Servidor) {
		s.Crudo = map[string]string{
			"GETSCRIPT":   "{2000000}\r\n",
			"LISTSCRIPTS": `"` + strings.Repeat("x", TopeLinea+10) + "\"\r\nOK\r\n",
		}
	})
	c := conectado(t, s)
	if _, err := c.Leer("x"); !errors.Is(err, ErrDemasiado) {
		t.Fatalf("literal de 2 MB: %v", err)
	}
	if _, err := c.Scripts(); !errors.Is(err, ErrDemasiado) {
		t.Fatalf("tras pasarse de un tope la conexión está rota: %v", err)
	}
	c2 := conectado(t, s)
	if _, err := c2.Scripts(); !errors.Is(err, ErrDemasiado) {
		t.Fatalf("línea enorme: %v", err)
	}
	// Demasiadas líneas.
	muchas := strings.Repeat("\"s\"\r\n", TopeLineas+1) + "OK\r\n"
	s3 := servidor(t, func(s *sieveprueba.Servidor) { s.Crudo = map[string]string{"LISTSCRIPTS": muchas} })
	if _, err := conectado(t, s3).Scripts(); !errors.Is(err, ErrDemasiado) {
		t.Fatalf("demasiadas líneas: %v", err)
	}
	// Un literal con un tamaño absurdo.
	s4 := servidor(t, func(s *sieveprueba.Servidor) { s.Crudo = map[string]string{"GETSCRIPT": "{99999999999}\r\n"} })
	if _, err := conectado(t, s4).Leer("x"); !errors.Is(err, ErrProtocolo) && !errors.Is(err, ErrDemasiado) {
		t.Fatalf("literal absurdo: %v", err)
	}
}

// servidorCrudo escucha y contesta lo que diga responder a cada línea recibida, después
// de mandar el saludo.
func servidorCrudo(t *testing.T, saludo string, responder func(linea string) string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = c.Write([]byte(saludo))
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					if responder != nil {
						if r := responder(string(buf[:n])); r != "" {
							_, _ = c.Write([]byte(r))
						}
					}
				}
			}()
		}
	}()
	return ln.Addr().String()
}

// Lo que no es un servidor ManageSieve se nota en el saludo, sin esperar al plazo.
func TestNoEsSieve(t *testing.T) {
	for _, saludo := range []string{
		"* OK [CAPABILITY IMAP4rev1] listo\r\n",
		"220 smtp.ejemplo.eu ESMTP\r\n",
	} {
		dir := servidorCrudo(t, saludo, nil)
		inicio := time.Now()
		_, err := abrir(t, dir, Opciones{Host: "127.0.0.1"})
		if !errors.Is(err, ErrNoEsSieve) {
			t.Fatalf("%q: quiero ErrNoEsSieve, no %v", saludo, err)
		}
		if time.Since(inicio) > 3*time.Second {
			t.Fatalf("%q: tardó %v", saludo, time.Since(inicio))
		}
	}
	// Un saludo bien formado pero sin nada de Sieve tampoco.
	dir := servidorCrudo(t, "\"SASL\" \"PLAIN\"\r\nOK\r\n", nil)
	if _, err := abrir(t, dir, Opciones{Host: "127.0.0.1"}); !errors.Is(err, ErrNoEsSieve) {
		t.Fatalf("sin SIEVE ni IMPLEMENTATION: %v", err)
	}
	// Un servidor que no dice nada: vale el plazo de quien llama.
	dir = servidorCrudo(t, "", nil)
	conn, err := net.Dial("tcp", dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, parar := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer parar()
	if _, err := Conectar(ctx, conn, Opciones{Host: "127.0.0.1"}); !errors.Is(err, ErrNoEsSieve) {
		t.Fatalf("callado: %v", err)
	}
}

// Lo que llega en claro detrás del OK de STARTTLS no se lee como si llegara cifrado.
func TestSTARTTLSSinInyeccion(t *testing.T) {
	saludo := "\"IMPLEMENTATION\" \"x\"\r\n\"SIEVE\" \"vacation\"\r\n\"SASL\" \"PLAIN\"\r\n\"STARTTLS\"\r\nOK\r\n"
	dir := servidorCrudo(t, saludo, func(linea string) string {
		if strings.HasPrefix(linea, "STARTTLS") {
			return "OK\r\n\"SASL\" \"PLAIN\"\r\nOK\r\n"
		}
		return ""
	})
	if _, err := abrir(t, dir, Opciones{Host: "127.0.0.1"}); !errors.Is(err, ErrProtocolo) {
		t.Fatalf("quiero ErrProtocolo por datos en claro tras STARTTLS, no %v", err)
	}
}

func TestComprobarSinVersion(t *testing.T) {
	s := servidor(t, func(s *sieveprueba.Servidor) { s.SinVersion = true })
	c := conectado(t, s)
	if err := c.Comprobar("keep;"); !errors.Is(err, ErrNoAdmitido) {
		t.Fatalf("sin VERSION no hay CHECKSCRIPT: %v", err)
	}
	if slices.Contains(s.Ordenes(), "CHECKSCRIPT") {
		t.Fatal("no se manda una orden que el servidor no tiene")
	}
}

func TestNombreValido(t *testing.T) {
	for _, n := range []string{"reglas", "mis reglas", "ñandú", `con "comillas"`} {
		if NombreValido(n) != nil {
			t.Errorf("%q vale", n)
		}
	}
	for _, n := range []string{"", "a\nb", "a\x00", "a ", "\xff", strings.Repeat("x", 600), "a\x7f"} {
		if NombreValido(n) == nil {
			t.Errorf("%q no vale", n)
		}
	}
}
