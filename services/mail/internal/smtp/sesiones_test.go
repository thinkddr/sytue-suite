// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Este paquete no tenía NI UN test —365 líneas, 0,0% de cobertura— y decide dos
// cosas que no se pueden probar en producción: quién puede enviar correo por el
// MSA y con qué remite. Aquí están las reglas que no necesitan base de datos,
// que son justo las de seguridad: el orden AUTH → MAIL → RCPT → DATA, el
// anti-spoofing del envelope-from y el tope de destinatarios.
//
// LO QUE NO SE PRUEBA AQUÍ, y por qué: `Deps.Inbound` y `Deps.Outbound` son
// structs concretos con un `*store.Store` dentro, así que todo lo que llega a
// tocarlos —el encolado real, el `Resolve` anti-relay del MX, el rate-limit por
// IP— exige Postgres y vive en los tests de integración. Convertirlos en
// interfaces solo para poder simularlos es un cambio de diseño que no toca hacer
// desde un fichero de tests.
package smtp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	"git.sytue.eu/sytue/one/services/mail/internal/config"
)

// verificador simula a Sytue ID: devuelve la identidad que se le diga, o falla.
type verificador struct {
	subject, tenant, address string
	err                      error
}

func (v verificador) Authenticate(_ context.Context, _, _ string) (string, string, string, error) {
	if v.err != nil {
		return "", "", "", v.err
	}
	return v.subject, v.tenant, v.address, nil
}

// sesionAutenticada devuelve una sesión de submission ya pasada por AUTH, con
// `MaxAuthFailuresPerMinPerIP` a cero para no rozar el `Inbound` (que necesita
// base de datos) y el tope de destinatarios que pida cada test.
func sesionAutenticada(t *testing.T, direccion string, maxDest int) *submissionSession {
	t.Helper()
	return &submissionSession{
		d: Deps{
			Cfg:    config.Config{MaxRecipients: maxDest},
			Logger: slog.New(slog.DiscardHandler),
		},
		authenticated: true,
		subject:       "sub-1",
		tenant:        "00000000-0000-0000-0000-000000000001",
		address:       strings.ToLower(direccion),
	}
}

// codigoSMTP saca el código de respuesta de un error de go-smtp. 0 si el error
// no es un *SMTPError: un fallo sin código es un 451 genérico de cara al cliente
// y en un test es una señal de que se rompió otra cosa.
func codigoSMTP(err error) int {
	var e *gosmtp.SMTPError
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

// SIN AUTENTICAR NO SE HACE NADA. Es la diferencia entre un MSA y un relay
// abierto: los tres verbos tienen que cortar por su cuenta, porque un cliente
// puede saltarse el orden y mandar MAIL FROM sin haber pasado por AUTH.
func TestSubmissionExigeAutenticacion(t *testing.T) {
	nueva := func() *submissionSession {
		return &submissionSession{d: Deps{Cfg: config.Config{MaxRecipients: 10}, Logger: slog.New(slog.DiscardHandler)}}
	}
	casos := map[string]func(*submissionSession) error{
		"MAIL": func(s *submissionSession) error { return s.Mail("quien@sytue.eu", nil) },
		"RCPT": func(s *submissionSession) error { return s.Rcpt("otra@sytue.eu", nil) },
		"DATA": func(s *submissionSession) error { return s.Data(strings.NewReader("hola")) },
	}
	for verbo, llamar := range casos {
		t.Run(verbo, func(t *testing.T) {
			if got := codigoSMTP(llamar(nueva())); got != 530 {
				t.Fatalf("%s sin AUTH devolvió %d, se esperaba 530", verbo, got)
			}
		})
	}
}

// EL ENVELOPE-FROM TIENE QUE SER EL DE QUIEN SE AUTENTICÓ. Sin esta regla,
// cualquier cuenta del servidor puede enviar correo firmado como otra —incluida
// la de administración—, y el correo sale con nuestro SPF y nuestra DKIM
// diciendo que es legítimo.
func TestSubmissionMailFromNoSePuedeFalsificar(t *testing.T) {
	s := sesionAutenticada(t, "ana@sytue.eu", 10)
	if got := codigoSMTP(s.Mail("jefa@sytue.eu", nil)); got != 550 {
		t.Fatalf("suplantar a otra cuenta devolvió %d, se esperaba 550", got)
	}
	if s.from != "" {
		t.Fatalf("el remite quedó fijado a %q tras rechazarlo", s.from)
	}
	// La MISMA dirección con otra caja sí pasa: el correo no distingue mayúsculas
	// en el dominio y la comparación es EqualFold, así que rechazarla sería
	// romperle el envío a quien tenga la dirección capitalizada en su cliente.
	if err := s.Mail("Ana@Sytue.EU", nil); err != nil {
		t.Fatalf("la propia dirección en otra caja fue rechazada: %v", err)
	}
	if s.from != "Ana@sytue.eu" {
		t.Fatalf("remite = %q", s.from)
	}
}

func TestSubmissionDireccionesInvalidas(t *testing.T) {
	s := sesionAutenticada(t, "ana@sytue.eu", 10)
	if got := codigoSMTP(s.Mail("sin-arroba", nil)); got != 501 {
		t.Fatalf("MAIL FROM inválido devolvió %d, se esperaba 501", got)
	}
	if got := codigoSMTP(s.Rcpt("tampoco", nil)); got != 501 {
		t.Fatalf("RCPT TO inválido devolvió %d, se esperaba 501", got)
	}
}

// EL TOPE DE DESTINATARIOS es lo único que separa un envío normal de usar el
// servidor como amplificador. 452 y no 5xx: es un límite temporal de la
// transacción, así que el cliente puede partir el envío y seguir.
func TestSubmissionTopeDeDestinatarios(t *testing.T) {
	s := sesionAutenticada(t, "ana@sytue.eu", 2)
	for i, dir := range []string{"una@sytue.eu", "dos@sytue.eu"} {
		if err := s.Rcpt(dir, nil); err != nil {
			t.Fatalf("destinatario %d rechazado: %v", i+1, err)
		}
	}
	if got := codigoSMTP(s.Rcpt("tres@sytue.eu", nil)); got != 452 {
		t.Fatalf("pasarse del tope devolvió %d, se esperaba 452", got)
	}
	if len(s.rcpts) != 2 {
		t.Fatalf("destinatarios aceptados = %d, se esperaban 2", len(s.rcpts))
	}
}

// DATA sin destinatarios se corta ANTES de tocar el encolado, que es lo que
// hace que este test no necesite base de datos: si algún día el orden cambiara,
// aquí saldría un panic por `Outbound` nil en vez de un 554.
func TestSubmissionDataSinDestinatarios(t *testing.T) {
	s := sesionAutenticada(t, "ana@sytue.eu", 10)
	if got := codigoSMTP(s.Data(strings.NewReader("hola"))); got != 554 {
		t.Fatalf("DATA sin destinatarios devolvió %d, se esperaba 554", got)
	}
}

func TestSubmissionResetVaciaLaTransaccion(t *testing.T) {
	s := sesionAutenticada(t, "ana@sytue.eu", 10)
	if err := s.Mail("ana@sytue.eu", nil); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}
	if err := s.Rcpt("otra@sytue.eu", nil); err != nil {
		t.Fatalf("RCPT TO: %v", err)
	}
	s.Reset()
	if s.from != "" || len(s.rcpts) != 0 {
		t.Fatalf("tras RSET queda from=%q rcpts=%v", s.from, s.rcpts)
	}
	// RSET NO desautentica, y es a propósito: el cliente reutiliza la conexión
	// para el siguiente mensaje sin volver a presentar credenciales.
	if !s.authenticated {
		t.Fatal("RSET desautenticó la sesión")
	}
}

// AUTH: solo PLAIN, y lo que devuelve el IdP se guarda normalizado.
func TestSubmissionAuth(t *testing.T) {
	nueva := func(v verificador) *submissionSession {
		return &submissionSession{
			d: Deps{Cfg: config.Config{MaxRecipients: 10}, Logger: slog.New(slog.DiscardHandler), Verifier: v},
		}
	}

	t.Run("mecanismo no soportado", func(t *testing.T) {
		if _, err := nueva(verificador{}).Auth(sasl.Login); !errors.Is(err, gosmtp.ErrAuthUnsupported) {
			t.Fatalf("LOGIN devolvió %v, se esperaba ErrAuthUnsupported", err)
		}
	})

	t.Run("credenciales buenas", func(t *testing.T) {
		s := nueva(verificador{subject: "sub-1", tenant: "t-1", address: "Ana@Sytue.EU"})
		srv, err := s.Auth(sasl.Plain)
		if err != nil {
			t.Fatalf("Auth(PLAIN): %v", err)
		}
		if _, _, err := srv.Next([]byte("\x00ana@sytue.eu\x00secreto")); err != nil {
			t.Fatalf("credenciales válidas rechazadas: %v", err)
		}
		if !s.authenticated {
			t.Fatal("la sesión no quedó autenticada")
		}
		// En minúsculas, porque `Mail` compara contra esto y una dirección con
		// mayúsculas del IdP haría fallar el anti-spoofing de arriba.
		if s.address != "ana@sytue.eu" {
			t.Fatalf("dirección guardada = %q", s.address)
		}
	})

	t.Run("credenciales malas", func(t *testing.T) {
		s := nueva(verificador{err: errors.New("no")})
		srv, err := s.Auth(sasl.Plain)
		if err != nil {
			t.Fatalf("Auth(PLAIN): %v", err)
		}
		if _, _, err := srv.Next([]byte("\x00ana@sytue.eu\x00mala")); !errors.Is(err, gosmtp.ErrAuthFailed) {
			t.Fatalf("credenciales malas devolvieron %v, se esperaba ErrAuthFailed", err)
		}
		if s.authenticated {
			t.Fatal("la sesión quedó autenticada tras un fallo de credenciales")
		}
	})
}

// EL MX ENTRANTE NO OFRECE AUTH: si lo hiciera sería a la vez un relay abierto
// para quien adivine una contraseña y un punto de fuerza bruta contra los
// buzones, expuesto al Internet entero en el puerto 25.
func TestMXNoOfreceAuth(t *testing.T) {
	s := &mxSession{}
	if mecs := s.AuthMechanisms(); len(mecs) != 0 {
		t.Fatalf("el MX anuncia mecanismos de AUTH: %v", mecs)
	}
	if _, err := s.Auth(sasl.Plain); !errors.Is(err, gosmtp.ErrAuthUnsupported) {
		t.Fatalf("el MX aceptó AUTH: %v", err)
	}
}

// EL REMITE VACÍO ES LEGÍTIMO en el MX: `MAIL FROM:<>` es el null sender de los
// rebotes (RFC 5321 §4.5.5). Rechazarlo deja los rebotes fuera y quien envía no
// se entera nunca de que su correo no llegó.
func TestMXAceptaElRemiteVacioDeLosRebotes(t *testing.T) {
	s := &mxSession{d: Deps{Cfg: config.Config{MaxRecipients: 10}, Logger: slog.New(slog.DiscardHandler)}}
	if err := s.Mail("", nil); err != nil {
		t.Fatalf("null sender rechazado: %v", err)
	}
	if s.from != "" {
		t.Fatalf("null sender quedó como %q", s.from)
	}
}

func TestMXNormalizaYValidaElRemite(t *testing.T) {
	s := &mxSession{d: Deps{Cfg: config.Config{MaxRecipients: 10}, Logger: slog.New(slog.DiscardHandler)}}
	if err := s.Mail("Alguien@Ejemplo.COM", nil); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}
	if s.from != "alguien@ejemplo.com" {
		t.Fatalf("remite = %q, se esperaba en minúsculas", s.from)
	}
	if got := codigoSMTP(s.Mail("sin-arroba", nil)); got != 501 {
		t.Fatalf("remite inválido devolvió %d, se esperaba 501", got)
	}
}

func TestMXDataSinDestinatarios(t *testing.T) {
	s := &mxSession{d: Deps{Cfg: config.Config{MaxRecipients: 10}, Logger: slog.New(slog.DiscardHandler)}}
	if got := codigoSMTP(s.Data(io.LimitReader(strings.NewReader("hola"), 4))); got != 554 {
		t.Fatalf("DATA sin destinatarios devolvió %d, se esperaba 554", got)
	}
}

// remoteIP con una conexión nula no puede reventar: el rate-limit por IP la
// llama en CADA conexión entrante, así que un panic aquí tira el MX entero.
func TestRemoteIPSinConexion(t *testing.T) {
	if ip := remoteIP(nil); ip != nil {
		t.Fatalf("remoteIP(nil) = %v", ip)
	}
}
