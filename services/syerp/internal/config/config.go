// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de SyERP desde el entorno del
// proceso (12-factor). Reutiliza el cargador común de platform/pkg/config y
// respeta EXACTAMENTE los nombres de variable del compose.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-syerp"

// DefaultPort es el puerto HTTP de SyERP (CONVENTIONS §6): el siguiente libre
// tras Sytue Photos.
const DefaultPort = "8098"

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=...").
var Version = "dev"

// ExportMaxFilasDefecto es el tope duro de filas de una exportación CSV
// (`GET /api/v1/{modelo}/export`).
//
// No está para que el fichero quepa en una hoja de cálculo —Excel admite un
// millón de filas— ni para acotar la memoria: la exportación va por tramos de
// 200 filas con el cursor, así que lo que crece con el tamaño de la
// organización no es la memoria del pod, es el TIEMPO de la petición. Y a la
// petición le da 30 s el router (ver NewRouter): pasado el plazo, el contexto
// se cancela a media escritura y la descarga acaba en un fichero cortado.
//
// Medido en el piloto (PostgreSQL 18 en contenedor, modelo con relación,
// importe y estado): 26.000 filas en 1,1 s y 940 KB de CSV, ~24.000 filas/s.
// A ese ritmo 50.000 filas son unos 2 s, y siguen cabiendo en el plazo aunque
// producción vaya DIEZ veces más lenta que el piloto; 100.000 ya no. Ahí está
// el número.
//
// Se puede subir con EXPORT_MAX_FILAS, pero subirlo sin subir también el plazo
// de la petición solo cambia el sitio por donde se corta el fichero.
const ExportMaxFilasDefecto = 50_000

// Config es la configuración completa de SyERP.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de cara al usuario (https://erp.sytue.eu).
	PublicURL string

	// DatabaseURL es el DSN PostgreSQL. Única fuente de verdad.
	DatabaseURL string
	// DBMaxConns es el tamaño máximo del pool.
	DBMaxConns int

	// ExportMaxFilas es el tope duro de filas de una exportación CSV. Ver
	// ExportMaxFilasDefecto para por qué ese número.
	ExportMaxFilas int

	// OIDC configura la validación de tokens de Sytue ID (JWKS).
	OIDCIssuer   string
	OIDCJWKSURL  string
	OIDCAudience string
	// OIDCAcceptedAudiences: la suite comparte sesión, así que el token puede
	// traer la audiencia de OTRA app.
	OIDCAcceptedAudiences []string

	// IntegrationsURL es el Sytue Integration Gateway (:8093). TODO el tráfico
	// hacia Stripe y Salesforce sale por ahí y no desde este servicio: el egress
	// a terceros está concentrado en un sitio, con su SSRF, su límite, su
	// auditoría y sus secretos sellados. Un conector que abre su propio cliente
	// HTTP se salta las cuatro cosas a la vez.
	IntegrationsURL string

	// AIURL es la base de Sytue AI (SYTUE_AI_URL, igual que en Hub). Vacía = el
	// ERP sin IA: las rutas de Sytue AI responden 503.
	AIURL string
	// Bases internas de las apps de las que un informe puede leer listados, con
	// el token de quien lo pide. Vacía = esa app no aparece como fuente.
	IDBaseURL       string
	ContactsBaseURL string
	CalendarBaseURL string
	RoomsBaseURL    string
	// DocsBaseURL es Sytue PDF: lee el texto del PDF de una factura para rellenar la ficha.
	DocsBaseURL string
	// BasesBaseURL es la raíz INTERNA de Sytue Bases: destino fijo de la fuente «Sytue
	// Bases» de los informes. El token de una conexión no se manda a ningún otro sitio.
	BasesBaseURL string

	// HubBaseURL es el Hub interno: una tarea que Sytue AI crea para quien la pidió
	// le llega también como notificación, con su propio token.
	HubBaseURL string
	// HubInternalSecret es el secreto de la puerta interna del Hub, la que usan los
	// servicios para avisar a otra persona (una tarea asignada).
	HubInternalSecret string
	// PedidosTiendaSecret protege la frontera de Pages. Vacío apaga las rutas:
	// ninguna instalación anterior expone una API interna nueva por accidente.
	PedidosTiendaSecret string
	// AppURL es la dirección de la app del ERP en el navegador, para los enlaces de
	// los correos (https://erp.sytue.eu/erp).
	AppURL string

	// SMTP del buzón con el que salen los avisos de obligaciones. Sin SMTPAddr o
	// sin SMTPFrom no sale ninguno y el resto del ERP funciona igual.
	SMTPAddr, SMTPFrom, SMTPUser, SMTPPassword, SMTPTLSServerName string

	// VERI*FACTU. Con VerifactuProduccion los registros van a la AEAT de verdad y
	// el QR apunta a su sede; si no, a su entorno de pruebas. VerifactuSello elige
	// las direcciones de quien presenta con certificado de sello. Sin certificado
	// no sale nada y ninguna organización lo puede activar.
	VerifactuProduccion bool
	VerifactuSello      bool
	VerifactuCertPEM    []byte
	VerifactuKeyPEM     []byte

	// SecretEncKey sella los secretos de firma de los webhooks entrantes
	// (AES-256, base64 en el entorno). Sin ella no se puede guardar ninguno: un
	// secreto con el que se pueden falsificar cobros no se guarda en claro ni
	// «de momento».
	SecretEncKey []byte
}

// Load construye la configuración leyendo el entorno.
func Load() (Config, error) {
	base, err := pcfg.LoadBase("", ServiceName)
	if err != nil {
		return Config{}, err
	}
	if _, ok := os.LookupEnv("VERSION"); !ok {
		base.Version = Version
	}

	l := pcfg.NewLoader("")

	cfg := Config{
		Base:        base,
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://erp.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		ExportMaxFilas: l.GetIntDefault("EXPORT_MAX_FILAS", ExportMaxFilasDefecto),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", "sytue-syerp"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),

		IntegrationsURL: l.GetDefault("INTEGRATIONS_BASE_URL", "http://integrations:8093"),

		AIURL:               l.Get("SYTUE_AI_URL"),
		IDBaseURL:           l.GetDefault("ID_BASE_URL", "http://id:8081"),
		ContactsBaseURL:     l.GetDefault("CONTACTS_BASE_URL", "http://contacts:8088"),
		CalendarBaseURL:     l.GetDefault("CALENDAR_BASE_URL", "http://calendar:8087"),
		RoomsBaseURL:        l.GetDefault("ROOMS_BASE_URL", "http://rooms:8085"),
		DocsBaseURL:         l.GetDefault("DOCS_BASE_URL", "http://docs:8083"),
		BasesBaseURL:        l.GetDefault("BASES_BASE_URL", "http://bases:8102"),
		HubBaseURL:          l.GetDefault("HUB_BASE_URL", "http://hub:8092"),
		HubInternalSecret:   l.HubInternalSecret(base.Env),
		PedidosTiendaSecret: l.SecretoOpcional(base.Env, "PEDIDOS_TIENDA_SECRET", pcfg.DevSyERPPedidosSecret),
		AppURL:              l.Get("ERP_APP_URL"),

		SMTPAddr:          l.Get("SMTP_ADDR"),
		SMTPFrom:          l.Get("SMTP_FROM"),
		SMTPUser:          l.Get("SMTP_USER"),
		SMTPPassword:      l.Get("SMTP_PASSWORD"),
		SMTPTLSServerName: l.Get("SMTP_TLS_SERVER_NAME"),

		VerifactuSello:   l.GetBoolDefault("VERIFACTU_SELLO", false),
		VerifactuCertPEM: []byte(l.Get("VERIFACTU_CERT_PEM")),
		VerifactuKeyPEM:  []byte(l.Get("VERIFACTU_KEY_PEM")),
	}
	switch entorno := l.GetDefault("VERIFACTU_ENTORNO", "pruebas"); entorno {
	case "produccion":
		cfg.VerifactuProduccion = true
	case "pruebas":
	default:
		return Config{}, fmt.Errorf("VERIFACTU_ENTORNO: %q no es «pruebas» ni «produccion»", entorno)
	}
	if cfg.AppURL == "" {
		cfg.AppURL = strings.TrimRight(cfg.PublicURL, "/") + "/erp"
	}

	// PORT (compose) tiene prioridad sobre HTTP_ADDR para fijar la escucha.
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":" + DefaultPort
	}

	if err := l.Err(); err != nil {
		return Config{}, err
	}
	if cfg.OIDCJWKSURL == "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuer + "/.well-known/jwks.json"
	}

	// Clave de sellado (base64 → 32 bytes AES-256). Opcional en dev; si se aporta
	// mal, se falla al arrancar en vez de quedarse con el cifrado roto.
	if crudo := l.Get("SECRET_ENC_KEY"); crudo != "" {
		clave, err := base64.StdEncoding.DecodeString(crudo)
		if err != nil {
			return Config{}, fmt.Errorf("SECRET_ENC_KEY: base64 inválido: %w", err)
		}
		if len(clave) != 32 {
			return Config{}, fmt.Errorf("SECRET_ENC_KEY: debe medir 32 bytes (AES-256), tiene %d", len(clave))
		}
		cfg.SecretEncKey = clave
	}
	if cfg.Env.IsProd() && len(cfg.SecretEncKey) != 32 {
		return Config{}, fmt.Errorf("SECRET_ENC_KEY es obligatoria en prod (AES-256)")
	}
	return cfg, nil
}
