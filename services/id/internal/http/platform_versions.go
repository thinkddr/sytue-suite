// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Qué versión corre CADA pieza de la plataforma.
//
// El panel enseñaba dos: la propia consola y Sytue ID. Con eso se responde «¿el
// frontend y el IdP van a la par?», que es media pregunta: una suite son trece
// servicios Go y once superficies web, y un despliegue se queda a medias por
// cualquiera de las veinticuatro. Saber que dos coinciden no dice nada de las
// otras veintidós, y el fallo típico —un rollout que no terminó en un chart
// concreto— es justo el que esas dos no ven.
//
// EL ABANICO VA AQUÍ Y NO EN EL BFF. La regla de la casa es que el backend es Go
// y el BFF solo hace de proxy; además, preguntar a veinticuatro sitios desde el
// navegador serían veinticuatro peticiones atravesando el ingress, cuando desde
// dentro del clúster son veinticuatro llamadas a DNS interno que no salen.
//
// NO SE INVENTA NADA. Un componente que no contesta sale como «no contesta», que
// es información —probablemente lo que alguien vino a buscar—, y nunca se
// rellena con la versión del release ni con la de un vecino. Un panel de
// versiones que adivina es peor que no tenerlo: convierte un despliegue roto en
// uno que parece bueno.
package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// componentTimeout acota CADA sondeo. Corto a propósito: son veinticuatro en
// paralelo y esta pantalla se mira cuando algo va mal, o sea justo cuando alguno
// va a tardar. Mejor decir «no contesta» en dos segundos que dejar la tabla
// entera esperando al más lento.
const componentTimeout = 2 * time.Second

// clienteSondeo se comparte entre sondeos. Uno nuevo por llamada dejaría
// veinticuatro conexiones a medio cerrar en cada refresco del panel; con uno
// solo, el pool las reutiliza. El tope real lo pone el contexto de cada sondeo:
// el del cliente es solo el cinturón de seguridad.
var clienteSondeo = &http.Client{Timeout: componentTimeout}

// Component es una pieza desplegada a la que se le puede preguntar la versión.
type Component struct {
	// Name es el identificador corto (`id`, `drive`, `fe-console`).
	Name string
	// Kind separa los servicios Go de las superficies web. La tabla los agrupa:
	// son dos cadenas de despliegue distintas y se rompen por separado.
	Kind string
	// URL es la base INTERNA del componente; se le pide /healthz.
	URL string
}

// ComponentVersion es lo que se responde por cada pieza.
type ComponentVersion struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Release string `json:"release"`
	Commit  string `json:"commit"`
	// OK false ⇒ no contestó (o contestó mal). `Error` dice por qué, en corto.
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// ParseComponents lee el catálogo del entorno. Formato: `nombre|tipo|url`,
// separados por comas.
//
// Una lista y no una convención de nombres calculada en Go: quien sabe qué está
// desplegado es el chart paraguas, que es el que enumera los subcharts y las
// superficies. Deducirlo aquí obligaría a que el binario supiera los nombres de
// los Service de Kubernetes, y entonces añadir un servicio exigiría tocar Go
// para algo que ya está escrito en el sitio donde se despliega.
//
// Las entradas mal formadas se DESCARTAN en silencio en vez de tumbar el
// arranque: esta pantalla es informativa, y dejar sin IdP a toda la suite porque
// sobra una coma en una lista de diagnóstico sería un pésimo intercambio.
func ParseComponents(raw string) []Component {
	out := make([]Component, 0, 24)
	for _, entrada := range strings.Split(raw, ",") {
		entrada = strings.TrimSpace(entrada)
		if entrada == "" {
			continue
		}
		partes := strings.Split(entrada, "|")
		if len(partes) != 3 {
			continue
		}
		name := strings.TrimSpace(partes[0])
		kind := strings.TrimSpace(partes[1])
		url := strings.TrimSuffix(strings.TrimSpace(partes[2]), "/")
		if name == "" || url == "" {
			continue
		}
		if kind == "" {
			kind = "service"
		}
		out = append(out, Component{Name: name, Kind: kind, URL: url})
	}
	return out
}

// versionDe pregunta a un componente. Nunca devuelve error: el fallo ES el dato.
func (s *Server) versionDe(ctx context.Context, c Component) ComponentVersion {
	out := ComponentVersion{Name: c.Name, Kind: c.Kind}
	ctx, cancel := context.WithTimeout(ctx, componentTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"/healthz", http.NoBody)
	if err != nil {
		out.Error = "url inválida"
		return out
	}
	req.Header.Set("Accept", "application/json")
	res, err := clienteSondeo.Do(req)
	if err != nil {
		// El mensaje de red trae la URL y el puerto; no aporta nada a quien mira
		// el panel y sí ensancha la tabla. El detalle está en el log del IdP.
		out.Error = "no contesta"
		return out
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		out.Error = "respondió " + res.Status
		return out
	}
	// Tope de lectura: /healthz es un objeto de tres campos. Sin él, un
	// componente que devolviera un flujo infinito por la razón que fuera se
	// llevaría por delante la memoria del IdP.
	var cuerpo struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<10)).Decode(&cuerpo); err != nil {
		out.Error = "respuesta ilegible"
		return out
	}
	out.OK = true
	out.Release = cuerpo.Version
	out.Commit = cuerpo.Commit
	return out
}

// handlePlatformVersions — GET /api/v1/platform/versions.
//
// Devuelve TODOS los componentes, contesten o no, para que la tabla tenga
// siempre las mismas filas: una fila que desaparece cuando el servicio se cae es
// la peor forma posible de enterarse de que se ha caído.
func (s *Server) handlePlatformVersions(w http.ResponseWriter, r *http.Request) {
	comps := s.cfg.Components
	items := make([]ComponentVersion, len(comps))
	// El contexto se pasa como PARÁMETRO en vez de capturarlo del request dentro
	// de la goroutine. Aquí da igual en la práctica —`wg.Wait()` retiene al
	// handler hasta que todas terminan, así que el contexto no se cancela debajo—
	// pero capturarlo deja la dependencia implícita: el día que alguien quite ese
	// Wait, o mueva estas consultas a segundo plano, se cancelarían todas al
	// responder y el fallo aparecería como un timeout aleatorio.
	ctx := r.Context()
	var wg sync.WaitGroup
	for i, c := range comps {
		wg.Add(1)
		go func(ctx context.Context, i int, c Component) {
			defer wg.Done()
			items[i] = s.versionDe(ctx, c)
		}(ctx, i, c)
	}
	wg.Wait()

	// Orden estable: por tipo y luego por nombre. La tabla no puede bailar entre
	// refrescos —se compara a ojo— y el orden de llegada es el de la red.
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].Kind != items[b].Kind {
			return items[a].Kind < items[b].Kind
		}
		return items[a].Name < items[b].Name
	})

	// `expected` es la versión que la mayoría declara. No se toma la del IdP como
	// referencia: si el que se quedó atrás es él, tomarlo como patrón marcaría a
	// los veintitrés correctos como desviados y dejaría limpio al único roto.
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"items":    items,
		"expected": versionMayoritaria(items),
	})
}

// versionMayoritaria devuelve la release que más componentes declaran. Vacía si
// no hay ninguna o si hay empate: sin mayoría clara, señalar desviados sería
// inventarse cuál es el bando bueno.
func versionMayoritaria(items []ComponentVersion) string {
	cuenta := map[string]int{}
	for _, it := range items {
		if it.OK && it.Release != "" {
			cuenta[it.Release]++
		}
	}
	mejor, empate := "", false
	for v, n := range cuenta {
		switch {
		case n > cuenta[mejor]:
			mejor, empate = v, false
		case v != mejor && n == cuenta[mejor]:
			empate = true
		}
	}
	if empate {
		return ""
	}
	return mejor
}
