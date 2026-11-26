// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

// Los clientes IMAP y SMTP de la edición local. El cliente del servidor de
// integraciones ya vive en el núcleo (`apps/nucleo/cuentas/correo`, M-01, con el
// marcador anti-SSRF inyectado por el servidor), pero es otro cliente: sin estado, una
// conexión por llamada, por páginas y sin UID. Este mantiene la sincronización por UID,
// CONDSTORE, IDLE, APPEND y la cola de cambios, que aquel no tiene; fundirlos es
// reescribir el servidor sobre este y no se hace sin poder probarlo contra él.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
	portero "git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// PlazoConexion es lo más que espera una conexión entera con el servidor.
const PlazoConexion = 2 * time.Minute

// Errores de red con significado para la persona.
var (
	// ErrAcceso: el servidor rechazó la contraseña o el token. La cuenta pasa a
	// «reconectar» y no se reintenta sola.
	ErrAcceso = errors.New("correo: el servidor rechazó el acceso; revisa la contraseña de la cuenta")
	// ErrSinRed: no se pudo hablar con el servidor (sin conexión, o no contesta).
	ErrSinRed = errors.New("correo: no se pudo conectar con el servidor")
	// ErrEnvioIncierto: la conexión se cortó a mitad del envío; puede haber salido.
	ErrEnvioIncierto = errors.New("correo: la conexión se cortó al enviar; mira en Enviados antes de repetirlo")
	// ErrRechazado: el servidor de salida rechazó el mensaje.
	ErrRechazado = errors.New("correo: el servidor de salida rechazó el mensaje")
)

// OAuthMicrosoftCorreo es el OAuth de Outlook y Microsoft 365 para IMAP y SMTP con
// XOAUTH2 (D-19), con el cliente público de `nube` (PKCE y 127.0.0.1).
func OAuthMicrosoftCorreo(cliente string) nube.OAuth {
	o := nube.OAuthMicrosoft(cliente)
	o.Alcances = []string{"https://outlook.office.com/IMAP.AccessAsUser.All", "https://outlook.office.com/SMTP.Send", "offline_access"}
	return o
}

// Red es cómo sale el correo a la red. Lo inyecta quien abre el buzón (el portero de
// la edición, §5.8); las pruebas ponen sus raíces de confianza.
type Red struct {
	// Marcar abre la conexión TCP (el portero de la edición); nil, ninguna: sin él no
	// sale nada (R-RED).
	Marcar func(ctx context.Context, red, direccion string) (net.Conn, error)
	// Raices son las autoridades de confianza; nil, las del sistema. Nunca se apaga la
	// comprobación del certificado.
	Raices *x509.CertPool
	// HTTP es el cliente para renovar tokens OAuth.
	HTTP *http.Client
	// Web es el cliente de lo que pide un correo (la baja de un clic, las imágenes
	// remotas); nil, uno que solo llega a direcciones públicas (fuera.go).
	Web *http.Client
	// Baja es el de la baja de un clic, si va aparte del de las imágenes (la cáscara
	// los pide al portero con su propósito: `red.Portero.Puntual`); nil, el de Web.
	Baja *http.Client
}

// ErrSinCifrar: una conexión sin TLS a un servidor que no es de este equipo.
var ErrSinCifrar = errors.New("correo: sin cifrar solo se puede hablar con un servidor de este equipo (localhost)")

// HostLocal dice si host es este mismo equipo: `localhost` o una dirección de bucle
// (127.0.0.0/8, ::1). Es lo único con lo que se habla sin cifrar (SeguridadNinguna): un
// servidor de pruebas, o un puente como el de Proton Mail, que escucha en 127.0.0.1.
func HostLocal(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (r Red) marcar(ctx context.Context, s Servidor) (net.Conn, error) {
	if s.Seguridad != SeguridadTLS && s.Seguridad != SeguridadSTARTTLS && !HostLocal(s.Host) {
		// Un cuentas.json escrito a mano no rebaja la conexión: sin cifrar, solo aquí.
		return nil, ErrSinCifrar
	}
	dir := net.JoinHostPort(s.Host, strconv.Itoa(s.Puerto))
	if r.Marcar == nil {
		return nil, conCausa(ErrSinRed, " ("+s.Host+")", portero.ErrDenegado)
	}
	conn, err := r.Marcar(ctx, "tcp", dir)
	if err != nil {
		return nil, conCausa(ErrSinRed, " ("+s.Host+")", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	// La conexión muere con el contexto: cancelar la sincronización la corta.
	parar := context.AfterFunc(ctx, func() { _ = conn.Close() })
	conn = &conexionParable{Conn: conn, parar: parar}
	if s.Seguridad != SeguridadTLS {
		return conn, nil
	}
	t := tls.Client(conn, r.tls(s.Host))
	if err := t.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, conCausa(ErrSinRed, " ("+s.Host+")", err)
	}
	return t, nil
}

func (r Red) tls(host string) *tls.Config {
	return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, RootCAs: r.Raices}
}

type conexionParable struct {
	net.Conn
	parar func() bool
}

func (c *conexionParable) Close() error {
	c.parar()
	return c.Conn.Close()
}

// credencial es lo que abre una cuenta: la contraseña, o un token de acceso recién
// renovado.
type credencial struct {
	contrasena, token string
}

func (b *Buzon) credencial(ctx context.Context, c Cuenta) (credencial, error) {
	return credencialCon(ctx, b.secretos, b.red, c)
}

// credencialCon es la credencial de la cuenta con ese llavero y esa red: la del buzón,
// o la del Remitente de otra app.
func credencialCon(ctx context.Context, secretos Secretos, red Red, c Cuenta) (credencial, error) {
	switch c.Acceso {
	case AccesoMicrosoft:
		refresco, err := secretos.Leer(claveRefresco(c.ID))
		if err != nil || refresco == "" {
			return credencial{}, ErrAcceso
		}
		hc := red.HTTP
		if hc == nil {
			hc = &http.Client{Transport: portero.Denegar{}, Timeout: time.Minute} // sin el del portero, ninguno
		}
		t, err := OAuthMicrosoftCorreo(c.ClienteOAuth).Renovar(ctx, hc, refresco)
		if err != nil {
			var errRed net.Error
			if errors.As(err, &errRed) {
				return credencial{}, ErrSinRed
			}
			return credencial{}, ErrAcceso
		}
		if t.Refresco != "" && t.Refresco != refresco {
			_ = secretos.Guardar(claveRefresco(c.ID), t.Refresco)
		}
		return credencial{token: t.Acceso}, nil
	default:
		p, err := secretos.Leer(claveContrasena(c.ID))
		if err != nil || p == "" {
			return credencial{}, ErrAcceso
		}
		return credencial{contrasena: p}, nil
	}
}

// xoauth2 es el mecanismo SASL de Microsoft y Google.
type xoauth2 struct{ usuario, token string }

func (a xoauth2) Start() (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + a.usuario + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}
func (a xoauth2) Next([]byte) ([]byte, error) { return nil, ErrAcceso }

func valido(s string) bool { return s != "" && !strings.ContainsAny(s, "\x00\x01\r\n") }

// ---- IMAP ----

// abrirIMAP conecta y entra. Devuelve ErrAcceso si el servidor rechaza la entrada y
// ErrSinRed si no se llega a él.
func (b *Buzon) abrirIMAP(ctx context.Context, c Cuenta) (*imapclient.Client, error) {
	return b.abrirIMAPCon(ctx, c, nil)
}

// abrirIMAPCon es abrirIMAP con quien atiende lo que el servidor cuenta sin que se le
// pregunte (EXISTS, EXPUNGE y FETCH durante IDLE); nil, se descarta.
func (b *Buzon) abrirIMAPCon(ctx context.Context, c Cuenta, h *imapclient.UnilateralDataHandler) (*imapclient.Client, error) {
	cred, err := b.credencial(ctx, c)
	if err != nil {
		return nil, err
	}
	conn, err := b.red.marcar(ctx, c.IMAP)
	if err != nil {
		return nil, err
	}
	opts := &imapclient.Options{
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader}, TLSConfig: b.red.tls(c.IMAP.Host),
		UnilateralDataHandler: h,
	}
	var cli *imapclient.Client
	if c.IMAP.Seguridad == SeguridadSTARTTLS {
		cli, err = imapclient.NewStartTLS(conn, opts)
		if err != nil {
			_ = conn.Close()
			return nil, conCausa(ErrSinRed, ": STARTTLS", err) // nunca baja a claro
		}
	} else {
		cli = imapclient.New(conn, opts)
	}
	if err := cli.WaitGreeting(); err != nil {
		_ = cli.Close()
		return nil, conCausa(ErrSinRed, "", err)
	}
	usuario := c.usuario(c.IMAP)
	switch {
	case cred.token != "":
		if !valido(usuario) || !valido(cred.token) {
			err = ErrAcceso
		} else {
			err = cli.Authenticate(xoauth2{usuario: usuario, token: cred.token})
		}
	default:
		err = cli.Login(usuario, cred.contrasena).Wait()
	}
	if err != nil {
		_ = cli.Close()
		var ie *imap.Error
		if errors.As(err, &ie) || errors.Is(err, ErrAcceso) {
			return nil, ErrAcceso
		}
		return nil, conCausa(ErrSinRed, "", err)
	}
	return cli, nil
}

// errorIMAP distingue una respuesta del servidor (el mensaje ya no está, la carpeta
// no existe: se descarta la acción) de una conexión caída (se reintenta luego).
func errorIMAP(err error) (delServidor bool) {
	var ie *imap.Error
	return errors.As(err, &ie)
}

// imapConexion es una conexión abierta para una operación en vivo.
type imapConexion struct{ cli *imapclient.Client }

// ---- SMTP ----

// enviarSMTP manda un mensaje ya compuesto. Nunca reintenta por su cuenta: lo que se
// corta después de DATA puede haber salido (ErrEnvioIncierto).
func (b *Buzon) enviarSMTP(ctx context.Context, c Cuenta, de string, para []string, crudo []byte) error {
	return mandarSMTP(ctx, b.secretos, b.red, c, de, para, crudo)
}

// mandarSMTP es enviarSMTP con ese llavero y esa red: la del buzón, o la del
// Remitente de otra app.
func mandarSMTP(ctx context.Context, secretos Secretos, red Red, c Cuenta, de string, para []string, crudo []byte) error {
	cred, err := credencialCon(ctx, secretos, red, c)
	if err != nil {
		return err
	}
	conn, err := red.marcar(ctx, c.SMTP)
	if err != nil {
		return err
	}
	var cli *smtp.Client
	if c.SMTP.Seguridad == SeguridadSTARTTLS {
		cli, err = smtp.NewClientStartTLS(conn, red.tls(c.SMTP.Host))
		if err != nil {
			_ = conn.Close()
			return conCausa(ErrSinRed, ": STARTTLS", err)
		}
	} else {
		cli = smtp.NewClient(conn)
	}
	defer func() { _ = cli.Close() }()
	cli.CommandTimeout, cli.SubmissionTimeout = PlazoConexion, PlazoConexion
	usuario := c.usuario(c.SMTP)
	var auth sasl.Client
	switch {
	case cred.token != "":
		auth = xoauth2{usuario: usuario, token: cred.token}
	case cli.SupportsAuth("PLAIN"):
		auth = sasl.NewPlainClient("", usuario, cred.contrasena)
	case cli.SupportsAuth("LOGIN"):
		auth = sasl.NewLoginClient(usuario, cred.contrasena)
	}
	if auth != nil {
		if err := cli.Auth(auth); err != nil {
			var se *smtp.SMTPError
			if errors.As(err, &se) {
				return ErrAcceso
			}
			return conCausa(ErrSinRed, "", err)
		}
	}
	if err := cli.Mail(de, &smtp.MailOptions{Size: int64(len(crudo))}); err != nil {
		return errorSMTP(err)
	}
	for _, d := range para {
		if err := cli.Rcpt(d, nil); err != nil {
			return errorSMTP(err)
		}
	}
	w, err := cli.Data()
	if err != nil {
		return errorSMTP(err)
	}
	if _, err := w.Write(crudo); err != nil {
		return ErrEnvioIncierto
	}
	if err := w.Close(); err != nil {
		var se *smtp.SMTPError
		if errors.As(err, &se) {
			return fmt.Errorf("%w: %s", ErrRechazado, se.Message)
		}
		return ErrEnvioIncierto
	}
	_ = cli.Quit()
	return nil
}

// conCausa da el error de la categoría que se decide aquí, con la causa solo como texto.
// No la envuelve a propósito: quien mira el error con errors.Is o errors.As tiene que ver esa
// categoría y no la de debajo (un *imap.Error de la causa haría pasar un corte de red
// por un rechazo del servidor en errorIMAP; un ErrSinRed bajo ErrMailSinRespuesta
// cambiaría lo que explica la pantalla).
func conCausa(categoria error, detalle string, causa error) error {
	return fmt.Errorf("%w%s: %v", categoria, detalle, causa) //nolint:errorlint // la causa no se envuelve a propósito (ver arriba).
}

func errorSMTP(err error) error {
	var se *smtp.SMTPError
	if errors.As(err, &se) {
		return fmt.Errorf("%w: %s", ErrRechazado, se.Message)
	}
	return conCausa(ErrSinRed, "", err)
}

// Probar comprueba una cuenta antes de darla de alta: entra por IMAP y por SMTP (sin
// enviar nada).
func (b *Buzon) Probar(ctx context.Context, c Cuenta) error {
	ctx, parar := context.WithTimeout(ctx, PlazoConexion)
	defer parar()
	cli, err := b.abrirIMAP(ctx, c)
	if err != nil {
		return err
	}
	_ = cli.Logout().Wait()
	_ = cli.Close()
	cred, err := b.credencial(ctx, c)
	if err != nil {
		return err
	}
	conn, err := b.red.marcar(ctx, c.SMTP)
	if err != nil {
		return err
	}
	var sc *smtp.Client
	if c.SMTP.Seguridad == SeguridadSTARTTLS {
		if sc, err = smtp.NewClientStartTLS(conn, b.red.tls(c.SMTP.Host)); err != nil {
			_ = conn.Close()
			return conCausa(ErrSinRed, ": STARTTLS", err)
		}
	} else {
		sc = smtp.NewClient(conn)
	}
	defer func() { _ = sc.Close() }()
	usuario := c.usuario(c.SMTP)
	var auth sasl.Client
	switch {
	case cred.token != "":
		auth = xoauth2{usuario: usuario, token: cred.token}
	case sc.SupportsAuth("PLAIN"):
		auth = sasl.NewPlainClient("", usuario, cred.contrasena)
	case sc.SupportsAuth("LOGIN"):
		auth = sasl.NewLoginClient(usuario, cred.contrasena)
	}
	if auth != nil {
		if err := sc.Auth(auth); err != nil {
			return fmt.Errorf("%w (SMTP)", ErrAcceso)
		}
	}
	_ = sc.Quit()
	return nil
}
