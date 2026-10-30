// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"net"
	"strings"

	"golang.org/x/net/publicsuffix"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Result es el veredicto de un mecanismo de autenticación (RFC 8601).
type Result string

const (
	ResultNone      Result = "none"
	ResultPass      Result = "pass"
	ResultFail      Result = "fail"
	ResultSoftFail  Result = "softfail"
	ResultNeutral   Result = "neutral"
	ResultTempError Result = "temperror"
	ResultPermError Result = "permerror"
)

// ToDomain mapea el resultado a domain.AuthResult para persistirlo.
func (r Result) ToDomain() domain.AuthResult {
	switch r {
	case ResultPass:
		return domain.AuthPass
	case ResultFail:
		return domain.AuthFail
	case ResultSoftFail:
		return domain.AuthSoftFail
	case ResultNeutral:
		return domain.AuthNeutral
	case ResultTempError:
		return domain.AuthTempError
	case ResultPermError:
		return domain.AuthPermError
	default:
		return domain.AuthNone
	}
}

// Resolver abstrae las consultas DNS (inyectable para test). En prod usa
// net.DefaultResolver (resolvers UE de Scaleway).
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
}

// VerifySPF evalúa SPF (RFC 7208) para la IP de conexión y el dominio de
// MAIL FROM. Implementación clean-room sobre el estándar: soporta los mecanismos
// más comunes (ip4, ip6, a, mx, include, all) con límite de 10 lookups DNS.
//
// Nota: para producción se recomienda envolver o sustituir por una librería
// permisiva dedicada y auditada (p. ej. blitiri.com.ar/go/spf, Apache-2.0); este
// evaluador cubre el camino feliz y los casos de hardening básicos.
func VerifySPF(ctx context.Context, res Resolver, ip net.IP, mailFromDomain string) Result {
	if mailFromDomain == "" || ip == nil {
		return ResultNone
	}
	ev := &spfEval{res: res, ip: ip, budget: 10}
	return ev.check(ctx, mailFromDomain)
}

type spfEval struct {
	res    Resolver
	ip     net.IP
	budget int
}

func (e *spfEval) check(ctx context.Context, domainName string) Result {
	if e.budget <= 0 {
		return ResultPermError
	}
	e.budget--
	record, ok := e.lookupSPFRecord(ctx, domainName)
	if !ok {
		return ResultNone
	}
	for _, term := range strings.Fields(record)[1:] { // omitir "v=spf1"
		qualifier := byte('+')
		if term != "" && (term[0] == '+' || term[0] == '-' || term[0] == '~' || term[0] == '?') {
			qualifier = term[0]
			term = term[1:]
		}
		if e.matches(ctx, term, domainName) {
			switch qualifier {
			case '-':
				return ResultFail
			case '~':
				return ResultSoftFail
			case '?':
				return ResultNeutral
			default:
				return ResultPass
			}
		}
	}
	return ResultNeutral
}

func (e *spfEval) lookupSPFRecord(ctx context.Context, domainName string) (string, bool) {
	txts, err := e.res.LookupTXT(ctx, domainName)
	if err != nil {
		return "", false
	}
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(t), "v=spf1") {
			return t, true
		}
	}
	return "", false
}

//nolint:gocognit // evaluador de mecanismos SPF (RFC 7208 §5): un bloque por mecanismo.
func (e *spfEval) matches(ctx context.Context, term, domainName string) bool {
	lower := strings.ToLower(term)
	switch {
	case lower == "all":
		return true
	case strings.HasPrefix(lower, "ip4:"), strings.HasPrefix(lower, "ip6:"):
		_, cidr, err := net.ParseCIDR(term[4:])
		if err != nil {
			if parsed := net.ParseIP(term[4:]); parsed != nil {
				return parsed.Equal(e.ip)
			}
			return false
		}
		return cidr.Contains(e.ip)
	case lower == "a" || strings.HasPrefix(lower, "a:"):
		host := domainName
		if i := strings.IndexByte(term, ':'); i >= 0 {
			host = term[i+1:]
		}
		e.budget--
		addrs, err := e.res.LookupIPAddr(ctx, host)
		if err != nil {
			return false
		}
		for _, a := range addrs {
			if a.IP.Equal(e.ip) {
				return true
			}
		}
	case lower == "mx" || strings.HasPrefix(lower, "mx:"):
		host := domainName
		if i := strings.IndexByte(term, ':'); i >= 0 {
			host = term[i+1:]
		}
		e.budget--
		mxs, err := e.res.LookupMX(ctx, host)
		if err != nil {
			return false
		}
		for _, mx := range mxs {
			addrs, _ := e.res.LookupIPAddr(ctx, mx.Host)
			for _, a := range addrs {
				if a.IP.Equal(e.ip) {
					return true
				}
			}
		}
	case strings.HasPrefix(lower, "include:"):
		if e.check(ctx, term[len("include:"):]) == ResultPass {
			return true
		}
	}
	return false
}

// DMARCResult combina SPF y DKIM con la política publicada del dominio (RFC
// 7489). Devuelve el veredicto y la disposición (none|quarantine|reject).
type DMARCResult struct {
	Result      Result
	Disposition string // none | quarantine | reject
	// Alineado: el `From:` está AUTENTICADO, haya o no política publicada: SPF en
	// pass con su dominio alineado, o una firma DKIM válida de un dominio alineado.
	// Es lo que debe mirar quien vaya a fiarse del remitente (el REPLY de una
	// invitación, p. ej.); «DKIM pass» a secas solo dice que alguien firmó.
	Alineado bool
}

// politicaDMARC es lo que se lee del registro `_dmarc`: la disposición que pide
// el dominio y lo estricta que quiere la alineación de cada mecanismo.
type politicaDMARC struct {
	disposicion  string // none | quarantine | reject
	dkimEstricto bool   // adkim=s
	spfEstricto  bool   // aspf=s
}

// VerifyDMARC evalúa el alineamiento DMARC. fromDomain es el dominio de la
// cabecera From (RFC 5322); spfDomain el de MAIL FROM; dkimDomains los `d=` de
// las firmas DKIM VÁLIDAS (los que devuelve VerifyDKIM, nunca el del From).
//
// La alineación es relajada por dominio organizativo salvo que la política pida
// estricta (`adkim=s` / `aspf=s`). Si el dominio del From no publica registro se
// consulta el de su dominio organizativo (RFC 7489 §6.6.3), que es donde un
// atacante iría a buscar el hueco: `algo.victima.example` casi nunca tiene el suyo.
func VerifyDMARC(ctx context.Context, res Resolver, fromDomain, spfDomain string, spf Result, dkimDomains []string, dkim Result) DMARCResult {
	policy, ok := lookupDMARCPolicy(ctx, res, fromDomain)
	if !ok {
		return DMARCResult{
			Result: ResultNone, Disposition: "none",
			Alineado: Alineado(fromDomain, spfDomain, spf, dkimDomains, dkim, false, false),
		}
	}
	if Alineado(fromDomain, spfDomain, spf, dkimDomains, dkim, policy.spfEstricto, policy.dkimEstricto) {
		return DMARCResult{Result: ResultPass, Disposition: "none", Alineado: true}
	}
	return DMARCResult{Result: ResultFail, Disposition: policy.disposicion}
}

// VerifyDMARCFroms evalúa DMARC sobre todas las direcciones/dominios From: del
// mensaje (RFC 5322 §3.6.2 y RFC 7489 §6.6.1, COM-05). Un mensaje válido lleva una
// sola cabecera From: con una sola dirección; si trae más de una (por ejemplo, la
// primera del dominio del atacante para pasar SPF/DKIM y la segunda suplantando a otro
// remitente, o una segunda cabecera añadida sin firmar sobre un DKIM legítimo), nunca
// queda alineado y falla DMARC con la disposición más estricta de los dominios afectados.
func VerifyDMARCFroms(ctx context.Context, res Resolver, fromDomains []string, spfDomain string, spf Result, dkimDomains []string, dkim Result) DMARCResult {
	if len(fromDomains) == 1 {
		return VerifyDMARC(ctx, res, fromDomains[0], spfDomain, spf, dkimDomains, dkim)
	}
	disp := "none"
	for _, d := range fromDomains {
		if p, ok := lookupDMARCPolicy(ctx, res, d); ok {
			disp = disposicionMasEstricta(disp, p.disposicion)
		}
	}
	return DMARCResult{Result: ResultFail, Disposition: disp, Alineado: false}
}

func disposicionMasEstricta(a, b string) string {
	peso := func(s string) int {
		switch s {
		case "reject":
			return 2
		case "quarantine":
			return 1
		default:
			return 0
		}
	}
	if peso(b) > peso(a) {
		return b
	}
	return a
}

// Alineado dice si el dominio del `From:` queda autenticado por SPF o por DKIM:
// el mecanismo tiene que haber pasado Y su dominio casar con el del From.
func Alineado(fromDomain, spfDomain string, spf Result, dkimDomains []string, dkim Result, spfEstricto, dkimEstricto bool) bool {
	if fromDomain == "" {
		return false
	}
	if spf == ResultPass && aligned(fromDomain, spfDomain, spfEstricto) {
		return true
	}
	if dkim != ResultPass {
		return false
	}
	for _, d := range dkimDomains {
		if aligned(fromDomain, d, dkimEstricto) {
			return true
		}
	}
	return false
}

// lookupDMARCPolicy lee la política del dominio del From y, si no publica
// ninguna, la de su dominio organizativo, donde manda `sp=` (o `p=` si no hay).
func lookupDMARCPolicy(ctx context.Context, res Resolver, fromDomain string) (politicaDMARC, bool) {
	fromDomain = normalizarDominio(fromDomain)
	if p, ok := leerDMARC(ctx, res, fromDomain, false); ok {
		return p, true
	}
	if org := dominioOrganizativo(fromDomain); org != fromDomain {
		return leerDMARC(ctx, res, org, true)
	}
	return politicaDMARC{}, false
}

func leerDMARC(ctx context.Context, res Resolver, dominio string, deSubdominio bool) (politicaDMARC, bool) {
	txts, err := res.LookupTXT(ctx, "_dmarc."+dominio)
	if err != nil {
		return politicaDMARC{}, false
	}
	for _, t := range txts {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=dmarc1") {
			continue
		}
		p := politicaDMARC{disposicion: "none"}
		var sp string
		for _, tag := range strings.Split(t, ";") {
			k, v, _ := strings.Cut(strings.TrimSpace(tag), "=")
			v = strings.ToLower(strings.TrimSpace(v))
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "p":
				p.disposicion = disposicionValida(v)
			case "sp":
				sp = v
			case "adkim":
				p.dkimEstricto = v == "s"
			case "aspf":
				p.spfEstricto = v == "s"
			}
		}
		if deSubdominio && sp != "" {
			p.disposicion = disposicionValida(sp)
		}
		return p, true
	}
	return politicaDMARC{}, false
}

// disposicionValida deja pasar solo los tres valores del RFC; cualquier otra cosa
// es «none» (un registro con una errata no puede acabar rechazando correo).
func disposicionValida(v string) string {
	switch v {
	case "quarantine", "reject":
		return v
	}
	return "none"
}

// aligned dice si `other` (el dominio de SPF o el `d=` de DKIM) casa con el del
// From. Estricta: iguales. Relajada: mismo dominio ORGANIZATIVO, que no es «uno
// termina en el otro»: `co.uk` no es de nadie y `a.github.io` no responde de
// `b.github.io`. Lo decide la lista pública de sufijos.
func aligned(fromDomain, other string, estricta bool) bool {
	fromDomain, other = normalizarDominio(fromDomain), normalizarDominio(other)
	if fromDomain == "" || other == "" {
		return false
	}
	if fromDomain == other {
		return true
	}
	if estricta {
		return false
	}
	return dominioOrganizativo(fromDomain) == dominioOrganizativo(other)
}

// dominioOrganizativo es el sufijo público más una etiqueta (RFC 7489 §3.2). Si
// el dominio no tiene uno —es él mismo un sufijo público, o una sola etiqueta—
// se devuelve tal cual: así solo alinea consigo mismo.
func dominioOrganizativo(d string) string {
	org, err := publicsuffix.EffectiveTLDPlusOne(d)
	if err != nil {
		return d
	}
	return org
}

func normalizarDominio(d string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
}
