// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// Redirecciones y cabeceras del proyecto
// ---------------------------------------------------------------------------
//
// Es lo que en Vercel se escribe en `vercel.json` y en Netlify en `_redirects`:
// mover una URL que cambió sin perder el enlace de nadie, y poner las cabeceras
// que la app no pone. Aquí no había forma, así que la única salida era
// resolverlo dentro del código de cada aplicación —y en un sitio estático, ni
// eso—.
//
// El cliente NO escribe expresiones regulares. Escribe rutas, y la plataforma las
// traduce: una regex de un tercero en el borde es un problema de rendimiento
// (retroceso catastrófico) y de seguridad que no compensa por la comodidad de
// unos pocos.

// Límites. Están juntos y nombrados porque son la respuesta a «cuántas caben», y
// el motivo es el mismo para los dos: cada regla es un objeto en el clúster.
const (
	MaxRedirects = 20
	MaxHeaders   = 20
)

// Redirect es una redirección de una ruta a otra.
type Redirect struct {
	// From es la ruta de origen. Empieza por «/» y admite un «*» final para decir
	// «y todo lo que cuelgue de aquí».
	From string `json:"from"`
	// To es el destino: una ruta del mismo sitio («/nuevo») o una URL https
	// completa. Con un origen comodín, el resto de la ruta se conserva.
	To string `json:"to"`
	// Status es 301, 302, 307 o 308. Por defecto 308, que preserva el método.
	Status int `json:"status,omitempty"`
}

// HTTPRules es la configuración de borde de un proyecto.
type HTTPRules struct {
	Redirects []Redirect        `json:"redirects,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// Scan y Value hacen que HTTPRules viaje sola a la columna jsonb.
//
// Sí, esto acopla un tipo del dominio a `database/sql`. La alternativa era una
// copia del struct en el store y dos conversiones a mano en cada consulta que lee
// un proyecto (que son seis): dos métodos pesan menos que una estructura
// duplicada que se desincroniza.
func (r *HTTPRules) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*r = HTTPRules{}
		return nil
	case []byte:
		if len(v) == 0 {
			*r = HTTPRules{}
			return nil
		}
		return json.Unmarshal(v, r)
	case string:
		if v == "" {
			*r = HTTPRules{}
			return nil
		}
		return json.Unmarshal([]byte(v), r)
	default:
		return fmt.Errorf("domain: no se puede leer HTTPRules desde %T", src)
	}
}

// Value serializa las reglas. Nunca devuelve NULL: la columna es NOT NULL con
// defecto `{}`, y un nil aquí obligaría a que cada INSERT se acordara del caso.
func (r HTTPRules) Value() (driver.Value, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// headerNameRe es un token de cabecera HTTP (RFC 7230). Sin esto, un nombre con
// dos puntos o espacios rompería la respuesta entera.
var headerNameRe = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+.^_` + "`" + `|~-]+$`)

// headersProhibidas son las que gobierna el propio servidor. Dejar que el cliente
// las escriba no le da nada y sí rompe la conexión: un Content-Length inventado
// deja la respuesta a medias y un Transfer-Encoding ajeno es un desincronizador
// de peticiones de manual.
var headersProhibidas = map[string]bool{
	"host":              true,
	"content-length":    true,
	"transfer-encoding": true,
	"connection":        true,
	"upgrade":           true,
	"keep-alive":        true,
}

// ValidateHTTPRules comprueba las reglas de borde de un proyecto.
func ValidateHTTPRules(r HTTPRules) error {
	if len(r.Redirects) > MaxRedirects {
		return fmt.Errorf("%w: como mucho %d redirecciones", ErrInvalidInput, MaxRedirects)
	}
	if len(r.Headers) > MaxHeaders {
		return fmt.Errorf("%w: como mucho %d cabeceras", ErrInvalidInput, MaxHeaders)
	}
	for _, rd := range r.Redirects {
		if err := validateRedirect(rd); err != nil {
			return err
		}
	}
	for name, value := range r.Headers {
		if !headerNameRe.MatchString(name) {
			return fmt.Errorf("%w: nombre de cabecera inválido: %q", ErrInvalidInput, name)
		}
		if headersProhibidas[strings.ToLower(name)] {
			return fmt.Errorf("%w: la cabecera %q la gobierna el servidor", ErrInvalidInput, name)
		}
		// Un salto de línea en el VALOR permite escribir cabeceras enteras de más
		// (CRLF injection). Es la razón por la que este bucle existe.
		if strings.ContainsAny(value, "\r\n") || len(value) > 1024 {
			return fmt.Errorf("%w: valor inválido para la cabecera %q", ErrInvalidInput, name)
		}
	}
	return nil
}

func validateRedirect(r Redirect) error {
	if !strings.HasPrefix(r.From, "/") || len(r.From) > 512 {
		return fmt.Errorf("%w: el origen de la redirección tiene que empezar por «/»", ErrInvalidInput)
	}
	// El comodín solo al final: en medio no significa nada obvio y el usuario
	// esperaría cosas distintas según de dónde venga.
	if i := strings.Index(r.From, "*"); i >= 0 && i != len(r.From)-1 {
		return fmt.Errorf("%w: el «*» solo puede ir al final del origen", ErrInvalidInput)
	}
	if strings.ContainsAny(r.From, "\r\n") || strings.ContainsAny(r.To, "\r\n") {
		return fmt.Errorf("%w: la redirección no puede llevar saltos de línea", ErrInvalidInput)
	}
	dest := strings.TrimSpace(r.To)
	if dest == "" || len(dest) > 1024 {
		return fmt.Errorf("%w: destino de redirección inválido", ErrInvalidInput)
	}
	if !strings.HasPrefix(dest, "/") && !strings.HasPrefix(dest, "https://") {
		return fmt.Errorf("%w: el destino tiene que ser una ruta o una URL https", ErrInvalidInput)
	}
	switch r.Status {
	case 0, 301, 302, 307, 308:
		return nil
	default:
		return fmt.Errorf("%w: el código de redirección tiene que ser 301, 302, 307 o 308", ErrInvalidInput)
	}
}

// RedirectRegex traduce el origen de una redirección a la expresión que entiende
// Traefik, y el destino a su plantilla de reemplazo.
//
// Aquí está el motivo de que el cliente no escriba regex: TODO lo suyo se escapa
// con QuoteMeta, así que un punto es un punto y un paréntesis es un paréntesis.
// Lo único que se convierte en patrón es el comodín final, que produce un grupo
// cuyo contenido se pega al destino.
func RedirectRegex(r Redirect) (pattern, replacement string) {
	origen := r.From
	comodin := strings.HasSuffix(origen, "*")
	if comodin {
		origen = strings.TrimSuffix(origen, "*")
	}
	// El host va en `.*` porque la misma app responde en varios (dominio de
	// plataforma, dominios propios) y la regla es del CAMINO, no del sitio.
	pattern = `^https?://[^/]+` + regexp.QuoteMeta(origen)
	replacement = r.To
	if comodin {
		pattern += `(.*)$`
		replacement += "${1}"
	} else {
		pattern += `/?$`
	}
	return pattern, replacement
}

// RedirectStatus es el código efectivo: 308 cuando no se indica.
//
// 308 y no 301 a propósito: el 301 permite al navegador convertir un POST en GET,
// que es la clase de sorpresa que aparece semanas después en un formulario.
func RedirectStatus(r Redirect) int {
	if r.Status == 0 {
		return 308
	}
	return r.Status
}
