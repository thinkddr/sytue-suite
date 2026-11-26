// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Cuentas de Google y de Microsoft: OAuth de cliente público con PKCE y vuelta a
// 127.0.0.1 (D-18), con el mismo `nube.OAuth` que la nube de Ajustes, y los permisos
// de calendario y contactos.

// Hosts de las API.
const (
	hostGoogleCalendar = "www.googleapis.com"
	hostGooglePeople   = "people.googleapis.com"
	hostGoogleTokens   = "oauth2.googleapis.com" //nolint:gosec // G101: es el host del servicio de tokens, no una credencial.
	hostGraph          = "graph.microsoft.com"
	hostMicrosoftLogin = "login.microsoftonline.com"
)

// OAuthGoogle pide calendario y contactos de Google.
func OAuthGoogle(cliente, secreto string) nube.OAuth {
	o := nube.OAuthGoogle(cliente, secreto)
	o.Alcances = []string{"https://www.googleapis.com/auth/calendar", "https://www.googleapis.com/auth/contacts"}
	return o
}

// OAuthMicrosoft pide calendario y contactos de Microsoft 365 u Outlook.
func OAuthMicrosoft(cliente string) nube.OAuth {
	o := nube.OAuthMicrosoft(cliente)
	o.Alcances = []string{"Calendars.ReadWrite", "Contacts.ReadWrite", "offline_access"}
	return o
}

func (r *Registro) oauthDe(c Cuenta) (nube.OAuth, error) {
	switch c.Proveedor {
	case ProveedorGoogle:
		return OAuthGoogle(c.ClienteID, r.Clientes.GoogleSecreto), nil
	case ProveedorMicrosoft:
		return OAuthMicrosoft(c.ClienteID), nil
	}
	return nube.OAuth{}, fmt.Errorf("agenda: %q no usa OAuth", c.Proveedor)
}

// ConectarOAuth da de alta una cuenta de Google o de Microsoft: abre el navegador del
// sistema con abrir, espera la vuelta y guarda el token de refresco en el llavero.
// cliente vacío usa el de la compilación. Es la otra función de alta de Ajustes.
func (r *Registro) ConectarOAuth(ctx context.Context, proveedor, cliente, rotulo string, abrir func(string) error) (Cuenta, error) {
	c := Cuenta{ID: uuid.NewString(), Proveedor: proveedor, Rotulo: strings.TrimSpace(rotulo), Servicios: []Servicio{Calendario, Contactos}}
	switch proveedor {
	case ProveedorGoogle:
		c.ClienteID = cliente
		if c.ClienteID == "" {
			c.ClienteID = r.Clientes.Google
		}
		c.OAuth = hostGoogleTokens
		c.Servidores = []Servidor{{Servicio: Calendario, Host: hostGoogleCalendar}, {Servicio: Contactos, Host: hostGooglePeople}}
		if c.Rotulo == "" {
			c.Rotulo = "Google"
		}
	case ProveedorMicrosoft:
		c.ClienteID = cliente
		if c.ClienteID == "" {
			c.ClienteID = r.Clientes.Microsoft
		}
		c.OAuth = hostMicrosoftLogin
		c.Servidores = []Servidor{{Servicio: Calendario, Host: hostGraph}, {Servicio: Contactos, Host: hostGraph}}
		if c.Rotulo == "" {
			c.Rotulo = "Microsoft"
		}
	default:
		return c, fmt.Errorf("agenda: %q no usa OAuth", proveedor)
	}
	o, err := r.oauthDe(c)
	if err != nil {
		return c, err
	}
	if r.red != nil && r.HTTP == nil {
		defer r.red.Permitir(red.Destino{Proposito: red.OAuth, Host: c.OAuth, Motivo: "alta"})()
	}
	t, err := o.Conectar(ctx, r.cliente(red.OAuth), abrir)
	if err != nil {
		return c, err
	}
	if t.Refresco == "" {
		return c, errors.New("agenda: el proveedor no dio acceso sin conexión (falta el token de refresco)")
	}
	if err := r.secretos.Guardar(secretoDe(c.ID, "refresco"), t.Refresco); err != nil {
		return c, err
	}
	return c, r.anadir(c)
}

// token es una fuente de tokens de acceso de una cuenta OAuth, que renueva con el de
// refresco y guarda el nuevo si el proveedor lo rota.
func (r *Registro) token(c Cuenta) (func(context.Context) (string, error), error) {
	o, err := r.oauthDe(c)
	if err != nil {
		return nil, err
	}
	refresco, err := r.secretos.Leer(secretoDe(c.ID, "refresco"))
	if err != nil || refresco == "" {
		return nil, ErrSinCuenta
	}
	var (
		mu     sync.Mutex
		acceso string
		vence  time.Time
	)
	return func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if acceso != "" && time.Now().Before(vence) {
			return acceso, nil
		}
		t, err := o.Renovar(ctx, r.cliente(red.OAuth), refresco)
		if err != nil {
			return "", errors.Join(ErrAcceso, err)
		}
		if t.Refresco != "" && t.Refresco != refresco {
			refresco = t.Refresco
			_ = r.secretos.Guardar(secretoDe(c.ID, "refresco"), refresco)
		}
		acceso, vence = t.Acceso, time.Now().Add(time.Duration(max(t.Expira-60, 30))*time.Second)
		return acceso, nil
	}, nil
}

func (r *Registro) remotoOAuth(c Cuenta, s Servicio) (Remoto, error) {
	tok, err := r.token(c)
	if err != nil {
		return nil, err
	}
	return remotoDe(c.Proveedor, s, tok, r.cliente(Proposito(s)))
}

// mismoHost dice si u es del host de base (los enlaces de página de Graph y Google se
// siguen solo dentro de su API: el token no sale hacia otro sitio).
func mismoHost(base, u string) bool {
	b, err1 := url.Parse(base)
	p, err2 := url.Parse(u)
	return err1 == nil && err2 == nil && p.Scheme == "https" && b.Scheme == "https" && strings.EqualFold(b.Host, p.Host)
}
