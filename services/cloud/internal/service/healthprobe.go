// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// ---------------------------------------------------------------------------
// Detección de la ruta de salud
// ---------------------------------------------------------------------------
//
// El campo «ruta de salud» obligaba a teclear una cadena mágica a ciegas, y
// equivocarse no da un error: da una app que se reinicia en bucle sin explicar por
// qué. Esto lo cambia por una comprobación: se sondea la app que YA está corriendo
// con las rutas de convención y se enseña cuál responde de verdad.
//
// Sugerir por convención sería adivinar; esto mide. La diferencia importa porque el
// valor que salga de aquí lo va a usar Kubernetes para decidir si la app vive.

// healthProbeTimeout acota CADA candidato. Corto a propósito: son varias peticiones
// seguidas contra la app del cliente y esto ocurre con alguien esperando delante de
// la pantalla. Una ruta de salud que tarda más de 3 s tampoco sirve como sonda.
const healthProbeTimeout = 3 * time.Second

// healthProbeConcurrency limita cuántos candidatos se sondean a la vez. Se sondea
// en paralelo para que la espera sea la del más lento y no la suma, pero acotado:
// esto va contra la app de producción del cliente y no puede parecerle una ráfaga.
const healthProbeConcurrency = 3

// HealthProbeReport es lo que se le devuelve a la interfaz.
type HealthProbeReport struct {
	// URL sondeada (la de producción del proyecto), para que quede claro contra qué
	// se ha comprobado.
	URL string `json:"url"`
	// Results trae TODOS los candidatos con su código, no solo el ganador: si la
	// recomendación no convence, se ve qué contestó cada uno.
	Results []domain.HealthProbeResult `json:"results"`
	// Recommended es la ruta a proponer, o "" si ninguna respondió.
	Recommended string `json:"recommended"`
	// Weak avisa de que la recomendada es la raíz: sirve, pero responde también con
	// la app rota por dentro.
	Weak bool `json:"weak"`
	// Reachable es false cuando NINGÚN candidato dio respuesta HTTP. Distingue «tu
	// app no tiene ruta de salud» de «tu app no está en pie», que no se arreglan
	// igual.
	Reachable bool `json:"reachable"`
}

// ProbeHealthPaths sondea las rutas de convención contra la URL de producción del
// proyecto y devuelve qué contestó cada una.
//
// La URL la construye el SERVIDOR a partir del slug y del dominio de la plataforma;
// no se acepta ninguna del cliente. Aun así el sondeo sale por el cliente endurecido
// del prober de uptime (guard anti-SSRF en el dial, sin seguir redirecciones): un
// dominio de la plataforma que resolviera a una IP interna no puede convertir esto
// en un escáner de la red del clúster.
func (s *Service) ProbeHealthPaths(ctx context.Context, projectID uuid.UUID) (HealthProbeReport, error) {
	proj, err := s.GetProject(ctx, projectID)
	if err != nil {
		return HealthProbeReport{}, err
	}
	base := strings.TrimRight(s.DeploymentURL(proj, domain.TargetProduction, nil), "/")
	report := HealthProbeReport{
		URL:     base,
		Results: make([]domain.HealthProbeResult, len(domain.HealthPathCandidates)),
	}

	client := newProbeClient()
	sem := make(chan struct{}, healthProbeConcurrency)
	var wg sync.WaitGroup
	for i, path := range domain.HealthPathCandidates {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			report.Results[i] = probeOne(ctx, client, base+path, path)
		}(i, path)
	}
	wg.Wait()

	for _, r := range report.Results {
		if r.Status > 0 {
			report.Reachable = true
			break
		}
	}
	report.Recommended = domain.RecommendHealthPath(report.Results)
	report.Weak = report.Recommended != "" && domain.IsWeakHealthPath(report.Recommended)
	return report, nil
}

// probeOne hace UNA petición y traduce la respuesta. Status 0 = no hubo respuesta
// (red, TLS, timeout): no es lo mismo que un 404, y por eso no se colapsan.
func probeOne(ctx context.Context, client *http.Client, url, path string) domain.HealthProbeResult {
	res := domain.HealthProbeResult{Path: path}
	reqCtx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
	defer cancel()
	// HEAD no vale: muchos frameworks no lo enrutan y devolverían 405 en una ruta
	// que con GET responde perfectamente. Se pregunta como preguntará la sonda.
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return res
	}
	req.Header.Set("User-Agent", probeUserAgent)
	start := time.Now()
	resp, err := client.Do(req)
	res.LatencyMs = int(time.Since(start).Milliseconds())
	if err != nil {
		return res
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		_ = resp.Body.Close()
	}()
	res.Status = resp.StatusCode
	// Solo 2xx. Un 3xx —la redirección al login, típica— es una app que arranca, no
	// una app sana, y la sonda de Kubernetes tampoco lo daría por bueno.
	res.Healthy = resp.StatusCode >= 200 && resp.StatusCode < 300
	return res
}

// ---------------------------------------------------------------------------
// Sugerencia de nombre de proyecto
// ---------------------------------------------------------------------------

// SuggestProjectSlug propone un nombre LIBRE para un proyecto nuevo, en el idioma
// negociado de la petición.
//
// Genera y comprueba: proponer sin mirar sería devolver al usuario al mismo 409 del
// que se le quiere sacar, y además con la sensación de que la sugerencia era falsa.
//
// Los candidatos se generan TODOS de golpe y se comprueban con una sola consulta.
// La versión que iba probando de uno en uno hacía hasta 8 viajes a la base de datos
// para una sugerencia; generar de más y preguntar una vez cuesta lo mismo que
// preguntar por el primero.
//
// Si ninguno está libre devuelve el último candidato en vez de un error: una
// sugerencia que quizá esté ocupada sigue siendo mejor punto de partida que un campo
// vacío, y la creación validará de todos modos. Por eso también se dice si está
// confirmado como libre, para que la interfaz no prometa de más.
func (s *Service) SuggestProjectSlug(ctx context.Context, lang string) (slug string, available bool, err error) {
	candidates := make([]string, domain.SlugSuggestionAttempts)
	for i := range candidates {
		candidates[i] = domain.SuggestSlug(lang, i)
	}
	taken, err := s.pstore.SlugsTaken(ctx, candidates)
	if err != nil {
		return "", false, err
	}
	for _, c := range candidates {
		if !taken[c] {
			return c, true, nil
		}
	}
	return candidates[len(candidates)-1], false, nil
}

// SlugCheck es la respuesta a «¿está libre este nombre?».
type SlugCheck struct {
	// Slug es el nombre comprobado, ya normalizado (minúsculas, sin espacios), que
	// es el que hay que enviar al crear.
	Slug string `json:"slug"`
	// Available: nadie en la plataforma lo tiene.
	Available bool `json:"available"`
	// Alternatives son nombres LIBRES derivados del que pidió el usuario. Vacío
	// cuando el suyo está disponible: no hay nada que ofrecer.
	Alternatives []string `json:"alternatives"`
	// Lang es el idioma con el que se construyeron las alternativas, para que el
	// cliente vea qué se negoció.
	Lang string `json:"lang"`
}

// slugAlternativePool son los candidatos que se generan para poder devolver
// slugAlternativesWanted libres. Se piden de más porque los derivados obvios
// («mi-tienda-2») son justo los que otro ya se llevó; caben todos en la MISMA
// consulta, así que el margen no cuesta un viaje extra.
const (
	slugAlternativePool    = 12
	slugAlternativesWanted = 5
)

// CheckSlug dice si el nombre que ha escrito el usuario está libre y, si no lo está,
// ofrece alternativas COMPROBADAS derivadas de él.
//
// Es la mitad que faltaba del sugeridor: sugerir un nombre al azar ayuda a quien no
// tiene ninguno en la cabeza, pero quien ya sabe cómo se llama su proyecto necesita
// saberlo al escribirlo, no al pulsar «crear» y comerse un 409 cuyo motivo no puede
// ver. Las alternativas salen de SU nombre, no del catálogo, porque «mi-tienda-2»
// sigue siendo su tienda y «duna-serena» no.
//
// Todo —el nombre y los candidatos— se comprueba con UNA consulta.
func (s *Service) CheckSlug(ctx context.Context, slug, lang string) (SlugCheck, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !domain.ValidSlug(slug) {
		return SlugCheck{}, fmt.Errorf(
			"%w: el nombre solo admite minúsculas, números y guiones (máx. 40)", domain.ErrInvalidInput,
		)
	}
	res := SlugCheck{Slug: slug, Alternatives: []string{}, Lang: lang}

	candidates := domain.SlugAlternatives(slug, lang, slugAlternativePool)
	taken, err := s.pstore.SlugsTaken(ctx, append([]string{slug}, candidates...))
	if err != nil {
		return SlugCheck{}, err
	}
	res.Available = !taken[slug]
	if res.Available {
		return res, nil
	}
	for _, c := range candidates {
		if taken[c] {
			continue
		}
		res.Alternatives = append(res.Alternatives, c)
		if len(res.Alternatives) == slugAlternativesWanted {
			break
		}
	}
	return res, nil
}
