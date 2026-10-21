// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config son las opciones globales, resueltas de flags + entorno.
type Config struct {
	API    string
	Token  string
	Output string // table|json
}

// bindGlobals registra las flags globales en fs y devuelve el Config que se
// rellena al hacer fs.Parse. --token queda en el history del shell: prioriza
// $SYTUE_CLOUD_TOKEN.
func bindGlobals(fs *flag.FlagSet) *Config {
	c := &Config{}
	fs.StringVar(&c.API, "api", envOr("SYTUE_CLOUD_API", "http://127.0.0.1:8090"), "URL base de la API ($SYTUE_CLOUD_API)")
	fs.StringVar(&c.Token, "token", os.Getenv("SYTUE_CLOUD_TOKEN"), "Bearer token; mejor via $SYTUE_CLOUD_TOKEN (--token queda en el history)")
	def := envOr("SYTUE_CLOUD_OUTPUT", "table")
	fs.StringVar(&c.Output, "output", def, "formato de salida: table|json")
	fs.StringVar(&c.Output, "o", def, "alias de --output")
	return c
}

// parseFlags parsea flags intercalados con argumentos posicionales. La stdlib
// `flag` para en el primer posicional; este bucle recoge los posicionales y
// sigue parseando, así `cmd <id> -o json` y `cmd -o json <id>` funcionan igual.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// Client es un cliente HTTP delgado sobre la API del control plane.
type Client struct {
	api   string
	token string
	hc    *http.Client
}

func newClient(c *Config) (*Client, error) {
	if c.Output != "table" && c.Output != "json" {
		return nil, fmt.Errorf("--output invalido %q (usa table|json)", c.Output)
	}
	if c.Token == "" {
		return nil, fmt.Errorf("falta token: exporta $SYTUE_CLOUD_TOKEN o pasa --token")
	}
	return &Client{
		api:   strings.TrimRight(c.API, "/"),
		token: c.Token,
		hc:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// request ejecuta method c.api+path con body opcional (JSON) y devuelve el
// cuerpo crudo. Mapea status>=400 a un error tipado sin volcar secretos.
func (cl *Client) request(ctx context.Context, method, path string, body any) ([]byte, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, cl.api+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cl.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := cl.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, apiError(resp.StatusCode, data)
	}
	return data, nil
}

// apiError construye el error a partir del cuerpo tipado {error:{code,message}}.
// Si el cuerpo no es ese contrato, solo reporta el status (no vuelca el body,
// que podría llevar datos sensibles).
func apiError(status int, body []byte) error {
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		if e.Error.Code != "" {
			return fmt.Errorf("API %d (%s): %s", status, e.Error.Code, e.Error.Message)
		}
		return fmt.Errorf("API %d: %s", status, e.Error.Message)
	}
	return fmt.Errorf("API %d %s", status, http.StatusText(status))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
