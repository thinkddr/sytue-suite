// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package gifcatalog consulta GIF animados CC0 en Commons. Solo guarda metadatos
// públicos y nunca recibe identidad, mensajes ni ficheros de una organización.
package gifcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

const (
	apiURL      = "https://commons.wikimedia.org/w/api.php"
	imageOrigin = "https://upload.wikimedia.org"
	userAgent   = "SytueRooms/1.0 (https://sytue.eu; CC0 GIF catalogue)"
	// MaxBytes mantiene acotadas las descargas y la memoria del selector.
	MaxBytes    = 8 << 20
	pageSize    = 24
	metadataTTL = 5 * time.Minute
)

// ErrUnavailable incluye los archivos que no acreditan la licencia permitida.
var ErrUnavailable = errors.New("GIF unavailable or not CC0")

// Item es un resultado: la imagen se pide a Rooms, nunca al host de Commons.
type Item struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Source  string `json:"source"`
	License string `json:"license"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Size    int64  `json:"size"`
}

// Page contiene una única tanda. Next es el cursor de Commons, no un máximo del catálogo.
type Page struct {
	Items []Item `json:"items"`
	Next  string `json:"next,omitempty"`
}

type metadata struct {
	Value string `json:"value"`
}
type imageInfo struct {
	URL      string              `json:"url"`
	Mime     string              `json:"mime"`
	Size     int64               `json:"size"`
	Width    int                 `json:"width"`
	Height   int                 `json:"height"`
	Duration float64             `json:"duration"`
	Metadata map[string]metadata `json:"extmetadata"`
}
type commonsPage struct {
	ID     int64       `json:"pageid"`
	Title  string      `json:"title"`
	Index  int         `json:"index"`
	Images []imageInfo `json:"imageinfo"`
}
type response struct {
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
	Continue struct {
		Offset int `json:"gsroffset"`
	} `json:"continue"`
	Query struct {
		Pages []commonsPage `json:"pages"`
	} `json:"query"`
}
type cached struct {
	page  commonsPage
	until time.Time
}

// Catalog mantiene como máximo 256 metadatos durante cinco minutos. No cachea
// originales animados ni deja crecer la memoria al recorrer el catálogo.
type Catalog struct {
	http  *http.Client
	mu    sync.Mutex
	items map[int64]cached
}

// New usa las barreras SSRF comunes y exige el mismo host en cada redirección.
func New() *Catalog {
	h := ssrfhttp.NewClient(15*time.Second, 3)
	previous := h.CheckRedirect
	h.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if !allowedURL(r.URL.String(), r.URL.Hostname()) {
			return ErrUnavailable
		}
		if r.URL.Hostname() != "commons.wikimedia.org" && r.URL.Hostname() != "upload.wikimedia.org" {
			return ErrUnavailable
		}
		if len(via) > 0 && r.URL.Hostname() != via[0].URL.Hostname() {
			return ErrUnavailable
		}
		return previous(r, via)
	}
	return &Catalog{http: h, items: make(map[int64]cached)}
}

func params() url.Values {
	return url.Values{
		"action":              {"query"},
		"format":              {"json"},
		"formatversion":       {"2"},
		"prop":                {"imageinfo"},
		"iiprop":              {"url|size|mime|extmetadata"},
		"iiextmetadatafilter": {"LicenseShortName|LicenseUrl|AttributionRequired|Restrictions"},
	}
}

func allowedURL(raw, host string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == host && u.User == nil
}

// licensed falla cerrado: CC0 1.0, sin atribución exigida ni restricciones, MIME
// real GIF y duración positiva. «Public domain» sin jurisdicción se excluye.
func licensed(p commonsPage) bool {
	if p.ID <= 0 || len(p.Images) != 1 {
		return false
	}
	i := p.Images[0]
	return cc0(i.Metadata) && validImage(i) && allowedURL(i.URL, strings.TrimPrefix(imageOrigin, "https://"))
}

func cc0(m map[string]metadata) bool {
	license, err := url.Parse(m["LicenseUrl"].Value)
	if err != nil || (license.Scheme != "http" && license.Scheme != "https") || license.Host != "creativecommons.org" || license.User != nil || !strings.HasPrefix(license.Path, "/publicdomain/zero/1.0/") {
		return false
	}
	if _, ok := m["Restrictions"]; !ok {
		return false
	}
	return m["LicenseShortName"].Value == "CC0" && m["AttributionRequired"].Value == "false" && m["Restrictions"].Value == ""
}

func validImage(i imageInfo) bool {
	return i.Mime == "image/gif" && i.Duration > 0 && i.Size > 0 && i.Size <= MaxBytes && i.Width > 0 && i.Height > 0 && i.Width <= 1024 && i.Height <= 1024
}

func item(p commonsPage) Item {
	i := p.Images[0]
	return Item{
		ID: p.ID, Title: strings.TrimSuffix(strings.TrimPrefix(p.Title, "File:"), ".gif"),
		Source:  "https://commons.wikimedia.org/w/index.php?curid=" + strconv.FormatInt(p.ID, 10),
		License: "CC0 1.0", Width: i.Width, Height: i.Height, Size: i.Size,
	}
}

func (c *Catalog) remember(p commonsPage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for id, entry := range c.items {
		if entry.until.Before(now) {
			delete(c.items, id)
		}
	}
	if len(c.items) >= 256 {
		for id := range c.items {
			delete(c.items, id)
			break
		}
	}
	c.items[p.ID] = cached{p, now.Add(metadataTTL)}
}

func (c *Catalog) query(ctx context.Context, q url.Values) (response, error) {
	var out response
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"?"+q.Encode(), http.NoBody)
	if err != nil {
		return out, err
	}
	r.Header.Set("User-Agent", userAgent)
	r.Header.Set("Accept", "application/json")
	res, err := c.http.Do(r)
	if err != nil {
		return out, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return out, fmt.Errorf("commons: status %d", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return out, err
	}
	if out.Error != nil {
		return out, ErrUnavailable
	}
	return out, nil
}

// Search busca términos literales en todo el índice, sin permitir inyectar
// operadores que retiren los filtros de formato/licencia. Solo baja una página.
func (c *Catalog) Search(ctx context.Context, text, cursor string) (Page, error) {
	out := Page{Items: []Item{}}
	if len(text) > 200 {
		return out, ErrUnavailable
	}
	offset := 0
	if cursor != "" {
		var err error
		offset, err = strconv.Atoi(cursor)
		if err != nil || offset < 0 || offset > 10000 {
			return out, ErrUnavailable
		}
	}
	q := params()
	terms := `filemime:"image/gif" filesize:<8192 haswbstatement:P275=Q6938433`
	for _, term := range strings.Fields(text) {
		terms += " " + strconv.Quote(term)
	}
	q.Set("generator", "search")
	q.Set("gsrsearch", terms)
	q.Set("gsrnamespace", "6")
	q.Set("gsrlimit", strconv.Itoa(pageSize))
	q.Set("gsroffset", strconv.Itoa(offset))
	res, err := c.query(ctx, q)
	if err != nil {
		return out, err
	}
	sort.Slice(res.Query.Pages, func(i, j int) bool { return res.Query.Pages[i].Index < res.Query.Pages[j].Index })
	for _, p := range res.Query.Pages {
		if licensed(p) {
			out.Items = append(out.Items, item(p))
			c.remember(p)
		}
	}
	if res.Continue.Offset > offset && res.Continue.Offset <= 10000 {
		out.Next = strconv.Itoa(res.Continue.Offset)
	}
	return out, nil
}

func (c *Catalog) resolve(ctx context.Context, id int64) (commonsPage, error) {
	c.mu.Lock()
	e, ok := c.items[id]
	c.mu.Unlock()
	if ok && e.until.After(time.Now()) {
		return e.page, nil
	}
	q := params()
	q.Set("pageids", strconv.FormatInt(id, 10))
	res, err := c.query(ctx, q)
	if err != nil {
		return commonsPage{}, err
	}
	if len(res.Query.Pages) != 1 || res.Query.Pages[0].ID != id || !licensed(res.Query.Pages[0]) {
		return commonsPage{}, ErrUnavailable
	}
	c.remember(res.Query.Pages[0])
	return res.Query.Pages[0], nil
}

// Content comprueba de nuevo la licencia cuando no está en la caché reciente,
// descarga de un host fijo y limita los bytes incluso si el servidor no los declara.
func (c *Catalog) Content(ctx context.Context, id int64) ([]byte, error) {
	if id <= 0 {
		return nil, ErrUnavailable
	}
	p, err := c.resolve(ctx, id)
	if err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Images[0].URL, http.NoBody)
	if err != nil {
		return nil, err
	}
	r.Header.Set("User-Agent", userAgent)
	res, err := c.http.Do(r)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if validation.AnimatedGIF(data, MaxBytes, 1024, 16<<20) != nil {
		return nil, ErrUnavailable
	}
	return data, nil
}
