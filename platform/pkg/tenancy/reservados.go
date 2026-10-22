// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tenancy

import "strings"

// reservados son los identificadores que NO puede tomar una organización ni una web de
// Sytue Web (WEB-08: `id.sites.<dominio>` o `login.sites.<dominio>` con aspecto oficial).
// Vivían en Sytue ID (services/id/internal/domain/onboarding.go) hasta que Sytue Web los
// necesitó para los slugs de sus webs. Para una organización:
// colisionan con los hosts propios de la suite (id.sytue.eu, mail.sytue.eu…),
// con nombres que necesitamos libres y —desde ADR 0056— con los nombres de
// página de las apps web: la organización viaja como segmento del path tras el
// prefijo de la app (`calcus.sytue.eu/calcus/<org>/edit`), y el middleware de cada
// app decide con su lista `sinOrg` si el primer segmento es una página o una
// organización. Una org llamada como una página secuestraría SUS URLs web: el
// middleware la trataría como página. El subdominio de una org es <slug>.sytue.eu,
// así que dejarlos pasar rompería el despliegue, no solo la estética.
var reservados = map[string]struct{}{
	// Hosts propios de la suite e infraestructura.
	"id": {}, "auth": {}, "login": {}, "admin": {}, "console": {}, "one": {}, "hub": {},
	"mail": {}, "smtp": {}, "imap": {}, "webmail": {}, "calendar": {}, "contacts": {},
	"drive": {}, "docs": {}, "sheets": {}, "slides": {}, "rooms": {}, "meet": {}, "chat": {},
	"cloud": {}, "api": {}, "app": {}, "apps": {}, "www": {}, "cdn": {}, "static": {},
	"status": {}, "support": {}, "help": {}, "billing": {}, "sytue": {}, "test": {},
	"soporte": {}, "ayuda": {},
	"dev": {}, "staging": {}, "integrations": {}, "search": {}, "ns1": {}, "ns2": {},
	"mx": {}, "autodiscover": {}, "autoconfig": {},
	// Productos, hosts y prefijos del catálogo de la web
	// (apps/frontend/packages/config/src/tokens.ts). Los mantiene al día
	// TestCatalogoDeProductosEstaReservado: Cast, Notepads y SyPhotos estrenaron
	// host sin entrar aquí.
	"cast": {}, "notes": {}, "photos": {}, "bases": {},
	// Sytue Web: el host del producto —`web` desde el 2026-10-04 y `pages`, el
	// anterior, que sigue sirviendo o redirigiendo— y el dominio bajo el que se
	// publican las webs (`<slug>.sites.<dominio>`). Una organización con cualquiera de
	// los tres nombres chocaría con ellos en el comodín `*.sytue.eu`: hasta ese día
	// `web.sytue.eu` SE SERVÍA como el host de una organización llamada «web».
	"pages": {}, "sites": {}, "web": {},
	// La ofimática, una app por producto desde el 2026-10-12: cada una tiene host y
	// prefijo propios (`calcus.sytue.eu/calcus`). `docs` se queda arriba: su host
	// sigue siendo el de la API del servicio de documentos.
	"docus": {}, "calcus": {}, "diapos": {}, "certipdf": {},
	// Hosts de infraestructura que no son productos de la web y que el test no ve:
	// los medios de Rooms, el relé de salida, Analytics, la IA y la forja.
	"sfu": {}, "rooms-sfu": {}, "matrix": {}, "mx-out": {}, "analytics": {}, "ai": {}, "git": {},
	"syphotos": {}, "syerp": {},
	// Hosts de Sytue Cloud bajo el apex: previews (`preview.<dominio>`), destino
	// CNAME de los dominios de cliente (`ingress.<dominio>`) y el registro de
	// imágenes. `apps` ya está arriba. Una organización con cualquiera de estos
	// nombres chocaría con el host de Cloud en el comodín `*.sytue.eu`.
	"preview": {}, "ingress": {}, "registry": {},

	// Nombres de página de las apps web: la unión de los `sinOrg` de
	// apps/frontend/apps/*/src/middleware.ts. La lista la mantiene al día
	// TestSinOrgDeLaWebEstaReservada (onboarding_test.go), que lee esos ficheros
	// y falla si una app estrena una página que aquí no esté.
	"abrir": {}, "access": {}, "account": {}, "actividad": {}, "album": {}, "albums": {},
	"audit": {}, "bienvenida": {}, "buscar": {}, "channels": {}, "compliance": {},
	"conexiones": {}, "confirm-alert": {}, "datos": {}, "descargas": {}, "devices": {},
	"dispositivos": {}, "duplicates": {}, "edit": {}, "editor-de-documentos": {}, "erp": {}, "events": {},
	"favorites": {}, "files": {}, "governance": {}, "groups": {}, "messages": {},
	"migrations": {}, "nearby": {}, "notebooks": {}, "notificaciones": {},
	"open-from-drive": {}, "p": {}, "pdf": {}, "pip": {}, "platform": {},
	"privileged-access": {}, "projects": {}, "public": {}, "roles": {}, "s": {},
	"sensitivity-labels": {}, "settings": {}, "share": {}, "shared": {}, "similar": {}, "sso": {},
	"team": {}, "templates": {}, "tenants": {}, "tokens": {}, "trash": {}, "usage": {},
	"users": {}, "wopi": {},
	// Sytue Cast (ADR 0061): la portada, el canal, la ficha, el incrustado y
	// los ficheros firmados van sin organización, y las páginas de trabajo
	// también se declaran porque sus nombres son slugs válidos.
	"c": {}, "embed": {}, "explore": {}, "manage": {}, "media": {}, "moderation": {},
	"oembed": {}, "studio": {}, "upload": {}, "watch": {},
	// La consola: peticiones de firma que llegan de otra organización (ADR 0059).
	"firmas-de-fuera": {},
	// Sytue Web: el asistente de alta y la ficha de un sitio (`edit` y `settings` ya
	// están arriba).
	"new": {}, "site": {},
	// Sytue Bases: la ficha de una base y el formulario público (`/f/<token>`),
	// que nunca lleva organización (`dev` ya está arriba).
	"base": {}, "f": {},
}

// prefijosReservados son los prefijos de slug que la plataforma usa para acuñar
// hosts a partir del slug de OTRA organización, así que ninguna organización puede
// empezar por ellos. Hoy solo `nas-`: Sytue Drive publica el túnel WebDAV de la
// organización `foo` en `nas-foo.<dominio>` (services/drive/internal/service/nas.go),
// de modo que una organización con el slug `nas-foo` tendría su subdominio
// `nas-foo.<dominio>` pisando ese túnel. Es el mismo mecanismo que el prefijo de las
// cuentas personales (`p-`), que Sytue ID trata aparte en SlugAllowedForOrg.
var prefijosReservados = []string{"nas-"}

// Reservado indica si el identificador está reservado por la plataforma: un nombre
// exacto de la lista, o un slug bajo un prefijo reservado (p. ej. `nas-`).
func Reservado(slug string) bool {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if _, ok := reservados[slug]; ok {
		return true
	}
	for _, p := range prefijosReservados {
		if strings.HasPrefix(slug, p) {
			return true
		}
	}
	return false
}
