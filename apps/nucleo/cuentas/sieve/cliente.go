// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package sieve es un cliente ManageSieve (RFC 5804) pequeño y en Go puro, y lo justo de
// Sieve (RFC 5228) para escribir y volver a leer una respuesta automática (RFC 5230)
// sin tocar las reglas que la persona ya tenga en su servidor (respuesta.go).
//
// El cliente no marca: recibe la conexión TCP ya abierta (en Sytue Office, por el portero
// de red) y pone él el cifrado. Reglas:
//
//   - STARTTLS es obligatorio. Sin cifrar solo se habla con un servidor de este equipo
//     (localhost), la misma regla que el correo de la edición; un servidor que no ofrece
//     STARTTLS no se usa, y nunca se baja a claro.
//   - Entra con SASL PLAIN (contraseña) o, con un token OAuth, con OAUTHBEARER o XOAUTH2
//     si el servidor los anuncia.
//   - Todo lo que llega del servidor es un dato no confiable: cada línea, cada literal y
//     cada respuesta tienen tope ([TopeLinea], [TopeLiteral], [TopeRespuesta],
//     [TopeLineas]); pasado el tope la conexión se da por rota.
//
// Un [Cliente] es de una sola goroutine y de una sola tarea: se abre, se usa y se cierra.
package sieve

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Topes de lo que se acepta del servidor.
const (
	// TopeLinea es lo más que mide una línea de respuesta, sin contar sus literales.
	TopeLinea = 64 << 10
	// TopeLiteral es lo más que mide un literal: un script entero.
	TopeLiteral = 1 << 20
	// TopeRespuesta es lo más que mide una respuesta entera, literales incluidos.
	TopeRespuesta = 4 << 20
	// TopeLineas es cuántas líneas de datos trae como mucho una respuesta (capacidades,
	// la lista de scripts).
	TopeLineas = 10000
	// topeCadenaEntrecomillada es el de una cadena entre comillas que se manda (RFC 5804:
	// `*1024QUOTED-CHAR`); lo que no cabe va como literal.
	topeCadenaEntrecomillada = 1024
	// topeRondasSASL son los retos que se contestan antes de dar la entrada por fallida.
	topeRondasSASL = 4
	// topeTextoError es lo que se guarda del texto de un error del servidor.
	topeTextoError = 300
)

// Errores del cliente.
var (
	// ErrNoEsSieve: lo que contesta en ese puerto no es un servidor ManageSieve.
	ErrNoEsSieve = errors.New("sieve: el servidor no habla ManageSieve")
	// ErrSinTLS: el servidor no ofrece STARTTLS y no es de este equipo.
	ErrSinTLS = errors.New("sieve: el servidor no ofrece cifrado (STARTTLS)")
	// ErrSinCifrar: se pidió hablar sin cifrar con un servidor que no es de este equipo.
	ErrSinCifrar = errors.New("sieve: sin cifrar solo se puede hablar con un servidor de este equipo (localhost)")
	// ErrSinMecanismo: el servidor no anuncia ningún mecanismo de entrada que valga.
	ErrSinMecanismo = errors.New("sieve: el servidor no ofrece una forma de entrar que sirva")
	// ErrAcceso: el servidor rechazó la contraseña o el token.
	ErrAcceso = errors.New("sieve: el servidor rechazó el acceso")
	// ErrDemasiado: el servidor mandó más de lo que se acepta.
	ErrDemasiado = errors.New("sieve: la respuesta del servidor es demasiado grande")
	// ErrProtocolo: el servidor contestó algo que no sigue el protocolo.
	ErrProtocolo = errors.New("sieve: respuesta del servidor no válida")
	// ErrNoAdmitido: el servidor no tiene esa orden (CHECKSCRIPT antes de la versión 1.0).
	ErrNoAdmitido = errors.New("sieve: el servidor no admite esa orden")
	// ErrNombre: un nombre de script que el protocolo no admite.
	ErrNombre = errors.New("sieve: nombre de script no válido")
	// ErrCerrado: la conexión ya no sirve (se cerró, se cortó o se pasó de un tope).
	ErrCerrado = errors.New("sieve: la conexión está cerrada")
)

// ErrorServidor es un NO o un BYE del servidor.
type ErrorServidor struct {
	// Orden es lo que se pidió (PUTSCRIPT…).
	Orden string
	// Respuesta es NO o BYE.
	Respuesta string
	// Codigo es el código de respuesta, en mayúsculas (QUOTA/MAXSIZE, NONEXISTENT…);
	// vacío si no trae.
	Codigo string
	// Texto es lo que dice el servidor, recortado y sin caracteres de control.
	Texto string
}

func (e *ErrorServidor) Error() string {
	s := "sieve: " + e.Orden + ": " + e.Respuesta
	if e.Codigo != "" {
		s += " (" + e.Codigo + ")"
	}
	if e.Texto != "" {
		s += ": " + e.Texto
	}
	return s
}

// Opciones son las de una conexión.
type Opciones struct {
	// Host es el nombre del servidor: el que se comprueba en su certificado y el que
	// decide si se admite hablar sin cifrar.
	Host string
	// Raices son las autoridades de confianza; nil, las del sistema. La comprobación del
	// certificado no se apaga nunca.
	Raices *x509.CertPool
	// SinCifrar es hablar sin TLS: solo vale con un servidor de este equipo
	// ([EsLocal]); con cualquier otro, [Conectar] falla con [ErrSinCifrar]. Sin él,
	// STARTTLS es obligatorio.
	SinCifrar bool
}

// Capacidades es lo que anuncia el servidor.
type Capacidades struct {
	Implementacion string
	// Extensiones de Sieve («vacation», «date», «include»…), en minúsculas.
	Extensiones []string
	// SASL son los mecanismos de entrada, en mayúsculas.
	SASL     []string
	STARTTLS bool
	// Version es la del protocolo («1.0»); vacía en un servidor anterior a RFC 5804, que
	// no tiene CHECKSCRIPT.
	Version string
	Owner   string
}

// Extension dice si el servidor admite esa extensión de Sieve.
func (c Capacidades) Extension(x string) bool {
	return slices.Contains(c.Extensiones, strings.ToLower(x))
}

// Mecanismo dice si el servidor ofrece ese mecanismo SASL.
func (c Capacidades) Mecanismo(m string) bool { return slices.Contains(c.SASL, strings.ToUpper(m)) }

// Script es una entrada de LISTSCRIPTS.
type Script struct {
	Nombre string
	Activo bool
}

// Cliente es una conexión ManageSieve.
type Cliente struct {
	conn    net.Conn
	r       *bufio.Reader
	w       *bufio.Writer
	host    string
	cifrada bool
	caps    Capacidades
	// total es lo leído de la respuesta en curso.
	total int
	// roto es por qué la conexión ya no sirve.
	roto  error
	parar func() bool
}

// EsLocal dice si host es este mismo equipo: `localhost` o una dirección de bucle.
func EsLocal(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

// Conectar lee el saludo del servidor por conn y sube a TLS con STARTTLS (salvo
// [Opciones.SinCifrar] con un servidor de este equipo). La conexión muere con ctx y
// toma su plazo. Si falla, conn queda cerrada.
func Conectar(ctx context.Context, conn net.Conn, o Opciones) (*Cliente, error) {
	if o.SinCifrar && !EsLocal(o.Host) {
		_ = conn.Close()
		return nil, ErrSinCifrar
	}
	if fin, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(fin)
	}
	c := &Cliente{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn), host: o.Host}
	c.parar = context.AfterFunc(ctx, func() { _ = c.conn.Close() })
	err := func() error {
		if err := c.leerCapacidades("saludo"); err != nil {
			return err
		}
		if o.SinCifrar {
			return nil
		}
		if !c.caps.STARTTLS {
			return ErrSinTLS
		}
		return c.subirATLS(ctx, o)
	}()
	if err != nil {
		_ = c.Cerrar()
		if ctx.Err() != nil && !errors.Is(err, ErrNoEsSieve) {
			return nil, fmt.Errorf("%w: %w", err, ctx.Err())
		}
		return nil, err
	}
	return c, nil
}

func (c *Cliente) subirATLS(ctx context.Context, o Opciones) error {
	if _, err := c.w.WriteString("STARTTLS\r\n"); err != nil {
		return c.romper(err)
	}
	if err := c.w.Flush(); err != nil {
		return c.romper(err)
	}
	if _, err := c.leerRespuesta("STARTTLS"); err != nil {
		return err
	}
	// Lo que llegue en claro detrás del OK no se lee nunca como si fuera cifrado (la
	// inyección de órdenes en STARTTLS).
	if c.r.Buffered() > 0 {
		return c.romper(fmt.Errorf("%w: datos en claro detrás de STARTTLS", ErrProtocolo))
	}
	host := strings.Trim(o.Host, "[]")
	t := tls.Client(c.conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, RootCAs: o.Raices})
	if err := t.HandshakeContext(ctx); err != nil {
		return c.romper(fmt.Errorf("sieve: TLS: %w", err))
	}
	c.conn, c.r, c.w, c.cifrada = t, bufio.NewReader(t), bufio.NewWriter(t), true
	// RFC 5804 §2.2: con TLS el servidor vuelve a anunciar sus capacidades.
	return c.leerCapacidades("STARTTLS")
}

// Capacidades son las últimas que anunció el servidor.
func (c *Cliente) Capacidades() Capacidades { return c.caps }

// Cifrada dice si la conexión va con TLS.
func (c *Cliente) Cifrada() bool { return c.cifrada }

// Cerrar corta la conexión sin despedirse.
func (c *Cliente) Cerrar() error {
	if c.parar != nil {
		c.parar()
	}
	if c.roto == nil {
		c.roto = ErrCerrado
	}
	return c.conn.Close()
}

// Salir se despide (LOGOUT) y cierra.
func (c *Cliente) Salir() error {
	if c.roto != nil {
		return c.Cerrar()
	}
	_, err := c.orden("LOGOUT")
	var es *ErrorServidor
	if errors.As(err, &es) && es.Respuesta == "BYE" {
		err = nil
	}
	// El servidor cierra detrás del OK: lo que diga el cierre ya no importa.
	_ = c.Cerrar()
	return err
}

// ---- Entrar ----

// Autenticar entra con usuario y contraseña (SASL PLAIN). Sin cifrar solo con un
// servidor de este equipo.
func (c *Cliente) Autenticar(usuario, contrasena string) error {
	if !c.cifrada && !EsLocal(c.host) {
		return ErrSinCifrar
	}
	if !c.caps.Mecanismo("PLAIN") {
		return ErrSinMecanismo
	}
	if usuario == "" || strings.ContainsRune(usuario+contrasena, 0) {
		return ErrAcceso
	}
	return c.autenticar("PLAIN", []byte("\x00"+usuario+"\x00"+contrasena), func([]byte) []byte { return nil })
}

// AutenticarToken entra con un token OAuth: OAUTHBEARER (RFC 7628) si el servidor lo
// anuncia y, si no, XOAUTH2. Sin ninguno de los dos, [ErrSinMecanismo].
func (c *Cliente) AutenticarToken(usuario, token string) error {
	if !c.cifrada && !EsLocal(c.host) {
		return ErrSinCifrar
	}
	if usuario == "" || token == "" || strings.ContainsAny(usuario+token, "\x00\x01\r\n") {
		return ErrAcceso
	}
	switch {
	case c.caps.Mecanismo("OAUTHBEARER"):
		gs2 := strings.NewReplacer("=", "=3D", ",", "=2C").Replace(usuario)
		inicial := "n,a=" + gs2 + ",\x01auth=Bearer " + token + "\x01\x01"
		// Ante un reto (el error en JSON) se contesta con un ^A y el servidor dice NO.
		return c.autenticar("OAUTHBEARER", []byte(inicial), func([]byte) []byte { return []byte{1} })
	case c.caps.Mecanismo("XOAUTH2"):
		inicial := "user=" + usuario + "\x01auth=Bearer " + token + "\x01\x01"
		return c.autenticar("XOAUTH2", []byte(inicial), func([]byte) []byte { return nil })
	}
	return ErrSinMecanismo
}

func (c *Cliente) autenticar(mecanismo string, inicial []byte, responder func(reto []byte) []byte) error {
	if c.roto != nil {
		return c.roto
	}
	if err := c.escribir("AUTHENTICATE", mecanismo, base64.StdEncoding.EncodeToString(inicial)); err != nil {
		return err
	}
	c.total = 0
	for ronda := 0; ; ronda++ {
		ts, err := c.leerLinea()
		if err != nil {
			return err
		}
		if fin, ok := esFinal(ts); ok {
			if err := c.cerrarRespuesta("AUTHENTICATE", fin, ts); err != nil {
				var es *ErrorServidor
				if errors.As(err, &es) {
					return fmt.Errorf("%w: %w", ErrAcceso, err)
				}
				return err
			}
			// Las capacidades pueden cambiar al entrar (OWNER, extensiones por persona).
			return c.leerCapacidadesCon("CAPABILITY")
		}
		if len(ts) != 1 || ts[0].tipo != 's' || ronda >= topeRondasSASL {
			return c.romper(fmt.Errorf("%w: reto de AUTHENTICATE", ErrProtocolo))
		}
		respuesta := "*" // cancela: el servidor contesta NO
		if reto, err := base64.StdEncoding.DecodeString(ts[0].texto); err == nil {
			respuesta = base64.StdEncoding.EncodeToString(responder(reto))
		}
		if err := c.escribirCadenas(respuesta); err != nil {
			return err
		}
	}
}

// ---- Órdenes ----

// Refrescar vuelve a pedir las capacidades (CAPABILITY).
func (c *Cliente) Refrescar() error { return c.leerCapacidadesCon("CAPABILITY") }

// Scripts son los scripts de la persona (LISTSCRIPTS).
func (c *Cliente) Scripts() ([]Script, error) {
	datos, err := c.orden("LISTSCRIPTS")
	if err != nil {
		return nil, err
	}
	var out []Script
	for _, ts := range datos {
		if len(ts) == 0 || len(ts) > 2 || ts[0].tipo != 's' || (len(ts) == 2 && (ts[1].tipo != 'a' || !strings.EqualFold(ts[1].texto, "ACTIVE"))) {
			return nil, c.romper(fmt.Errorf("%w: LISTSCRIPTS", ErrProtocolo))
		}
		out = append(out, Script{Nombre: ts[0].texto, Activo: len(ts) == 2})
	}
	return out, nil
}

// Leer es el texto de un script (GETSCRIPT).
func (c *Cliente) Leer(nombre string) (string, error) {
	if err := NombreValido(nombre); err != nil {
		return "", err
	}
	datos, err := c.orden("GETSCRIPT", nombre)
	if err != nil {
		return "", err
	}
	if len(datos) != 1 || len(datos[0]) != 1 || datos[0][0].tipo != 's' {
		return "", c.romper(fmt.Errorf("%w: GETSCRIPT", ErrProtocolo))
	}
	return datos[0][0].texto, nil
}

// Comprobar pide al servidor que valide un script sin guardarlo (CHECKSCRIPT, desde la
// versión 1.0 del protocolo; antes, [ErrNoAdmitido]).
func (c *Cliente) Comprobar(texto string) error {
	if c.caps.Version == "" {
		return ErrNoAdmitido
	}
	return c.ordenScript("CHECKSCRIPT", "", texto)
}

// Subir guarda un script con ese nombre (PUTSCRIPT): lo crea o lo sustituye. El
// servidor lo valida antes de guardarlo; si es el activo, lo nuevo vale en el acto.
func (c *Cliente) Subir(nombre, texto string) error {
	if err := NombreValido(nombre); err != nil {
		return err
	}
	return c.ordenScript("PUTSCRIPT", nombre, texto)
}

// Activar deja activo ese script (SETACTIVE); con "", ninguno.
func (c *Cliente) Activar(nombre string) error {
	if nombre != "" {
		if err := NombreValido(nombre); err != nil {
			return err
		}
	}
	_, err := c.orden("SETACTIVE", nombre)
	return err
}

// NombreValido comprueba un nombre de script (RFC 5804 §1.6): UTF-8, sin caracteres de
// control ni separadores de línea o párrafo, y de un tamaño razonable.
func NombreValido(nombre string) error {
	if nombre == "" || len(nombre) > 512 || !utf8.ValidString(nombre) {
		return ErrNombre
	}
	for _, r := range nombre {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 {
			return ErrNombre
		}
	}
	return nil
}

// orden manda una orden con sus cadenas y lee su respuesta.
func (c *Cliente) orden(nombre string, args ...string) ([][]token, error) {
	if c.roto != nil {
		return nil, c.roto
	}
	if err := c.escribir(nombre, args...); err != nil {
		return nil, err
	}
	return c.leerRespuesta(nombre)
}

// ordenScript manda una orden con un script como literal, detrás del nombre si lo hay.
// Las dos que lo hacen (CHECKSCRIPT y PUTSCRIPT) solo dicen si fue bien.
func (c *Cliente) ordenScript(orden, nombre, texto string) error {
	if c.roto != nil {
		return c.roto
	}
	var b strings.Builder
	b.WriteString(orden)
	if nombre != "" {
		b.WriteByte(' ')
		b.WriteString(cadena(nombre))
	}
	b.WriteByte(' ')
	b.WriteString(literal(texto))
	b.WriteString("\r\n")
	if _, err := c.w.WriteString(b.String()); err != nil {
		return c.romper(err)
	}
	if err := c.w.Flush(); err != nil {
		return c.romper(err)
	}
	_, err := c.leerRespuesta(orden)
	return err
}

func (c *Cliente) escribir(orden string, args ...string) error {
	var b strings.Builder
	b.WriteString(orden)
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(cadena(a))
	}
	b.WriteString("\r\n")
	if _, err := c.w.WriteString(b.String()); err != nil {
		return c.romper(err)
	}
	if err := c.w.Flush(); err != nil {
		return c.romper(err)
	}
	return nil
}

// escribirCadenas manda una línea hecha solo de cadenas (la respuesta a un reto SASL).
func (c *Cliente) escribirCadenas(args ...string) error {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(cadena(a))
	}
	b.WriteString("\r\n")
	if _, err := c.w.WriteString(b.String()); err != nil {
		return c.romper(err)
	}
	if err := c.w.Flush(); err != nil {
		return c.romper(err)
	}
	return nil
}

// cadena es s como cadena del protocolo: entre comillas si se puede, y si no (larga, con
// saltos de línea o NUL, o UTF-8 no válido), como literal que no espera (`{n+}`).
func cadena(s string) string {
	if len(s) <= topeCadenaEntrecomillada && utf8.ValidString(s) && !strings.ContainsAny(s, "\x00\r\n") {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
	}
	return literal(s)
}

func literal(s string) string { return "{" + strconv.Itoa(len(s)) + "+}\r\n" + s }

// ---- Leer del servidor ----

// token es una pieza de una línea de respuesta: átomo ('a'), cadena ('s') o paréntesis.
type token struct {
	tipo  byte
	texto string
}

func (c *Cliente) romper(err error) error {
	if c.roto == nil {
		c.roto = err
	}
	_ = c.conn.Close()
	return err
}

// leerLinea lee una línea de respuesta, con sus literales. Sin espacio delante de nada:
// el protocolo es línea a línea.
func (c *Cliente) leerLinea() ([]token, error) {
	if c.roto != nil {
		return nil, c.roto
	}
	var ts []token
	n := 0
	leer := func() (byte, error) {
		b, err := c.r.ReadByte()
		if err != nil {
			return 0, c.romper(err)
		}
		n++
		c.total++
		if n > TopeLinea || c.total > TopeRespuesta {
			return 0, c.romper(ErrDemasiado)
		}
		return b, nil
	}
	for {
		b, err := leer()
		if err != nil {
			return nil, err
		}
		switch b {
		case ' ', '\t':
		case '\r':
			if b, err = leer(); err != nil {
				return nil, err
			}
			if b != '\n' {
				return nil, c.romper(fmt.Errorf("%w: CR sin LF", ErrProtocolo))
			}
			return ts, nil
		case '\n':
			return ts, nil
		case '(', ')':
			ts = append(ts, token{tipo: b})
		case '"':
			s, err := c.leerEntrecomillada(leer)
			if err != nil {
				return nil, err
			}
			ts = append(ts, token{tipo: 's', texto: s})
		case '{':
			s, err := c.leerLiteral(leer)
			if err != nil {
				return nil, err
			}
			ts = append(ts, token{tipo: 's', texto: s})
		default:
			a, err := c.leerAtomo(b, leer)
			if err != nil {
				return nil, err
			}
			ts = append(ts, token{tipo: 'a', texto: a})
		}
	}
}

// leerEntrecomillada lee una cadena entre comillas, ya abiertas, hasta las que la cierran.
func (c *Cliente) leerEntrecomillada(leer func() (byte, error)) (string, error) {
	var s []byte
	for {
		b, err := leer()
		if err != nil {
			return "", err
		}
		if b == '"' {
			return string(s), nil
		}
		if b == '\r' || b == '\n' {
			return "", c.romper(fmt.Errorf("%w: cadena sin cerrar", ErrProtocolo))
		}
		if b == '\\' {
			if b, err = leer(); err != nil {
				return "", err
			}
		}
		s = append(s, b)
	}
}

// leerAtomo lee un átomo que empieza por b, hasta lo que ya no es de él (un blanco, el
// fin de línea, un paréntesis, unas comillas o un literal), que se queda sin leer.
func (c *Cliente) leerAtomo(b byte, leer func() (byte, error)) (string, error) {
	a := []byte{b}
	for c.sigueAtomo() {
		b, err := leer()
		if err != nil {
			return "", err
		}
		a = append(a, b)
	}
	return string(a), nil
}

// sigueAtomo dice si lo próximo aún es del átomo. Si no se puede mirar, no: el átomo
// acaba ahí y el fallo lo da la próxima lectura.
func (c *Cliente) sigueAtomo() bool {
	sig, err := c.r.Peek(1)
	return err == nil && strings.IndexByte(" \t\r\n()\"{", sig[0]) < 0
}

// leerLiteral lee `{n}` (o `{n+}`) CRLF y los n octetos.
func (c *Cliente) leerLiteral(leer func() (byte, error)) (string, error) {
	var cifras []byte
	for {
		b, err := leer()
		if err != nil {
			return "", err
		}
		if b == '}' {
			break
		}
		if b == '+' {
			if b, err = leer(); err != nil {
				return "", err
			}
			if b != '}' {
				return "", c.romper(fmt.Errorf("%w: literal", ErrProtocolo))
			}
			break
		}
		if b < '0' || b > '9' || len(cifras) >= 9 {
			return "", c.romper(fmt.Errorf("%w: literal", ErrProtocolo))
		}
		cifras = append(cifras, b)
	}
	tam, err := strconv.Atoi(string(cifras))
	if err != nil {
		return "", c.romper(fmt.Errorf("%w: literal", ErrProtocolo))
	}
	for _, quiero := range []byte{'\r', '\n'} {
		b, err := leer()
		if err != nil {
			return "", err
		}
		if b != quiero {
			return "", c.romper(fmt.Errorf("%w: literal", ErrProtocolo))
		}
	}
	if tam > TopeLiteral || c.total+tam > TopeRespuesta {
		return "", c.romper(ErrDemasiado)
	}
	buf := make([]byte, tam)
	if _, err := io.ReadFull(c.r, buf); err != nil {
		return "", c.romper(err)
	}
	c.total += tam
	return string(buf), nil
}

// esFinal dice si la línea es la respuesta final (OK, NO, BYE) y cuál.
func esFinal(ts []token) (string, bool) {
	if len(ts) == 0 || ts[0].tipo != 'a' {
		return "", false
	}
	switch r := strings.ToUpper(ts[0].texto); r {
	case "OK", "NO", "BYE":
		return r, true
	}
	return "", false
}

// leerRespuesta lee las líneas de datos hasta la respuesta final de orden.
func (c *Cliente) leerRespuesta(orden string) ([][]token, error) {
	c.total = 0
	var datos [][]token
	for {
		ts, err := c.leerLinea()
		if err != nil {
			return nil, err
		}
		if fin, ok := esFinal(ts); ok {
			return datos, c.cerrarRespuesta(orden, fin, ts)
		}
		if len(datos) >= TopeLineas {
			return nil, c.romper(ErrDemasiado)
		}
		datos = append(datos, ts)
	}
}

// cerrarRespuesta interpreta la línea final: `OK|NO|BYE [(código …)] [texto]`.
func (c *Cliente) cerrarRespuesta(orden, fin string, ts []token) error {
	if fin == "OK" {
		return nil
	}
	e := &ErrorServidor{Orden: orden, Respuesta: fin}
	resto := ts[1:]
	if len(resto) > 0 && resto[0].tipo == '(' {
		cierre := slices.IndexFunc(resto, func(t token) bool { return t.tipo == ')' })
		if cierre < 0 {
			return c.romper(fmt.Errorf("%w: código de respuesta", ErrProtocolo))
		}
		if cierre > 1 && resto[1].tipo == 'a' {
			e.Codigo = strings.ToUpper(limpiar(resto[1].texto, 64))
		}
		resto = resto[cierre+1:]
	}
	if len(resto) > 0 && resto[0].tipo == 's' {
		e.Texto = limpiar(resto[0].texto, topeTextoError)
	}
	if fin == "BYE" {
		_ = c.romper(e)
	}
	return e
}

// limpiar recorta un texto del servidor y le quita los caracteres de control.
func limpiar(s string, tope int) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if len(s) > tope {
		corte := tope
		for corte > 0 && !utf8.RuneStart(s[corte]) {
			corte--
		}
		s = s[:corte] + "…"
	}
	return s
}

func (c *Cliente) leerCapacidadesCon(orden string) error {
	if c.roto != nil {
		return c.roto
	}
	if err := c.escribir(orden); err != nil {
		return err
	}
	return c.leerCapacidades(orden)
}

// leerCapacidades lee una respuesta de capacidades (el saludo, tras STARTTLS o tras
// CAPABILITY). Cada línea es `"NOMBRE" ["valor"]`; algo distinto es que ahí no hay un
// servidor ManageSieve.
func (c *Cliente) leerCapacidades(orden string) error {
	c.total = 0
	var caps Capacidades
	sieve := false
	for lineas := 0; ; lineas++ {
		ts, err := c.leerLinea()
		if err != nil {
			if orden == "saludo" && !errors.Is(err, ErrDemasiado) {
				return fmt.Errorf("%w: %w", ErrNoEsSieve, err)
			}
			return err
		}
		if fin, ok := esFinal(ts); ok {
			if err := c.cerrarRespuesta(orden, fin, ts); err != nil {
				return err
			}
			if !sieve {
				return c.romper(ErrNoEsSieve)
			}
			c.caps = caps
			return nil
		}
		if lineas >= TopeLineas {
			return c.romper(ErrDemasiado)
		}
		nombre, valor, ok := capacidad(ts)
		if !ok {
			return c.romper(ErrNoEsSieve)
		}
		if caps.anotar(nombre, valor) {
			sieve = true
		}
	}
}

// capacidad lee una línea de capacidades, `"NOMBRE" ["valor"]`; ok es falso si no tiene
// esa forma.
func capacidad(ts []token) (nombre, valor string, ok bool) {
	if len(ts) == 0 || len(ts) > 2 || ts[0].tipo != 's' || (len(ts) == 2 && ts[1].tipo != 's') {
		return "", "", false
	}
	if len(ts) == 2 {
		valor = ts[1].texto
	}
	return ts[0].texto, valor, true
}

// anotar apunta una capacidad y dice si es de las que solo anuncia un servidor
// ManageSieve (IMPLEMENTATION o SIEVE).
func (caps *Capacidades) anotar(nombre, valor string) (deSieve bool) {
	switch strings.ToUpper(nombre) {
	case "IMPLEMENTATION":
		caps.Implementacion = limpiar(valor, 200)
		return true
	case "SIEVE":
		caps.Extensiones = strings.Fields(strings.ToLower(valor))
		return true
	case "SASL":
		caps.SASL = strings.Fields(strings.ToUpper(valor))
	case "STARTTLS":
		caps.STARTTLS = true
	case "VERSION":
		caps.Version = limpiar(valor, 20)
	case "OWNER":
		caps.Owner = limpiar(valor, 200)
	}
	return false
}
