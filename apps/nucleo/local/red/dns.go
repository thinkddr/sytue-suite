// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package red

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// # Consultas al DNS
//
// El alta de una cuenta busca sus servidores en los registros SRV y TXT del dominio que
// escribe la persona (RFC 6186 para el correo, RFC 6764 para CalDAV y CardDAV). Una
// consulta DNS también sale del equipo (al servidor de nombres del sistema), así que pasa
// por el portero como lo demás: [Portero.Resolvedor] es el resolvedor de ese alta, que
// con «Sin conexión» no pregunta nada, que la prueba de arranque ve ([VigilarDePrueba]:
// red «dns» y el nombre que se consulta) y que, mientras dura cada consulta, Ajustes →
// Red enseña como temporal con su motivo. Quien lo usa consulta SOLO tras el gesto de la
// persona («Buscar los servidores», añadir la cuenta), como con [Portero.Puntual]. Lo que
// contesta el DNS es un dato no confiable: quien lo lee comprueba los nombres y los
// puertos, y la conexión con ellos va por el portero.

// consultasDNS son las consultas de un [Resolvedor]: las cumple `net.Resolver`.
type consultasDNS interface {
	LookupSRV(ctx context.Context, servicio, proto, nombre string) (string, []*net.SRV, error)
	LookupTXT(ctx context.Context, nombre string) ([]string, error)
}

// Resolvedor es el DNS del alta de una cuenta, por el portero ([Portero.Resolvedor]).
// Cumple lo que piden `correo.Descubridor.Resolver` y `dav.Resolutor`.
type Resolvedor struct {
	p      *Portero
	pr     Proposito
	motivo string
}

// Resolvedor es el DNS para pr (el alta de una cuenta: [Descubrimiento]); motivo es lo
// que enseña Ajustes → Red mientras dura cada consulta (el dominio que se busca).
func (p *Portero) Resolvedor(pr Proposito, motivo string) *Resolvedor {
	return &Resolvedor{p: p, pr: pr, motivo: motivo}
}

// LookupSRV consulta los registros SRV de `_<servicio>._<proto>.<nombre>`.
func (r *Resolvedor) LookupSRV(ctx context.Context, servicio, proto, nombre string) (string, []*net.SRV, error) {
	dns, cerrar, err := r.abrir("_"+servicio+"._"+proto+"."+nombre, nombre)
	if err != nil {
		return "", nil, err
	}
	defer cerrar()
	return dns.LookupSRV(ctx, servicio, proto, nombre)
}

// LookupTXT consulta los registros TXT de nombre.
func (r *Resolvedor) LookupTXT(ctx context.Context, nombre string) ([]string, error) {
	dns, cerrar, err := r.abrir(nombre, nombre)
	if err != nil {
		return nil, err
	}
	defer cerrar()
	return dns.LookupTXT(ctx, nombre)
}

// abrir deja pasar la consulta de consulta, del nombre host, si se puede: no con «Sin
// conexión» ni con algo que no es un nombre de dominio. Mientras dura, Ajustes → Red la
// enseña (cerrar la quita).
func (r *Resolvedor) abrir(consulta, host string) (dns consultasDNS, cerrar func(), err error) {
	avisar(r.pr, "dns", consulta)
	host = normalizar(Destino{Host: host}).Host
	if host == "" || !strings.Contains(host, ".") || strings.ContainsAny(host, "/:@ []%") || EsLocal(host) || net.ParseIP(host) != nil {
		return nil, nil, fmt.Errorf("%w: %q no es un dominio que consultar", ErrDenegado, host)
	}
	if r.p.SinConexion() {
		return nil, nil, fmt.Errorf("%w: «Sin conexión» está encendido (%s)", ErrDenegado, host)
	}
	r.p.mu.Lock()
	dns = r.p.dns
	r.p.mu.Unlock()
	if dns == nil {
		return nil, nil, fmt.Errorf("%w: no hay a quién preguntar por %s", ErrDenegado, host)
	}
	// Con el puerto del DNS: lo que se enseña no abre a ese dominio nada más (ni https).
	return dns, r.p.Permitir(Destino{Proposito: r.pr, Host: host, Puerto: 53, Motivo: r.motivo}), nil
}
