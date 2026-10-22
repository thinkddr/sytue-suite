// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Denunciar y bloquear en Cast (ADR 0074). Quién denuncia y quién bloquea lo pone el
// servicio a partir del token: aquí solo viaja lo denunciado y la persona bloqueada.

// CastMotivos es la lista cerrada del servicio, en el orden en que se enseña.
var CastMotivos = []string{"harassment", "sexual", "violence", "spam", "copyright", "other"}

// Qué se denuncia.
const (
	CastReportVideo   = "video"
	CastReportComment = "comment"
	CastReportChat    = "chat"
	CastReportPerson  = "person"
	CastReportChannel = "channel"
)

// Lo que se hace con una denuncia.
const (
	CastActionDismiss   = "dismiss"
	CastActionDelete    = "delete"
	CastActionUnlist    = "unlist"
	CastActionUnpublish = "unpublish"
	CastActionBlock     = "block"
)

// CastNewReport es una denuncia. Para un canal, Org; para lo demás, VideoID y —si es
// un comentario, un mensaje o una persona— Target (su id o su sub).
type CastNewReport struct {
	Kind    string
	VideoID string
	Org     string
	Target  string
	Reason  string
	Details string
}

func (r CastNewReport) ruta() string {
	if r.Kind == CastReportChannel {
		return "/channels/" + url.PathEscape(r.Org) + "/reports"
	}
	return "/videos/" + url.PathEscape(r.VideoID) + "/reports"
}

type cuerpoDenunciaCast struct {
	Kind    string `json:"kind"`
	Target  string `json:"target,omitempty"`
	Reason  string `json:"reason"`
	Details string `json:"details,omitempty"`
	Altcha  string `json:"altcha,omitempty"`
}

// Report manda una denuncia con sesión. El servicio contesta lo mismo haya o no una
// igual ya abierta.
func (c *Cast) Report(ctx context.Context, r CastNewReport) error {
	_, err := PostJSON[struct{}](ctx, c.c, r.ruta(), cuerpoDenunciaCast{Kind: r.Kind, Target: r.Target, Reason: r.Reason, Details: r.Details})
	return err
}

// PublicReport la manda sin sesión, que solo vale para lo que se ve sin sesión: pide
// la prueba de trabajo, la resuelve aquí (décimas de segundo) y la adjunta.
func (c *Cast) PublicReport(ctx context.Context, r CastNewReport) error {
	reto, err := JSON[retoAltcha](ctx, c.pub, Request{Path: "/reports/altcha", Anonymous: true})
	if err != nil {
		return err
	}
	solucion, err := resolverAltcha(ctx, reto)
	if err != nil {
		return err
	}
	datos, err := json.Marshal(cuerpoDenunciaCast{Kind: r.Kind, Target: r.Target, Reason: r.Reason, Details: r.Details, Altcha: solucion})
	if err != nil {
		return err
	}
	_, err = JSON[struct{}](ctx, c.pub, Request{
		Method: http.MethodPost, Path: r.ruta(), Body: datos, Anonymous: true,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	return err
}

// retoAltcha es el reto de la prueba de trabajo (estándar ALTCHA, `platform/pkg/altcha`).
type retoAltcha struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	MaxNumber int64  `json:"maxnumber"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

// resolverAltcha busca el número `n` ≤ maxnumber tal que SHA-256(salt + n) sea el reto
// y devuelve la carga que el servicio comprueba. Es el mismo contrato que resuelve el
// navegador (`@sytue/config`, altcha.ts): este módulo no puede importar `platform`.
func resolverAltcha(ctx context.Context, r retoAltcha) (string, error) {
	if r.Algorithm != "SHA-256" {
		return "", errors.New("cast: prueba de trabajo de un algoritmo desconocido")
	}
	for n := int64(0); n <= r.MaxNumber; n++ {
		if n%4096 == 0 && ctx.Err() != nil {
			return "", ctx.Err()
		}
		h := sha256.Sum256([]byte(r.Salt + strconv.FormatInt(n, 10)))
		if hex.EncodeToString(h[:]) != r.Challenge {
			continue
		}
		b, err := json.Marshal(struct {
			Algorithm string `json:"algorithm"`
			Challenge string `json:"challenge"`
			Number    int64  `json:"number"`
			Salt      string `json:"salt"`
			Signature string `json:"signature"`
		}{r.Algorithm, r.Challenge, n, r.Salt, r.Signature})
		return base64.StdEncoding.EncodeToString(b), err
	}
	return "", errors.New("cast: la prueba de trabajo no tiene solución")
}

// CastBlocked es una persona bloqueada, por quien mira o por su organización.
type CastBlocked struct {
	Sub       string    `json:"sub"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Block bloquea a alguien que aparece en ese vídeo: su dueño o quien ha escrito en él.
func (c *Cast) Block(ctx context.Context, video, sub string) error {
	_, err := PostJSON[struct{}](ctx, c.c, "/videos/"+url.PathEscape(video)+"/blocks", map[string]string{"sub": sub})
	return err
}

func (c *Cast) bloqueados(ctx context.Context, ruta string) ([]CastBlocked, error) {
	r, err := JSON[struct {
		Blocks []CastBlocked `json:"blocks"`
	}](ctx, c.c, Request{Path: ruta})
	return r.Blocks, err
}

func (c *Cast) desbloquear(ctx context.Context, ruta, sub string) error {
	_, err := JSON[struct{}](ctx, c.c, Request{Method: http.MethodDelete, Path: ruta + "/" + url.PathEscape(sub)})
	return err
}

// Blocks es a quién tiene bloqueado quien mira.
func (c *Cast) Blocks(ctx context.Context) ([]CastBlocked, error) {
	return c.bloqueados(ctx, "/blocks")
}

// Unblock quita un bloqueo propio.
func (c *Cast) Unblock(ctx context.Context, sub string) error {
	return c.desbloquear(ctx, "/blocks", sub)
}

// OrgBlocks es a quién tiene bloqueado la organización; solo para quien la administra.
func (c *Cast) OrgBlocks(ctx context.Context) ([]CastBlocked, error) {
	return c.bloqueados(ctx, "/moderation/blocks")
}

// UnblockOrg quita un bloqueo de la organización.
func (c *Cast) UnblockOrg(ctx context.Context, sub string) error {
	return c.desbloquear(ctx, "/moderation/blocks", sub)
}

// CastReport es una denuncia vista por quien modera. No trae quién denunció: solo si
// tenía cuenta.
type CastReport struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	VideoID    string    `json:"video_id"`
	TargetID   string    `json:"target_id"`
	TargetSub  string    `json:"target_sub"`
	TargetName string    `json:"target_name"`
	Excerpt    string    `json:"excerpt"`
	Reason     string    `json:"reason"`
	Details    string    `json:"details"`
	Anonymous  bool      `json:"anonymous"`
	Status     string    `json:"status"`
	Resolution string    `json:"resolution"`
	CreatedAt  time.Time `json:"created_at"`
	// Actions son las que el servicio admite para esta denuncia.
	Actions []string `json:"actions"`
}

// OpenReports es cuántas denuncias hay sin resolver en la organización. Solo para
// quien la administra (403 para los demás); de paso le apunta como moderador, que es a
// quien el servicio avisa de las nuevas.
func (c *Cast) OpenReports(ctx context.Context) (int, error) {
	r, err := JSON[struct {
		Open int `json:"open"`
	}](ctx, c.c, Request{Path: "/moderation/summary"})
	return r.Open, err
}

// Reports es la cola de la organización: las abiertas, o las resueltas.
func (c *Cast) Reports(ctx context.Context, resueltas bool) ([]CastReport, error) {
	v := url.Values{"status": {"open"}}
	if resueltas {
		v.Set("status", "resolved")
	}
	r, err := JSON[struct {
		Reports []CastReport `json:"reports"`
	}](ctx, c.c, Request{Path: "/moderation/reports", Query: v})
	return r.Reports, err
}

// ResolveReport aplica una de las acciones que la denuncia admite.
func (c *Cast) ResolveReport(ctx context.Context, id, accion string) error {
	_, err := PostJSON[struct{}](ctx, c.c, "/moderation/reports/"+url.PathEscape(id), map[string]string{"action": accion})
	return err
}
