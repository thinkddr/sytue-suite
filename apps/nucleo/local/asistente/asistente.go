// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package asistente es la IA de Sytue Office: OPCIONAL y con el proveedor que elija la
// persona (PLAN-EDICION-LOCAL D-15, §5.9).
//
//   - Con clave propia: Anthropic, OpenAI o Google Gemini. El cliente habla directamente
//     con el proveedor; no pasa por ningún servicio de Sytue.
//   - Un modelo en este equipo (Ollama, LM Studio o cualquier servidor compatible con
//     OpenAI en localhost): IA sin Internet y sin clave.
//   - La IA del sistema operativo, donde hay una puerta para apps de terceros:
//     Apple Intelligence en macOS (por Atajos), y Foundry Local en Windows (el runtime de
//     modelos en el dispositivo de Microsoft, el de los PC Copilot+). Ver sistema_*.go.
//
// Apagada por defecto. Sin proveedor, [Asistente.Funcion] devuelve nil y las pantallas
// ocultan sus botones de IA, que es lo que ya hacen con la función a nil.
//
// El prompt es EL MISMO que el del servicio de IA de la edición conectada
// (`ia/proveedor`, movido desde services/ai): el contexto del documento va como DATOS
// delimitados con un nonce, nunca como órdenes.
package asistente

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	provider "git.sytue.eu/sytue/one/apps/nucleo/ia/proveedor"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
)

// Proveedores que se ofrecen, en el orden de Ajustes: lo local primero.
const (
	Ninguno   = ""
	Sistema   = "sistema"
	Local     = "local"
	Anthropic = "anthropic"
	OpenAI    = "openai"
	Gemini    = "gemini"
	Otro      = "otro" // cualquier servidor compatible con OpenAI
)

// GeminiURL es el extremo compatible con OpenAI de Google Gemini. Vive aquí y no en
// `ia/proveedor` porque solo lo usa el dispositivo de la persona con SU clave: el
// servicio de IA no admite Gemini. Por eso cuenta en las conexiones del dispositivo
// (compliance/supply-chain/conexiones-del-dispositivo.tsv) y no en la línea base de la
// pasarela.
const GeminiURL = "https://generativelanguage.googleapis.com/v1beta/openai"

// Preajuste describe un proveedor para Ajustes.
type Preajuste struct {
	ID       string
	Nombre   string
	URL      string // dirección por defecto (editable en Local y Otro)
	Modelo   string // modelo por defecto (editable siempre)
	Clave    bool   // pide clave de API
	EditaURL bool
}

// Preajustes es la lista de proveedores. Los modelos son el punto de partida: la persona
// los cambia por el que tenga contratado.
func Preajustes() []Preajuste {
	return []Preajuste{
		{ID: Ninguno, Nombre: "Sin IA"},
		{ID: Sistema, Nombre: NombreSistema()},
		{ID: Local, Nombre: "Modelo en este equipo (Ollama, LM Studio)", URL: "http://localhost:11434/v1", Modelo: "llama3.2", EditaURL: true},
		{ID: Anthropic, Nombre: "Anthropic (Claude)", URL: provider.BaseURLs[provider.ProviderAnthropic], Modelo: "claude-sonnet-5-5", Clave: true},
		{ID: OpenAI, Nombre: "OpenAI", URL: provider.BaseURLs[provider.ProviderOpenAI], Modelo: "gpt-5", Clave: true},
		{ID: Gemini, Nombre: "Google Gemini", URL: GeminiURL, Modelo: "gemini-2.5-flash", Clave: true},
		{ID: Otro, Nombre: "Otro compatible con OpenAI", URL: "https://", Clave: true, EditaURL: true},
	}
}

// PreajusteDe devuelve el preajuste id.
func PreajusteDe(id string) (Preajuste, bool) {
	for _, p := range Preajustes() {
		if p.ID == id {
			return p, true
		}
	}
	return Preajuste{}, false
}

// Config es lo que se guarda en `ia.json`. La clave NO: va a secretos.
type Config struct {
	Proveedor string `json:"proveedor"`
	Modelo    string `json:"modelo,omitempty"`
	URL       string `json:"url,omitempty"`
	// Avisado: la persona ya vio a quién se envía el texto (primera vez).
	Avisado bool `json:"avisado,omitempty"`
}

const nombreClave = "ia-clave"

// ErrApagada: no hay proveedor configurado.
var ErrApagada = errors.New("asistente: la IA está apagada")

// ErrSinIANativa: este sistema no ofrece una IA del sistema a apps de terceros.
var ErrSinIANativa = errors.New("asistente: este sistema no ofrece IA propia a otras apps")

// Asistente atiende las peticiones de IA de las apps.
type Asistente struct {
	ruta     string
	secretos *secretos.Almacen
	// HTTP es el cliente para los proveedores (el del portero); nil, ninguno: falla
	// cerrado.
	HTTP *http.Client

	mu  sync.Mutex
	cfg Config
	// sistema es el puente con la IA del sistema; se puede sustituir en pruebas.
	sistema func(ctx context.Context, cfg Config, system, user string) (string, error)
}

// Abrir lee la configuración de e.
func Abrir(e *espacio.Espacio, s *secretos.Almacen) *Asistente {
	a := &Asistente{ruta: e.Ajuste("ia.json"), secretos: s, sistema: completarSistema}
	_ = espacio.LeerJSON(a.ruta, &a.cfg)
	return a
}

// Config devuelve la configuración actual.
func (a *Asistente) Config() Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

// Clave devuelve la clave guardada (para Ajustes, que la enseña tapada).
func (a *Asistente) Clave() string {
	v, _ := a.secretos.Leer(nombreClave)
	return v
}

// Guardar cambia la configuración; clave vacía con un proveedor que no la pide la borra.
func (a *Asistente) Guardar(cfg Config, clave string) error {
	p, ok := PreajusteDe(cfg.Proveedor)
	if !ok {
		return fmt.Errorf("asistente: proveedor desconocido %q", cfg.Proveedor)
	}
	if cfg.URL != "" {
		u, err := url.Parse(cfg.URL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && (u.Scheme != "http" || !esLocal(u.Hostname()))) {
			return errors.New("asistente: la dirección tiene que ser https, o http solo en este equipo")
		}
	}
	if !p.Clave {
		clave = ""
	}
	if err := a.secretos.Guardar(nombreClave, clave); err != nil {
		return err
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	return espacio.EscribirJSON(a.ruta, cfg)
}

func esLocal(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// Activa dice si hay un proveedor elegido.
func (a *Asistente) Activa() bool { return a.Config().Proveedor != Ninguno }

// Destino es a quién se envía el texto, para el aviso de primera vez.
func (a *Asistente) Destino() string {
	return a.DestinoCon(func(p Preajuste) string { return p.Nombre }, "en este equipo")
}

// DestinoCon es Destino con los textos de la pantalla que lo enseña: el nombre de cada
// proveedor y «en este equipo», en su idioma.
func (a *Asistente) DestinoCon(nombre func(Preajuste) string, enEsteEquipo string) string {
	cfg := a.Config()
	p, _ := PreajusteDe(cfg.Proveedor)
	switch cfg.Proveedor {
	case Ninguno:
		return ""
	case Sistema:
		return nombre(p) + " (" + enEsteEquipo + ")"
	}
	if u, err := url.Parse(cmp.Or(cfg.URL, p.URL)); err == nil && u.Host != "" {
		if esLocal(u.Hostname()) {
			return nombre(p) + " (" + enEsteEquipo + ")"
		}
		return nombre(p) + " — " + u.Host
	}
	return nombre(p)
}

// Direccion es la dirección a la que habla el proveedor elegido (la que se le abre al
// portero de red, `local/red`); vacía sin IA o con la del sistema, que no sale por HTTP
// configurable.
func (a *Asistente) Direccion() string {
	cfg := a.Config()
	if cfg.Proveedor == Ninguno || cfg.Proveedor == Sistema {
		return ""
	}
	p, _ := PreajusteDe(cfg.Proveedor)
	return strings.TrimSpace(cmp.Or(cfg.URL, p.URL))
}

// Funcion es lo que se enchufa a las pantallas (`Asistir`); nil con la IA apagada.
func (a *Asistente) Funcion() func(context.Context, api.PeticionIA) (string, error) {
	if !a.Activa() {
		return nil
	}
	return a.Asistir
}

// Asistir manda la petición al proveedor elegido y devuelve su texto.
func (a *Asistente) Asistir(ctx context.Context, q api.PeticionIA) (string, error) {
	cfg := a.Config()
	if cfg.Proveedor == Ninguno {
		return "", ErrApagada
	}
	system := provider.ConIdioma(provider.SystemPrompt, q.Idioma)
	user := provider.BuildUserMessage(provider.Request{Action: q.Accion, Instruction: q.Instruccion, Context: q.Contexto, Kind: q.Tipo})
	if cfg.Proveedor == Sistema {
		if len(q.Imagen) > 0 {
			return "", errors.New("asistente: la IA del sistema no recibe imágenes desde aquí")
		}
		return a.sistema(ctx, cfg, system, user)
	}
	p, err := a.proveedor(cfg)
	if err != nil {
		return "", err
	}
	opts := provider.Opciones{Esfuerzo: q.Esfuerzo}
	if len(q.Imagen) > 0 {
		opts.Imagen = &provider.Imagen{Datos: q.Imagen, Tipo: cmp.Or(q.ImagenTipo, "image/jpeg")}
	}
	res, err := p.Complete(ctx, system, user, opts)
	if err != nil {
		return "", err
	}
	return res.Texto, nil
}

// Probar manda una petición mínima para comprobar la configuración desde Ajustes.
func (a *Asistente) Probar(ctx context.Context) (string, error) {
	return a.Asistir(ctx, api.PeticionIA{Accion: "responder", Instruccion: "Responde solo con la palabra «listo»."})
}

func (a *Asistente) proveedor(cfg Config) (provider.Provider, error) {
	pre, _ := PreajusteDe(cfg.Proveedor)
	hc := a.HTTP
	if hc == nil {
		// Sin el del portero, ninguno: lo no configurado falla cerrado (R-RED).
		hc = &http.Client{Transport: red.Denegar{}, Timeout: 120 * time.Second}
	}
	pc := provider.Config{
		Model:      strings.TrimSpace(cmp.Or(cfg.Modelo, pre.Modelo)),
		BaseURL:    strings.TrimSpace(cmp.Or(cfg.URL, pre.URL)),
		MaxTokens:  4096,
		Timeout:    120 * time.Second,
		HTTPClient: hc,
		APIKey:     a.Clave(),
	}
	switch cfg.Proveedor {
	case Anthropic:
		pc.Provider = provider.ProviderAnthropic
	case OpenAI:
		pc.Provider = provider.ProviderOpenAI
	case Gemini:
		pc.Provider = provider.ProviderGemini
	case Local, Otro:
		pc.Provider = provider.ProviderOpenAICompat
		if cfg.Proveedor == Local && pc.APIKey == "" {
			// Ollama y LM Studio no piden clave, pero el cliente compatible manda una.
			pc.APIKey = "local"
		}
	default:
		return nil, fmt.Errorf("asistente: proveedor desconocido %q", cfg.Proveedor)
	}
	if pc.Model == "" {
		return nil, errors.New("asistente: falta el modelo")
	}
	if pc.APIKey == "" {
		return nil, errors.New("asistente: falta la clave de API")
	}
	return provider.New(pc)
}
