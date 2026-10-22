// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"net/netip"
	"sort"
	"time"
)

// AccessAction es la decisión de una política de Acceso Condicional (paridad Entra
// Conditional Access). Se evalúa en el login, tras verificar la credencial y antes
// de emitir la sesión.
type AccessAction string

const (
	// AccessAllow permite el login sin exigencias adicionales (y es el DEFAULT seguro
	// cuando ninguna política aplica: sin políticas ⇒ nadie se queda fuera).
	AccessAllow AccessAction = "allow"
	// AccessRequireMFA fuerza el segundo factor aunque la sesión no lo pidiera.
	AccessRequireMFA AccessAction = "require_mfa"
	// AccessBlock deniega el login.
	AccessBlock AccessAction = "block"
)

// ValidAccessAction indica si s es una acción canónica.
func ValidAccessAction(s string) bool {
	switch AccessAction(s) {
	case AccessAllow, AccessRequireMFA, AccessBlock:
		return true
	default:
		return false
	}
}

// severity ordena las acciones para "gana la más restrictiva": block > require_mfa >
// allow. Una acción desconocida cae a 0 (menos que allow), nunca sobrepuja.
func (a AccessAction) severity() int {
	switch a {
	case AccessBlock:
		return 3
	case AccessRequireMFA:
		return 2
	case AccessAllow:
		return 1
	default:
		return 0
	}
}

// AccessPolicy es una política de Acceso Condicional de una organización. Sus
// condiciones se combinan en AND: para que la política aplique, TODAS las que estén
// fijadas deben cumplirse (las vacías son comodín). Las exclusiones eximen al usuario
// aunque cumpla el resto (break-glass de administradores).
type AccessPolicy struct {
	ID       string
	OrgID    string
	Name     string
	Enabled  bool
	Priority int
	Action   AccessAction

	// A quién aplica. IncludeUsers/IncludeGroups vacíos AMBOS ⇒ todos los usuarios.
	IncludeUsers  []string
	IncludeGroups []string
	// Exentos (break-glass): el usuario en cualquiera de estas listas no sufre la política.
	ExcludeUsers  []string
	ExcludeGroups []string

	// Condición de red (CIDRs). Vacío ⇒ cualquier IP. IPNegate=true invierte la
	// condición (aplica cuando la IP NO está en ningún rango: "fuera de la red").
	IPRanges []string
	IPNegate bool

	// Condición de app. Vacío ⇒ cualquier client_id.
	ClientIDs []string

	// Condición de dispositivo de confianza (paridad Entra device-based CA). true ⇒ la
	// política aplica su acción cuando el login NO viene de un dispositivo `trusted` del
	// usuario. false ⇒ condición comodín (no mira el dispositivo).
	RequireTrustedDevice bool

	// Franja horaria opcional (UTC). DaysMask: bitmask de días (bit i = time.Weekday i,
	// Domingo=0); 0 ⇒ todos. StartMinute/EndMinute: minuto-del-día [start,end); -1 ⇒ sin
	// restricción. Start>End ⇒ ventana que cruza medianoche.
	DaysMask    int
	StartMinute int
	EndMinute   int

	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AccessContext es el contexto del intento de login que evalúa el motor.
type AccessContext struct {
	UserID   string
	GroupIDs []string // grupos del usuario EN la org (UUIDs)
	IP       string   // IP de origen del request
	ClientID string   // client_id OAuth de la app pendiente (puede ir vacío)
	// TrustedDevice indica si el request viene de un dispositivo de confianza del
	// usuario (cookie de dispositivo firmada que resuelve a un devices.trusted=true).
	// Solo lo consultan las políticas con RequireTrustedDevice.
	TrustedDevice bool
	Now           time.Time
}

// AccessDecision es el resultado de evaluar las políticas de una org para un intento.
// PolicyID/PolicyName identifican la política decisoria (para auditoría); vacíos si
// ninguna aplicó (default allow).
type AccessDecision struct {
	Action     AccessAction
	PolicyID   string
	PolicyName string
}

// EvaluateAccess resuelve la acción para un intento de login combinando las políticas
// que aplican con la MÁS RESTRICTIVA. Contrato anti-lockout:
//   - Sin políticas (o ninguna aplicable) ⇒ AccessAllow (nadie se queda fuera).
//   - Un usuario EXCLUIDO de una política no la sufre, aunque cumpla el resto.
//
// Es una función pura (sin E/S): toda la lógica de decisión es testeable offline.
func EvaluateAccess(policies []AccessPolicy, actx AccessContext) AccessDecision {
	decision := AccessDecision{Action: AccessAllow}
	// Orden estable por prioridad para un desempate determinista del PolicyID cuando
	// dos políticas comparten la acción más restrictiva.
	sorted := make([]AccessPolicy, len(policies))
	copy(sorted, policies)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })

	for i := range sorted {
		p := &sorted[i]
		if !p.Enabled {
			continue
		}
		if p.excludes(actx.UserID, actx.GroupIDs) {
			continue // break-glass: exento de ESTA política.
		}
		if !p.applies(actx) {
			continue
		}
		if p.Action.severity() > decision.Action.severity() {
			decision = AccessDecision{Action: p.Action, PolicyID: p.ID, PolicyName: p.Name}
		}
	}
	return decision
}

// excludes indica si el usuario (o alguno de sus grupos) está exento de la política.
func (p *AccessPolicy) excludes(userID string, groupIDs []string) bool {
	return containsStr(p.ExcludeUsers, userID) || intersects(p.ExcludeGroups, groupIDs)
}

// applies indica si TODAS las condiciones fijadas de la política se cumplen (AND).
func (p *AccessPolicy) applies(actx AccessContext) bool {
	return p.appliesToSubject(actx.UserID, actx.GroupIDs) &&
		p.matchesIP(actx.IP) &&
		p.matchesClient(actx.ClientID) &&
		p.matchesDevice(actx.TrustedDevice) &&
		p.matchesTime(actx.Now)
}

// matchesDevice: sin exigencia (RequireTrustedDevice=false) ⇒ comodín. Con exigencia,
// la política aplica SOLO cuando el login NO viene de un dispositivo de confianza (el
// dispositivo de confianza queda EXENTO de la acción). Un dispositivo desconocido (sin
// cookie o sin registrar) llega como trusted=false: aplica — el default seguro para
// "exigir dispositivo gestionado".
func (p *AccessPolicy) matchesDevice(trusted bool) bool {
	if !p.RequireTrustedDevice {
		return true
	}
	return !trusted
}

// PoliciesNeedDevice indica si alguna política HABILITADA exige dispositivo de
// confianza — permite al cableado saltarse la resolución del dispositivo (una lectura
// de BD) cuando ninguna política la necesita (caso común).
func PoliciesNeedDevice(policies []AccessPolicy) bool {
	for i := range policies {
		if policies[i].Enabled && policies[i].RequireTrustedDevice {
			return true
		}
	}
	return false
}

// appliesToSubject: include vacío (ambos) ⇒ todos; si no, el usuario debe estar en
// include_users o en algún include_group.
func (p *AccessPolicy) appliesToSubject(userID string, groupIDs []string) bool {
	if len(p.IncludeUsers) == 0 && len(p.IncludeGroups) == 0 {
		return true
	}
	return containsStr(p.IncludeUsers, userID) || intersects(p.IncludeGroups, groupIDs)
}

// matchesIP: sin rangos ⇒ cualquiera. Con rangos, in = la IP cae en alguno; el
// resultado se invierte si IPNegate. Una IP no parseable se trata como "no está en
// ningún rango" (in=false): con negate=true (fuera de la red) sí aplica — el default
// seguro para una IP desconocida en una regla de "exigir MFA fuera de la oficina".
func (p *AccessPolicy) matchesIP(ip string) bool {
	if len(p.IPRanges) == 0 {
		return true
	}
	in := false
	if addr, err := netip.ParseAddr(ip); err == nil {
		for _, cidr := range p.IPRanges {
			pfx, err := netip.ParsePrefix(cidr)
			if err == nil && pfx.Contains(addr) {
				in = true
				break
			}
		}
	}
	if p.IPNegate {
		return !in
	}
	return in
}

// matchesClient: sin client_ids ⇒ cualquier app; si no, el client debe estar en la
// lista (y no ser vacío: en los flujos sin app conocida la condición no casa).
func (p *AccessPolicy) matchesClient(clientID string) bool {
	if len(p.ClientIDs) == 0 {
		return true
	}
	return clientID != "" && containsStr(p.ClientIDs, clientID)
}

// matchesTime: franja horaria opcional en UTC. Sin ventana (start<0 o end<0) y sin
// días ⇒ siempre. Con días, el día actual debe estar en la máscara; con ventana, el
// minuto-del-día debe caer en [start,end) (o fuera de [end,start) si cruza medianoche).
func (p *AccessPolicy) matchesTime(now time.Time) bool {
	t := now.UTC()
	if p.DaysMask != 0 && p.DaysMask&(1<<uint(t.Weekday())) == 0 {
		return false
	}
	if p.StartMinute < 0 || p.EndMinute < 0 || p.StartMinute == p.EndMinute {
		return true // sin ventana horaria (solo días, o nada).
	}
	m := t.Hour()*60 + t.Minute()
	if p.StartMinute < p.EndMinute {
		return m >= p.StartMinute && m < p.EndMinute
	}
	// Ventana que cruza medianoche (p. ej. 22:00–06:00).
	return m >= p.StartMinute || m < p.EndMinute
}

func containsStr(ss []string, v string) bool {
	if v == "" {
		return false
	}
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

// intersects indica si a y b comparten algún elemento no vacío.
func intersects(a, b []string) bool {
	for _, x := range a {
		if x == "" {
			continue
		}
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
