// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Web desde el entorno
// (12-factor), sin prefijo. La lista de variables es CERRADA (PAGES-ENCARGO §2.1):
// el gate de helm-render-test.sh exige que el chart pase solo lo que algún
// config.go lee con Get/Require/SecretoOpcional literal, así que TODAS se leen aquí.
//
// Dos modos, un binario: la API (Cargar) y el servidor de las webs publicadas
// (`pages sitios`, CargarSitios), que no tiene base de datos, OIDC ni secretos.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
)

// ServiceName es el nombre lógico del servicio (atributo de logs y telemetría).
const ServiceName = "sytue-pages"

// DefaultPort es el puerto HTTP de Sytue Web, el mismo en los dos modos (§2).
const DefaultPort = "8100"

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=...").
var Version = "dev"

// Sitios es lo que necesita el servidor de las webs publicadas, y la parte común
// con la API.
type Sitios struct {
	pcfg.Base

	// S3 es el bucket `pages-sitios`. Todas las claves de Pages cuelgan de
	// objectstore.PrefijoCompartido (A3), así que no hay almacén remoto por tenant.
	S3 objectstore.Config
	// SitesDomain es el dominio de las webs: <slug>.<SitesDomain>. Con el dominio de
	// contenido de clientes puesto (SYTUE_CLIENT_CONTENT_DOMAIN, `global.dominioDeClientes`,
	// decisión del dueño), es `sites.<ese dominio>`: una web de un cliente no cuelga del
	// dominio registrable de la suite, donde podría poner cookies a toda ella (WEB-09).
	SitesDomain string
	// DominioDeClientes es ese dominio de contenido de clientes; vacío, el de hoy.
	DominioDeClientes string
	// SitesDomainAnterior es PAGES_SITES_DOMAIN cuando el de clientes lo sustituye: sus
	// direcciones ya solo redirigen a la nueva y siguen valiendo como destino CNAME de
	// los dominios propios que ya apuntan a él.
	SitesDomainAnterior string
	// CastOrigin es el origen de los incrustados de Sytue Cast (CSP de las webs y
	// DatosRender.origins.cast).
	CastOrigin string
	// MapaRefresco es cada cuánto se relee el mapa host → sitio de S3.
	MapaRefresco time.Duration
}

// Config es la configuración completa de la API de Sytue Web.
type Config struct {
	Sitios

	// PublicURL es la URL pública del host del producto (webhooks, enlaces).
	PublicURL string

	DatabaseURL string
	DBMaxConns  int

	// OIDC: JWT de Sytue ID. La suite comparte sesión, así que se acepta el token
	// si su aud contiene ALGUNA de las audiencias de primera parte.
	OIDCIssuer            string
	OIDCJWKSURL           string
	OIDCAcceptedAudiences []string

	// SecretEncKey es la KEK de keycrypto (32 bytes) con la que se sellan las
	// credenciales de pasarelas y las claves de IA. Vacía (solo en dev) = no se
	// pueden guardar; fuera de dev es obligatoria.
	SecretEncKey []byte

	// PublicadorSecret es la verja `X-Sytue-Publicador` de publicaciones y
	// exportaciones (A2). Obligatoria fuera de dev.
	PublicadorSecret string

	// IPApex es la 2.ª IP del frontal, la que se enseña para el A del ápex de un
	// dominio propio. Vacía fuera de dev = dominios propios apagados.
	IPApex string
	// AbusoCorreo es a quién llegan las denuncias de contenido.
	AbusoCorreo string

	// Cupo de trabajos de Sytue AI por organización y mes, según el plan (§20.2).
	IACupoPro        int
	IACupoEnterprise int

	// IDURL y IDEstadoOrgsSecret: el estado de las organizaciones en Sytue ID
	// (A11). El secreto es obligatorio fuera de dev.
	IDURL              string
	IDEstadoOrgsSecret string

	// HubBaseURL y HubInternalSecret: avisos del Hub. Sin URL no se avisa.
	HubBaseURL        string
	HubInternalSecret string

	// Sytue AI por la ruta interna (A8).
	AIURL    string
	AISecret string

	// SyERP: pedidos y facturas de la tienda (§11).
	SyERPURL           string
	SyERPPedidosSecret string

	// BasesURL es la raíz INTERNA de Sytue Bases: destino fijo de «tabla desde una vista».
	// El token de una conexión no se manda a ningún otro sitio.
	BasesURL string

	// Sytue Analytics: alta de hosts y panel (§12). Sin URL, sin analítica.
	AnalyticsURL    string
	AnalyticsSecret string

	// Correo es el relé de salida. Sin SMTP_FROM no sale correo y la tienda no
	// puede pasar a modo real.
	Correo correo.Config
}

// CargarSitios lee la configuración del modo `sitios`.
func CargarSitios() (Sitios, error) {
	l := pcfg.NewLoader("")
	s, err := cargarSitios(l)
	if err != nil {
		return Sitios{}, err
	}
	return s, l.Err()
}

func cargarSitios(l *pcfg.Loader) (Sitios, error) {
	base, err := pcfg.LoadBase("", ServiceName)
	if err != nil {
		return Sitios{}, err
	}
	if _, ok := os.LookupEnv("VERSION"); !ok {
		base.Version = Version
	}
	// PORT (compose y chart) manda sobre HTTP_ADDR.
	if port := l.Get("PORT"); port != "" {
		base.HTTPAddr = ":" + port
	} else if base.HTTPAddr == "" || base.HTTPAddr == ":8080" {
		base.HTTPAddr = ":" + DefaultPort
	}
	return Sitios{
		Base: base,
		S3: objectstore.Config{
			Endpoint:  l.GetDefault("S3_ENDPOINT", "minio:9000"),
			Region:    l.GetDefault("S3_REGION", "fr-par"),
			AccessKey: l.Require("S3_ACCESS_KEY"),
			SecretKey: l.Require("S3_SECRET_KEY"),
			UseSSL:    l.GetBoolDefault("S3_USE_SSL", false),
			Bucket:    l.GetDefault("S3_BUCKET", "sytue-dev-pages-sitios"),
		},
		SitesDomain:  l.GetDefault("PAGES_SITES_DOMAIN", "sites.sytue.localhost"),
		CastOrigin:   l.GetDefault("PAGES_CAST_ORIGIN", "https://cast.sytue.localhost"),
		MapaRefresco: l.GetDurationDefault("PAGES_MAPA_REFRESCO", 10*time.Second),
	}.conDominioDeClientes(l.Get("SYTUE_CLIENT_CONTENT_DOMAIN")), nil
}

// conDominioDeClientes pasa las webs al dominio de contenido de clientes si lo hay.
func (s Sitios) conDominioDeClientes(dominio string) Sitios {
	dominio = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(dominio)), ".")
	if dominio == "" {
		return s
	}
	s.DominioDeClientes = dominio
	s.SitesDomain, s.SitesDomainAnterior = "sites."+dominio, s.SitesDomain
	return s
}

// Cargar lee la configuración de la API.
func Cargar() (Config, error) {
	l := pcfg.NewLoader("")
	s, err := cargarSitios(l)
	if err != nil {
		return Config{}, err
	}
	env := s.Env
	cfg := Config{
		Sitios:      s,
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://web.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),

		PublicadorSecret: l.SecretoOpcional(env, "PAGES_PUBLICADOR_SECRET", pcfg.DevPagesPublicadorSecret),
		AbusoCorreo:      l.GetDefault("PAGES_ABUSO_CORREO", "abuse@sytue.localhost"),
		IACupoPro:        l.GetIntDefault("PAGES_IA_CUPO_PRO", 100),
		IACupoEnterprise: l.GetIntDefault("PAGES_IA_CUPO_ENTERPRISE", 500),

		IDURL:              l.GetDefault("ID_URL", "http://id:8081"),
		IDEstadoOrgsSecret: l.SecretoOpcional(env, "ID_ESTADO_ORGS_SECRET", pcfg.DevIDEstadoOrgsSecret),

		HubBaseURL: l.Get("HUB_BASE_URL"),

		AIURL:    l.GetDefault("SYTUE_AI_URL", "http://ai:8084"),
		AISecret: l.SecretoOpcional(env, "AI_PAGES_SECRET", pcfg.DevAIPagesSecret),

		SyERPURL:           l.GetDefault("SYERP_URL", "http://syerp:8098"),
		BasesURL:           l.GetDefault("BASES_URL", "http://bases:8102"),
		SyERPPedidosSecret: l.SecretoOpcional(env, "SYERP_PEDIDOS_SECRET", pcfg.DevSyERPPedidosSecret),

		AnalyticsURL:    l.Get("ANALYTICS_API_URL"),
		AnalyticsSecret: l.SecretoOpcional(env, "ANALYTICS_PAGES_SECRET", pcfg.DevAnalyticsPagesSecret),

		Correo: l.Correo(),
	}
	if cfg.HubBaseURL != "" {
		cfg.HubInternalSecret = l.HubInternalSecret(env)
	}
	ipApex := l.Get("PAGES_IP_APEX")
	crudaKEK := l.Get("SECRET_ENC_KEY")
	if err := l.Err(); err != nil {
		return Config{}, err
	}

	var errs []error
	if env != pcfg.EnvDev {
		// Sin estos dos no se publica ni se sabe que una organización está
		// suspendida: vacíos fuera de dev son un error, no una función apagada.
		if cfg.PublicadorSecret == "" {
			errs = append(errs, errors.New("PAGES_PUBLICADOR_SECRET es obligatorio fuera de desarrollo"))
		}
		if cfg.IDEstadoOrgsSecret == "" {
			errs = append(errs, errors.New("ID_ESTADO_ORGS_SECRET es obligatorio fuera de desarrollo"))
		}
	}
	if cfg.SecretEncKey, err = kek(crudaKEK, env); err != nil {
		errs = append(errs, err)
	}
	if cfg.IPApex, err = ipDelApex(ipApex, env); err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	if cfg.OIDCJWKSURL == "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuer + "/.well-known/jwks.json"
	}
	return cfg, nil
}

// kek decodifica SECRET_ENC_KEY (base64 de 32 bytes). En dev puede faltar; fuera,
// no: sin ella no se pueden sellar credenciales, y nunca se guardan en claro.
func kek(cruda string, env pcfg.Environment) ([]byte, error) {
	if cruda == "" {
		if env == pcfg.EnvDev {
			return nil, nil
		}
		return nil, errors.New("SECRET_ENC_KEY es obligatoria fuera de desarrollo (AES-256)")
	}
	clave, err := base64.StdEncoding.DecodeString(cruda)
	if err != nil {
		return nil, fmt.Errorf("SECRET_ENC_KEY: base64 inválido: %w", err)
	}
	if len(clave) != 32 {
		return nil, fmt.Errorf("SECRET_ENC_KEY: debe medir 32 bytes (AES-256), tiene %d", len(clave))
	}
	return clave, nil
}

// ipDelApex valida PAGES_IP_APEX. En dev, por defecto 127.0.0.1. Fuera de dev,
// vacía apaga los dominios propios (la pestaña lo dice) sin tumbar el servicio, y
// cualquier cosa que no sea una IPv4 pública es un error: enseñarla a un cliente
// para su registro A le dejaría el dominio apuntando a otra parte.
func ipDelApex(v string, env pcfg.Environment) (string, error) {
	if env == pcfg.EnvDev {
		if v == "" {
			return "127.0.0.1", nil
		}
		return v, nil
	}
	if v == "" {
		return "", nil
	}
	ip, err := netip.ParseAddr(v)
	if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return "", fmt.Errorf("PAGES_IP_APEX: %q no es una IPv4 pública", v)
	}
	return ip.String(), nil
}
