// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package personas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// vidaNombre es cuánto se recuerda un nombre: un cambio de nombre tarda eso en
// llegar a la campana, y una página de avisos no pregunta a Sytue ID cada vez.
const vidaNombre = 10 * time.Minute

// NombresOrgs da el nombre visible de las organizaciones por su slug, con
// `POST /internal/orgs/names` de Sytue ID y su secreto propio. Solo se le pregunta por
// las organizaciones de lo que ya va dirigido a la persona (un aviso, una invitación):
// esa es la verja, y es de quien llama. Lo usan el Hub, Rooms y Calendar.
//
// ponytail: caché en memoria por réplica y sin tope de tamaño; la acotan las
// organizaciones que avisan a alguien de fuera. Con muchas, un LRU.
type NombresOrgs struct {
	url, secreto string
	http         *http.Client
	mu           sync.Mutex
	cache        map[string]nombreEnCache
}

type nombreEnCache struct {
	nombre string
	hasta  time.Time
}

// NuevosNombresOrgs pregunta a Sytue ID en `base`; sin secreto devuelve nil.
func NuevosNombresOrgs(base, secreto string) *NombresOrgs {
	if secreto == "" {
		return nil
	}
	return &NombresOrgs{
		url: strings.TrimRight(base, "/") + "/internal/orgs/names", secreto: secreto,
		http: &http.Client{Timeout: 2 * time.Second}, cache: map[string]nombreEnCache{},
	}
}

// Nombres pregunta a Sytue ID por lo que no tiene en caché. Si no contesta, se queda
// con lo que tenía: el aviso sale igual, con el identificador.
func (c *NombresOrgs) Nombres(ctx context.Context, slugs []string) map[string]string {
	out := map[string]string{}
	if c == nil || len(slugs) == 0 {
		return out
	}
	var faltan []string
	ahora := time.Now()
	c.mu.Lock()
	for _, s := range slugs {
		if e, ok := c.cache[s]; ok && ahora.Before(e.hasta) {
			if e.nombre != "" {
				out[s] = e.nombre
			}
		} else {
			faltan = append(faltan, s)
		}
	}
	c.mu.Unlock()
	if len(faltan) == 0 {
		return out
	}
	nuevos, err := c.pedir(ctx, faltan)
	if err != nil {
		log.From(ctx).Warn("personas: nombres de organizaciones", "error", err)
		return out
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range faltan {
		// Uno sin nombre también se recuerda: no se vuelve a preguntar en cada página.
		c.cache[s] = nombreEnCache{nombre: nuevos[s], hasta: ahora.Add(vidaNombre)}
		if nuevos[s] != "" {
			out[s] = nuevos[s]
		}
	}
	return out
}

func (c *NombresOrgs) pedir(ctx context.Context, slugs []string) (map[string]string, error) {
	cuerpo, _ := json.Marshal(map[string][]string{"slugs": slugs})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(cuerpo))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secreto)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("sytue id: /internal/orgs/names respondió " + resp.Status)
	}
	var out struct {
		Orgs []struct{ Slug, Name string } `json:"orgs"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}
	nombres := make(map[string]string, len(out.Orgs))
	for _, o := range out.Orgs {
		nombres[o.Slug] = o.Name
	}
	return nombres, nil
}
