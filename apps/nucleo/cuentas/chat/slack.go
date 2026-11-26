// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ----- Lo movido del servicio de integraciones (external_chat_slack.go) -----

// Identificadores de Slack que se aceptan en una petición: un canal (`C…`, `D…`, `G…`)
// y la marca de tiempo de un mensaje. Eran `externalSlackID` y `externalSlackTS`.
var (
	IDCanalSlack = regexp.MustCompile(`^[CDG][A-Z0-9]{2,31}$`)
	TSSlack      = regexp.MustCompile(`^\d{1,20}\.\d{1,10}$`)
)

// HoraSlack pasa un `ts` de Slack («1700000000.000200») a tiempo, en UTC. Era la cuenta
// de `externalSlackMessage`.
func HoraSlack(ts string) time.Time {
	seg, frac, _ := strings.Cut(ts, ".")
	s, _ := strconv.ParseInt(seg, 10, 64)
	frac = (frac + "000000000")[:9]
	n, _ := strconv.ParseInt(frac, 10, 64)
	return time.Unix(s, n).UTC()
}

// MensajeSlack es un mensaje tal como lo da la Web API. Lo que usaba el servicio
// (`slackMessage`) más lo que usa Sytue Office.
type MensajeSlack struct {
	Type       string         `json:"type"`
	Subtype    string         `json:"subtype"`
	User       string         `json:"user"`
	BotID      string         `json:"bot_id"`
	Username   string         `json:"username"`
	Text       string         `json:"text"`
	TS         string         `json:"ts"`
	ThreadTS   string         `json:"thread_ts"`
	ReplyCount int            `json:"reply_count"`
	ReplyUsers []string       `json:"reply_users"`
	Files      []FicheroSlack `json:"files"`
	LatestRep  string         `json:"latest_reply"`
	Edited     *struct {
		TS string `json:"ts"`
	} `json:"edited"`
	Reactions []struct {
		Name  string   `json:"name"`
		Count int      `json:"count"`
		Users []string `json:"users"`
	} `json:"reactions"`
}

// FicheroSlack es un fichero de un mensaje.
type FicheroSlack struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Mimetype    string `json:"mimetype"`
	Size        int64  `json:"size"`
	URLPrivada  string `json:"url_private"`
	URLDescarga string `json:"url_private_download"`
	Miniatura   string `json:"thumb_360"`
	Shares      struct {
		Public  map[string][]struct{ TS string } `json:"public"`
		Private map[string][]struct{ TS string } `json:"private"`
	} `json:"shares"`
}

// CanalSlack es una conversación tal como la da la Web API.
type CanalSlack struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	User     string `json:"user"`
	Created  int64  `json:"created"`
	Private  bool   `json:"is_private"`
	IM       bool   `json:"is_im"`
	Mpim     bool   `json:"is_mpim"`
	Archived bool   `json:"is_archived"`
	Member   bool   `json:"is_member"`
	Topic    struct {
		Value string `json:"value"`
	} `json:"topic"`
	Purpose struct {
		Value string `json:"value"`
	} `json:"purpose"`
}

// ErrSlack es lo que contesta la Web API con `ok: false`: su código de error, sin más.
type ErrSlack struct{ Codigo string }

func (e *ErrSlack) Error() string { return "chat: Slack respondió «" + e.Codigo + "»" }

// MetaSlack es la paginación de una respuesta: el cursor de la siguiente (vacío: no hay).
type MetaSlack struct {
	Siguiente string `json:"next_cursor"`
}

// ----- El cliente -----

// Slack es el cliente de la Web API de Slack. En el dispositivo lleva el token de la
// persona (el de su app interna, D-20); en la pasarela, ninguno (lo pone el runtime).
//
// Las tres formas de llamar a un método son las que admite Slack; cada uno usa la suya:
// [Slack.Llamar] (POST de formulario, Sytue Office), [Slack.Consultar] (GET, la pasarela
// al leer) y [Slack.Publicar] (POST con JSON, la pasarela al escribir). Las tres
// comprueban `ok` y dan [*ErrSlack] si es falso.
type Slack struct {
	// Base es la de la Web API (`…/api`): la pone quien lo usa.
	Base   string
	Token  Token
	HTTP   HTTPDoer
	Limite int64
}

func (s *Slack) c() cliente { return cliente{http: s.HTTP, token: s.Token, limite: s.Limite} }

func (s *Slack) metodo(metodo string) (string, error) {
	if metodo == "" || strings.ContainsAny(metodo, "/?#:\\ \r\n") {
		return "", ErrEntrada
	}
	return strings.TrimRight(s.Base, "/") + "/" + metodo, nil
}

// Llamar llama a metodo con los parámetros en un formulario.
func (s *Slack) Llamar(ctx context.Context, metodo string, params url.Values, out any) error {
	d, err := s.metodo(metodo)
	if err != nil {
		return err
	}
	if params == nil {
		params = url.Values{}
	}
	return s.leer(ctx, http.MethodPost, d, params, out)
}

// Consultar llama a metodo por GET con los parámetros en la consulta.
func (s *Slack) Consultar(ctx context.Context, metodo string, params url.Values, out any) error {
	d, err := s.metodo(metodo)
	if err != nil {
		return err
	}
	if len(params) > 0 {
		d += "?" + params.Encode()
	}
	return s.leer(ctx, http.MethodGet, d, nil, out)
}

// Publicar llama a metodo con el cuerpo en JSON.
func (s *Slack) Publicar(ctx context.Context, metodo string, cuerpo, out any) error {
	d, err := s.metodo(metodo)
	if err != nil {
		return err
	}
	if cuerpo == nil {
		cuerpo = map[string]any{}
	}
	return s.leer(ctx, http.MethodPost, d, cuerpo, out)
}

func (s *Slack) leer(ctx context.Context, metodo, destino string, cuerpo, out any) error {
	var raw json.RawMessage
	if err := s.c().pedir(ctx, metodo, destino, cuerpo, &raw); err != nil {
		return err
	}
	var base struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &base); err != nil {
		return ErrRespuesta
	}
	if !base.OK {
		return &ErrSlack{Codigo: base.Error}
	}
	if out != nil && json.Unmarshal(raw, out) != nil {
		return ErrRespuesta
	}
	return nil
}
