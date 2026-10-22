// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Reglas del alta inicial guiada: recorridos, identificador de la organización,
// verificación DNS del dominio propio y catálogo de roles recomendados. Todo
// aquí es PURO (sin BD ni red salvo el resolver que se inyecta), para que el
// comportamiento que el usuario ve —«disponible», «verificado», «este rol podrá
// hacer X»— sea comprobable en test sin depender del entorno.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/mail"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/dominio"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// OnboardingMode es el recorrido elegido en el primer paso. Por debajo AMBOS
// crean una organización (modelo de datos único); la diferencia es lo que se le
// pide al usuario: en individual no hay personas ni roles que repartir.
type OnboardingMode string

const (
	OnboardingIndividual   OnboardingMode = "individual"
	OnboardingOrganization OnboardingMode = "organization"
)

// Pasos del asistente (identificadores estables: los usa el frontend para
// retomar y el backend para validar que el paso guardado existe en el recorrido).
const (
	StepMode     = "modo"
	StepIdentity = "identidad"
	StepPeople   = "personas"
	StepBilling  = "facturacion"
	StepRoles    = "roles"
	StepDone     = "listo"
)

// OnboardingSteps devuelve el recorrido del modo, en orden. El recorrido
// individual SALTA personas y roles: a quien se da de alta para sí mismo no se
// le pregunta a quién invita ni cómo reparte permisos.
func OnboardingSteps(m OnboardingMode) []string {
	if m == OnboardingIndividual {
		return []string{StepMode, StepIdentity, StepBilling, StepDone}
	}
	return []string{StepMode, StepIdentity, StepPeople, StepBilling, StepRoles, StepDone}
}

// ValidOnboardingMode indica si el modo es uno de los dos recorridos.
func ValidOnboardingMode(m OnboardingMode) bool {
	return m == OnboardingIndividual || m == OnboardingOrganization
}

// ValidOnboardingStep indica si el paso pertenece al recorrido del modo. Evita
// que el navegador guarde un paso inventado (o el de otro recorrido) y deje al
// usuario atascado al retomar.
func ValidOnboardingStep(m OnboardingMode, step string) bool {
	for _, s := range OnboardingSteps(m) {
		if s == step {
			return true
		}
	}
	return false
}

// SlugReserved indica si el identificador está reservado por la plataforma. La lista
// vive en platform/pkg/tenancy/reservados.go desde que Sytue Web la usa para los slugs
// de sus webs (WEB-08).
func SlugReserved(slug string) bool { return tenancy.Reservado(slug) }

// PersonalSlugPrefix es el prefijo de los slugs de las cuentas personales.
// Queda RESERVADO para ellas: una organización no puede elegir un slug que
// empiece así (ver SlugAllowedForOrg), o una alta podría chocar con el slug
// derivado del correo de una persona, o hacerse pasar por ella.
const PersonalSlugPrefix = tenancy.PersonalSlugPrefix

// PersonalSlug deriva el slug de la cuenta personal de un correo. Un correo NO
// es un slug válido —lleva `@` y puntos, y el slug es subdominio y namespace—,
// así que se usa un resumen: estable para el mismo correo, sin colisiones
// prácticas y sin filtrar la dirección en la URL del host. Son 18 caracteres,
// dentro del límite de 63 de una etiqueta DNS.
func PersonalSlug(email string) string {
	sum := sha256.Sum256([]byte(NormalizeEmail(email)))
	return PersonalSlugPrefix + hex.EncodeToString(sum[:8])
}

// SlugAllowedForOrg indica si una ORGANIZACIÓN puede tomar el slug: válido, no
// reservado y fuera del prefijo de las cuentas personales.
func SlugAllowedForOrg(slug string) bool {
	slug = strings.ToLower(strings.TrimSpace(slug))
	return !SlugReserved(slug) && !strings.HasPrefix(slug, PersonalSlugPrefix)
}

// --- Dominio propio: registro de verificación y comprobación DNS REAL ---

// La verificación del TXT VIVE AHORA EN platform/pkg/dominio, porque Sytue Web
// la necesita para los dominios de sus webs. Estos alias existen para que el
// resto de Sytue ID —http y sus tests— la siga nombrando por el dominio, como
// hace clientip.go con la lista de proxies.
type (
	Resolver  = dominio.Resolver
	DNSStatus = dominio.DNSStatus
	DNSCheck  = dominio.DNSCheck
)

const (
	DomainVerified    = dominio.DomainVerified
	DomainPending     = dominio.DomainPending
	DomainMismatch    = dominio.DomainMismatch
	DomainUnreachable = dominio.DomainUnreachable
)

var (
	RecordName     = dominio.RecordName
	RecordValue    = dominio.RecordValue
	NewDomainToken = dominio.NewDomainToken
	CheckDomainTXT = dominio.CheckDomainTXT
)

// NormalizeDomain es dominio.Normalizar para el dominio de una ORGANIZACIÓN:
// además quita `www.` (la gente pega la dirección de su web, y lo que se
// verifica es la organización, no un host) y envuelve el error en
// ErrInvalidInput, que es lo que espera la capa http.
func NormalizeDomain(raw string) (string, error) {
	d, err := dominio.Normalizar(raw)
	if err == nil {
		d = strings.TrimPrefix(d, "www.")
		if !strings.Contains(d, ".") {
			err = fmt.Errorf("%w: falta la extensión", dominio.ErrNoValido)
		}
	}
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	return d, nil
}

// --- Invitaciones en lote ---

// ParseInviteEmails separa una lista pegada por el usuario (comas, espacios,
// saltos de línea o punto y coma), normaliza y deduplica. Devuelve además lo que
// NO es un correo, para decírselo tal cual en vez de fallar en bloque.
func ParseInviteEmails(raw string) (ok, bad []string) {
	seen := make(map[string]struct{})
	for _, tok := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	}) {
		addr, err := mail.ParseAddress(tok)
		if err != nil {
			bad = append(bad, tok)
			continue
		}
		e := strings.ToLower(strings.TrimSpace(addr.Address))
		if _, dup := seen[e]; dup {
			continue
		}
		seen[e] = struct{}{}
		ok = append(ok, e)
	}
	return ok, bad
}

// --- Roles recomendados ---

// RecommendedRole es un rol PREPARADO que el alta ofrece crear con un clic. No
// es un sistema paralelo: se materializa como un rol personalizado (mig 0019)
// con permisos del catálogo FIJO, editable y borrable después como cualquier otro.
type RecommendedRole struct {
	// Key es el identificador estable que envía el frontend.
	Key string
	// Name es el nombre con el que se crea el rol en la organización.
	Name string
	// Summary explica en llano QUÉ PODRÁ HACER quien lo tenga (y qué no). Es
	// literalmente lo que se le enseña al usuario: si cambian los permisos, cambia.
	Summary     string
	Permissions []Permission
}

// RecommendedRoles es el catálogo del paso «Roles recomendados». Cada uno concede
// permisos REALES del catálogo (roles.go) y su descripción coincide con ellos.
var RecommendedRoles = []RecommendedRole{
	{
		Key:  "soporte",
		Name: "Soporte",
		Summary: "Ayuda a tu gente con su cuenta: dar de alta y de baja personas, " +
			"reiniciar contraseñas y cerrar sesiones. No ve la facturación ni cambia " +
			"la seguridad de la organización.",
		Permissions: []Permission{PermUsersRead, PermUsersManage},
	},
	{
		Key:  "facturacion",
		Name: "Facturación",
		Summary: "Se ocupa de las facturas y del plan. No puede ver el directorio de " +
			"personas ni tocar la configuración.",
		Permissions: []Permission{PermBillingManage},
	},
	{
		Key:  "seguridad",
		Name: "Seguridad y cumplimiento",
		Summary: "Decide desde dónde se puede entrar, configura el acceso con la cuenta " +
			"de la empresa y revisa el registro de actividad. No gestiona personas ni " +
			"facturación.",
		Permissions: []Permission{PermAccessPoliciesManage, PermSSOManage, PermAuditView},
	},
	{
		Key:  "aplicaciones",
		Name: "Gestión de aplicaciones",
		Summary: "Conecta y retira las aplicaciones de terceros de la organización. No " +
			"toca personas, facturación ni seguridad.",
		Permissions: []Permission{PermAppsManage},
	},
}

// RecommendedRoleByKey busca un rol recomendado por su clave.
func RecommendedRoleByKey(key string) (RecommendedRole, bool) {
	for _, r := range RecommendedRoles {
		if r.Key == key {
			return r, true
		}
	}
	return RecommendedRole{}, false
}

// PermissionStrings devuelve los permisos del rol como strings (lo que espera el
// store y lo que valida NormalizePermissions).
func (r RecommendedRole) PermissionStrings() []string {
	out := make([]string, 0, len(r.Permissions))
	for _, p := range r.Permissions {
		out = append(out, string(p))
	}
	return out
}
