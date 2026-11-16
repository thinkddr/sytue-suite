// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"crypto/subtle"
	"net"
	"strings"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// checkDomainDNS comprueba el DNS de un dominio propio y devuelve el MOTIVO, no
// solo un sí o un no.
//
// La PROPIEDAD se prueba con el TXT `_sytue-challenge.<host>` (ver
// KubePlatform.VerifyDomain para el porqué de no aceptar el CNAME como prueba) y
// es lo único que decide `Verified`. El enrutado se mira DESPUÉS y solo para
// avisar: un dominio con el TXT bien pero el CNAME a otro sitio se verifica igual,
// pero se le dice al cliente que el certificado no llegará hasta que apunte.
//
// Los errores de resolución nunca son fallos duros: NXDOMAIN o un DNS sin
// propagar son la situación normal de un dominio recién añadido y se traducen en
// el motivo correspondiente.
func checkDomainDNS(ctx context.Context, r dnsResolver, hostname, token, ingressTarget string) domain.DomainCheck {
	host := normHost(hostname)
	token = strings.TrimSpace(token)
	if token == "" {
		// Sin token no hay reto que satisfacer: fail-closed (nunca se verifica solo).
		return domain.DomainCheck{Reason: domain.DomainCheckTXTMissing}
	}
	records, err := r.LookupTXT(ctx, challengeLabel+host)
	if err != nil || len(records) == 0 {
		return domain.DomainCheck{Reason: domain.DomainCheckTXTMissing}
	}
	found := false
	for _, rec := range records {
		// Algunos resolvers/proveedores devuelven el valor entrecomillado.
		if subtle.ConstantTimeCompare(
			[]byte(strings.Trim(strings.TrimSpace(rec), `"`)), []byte(token),
		) == 1 {
			found = true
			break
		}
	}
	if !found {
		return domain.DomainCheck{Reason: domain.DomainCheckTXTMismatch}
	}

	check := domain.DomainCheck{Verified: true, Reason: domain.DomainCheckOK}
	target := normHost(ingressTarget)
	if target == "" {
		return check // sin ingress configurado (dev) no hay enrutado que comparar
	}
	if pointsTo, where := routesTo(ctx, r, host, target); !pointsTo {
		check.Reason = domain.DomainCheckCNAMEMismatch
		check.Expected = target
		check.Found = where
	}
	return check
}

// routesTo dice si host llega a target: por CNAME directo o, para el ápex (que no
// admite CNAME) y los proveedores que aplanan, porque comparten alguna IP. Si no,
// devuelve a dónde apunta hoy (destino del CNAME o primera IP; vacío si no
// resuelve) para que el aviso diga «apunta a X», no solo «está mal».
func routesTo(ctx context.Context, r dnsResolver, host, target string) (bool, string) {
	cname, _ := r.LookupCNAME(ctx, host)
	cname = normHost(cname)
	if cname == target {
		return true, ""
	}
	hostIPs, _ := r.LookupHost(ctx, host)
	if len(hostIPs) > 0 {
		targetIPs, _ := r.LookupHost(ctx, target)
		for _, a := range hostIPs {
			for _, b := range targetIPs {
				if a == b {
					return true, ""
				}
			}
		}
	}
	if cname != "" && cname != host {
		return false, cname
	}
	if len(hostIPs) > 0 {
		return false, hostIPs[0]
	}
	return false, ""
}

func normHost(h string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), "."))
}

// DNSDomainVerifier comprueba dominios propios solo con DNS, sin clúster.
//
// Lo usa cloud-api para el «Comprobar ahora» del panel: la API no tiene (ni debe
// tener) cliente de Kubernetes, y comprobar un dominio no necesita más que el
// resolvedor. Es la MISMA comprobación que hace KubePlatform.VerifyDomain en el
// bucle del controller, con el mismo resolvedor del sistema; no abre ninguna red
// nueva (solo consultas DNS, sin conectar a los hosts del cliente).
type DNSDomainVerifier struct {
	resolver dnsResolver
}

// NewDNSDomainVerifier construye el verificador con el resolvedor del sistema.
func NewDNSDomainVerifier() *DNSDomainVerifier {
	return &DNSDomainVerifier{resolver: net.DefaultResolver}
}

// VerifyDomain implementa service.DomainVerifier.
func (v *DNSDomainVerifier) VerifyDomain(ctx context.Context, hostname, token, ingressTarget string) (domain.DomainCheck, error) {
	return checkDomainDNS(ctx, v.resolver, hostname, token, ingressTarget), nil
}
