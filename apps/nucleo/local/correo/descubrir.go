// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ---- Autodescubrimiento de los servidores de una dirección ----
//
// Cuando el dominio no es de un proveedor conocido (Preajustes), se buscan sus servidores
// como lo hacen Thunderbird y los clientes de correo:
//
//  1. el fichero de configuración que publica el propio dominio (autoconfig de Mozilla):
//     `https://autoconfig.<dominio>/mail/config-v1.1.xml` y
//     `https://<dominio>/.well-known/autoconfig/mail/config-v1.1.xml`;
//  2. los registros SRV del DNS (RFC 6186 y RFC 8314): `_imaps._tcp`, `_imap._tcp`,
//     `_submissions._tcp` y `_submission._tcp`.
//
// Solo se hace cuando la persona lo pide («Buscar los servidores»), por el cliente que
// le pasa la cáscara (el permiso puntual del portero) y solo por https, y el DNS por el
// resolvedor del portero (`red.Portero.Resolvedor`: con «Sin conexión» no se pregunta).
// Lo que llega es un dato no confiable: solo se aceptan nombres de host y puertos
// válidos, y la persona ve el resultado en el formulario antes de conectar.

// ErrSinDescubrir: el dominio no publica sus servidores.
var ErrSinDescubrir = errors.New("correo: el dominio no dice cuáles son sus servidores")

// Descubrimiento es lo que se ha encontrado de una dirección.
type Descubrimiento struct {
	IMAP, SMTP Servidor
	// Origen dice de dónde salió: "autoconfig" o "srv".
	Origen string
}

// Descubridor busca los servidores de una dirección. HTTP es el cliente del fichero de
// configuración (nil: no se pide); Resolver, el del DNS (nil: no se miran los SRV). Ni
// uno ni otro salen a la red por su cuenta: los da el portero.
type Descubridor struct {
	HTTP     *http.Client
	Resolver interface {
		LookupSRV(ctx context.Context, servicio, proto, nombre string) (string, []*net.SRV, error)
	}
}

// Descubrir busca los servidores IMAP y SMTP de direccion.
func (d Descubridor) Descubrir(ctx context.Context, direccion string) (Descubrimiento, error) {
	_, dominio, ok := strings.Cut(strings.TrimSpace(direccion), "@")
	dominio = strings.ToLower(strings.TrimSuffix(dominio, "."))
	if !ok || !hostValido(dominio) {
		return Descubrimiento{}, fmt.Errorf("correo: %q no es una dirección", direccion)
	}
	ctx, parar := context.WithTimeout(ctx, 20*time.Second)
	defer parar()
	if d.HTTP != nil {
		for _, u := range []string{
			"https://autoconfig." + dominio + "/mail/config-v1.1.xml?emailaddress=" + url.QueryEscape(direccion),
			"https://" + dominio + "/.well-known/autoconfig/mail/config-v1.1.xml",
		} {
			if r, err := d.autoconfig(ctx, u); err == nil {
				return r, nil
			}
		}
	}
	return d.srv(ctx, dominio)
}

// configMozilla es lo que se lee de `config-v1.1.xml`.
type configMozilla struct {
	Proveedor struct {
		Entrantes []servidorMozilla `xml:"incomingServer"`
		Salientes []servidorMozilla `xml:"outgoingServer"`
	} `xml:"emailProvider"`
}

type servidorMozilla struct {
	Tipo      string `xml:"type,attr"`
	Host      string `xml:"hostname"`
	Puerto    int    `xml:"port"`
	Seguridad string `xml:"socketType"`
}

func (s servidorMozilla) servidor() (Servidor, bool) {
	h := strings.ToLower(strings.TrimSpace(s.Host))
	if !hostValido(h) || s.Puerto <= 0 || s.Puerto > 65535 || strings.Contains(h, "%") {
		return Servidor{}, false
	}
	var seg string
	switch strings.ToUpper(strings.TrimSpace(s.Seguridad)) {
	case "SSL", "TLS":
		seg = SeguridadTLS
	case "STARTTLS":
		seg = SeguridadSTARTTLS
	default:
		return Servidor{}, false // sin cifrar no se propone nunca
	}
	return Servidor{Host: h, Puerto: s.Puerto, Seguridad: seg}, true
}

func (d Descubridor) autoconfig(ctx context.Context, direccion string) (Descubrimiento, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, direccion, http.NoBody)
	if err != nil {
		return Descubrimiento{}, err
	}
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return Descubrimiento{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Descubrimiento{}, ErrSinDescubrir
	}
	var cfg configMozilla
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&cfg); err != nil {
		return Descubrimiento{}, err
	}
	var r Descubrimiento
	for _, s := range cfg.Proveedor.Entrantes {
		if v, ok := s.servidor(); ok && strings.EqualFold(s.Tipo, "imap") && r.IMAP.Host == "" {
			r.IMAP = v
		}
	}
	for _, s := range cfg.Proveedor.Salientes {
		if v, ok := s.servidor(); ok && strings.EqualFold(s.Tipo, "smtp") && r.SMTP.Host == "" {
			r.SMTP = v
		}
	}
	if r.IMAP.Host == "" || r.SMTP.Host == "" {
		return Descubrimiento{}, ErrSinDescubrir
	}
	r.Origen = "autoconfig"
	return r, nil
}

func (d Descubridor) srv(ctx context.Context, dominio string) (Descubrimiento, error) {
	res := d.Resolver
	if res == nil {
		return Descubrimiento{}, ErrSinDescubrir
	}
	buscar := func(servicio, seguridad string) (Servidor, bool) {
		_, ss, err := res.LookupSRV(ctx, servicio, "tcp", dominio)
		if err != nil {
			return Servidor{}, false
		}
		for _, s := range ss { // ya vienen por prioridad y peso
			h := strings.ToLower(strings.TrimSuffix(s.Target, "."))
			if h == "" || !hostValido(h) || s.Port == 0 {
				continue // «.» es «no hay servicio» (RFC 2782)
			}
			return Servidor{Host: h, Puerto: int(s.Port), Seguridad: seguridad}, true
		}
		return Servidor{}, false
	}
	var r Descubrimiento
	var ok bool
	if r.IMAP, ok = buscar("imaps", SeguridadTLS); !ok {
		r.IMAP, ok = buscar("imap", SeguridadSTARTTLS)
	}
	if !ok {
		return Descubrimiento{}, ErrSinDescubrir
	}
	if r.SMTP, ok = buscar("submissions", SeguridadTLS); !ok {
		if r.SMTP, ok = buscar("submission", SeguridadSTARTTLS); !ok {
			return Descubrimiento{}, ErrSinDescubrir
		}
	}
	r.Origen = "srv"
	return r, nil
}

// hostValido: un nombre DNS de letras, cifras, guiones y puntos, con algún punto.
func hostValido(h string) bool {
	if h == "" || len(h) > 253 || !strings.Contains(h, ".") || strings.HasPrefix(h, ".") || strings.HasPrefix(h, "-") {
		return false
	}
	for _, trozo := range strings.Split(h, ".") {
		if trozo == "" || len(trozo) > 63 {
			return false
		}
		for _, r := range trozo {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	_, err := strconv.Atoi(strings.ReplaceAll(h, ".", "")) // una IP no es un nombre
	return err != nil
}
