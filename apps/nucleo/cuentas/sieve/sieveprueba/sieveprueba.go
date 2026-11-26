// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package sieveprueba es un servidor ManageSieve (RFC 5804) en memoria para las pruebas
// del cliente de `cuentas/sieve` y de la respuesta automática de Mail: saludo y
// capacidades, STARTTLS con un certificado propio para 127.0.0.1 y localhost, SASL PLAIN,
// OAUTHBEARER y XOAUTH2, LISTSCRIPTS, GETSCRIPT (como literal), PUTSCRIPT, CHECKSCRIPT,
// SETACTIVE, DELETESCRIPT y LOGOUT, con literales `{n+}` y `{n}` en lo que llega. Valida
// los scripts como un servidor de verdad en lo que importa aquí: que lo que piden en
// `require` lo tenga y que un `include` sin `:optional` apunte a un script que existe.
//
// Se puede torcer para probar los casos malos: sin STARTTLS, sin VERSION (sin
// CHECKSCRIPT), sin `include`, respuestas de error a una orden y respuestas crudas.
//
// Solo lo importan pruebas (y la prueba visual de Mail, desde fuera del repositorio).
package sieveprueba

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Servidor es el servidor de mentira. Los campos de configuración se ponen antes de
// [Servidor.Escuchar].
type Servidor struct {
	Usuario, Contrasena string
	// Token es el que acepta OAUTHBEARER y XOAUTH2.
	Token string
	// Extensiones de Sieve que anuncia; nil, las de serie (con vacation, date,
	// relational e include).
	Extensiones []string
	// SASL que anuncia; nil, PLAIN.
	SASL []string
	// SinTLS: no ofrece STARTTLS. SinVersion: no anuncia VERSION (sin CHECKSCRIPT).
	SinTLS, SinVersion bool
	// Fallos es la respuesta de error que se da a una orden (`NO (QUOTA/MAXSIZE) "…"`),
	// sin hacer nada.
	Fallos map[string]string
	// Crudo es lo que se manda tal cual en vez de la respuesta a una orden (para probar
	// respuestas rotas o enormes).
	Crudo map[string]string
	// NombresLiteral: LISTSCRIPTS manda los nombres como literales.
	NombresLiteral bool
	// En es dónde escuchar; vacío, 127.0.0.1 en un puerto libre.
	En string
	// Nombres son los nombres de más para los que vale su certificado (además de
	// localhost y 127.0.0.1), para probar un servidor «de fuera» marcando aquí.
	Nombres []string

	mu       sync.Mutex
	scripts  map[string]string
	activo   string
	ordenes  []string
	cifradas map[string]bool
	ln       net.Listener
	cert     tls.Certificate
	raices   *x509.CertPool
	abiertas sync.WaitGroup
}

// ExtensionesDeSerie son las que anuncia sin configurar nada.
var ExtensionesDeSerie = []string{"fileinto", "reject", "envelope", "vacation", "date", "relational", "include", "imap4flags"}

// Escuchar arranca el servidor en [Servidor.En] (de serie, 127.0.0.1 en un puerto libre).
func (s *Servidor) Escuchar() error {
	if err := s.certificado(); err != nil {
		return err
	}
	en := s.En
	if en == "" {
		en = "127.0.0.1:0"
	}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", en)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.ln = ln
	if s.scripts == nil {
		s.scripts = map[string]string{}
	}
	s.cifradas = map[string]bool{}
	s.mu.Unlock()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.abiertas.Add(1)
			go func() {
				defer s.abiertas.Done()
				s.atender(c)
			}()
		}
	}()
	return nil
}

// Direccion es «127.0.0.1:puerto».
func (s *Servidor) Direccion() string { return s.ln.Addr().String() }

// Puerto es el puerto en que escucha.
func (s *Servidor) Puerto() int {
	if a, ok := s.ln.Addr().(*net.TCPAddr); ok {
		return a.Port
	}
	return 0
}

// Raices son las autoridades en que confiar para su certificado.
func (s *Servidor) Raices() *x509.CertPool { return s.raices }

// Cerrar para el servidor.
func (s *Servidor) Cerrar() {
	if s.ln != nil {
		_ = s.ln.Close()
	}
}

// Poner deja un script en el servidor (activo, si se pide), como si lo hubiera escrito
// la persona con otro programa.
func (s *Servidor) Poner(nombre, texto string, activo bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scripts == nil {
		s.scripts = map[string]string{}
	}
	s.scripts[nombre] = texto
	if activo {
		s.activo = nombre
	}
}

// Script es el texto de un script y si existe.
func (s *Servidor) Script(nombre string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.scripts[nombre]
	return t, ok
}

// Scripts son los nombres de los scripts, en orden.
func (s *Servidor) Scripts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := make([]string, 0, len(s.scripts))
	for n := range s.scripts {
		ns = append(ns, n)
	}
	slices.Sort(ns)
	return ns
}

// Activo es el script activo ("" si ninguno).
func (s *Servidor) Activo() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activo
}

// Ordenes son las órdenes recibidas, en orden (solo el nombre).
func (s *Servidor) Ordenes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.ordenes)
}

// EntroCifrada dice si alguna entrada de usuario se hizo ya con TLS (y ninguna sin él).
func (s *Servidor) EntroCifrada(usuario string) (cifrada, entro bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cifradas[usuario]
	return c, ok
}

func (s *Servidor) certificado() error {
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "sieveprueba"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true,
		DNSNames: append([]string{"localhost"}, s.Nombres...), IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		return err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return err
	}
	s.raices = x509.NewCertPool()
	s.raices.AddCert(cert)
	s.cert = tls.Certificate{Certificate: [][]byte{der}, PrivateKey: clave, Leaf: cert}
	return nil
}

// ---- Una conexión ----

type conexion struct {
	s       *Servidor
	c       net.Conn
	r       *bufio.Reader
	w       *bufio.Writer
	cifrada bool
	usuario string
}

func (s *Servidor) atender(c net.Conn) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(time.Minute))
	k := &conexion{s: s, c: c, r: bufio.NewReader(c), w: bufio.NewWriter(c)}
	k.capacidades()
	k.ok("listo")
	for {
		orden, args, err := k.leerOrden()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.ordenes = append(s.ordenes, orden)
		crudo, hayCrudo := s.Crudo[orden]
		fallo, hayFallo := s.Fallos[orden]
		s.mu.Unlock()
		if hayCrudo {
			_, _ = k.w.WriteString(crudo)
			_ = k.w.Flush()
			continue
		}
		if hayFallo {
			k.linea(fallo)
			continue
		}
		if !k.ejecutar(orden, args) {
			return
		}
	}
}

func (k *conexion) linea(s string) {
	_, _ = k.w.WriteString(s + "\r\n")
	_ = k.w.Flush()
}

func (k *conexion) ok(texto string) { k.linea("OK " + cita(texto)) }
func (k *conexion) no(codigo, texto string) {
	if codigo != "" {
		k.linea("NO (" + codigo + ") " + cita(texto))
		return
	}
	k.linea("NO " + cita(texto))
}

func cita(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }

func (k *conexion) capacidades() {
	s := k.s
	ext := s.Extensiones
	if ext == nil {
		ext = ExtensionesDeSerie
	}
	sasl := s.SASL
	if sasl == nil {
		sasl = []string{"PLAIN"}
	}
	k.linea(`"IMPLEMENTATION" "sieveprueba"`)
	k.linea(`"SIEVE" ` + cita(strings.Join(ext, " ")))
	k.linea(`"SASL" ` + cita(strings.Join(sasl, " ")))
	if !s.SinTLS && !k.cifrada {
		k.linea(`"STARTTLS"`)
	}
	if !s.SinVersion {
		k.linea(`"VERSION" "1.0"`)
	}
	if k.usuario != "" {
		k.linea(`"OWNER" ` + cita(k.usuario))
	}
}

// leerOrden lee una orden: un átomo y sus cadenas (entre comillas o literales).
func (k *conexion) leerOrden() (string, []string, error) {
	var orden string
	var args []string
	for {
		b, err := k.r.ReadByte()
		if err != nil {
			return "", nil, err
		}
		switch b {
		case ' ':
		case '\r':
			if _, err := k.r.ReadByte(); err != nil {
				return "", nil, err
			}
			return strings.ToUpper(orden), args, nil
		case '\n':
			return strings.ToUpper(orden), args, nil
		case '"':
			v, err := k.leerEntrecomillada()
			if err != nil {
				return "", nil, err
			}
			args = append(args, v)
		case '{':
			v, err := k.leerLiteral()
			if err != nil {
				return "", nil, err
			}
			args = append(args, v)
		default:
			a := k.leerAtomo(b)
			if orden == "" {
				orden = a
			} else {
				args = append(args, a)
			}
		}
	}
}

// leerEntrecomillada lee una cadena entre comillas, ya abiertas, hasta las que la cierran.
func (k *conexion) leerEntrecomillada() (string, error) {
	var v []byte
	for {
		b, err := k.r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == '"' {
			return string(v), nil
		}
		if b == '\\' {
			if b, err = k.r.ReadByte(); err != nil {
				return "", err
			}
		}
		v = append(v, b)
	}
}

// leerLiteral lee un literal, `{n}` o `{n+}`, ya abierto: el tamaño, el CRLF y los n
// octetos.
func (k *conexion) leerLiteral() (string, error) {
	cab, err := k.r.ReadString('}')
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(cab, "}"), "+"))
	if err != nil || n < 0 || n > 4<<20 {
		return "", errors.New("literal no válido")
	}
	if _, err := k.r.Discard(2); err != nil { // CRLF
		return "", err
	}
	v := make([]byte, n)
	if _, err := io.ReadFull(k.r, v); err != nil {
		return "", err
	}
	return string(v), nil
}

// leerAtomo lee un átomo que empieza por b, hasta un blanco o el fin de línea.
func (k *conexion) leerAtomo(b byte) string {
	a := []byte{b}
	for {
		p, err := k.r.Peek(1)
		if err != nil || p[0] == ' ' || p[0] == '\r' || p[0] == '\n' {
			return string(a)
		}
		b, _ = k.r.ReadByte()
		a = append(a, b)
	}
}

// ejecutar atiende una orden; false es cerrar la conexión.
func (k *conexion) ejecutar(orden string, args []string) bool {
	s := k.s
	conectado := k.usuario != ""
	switch orden {
	case "CAPABILITY":
		k.capacidades()
		k.ok("capacidades")
	case "NOOP":
		k.ok("nada")
	case "LOGOUT":
		k.ok("adiós")
		return false
	case "STARTTLS":
		if s.SinTLS || k.cifrada {
			k.no("", "sin STARTTLS")
			return true
		}
		k.ok("adelante")
		t := tls.Server(k.c, &tls.Config{Certificates: []tls.Certificate{s.cert}, MinVersion: tls.VersionTLS12})
		if err := t.HandshakeContext(context.Background()); err != nil {
			return false
		}
		k.c, k.r, k.w, k.cifrada = t, bufio.NewReader(t), bufio.NewWriter(t), true
		k.capacidades()
		k.ok("TLS")
	case "AUTHENTICATE":
		return k.autenticar(args)
	default:
		if !conectado {
			k.no("", "primero hay que entrar")
			return true
		}
		k.orden(orden, args)
	}
	return true
}

func (k *conexion) autenticar(args []string) bool {
	s := k.s
	if len(args) == 0 {
		k.no("", "falta el mecanismo")
		return true
	}
	mec := strings.ToUpper(args[0])
	sasl := s.SASL
	if sasl == nil {
		sasl = []string{"PLAIN"}
	}
	if !slices.Contains(sasl, mec) {
		k.no("", "mecanismo no admitido")
		return true
	}
	var inicial []byte
	if len(args) > 1 {
		v, err := base64.StdEncoding.DecodeString(args[1])
		if err != nil {
			k.no("", "base64")
			return true
		}
		inicial = v
	} else {
		k.linea(`""`)
		_, resp, err := k.leerOrden()
		if err != nil {
			return false
		}
		_ = resp
		k.no("", "sin respuesta inicial")
		return true
	}
	usuario, bien := s.credenciales(mec, inicial)
	if !bien {
		if mec != "PLAIN" {
			// El reto de error de OAuth: el cliente contesta y entonces llega el NO.
			k.linea(cita(base64.StdEncoding.EncodeToString([]byte(`{"status":"401"}`))))
			if _, _, err := k.leerOrden(); err != nil {
				return false
			}
		}
		k.no("", "credenciales no válidas")
		return true
	}
	k.usuario = usuario
	s.mu.Lock()
	if prev, ok := s.cifradas[usuario]; !ok || prev {
		s.cifradas[usuario] = k.cifrada
	}
	s.mu.Unlock()
	k.ok("dentro")
	return true
}

// credenciales lee la respuesta inicial de un mecanismo SASL: quién dice ser y si sus
// credenciales son las del servidor.
func (s *Servidor) credenciales(mec string, inicial []byte) (usuario string, bien bool) {
	switch mec {
	case "PLAIN":
		partes := strings.Split(string(inicial), "\x00")
		if len(partes) == 3 {
			usuario = partes[1]
			bien = partes[1] == s.Usuario && partes[2] == s.Contrasena && s.Contrasena != ""
		}
	case "XOAUTH2":
		for _, p := range strings.Split(string(inicial), "\x01") {
			if v, ok := strings.CutPrefix(p, "user="); ok {
				usuario = v
			}
			if v, ok := strings.CutPrefix(p, "auth=Bearer "); ok {
				bien = v == s.Token && s.Token != ""
			}
		}
		bien = bien && usuario == s.Usuario
	case "OAUTHBEARER":
		cab, resto, _ := strings.Cut(string(inicial), "\x01")
		if v, ok := strings.CutPrefix(cab, "n,a="); ok {
			usuario = strings.NewReplacer("=2C", ",", "=3D", "=").Replace(strings.TrimSuffix(v, ","))
		}
		for _, p := range strings.Split(resto, "\x01") {
			if v, ok := strings.CutPrefix(p, "auth=Bearer "); ok {
				bien = v == s.Token && s.Token != ""
			}
		}
		bien = bien && usuario == s.Usuario
	}
	return usuario, bien
}

func (k *conexion) orden(orden string, args []string) {
	s := k.s
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	switch orden {
	case "LISTSCRIPTS":
		k.listar()
	case "GETSCRIPT":
		t, ok := s.Script(arg(0))
		if !ok {
			k.no("NONEXISTENT", "no existe")
			return
		}
		k.linea("{" + strconv.Itoa(len(t)) + "}\r\n" + t)
		k.ok("script")
	case "CHECKSCRIPT":
		if s.SinVersion {
			k.no("", "orden desconocida")
			return
		}
		if err := s.validar(arg(0)); err != nil {
			k.no("", err.Error())
			return
		}
		k.ok("bien")
	case "PUTSCRIPT":
		if err := s.validar(arg(1)); err != nil {
			k.no("", err.Error())
			return
		}
		s.Poner(arg(0), arg(1), false)
		k.ok("guardado")
	case "SETACTIVE":
		k.activar(arg(0))
	case "DELETESCRIPT":
		k.borrar(arg(0))
	default:
		k.no("", "orden desconocida")
	}
}

// listar contesta LISTSCRIPTS: los nombres, con ACTIVE detrás del activo.
func (k *conexion) listar() {
	s := k.s
	for _, n := range s.Scripts() {
		nombre := cita(n)
		if s.NombresLiteral {
			nombre = "{" + strconv.Itoa(len(n)) + "}\r\n" + n
		}
		if n == s.Activo() {
			nombre += " ACTIVE"
		}
		k.linea(nombre)
	}
	k.ok("lista")
}

// activar contesta SETACTIVE: deja activo ese script ("" es ninguno), si existe.
func (k *conexion) activar(nombre string) {
	s := k.s
	s.mu.Lock()
	_, ok := s.scripts[nombre]
	if nombre == "" || ok {
		s.activo = nombre
	}
	s.mu.Unlock()
	if !ok && nombre != "" {
		k.no("NONEXISTENT", "no existe")
		return
	}
	k.ok("activo")
}

// borrar contesta DELETESCRIPT: borra el script si existe y no es el activo.
func (k *conexion) borrar(nombre string) {
	s := k.s
	s.mu.Lock()
	_, ok := s.scripts[nombre]
	activo := s.activo == nombre
	if ok && !activo {
		delete(s.scripts, nombre)
	}
	s.mu.Unlock()
	switch {
	case !ok:
		k.no("NONEXISTENT", "no existe")
	case activo:
		k.no("ACTIVE", "está activo")
	default:
		k.ok("borrado")
	}
}

// validar es la comprobación de un script: lo que pide en `require` lo tiene el servidor,
// y un `include` sin `:optional` apunta a un script que existe.
func (s *Servidor) validar(texto string) error {
	ext := s.Extensiones
	if ext == nil {
		ext = ExtensionesDeSerie
	}
	for _, linea := range strings.Split(texto, "\n") {
		linea = strings.TrimSpace(linea)
		if resto, ok := strings.CutPrefix(linea, "require "); ok {
			for _, x := range strings.Split(strings.Trim(resto, "[];"), ",") {
				x = strings.Trim(strings.TrimSpace(x), `"`)
				if x != "" && !slices.Contains(ext, x) {
					return fmt.Errorf("require: extensión desconocida %q", x)
				}
			}
		}
		if resto, ok := strings.CutPrefix(linea, "include "); ok && !strings.Contains(resto, ":optional") {
			i := strings.Index(resto, `"`)
			j := strings.LastIndex(resto, `"`)
			if i >= 0 && j > i {
				if _, existe := s.Script(resto[i+1 : j]); !existe {
					return fmt.Errorf("include: no existe %q", resto[i+1:j])
				}
			}
		}
	}
	return nil
}
