// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dav

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Autodescubrimiento estándar (RFC 6764, S-04): con el correo de la persona se busca
// el servidor de su dominio. Primero los registros SRV (`_caldavs._tcp`, `_carddavs._tcp`)
// y su TXT `path=`; si no los hay, `https://<dominio>/.well-known/caldav` (o `carddav`),
// siguiendo la redirección que dé el servidor. Es red hacia el dominio de la cuenta, que
// la persona pide al darla de alta.

// Resolutor hace las consultas DNS del descubrimiento. `net.Resolver` lo cumple; las
// pruebas ponen uno de mentira.
type Resolutor interface {
	LookupSRV(ctx context.Context, servicio, proto, nombre string) (string, []*net.SRV, error)
	LookupTXT(ctx context.Context, nombre string) ([]string, error)
}

// Descubierto es lo que se encontró para un dominio.
type Descubierto struct {
	// Calendario y Contactos son las URL de contexto (vacía la que no hay).
	Calendario, Contactos string
}

// ErrSinDescubrir: el dominio no anuncia CalDAV ni CardDAV.
var ErrSinDescubrir = errors.New("dav: el dominio no anuncia un servidor de calendario ni de contactos")

// Descubrir busca los servidores CalDAV y CardDAV del dominio de correo. hc es la red
// (la petición a `.well-known` no lleva credenciales); res, el DNS (nil, sin SRV).
func Descubrir(ctx context.Context, correo string, res Resolutor, hc *http.Client) (Descubierto, error) {
	dominio := correo
	if i := strings.LastIndex(correo, "@"); i >= 0 {
		dominio = correo[i+1:]
	}
	dominio = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(dominio), "."))
	if dominio == "" || strings.ContainsAny(dominio, "/?#@ []") || strings.Count(dominio, ":") > 1 {
		return Descubierto{}, fmt.Errorf("dav: dominio no válido %q", dominio)
	}
	var d Descubierto
	d.Calendario = descubrirUno(ctx, dominio, "caldav", res, hc)
	d.Contactos = descubrirUno(ctx, dominio, "carddav", res, hc)
	if d.Calendario == "" && d.Contactos == "" {
		return d, ErrSinDescubrir
	}
	return d, nil
}

func descubrirUno(ctx context.Context, dominio, servicio string, res Resolutor, hc *http.Client) string {
	if u := descubrirPorSRV(ctx, dominio, servicio, res, hc); u != "" {
		return u
	}
	return contexto(ctx, "https://"+dominio+"/.well-known/"+servicio, hc)
}

// descubrirPorSRV prueba, en orden, los servidores que anuncia el SRV del servicio (con
// la ruta que diga su TXT). "" si no hay DNS, no anuncia ninguno o no contesta ninguno.
func descubrirPorSRV(ctx context.Context, dominio, servicio string, res Resolutor, hc *http.Client) string {
	if res == nil {
		return ""
	}
	_, srvs, err := res.LookupSRV(ctx, servicio+"s", "tcp", dominio)
	if err != nil {
		return ""
	}
	for _, s := range srvs {
		host := strings.TrimSuffix(s.Target, ".")
		if host == "" || host == "." {
			continue // «.» es que el servicio no existe (RFC 2782)
		}
		ruta := rutaTXT(ctx, dominio, servicio, res)
		base := "https://" + host
		if s.Port != 443 && s.Port != 0 {
			base = "https://" + net.JoinHostPort(host, strconv.Itoa(int(s.Port)))
		}
		if u := contexto(ctx, base+ruta, hc); u != "" {
			return u
		}
	}
	return ""
}

// rutaTXT es la ruta del contexto que da el TXT del servicio (`path=/…`, RFC 6764 §4) o,
// si no da ninguna, la de `.well-known`.
func rutaTXT(ctx context.Context, dominio, servicio string, res Resolutor) string {
	ruta := "/.well-known/" + servicio
	txts, err := res.LookupTXT(ctx, "_"+servicio+"s._tcp."+dominio)
	if err != nil {
		return ruta
	}
	for _, t := range txts {
		if v, ok := strings.CutPrefix(strings.TrimSpace(t), "path="); ok && strings.HasPrefix(v, "/") {
			ruta = v
		}
	}
	return ruta
}

// contexto sigue la redirección de `.well-known` a mano: un PROPFIND que el cliente de
// Go siguiera solo se convertiría en GET. Devuelve la URL que contesta 207 a la
// pregunta por el principal, o "".
func contexto(ctx context.Context, direccion string, hc *http.Client) string {
	if hc == nil {
		return ""
	}
	sin := *hc
	sin.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	u, err := url.Parse(direccion)
	if err != nil {
		return ""
	}
	for range 5 {
		req, err := http.NewRequestWithContext(ctx, "PROPFIND", u.String(), strings.NewReader(`<d:propfind xmlns:d="DAV:"><d:prop><d:current-user-principal/></d:prop></d:propfind>`))
		if err != nil {
			return ""
		}
		req.Header.Set("Depth", "0")
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
		resp, err := sin.Do(req)
		if err != nil {
			return ""
		}
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusMultiStatus, resp.StatusCode == http.StatusUnauthorized:
			// 401: hay servidor y pide la contraseña; la URL vale.
			return u.String()
		case resp.StatusCode >= 300 && resp.StatusCode < 400:
			loc, err := url.Parse(resp.Header.Get("Location"))
			if err != nil || resp.Header.Get("Location") == "" {
				return ""
			}
			sig := u.ResolveReference(loc)
			if sig.Scheme != "https" && (sig.Scheme != "http" || u.Scheme != "http") {
				return ""
			}
			u = sig
		default:
			return ""
		}
	}
	return ""
}

// Preajuste es un proveedor conocido: sus direcciones no hace falta descubrirlas.
type Preajuste struct {
	Proveedor             string
	Rotulo                string
	Calendario, Contactos string
	// Hosts con un punto delante valen para los subdominios del proveedor (los
	// servidores `pNN-` de iCloud).
	Hosts []string
	// Ayuda dice de dónde sale la contraseña de aplicación.
	Ayuda string
}

// Preajustes de los proveedores con contraseña de aplicación.
var Preajustes = []Preajuste{
	{
		Proveedor: "icloud", Rotulo: "iCloud", Calendario: "https://caldav.icloud.com/", Contactos: "https://contacts.icloud.com/",
		Hosts: []string{"caldav.icloud.com", "contacts.icloud.com", ".icloud.com"}, Ayuda: "appleid.apple.com",
	},
	{
		Proveedor: "fastmail", Rotulo: "Fastmail", Calendario: "https://caldav.fastmail.com/dav/", Contactos: "https://carddav.fastmail.com/dav/",
		Hosts: []string{"caldav.fastmail.com", "carddav.fastmail.com"}, Ayuda: "app.fastmail.com",
	},
	{
		Proveedor: "sytue", Rotulo: "Sytue", Calendario: "https://calendar.sytue.eu/dav/", Contactos: "https://contacts.sytue.eu/dav/",
		Hosts: []string{"calendar.sytue.eu", "contacts.sytue.eu"}, Ayuda: "id.sytue.eu",
	},
	{
		Proveedor: "yahoo", Rotulo: "Yahoo", Calendario: "https://caldav.calendar.yahoo.com/", Contactos: "https://carddav.address.yahoo.com/",
		Hosts: []string{"caldav.calendar.yahoo.com", "carddav.address.yahoo.com"}, Ayuda: "login.yahoo.com",
	},
}

// PreajusteDe devuelve el preajuste de un proveedor.
func PreajusteDe(proveedor string) (Preajuste, bool) {
	for _, p := range Preajustes {
		if p.Proveedor == proveedor {
			return p, true
		}
	}
	return Preajuste{}, false
}
