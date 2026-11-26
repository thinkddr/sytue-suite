// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"net/url"
	"strings"
)

// Lo que la nube le dice al portero de red (`local/red`, §5.8): a qué servidores va cada
// cuenta conectada. Sin cuentas, a ninguno.

// Hosts son los servidores a los que habla una cuenta: su WebDAV, o la API del
// proveedor y su servidor de tokens (renovar el acceso). Un nombre con un punto delante
// vale para sus subdominios: los servidores de descarga a los que redirige OneDrive.
func Hosts(c Cuenta) []string {
	switch c.Tipo {
	case SytueDrive, WebDAV:
		if u, err := url.Parse(c.URL); err == nil && u.Hostname() != "" && !strings.HasPrefix(u.Hostname(), ".") {
			return []string{u.Hostname()}
		}
		return nil
	case Google:
		return []string{"www.googleapis.com", hostDe(OAuthGoogle("", "").Token)}
	case OneDrive:
		return []string{hostDe(apiGraph), hostDe(OAuthMicrosoft("").Token), ".sharepoint.com", ".1drv.com"}
	}
	return nil
}

// HostsDeAlta son los servidores del intercambio OAuth de un alta (el código por el
// token), mientras dura: el navegador del sistema hace el resto.
func HostsDeAlta(tipo string) []string {
	switch tipo {
	case Google:
		return []string{hostDe(OAuthGoogle("", "").Token)}
	case OneDrive:
		return []string{hostDe(OAuthMicrosoft("").Token)}
	}
	return nil
}

func hostDe(direccion string) string {
	u, err := url.Parse(direccion)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
