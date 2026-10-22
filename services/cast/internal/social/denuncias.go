// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package social

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

// Denunciar contenido o personas (docs/decisions.md, «Cast: denuncia y bloqueo»).
//
// La denuncia vive en la organización del CONTENIDO —la del vídeo o la del
// canal—, no en la de quien denuncia, y le llega a quien puede actuar: quienes
// administran esa organización, en una cola. Quien denuncia no sale nunca por la
// API: ni al denunciado ni a quien modera. Se guarda para la auditoría.
//
// Nadie ve una denuncia contra sí mismo, administre lo que administre (como en
// Rooms, ADR 0073): ni en la cola, ni en la cuenta de abiertas, ni en la traza,
// ni para resolverla. Esas las ve el operador de la plataforma.

// Qué se denuncia.
const (
	DenunciaVideo      = "video"
	DenunciaComentario = "comment"
	DenunciaChat       = "chat"
	DenunciaPersona    = "person"
	DenunciaCanal      = "channel"
)

// Qué se hace con una denuncia.
const (
	// AccionDescartar la cierra sin tocar nada.
	AccionDescartar = "dismiss"
	// AccionBorrar retira el comentario o el mensaje denunciado.
	AccionBorrar = "delete"
	// AccionRetirar saca el vídeo de la portada y del canal: pasa a `unlisted`.
	AccionRetirar = "unlist"
	// AccionDespublicar lo deja solo para su organización: pasa a `org`.
	AccionDespublicar = "unpublish"
	// AccionBloquear impide a la persona denunciada comentar y escribir en toda la
	// organización.
	AccionBloquear = "block"
)

// MaxDetalle es el tope del texto libre, el mismo que el CHECK de la migración.
const MaxDetalle = 1000

// maxExtracto es lo que se guarda del contenido denunciado.
const maxExtracto = 300

// ErrBloqueado: quien escribe está bloqueado por el dueño del vídeo o por su
// organización.
var ErrBloqueado = errors.New("social: bloqueado")

// motivos es la lista cerrada, la misma que el CHECK de la migración.
var motivos = map[string]bool{
	"harassment": true, "sexual": true, "violence": true, "spam": true, "copyright": true, "other": true,
}

// NuevaDenuncia es lo que manda quien denuncia.
type NuevaDenuncia struct {
	Kind string
	// Target es el id del comentario o del mensaje, o el sub de la persona. Para un
	// vídeo o un canal no hace falta: lo dice la ruta.
	Target  string
	Reason  string
	Details string
}

// Denuncia es una fila de la cola, tal como la ve quien modera. No lleva quién
// denunció: solo si tenía cuenta.
type Denuncia struct {
	ID         uuid.UUID  `json:"id"`
	Kind       string     `json:"kind"`
	VideoID    *uuid.UUID `json:"video_id,omitempty"`
	TargetID   string     `json:"target_id"`
	TargetSub  string     `json:"target_sub,omitempty"`
	TargetName string     `json:"target_name,omitempty"`
	Excerpt    string     `json:"excerpt,omitempty"`
	Reason     string     `json:"reason"`
	Details    string     `json:"details,omitempty"`
	Anonymous  bool       `json:"anonymous"`
	Status     string     `json:"status"`
	Resolution string     `json:"resolution,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	// ResolvedFrom dice desde qué cola se resolvió: "org" o "platform".
	ResolvedFrom string `json:"resolved_from,omitempty"`
	// Tenant es la organización dueña del contenido. Solo la usa la cola del
	// operador, que es de todas; en la de una organización es la suya.
	Tenant string `json:"tenant"`
	// Actions son las acciones que admite: depende de qué se denunció.
	Actions []string `json:"actions"`
}

// Moderacion es lo que la denuncia necesita de fuera y `New` no recibe.
type Moderacion struct {
	// Hub avisa a quien modera. nil = sin avisos (la cola sigue siendo la verdad).
	Hub *hubnotify.Client
	// PublicURL es la web de Cast, para el enlace del aviso.
	PublicURL string
	// Secreto trocea la IP de quien denuncia sin cuenta. Vacío = sin denuncia anónima.
	Secreto []byte
}

// ConModeracion enchufa los avisos y la clave de las denuncias anónimas.
func (s *Servicio) ConModeracion(m Moderacion) *Servicio {
	s.mod = m
	return s
}

// claveDe es la clave de deduplicación de quien denuncia: su sub, o un HMAC de su
// IP si no tiene cuenta (la IP no se guarda).
func (s *Servicio) claveDe(quien Quien) (string, error) {
	if !quien.Anonimo && quien.Sub != "" {
		return "sub:" + quien.Sub, nil
	}
	if len(s.mod.Secreto) == 0 || quien.IP == "" {
		return "", ErrProhibido
	}
	mac := hmac.New(sha256.New, s.mod.Secreto)
	mac.Write([]byte("cast-report:" + quien.IP))
	return "anon:" + hex.EncodeToString(mac.Sum(nil)[:16]), nil
}

func validarDenuncia(d *NuevaDenuncia) error {
	if !motivos[d.Reason] {
		return fmt.Errorf("%w: motivo desconocido", ErrEntradaInvalida)
	}
	d.Details = strings.TrimSpace(d.Details)
	if utf8.RuneCountInString(d.Details) > MaxDetalle {
		return fmt.Errorf("%w: el detalle pasa de %d caracteres", ErrEntradaInvalida, MaxDetalle)
	}
	return nil
}

// Denunciar registra una denuncia sobre un vídeo, o sobre un comentario, un
// mensaje o una persona DENTRO de ese vídeo. Sin sesión solo vale para lo que se
// ve sin sesión (vídeos public o unlisted y sus comentarios). Repetirla es
// inocuo: una abierta por persona y objetivo.
func (s *Servicio) Denunciar(ctx context.Context, quien Quien, videoID uuid.UUID, d NuevaDenuncia) error {
	if err := validarDenuncia(&d); err != nil {
		return err
	}
	clave, err := s.claveDe(quien)
	if err != nil {
		return err
	}
	if res, err := s.ritmoDenuncias.Allow(ctx, clave); err != nil || !res.Allowed {
		return ErrDemasiadoRapido
	}
	v, _, err := s.resolver(ctx, quien, videoID)
	if err != nil {
		return err
	}
	if quien.Anonimo && d.Kind == DenunciaChat {
		// El chat no se lee sin sesión: tampoco se denuncia.
		return ErrProhibido
	}
	f := fila{tenant: v.Tenant, kind: d.Kind, videoID: &v.ID, reason: d.Reason, details: d.Details, clave: clave}
	if !quien.Anonimo {
		f.reporterSub, f.reporterTenant = quien.Sub, quien.Tenant
	}
	nueva, err := s.store.denunciar(ctx, v, d.Target, &f)
	if err != nil || !nueva {
		return err
	}
	s.avisarDenuncia(ctx, v, f)
	return nil
}

// DenunciarCanal registra una denuncia sobre el canal público de una
// organización. `existe` dice si ese canal se ve: lo comprueba la capa HTTP con
// la misma lectura que lo pinta.
func (s *Servicio) DenunciarCanal(ctx context.Context, quien Quien, org string, d NuevaDenuncia) error {
	if !tenancy.ValidSlug(org) {
		return ErrNoExiste
	}
	if err := validarDenuncia(&d); err != nil {
		return err
	}
	clave, err := s.claveDe(quien)
	if err != nil {
		return err
	}
	if res, err := s.ritmoDenuncias.Allow(ctx, clave); err != nil || !res.Allowed {
		return ErrDemasiadoRapido
	}
	f := fila{tenant: org, kind: DenunciaCanal, targetID: org, targetName: org, reason: d.Reason, details: d.Details, clave: clave}
	if !quien.Anonimo {
		f.reporterSub, f.reporterTenant = quien.Sub, quien.Tenant
	}
	nueva, err := s.store.insertarDenuncia(ctx, &f)
	if err != nil || !nueva {
		return err
	}
	s.avisarDenuncia(ctx, Video{Tenant: org}, f)
	return nil
}

// avisarDenuncia manda el aviso del Hub a quien puede actuar: quienes administran
// la organización (los que Cast ha visto) y, si lo denunciado es de OTRA persona
// en su vídeo, el dueño del vídeo. Nunca al denunciado ni a quien denuncia, y sin
// decir quién fue.
func (s *Servicio) avisarDenuncia(ctx context.Context, v Video, f fila) {
	if s.mod.Hub == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	para, err := s.store.moderadores(ctx, v.Tenant)
	if err != nil {
		return
	}
	cola := enlaceCast(s.mod.PublicURL, v.Tenant, "moderation")
	destinos := map[string]string{}
	for _, sub := range para {
		destinos[sub] = cola
	}
	if _, ya := destinos[v.OwnerSub]; !ya && v.OwnerSub != "" && f.videoID != nil {
		// Quien no administra no ve la cola: su enlace es el vídeo, donde ya modera.
		destinos[v.OwnerSub] = enlaceCast(s.mod.PublicURL, v.Tenant, "watch/"+f.videoID.String())
	}
	delete(destinos, f.targetSub)
	delete(destinos, f.reporterSub)
	for sub, enlace := range destinos {
		s.mod.Hub.SendAsync(ctx, hubnotify.Notification{
			Tenant: v.Tenant, Subject: sub, Source: "cast", Type: "cast.report", Level: "warning",
			Title: "Nueva denuncia en Sytue Cast", URL: enlace,
			Params: map[string]string{"kind": f.kind, "reason": f.reason},
		})
	}
}

// enlaceCast es una página de la web de Cast dentro de una organización (ADR 0056).
func enlaceCast(publicURL, org, ruta string) string {
	u, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	out := url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/cast/" + org + "/" + ruta}
	return out.String()
}

// ── La cola ─────────────────────────────────────────────────────────────────

// modera: la cola es de quienes administran SU organización. No hay otra forma
// de llegar a la de otra: la organización sale del token, nunca de la petición.
func modera(quien Quien) error {
	if quien.Anonimo || quien.Sub == "" || quien.Tenant == "" || !quien.Admin {
		return ErrProhibido
	}
	return nil
}

// Denuncias lista la cola de la organización de quien pregunta: las abiertas, o
// las resueltas con `resueltas`. Más recientes primero.
func (s *Servicio) Denuncias(ctx context.Context, quien Quien, resueltas bool, cursor string, limite int) ([]Denuncia, string, error) {
	if err := modera(quien); err != nil {
		return nil, "", err
	}
	return s.cola(ctx, quien, quien.Tenant, resueltas, cursor, limite)
}

// DenunciasOperador es la cola del operador de la plataforma: la de TODAS las
// organizaciones, también las denuncias contra quien las administra. Quien llama
// ya pasó por Sytue ID (platform/pkg/operador), que dejó la traza.
func (s *Servicio) DenunciasOperador(ctx context.Context, quien Quien, resueltas bool, cursor string, limite int) ([]Denuncia, string, error) {
	if !quien.Operador || quien.Sub == "" {
		return nil, "", ErrProhibido
	}
	return s.cola(ctx, quien, "", resueltas, cursor, limite)
}

// cola pagina la cola de `tenant`, o la de todas con "". Nadie ve las denuncias
// contra sí mismo, tampoco el operador.
func (s *Servicio) cola(ctx context.Context, quien Quien, tenant string, resueltas bool, cursor string, limite int) ([]Denuncia, string, error) {
	desde, err := descifrarCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	limite = acotar(limite, 50, 200)
	filas, err := s.store.denuncias(ctx, tenant, quien.Sub, resueltas, desde, limite+1)
	if err != nil {
		return nil, "", err
	}
	siguiente := ""
	if len(filas) > limite {
		filas = filas[:limite]
		ultimo := filas[len(filas)-1]
		siguiente = cifrarCursor(cursorDe{ultimo.CreatedAt, ultimo.ID})
	}
	for i := range filas {
		filas[i].Actions = accionesDe(filas[i], quien)
	}
	return filas, siguiente, nil
}

// DenunciasAbiertas es cuántas hay sin resolver. De paso apunta a quien pregunta
// como moderador de su organización, que es a quien se avisa de las nuevas.
func (s *Servicio) DenunciasAbiertas(ctx context.Context, quien Quien) (int, error) {
	if err := modera(quien); err != nil {
		return 0, err
	}
	return s.store.abiertas(ctx, quien.Tenant, quien.Sub)
}

// accionesDe son las acciones que admite una denuncia abierta.
func accionesDe(d Denuncia, quien Quien) []string {
	if d.Status != "open" {
		return []string{}
	}
	out := []string{AccionDescartar}
	switch d.Kind {
	case DenunciaComentario, DenunciaChat:
		out = append(out, AccionBorrar)
	case DenunciaVideo:
		out = append(out, AccionRetirar, AccionDespublicar)
	}
	if d.TargetSub != "" && d.TargetSub != quien.Sub {
		out = append(out, AccionBloquear)
	}
	return out
}

// ResolverDenuncia aplica una acción a una denuncia abierta de la organización de quien
// pregunta. Una denuncia de otra organización no existe.
func (s *Servicio) ResolverDenuncia(ctx context.Context, quien Quien, id uuid.UUID, accion string) error {
	if err := modera(quien); err != nil {
		return err
	}
	return s.resolverDenuncia(ctx, quien, id, accion)
}

// OrgDeDenuncia dice de qué organización es una denuncia, para que el operador
// diga a Sytue ID sobre cuál va a actuar ANTES de hacerlo.
func (s *Servicio) OrgDeDenuncia(ctx context.Context, id uuid.UUID) (string, error) {
	return s.store.orgDeDenuncia(ctx, id)
}

// ResolverDenunciaOperador es ResolverDenuncia desde la cola del operador:
// `quien.Tenant` es la organización de la denuncia (OrgDeDenuncia), y todo queda
// a nombre del operador y marcado como hecho desde la plataforma.
func (s *Servicio) ResolverDenunciaOperador(ctx context.Context, quien Quien, id uuid.UUID, accion string) error {
	if !quien.Operador || quien.Sub == "" || quien.Tenant == "" {
		return ErrProhibido
	}
	return s.resolverDenuncia(ctx, quien, id, accion)
}

func (s *Servicio) resolverDenuncia(ctx context.Context, quien Quien, id uuid.UUID, accion string) error {
	d, err := s.store.resolverDenuncia(ctx, quien, id, accion)
	if err != nil {
		return err
	}
	if accion == AccionBorrar && d.Kind == DenunciaChat && d.VideoID != nil {
		if mid, err := uuid.Parse(d.TargetID); err == nil {
			s.hub.Difundir(quien.Tenant, *d.VideoID, Trama{Type: TramaBorrado, ID: &mid})
		}
	}
	return nil
}

// Auditoria es la traza de moderación de la organización de quien pregunta.
func (s *Servicio) Auditoria(ctx context.Context, quien Quien, limite int) ([]auditlog.Entrada, error) {
	if err := modera(quien); err != nil {
		return nil, err
	}
	return s.store.auditoria(ctx, quien.Tenant, quien.Sub, limite)
}

// ── Store ───────────────────────────────────────────────────────────────────

// fila es una denuncia a punto de guardarse.
type fila struct {
	tenant, kind                string
	videoID                     *uuid.UUID
	targetID, targetSub         string
	targetName, excerpt         string
	reason, details             string
	reporterSub, reporterTenant string
	clave                       string
}

// enOrg abre una transacción en una organización.
func (s *store) enOrg(ctx context.Context, tenant string, fn func(tx pgx.Tx) error) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		return fn(tx)
	})
}

// auditar deja una fila en la traza, en la transacción de lo que audita.
func auditar(ctx context.Context, tx pgx.Tx, tenant, actor, accion, objetivo string, detalle map[string]any) error {
	b, err := json.Marshal(detalle)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO moderation_audit (tenant, actor, action, target, detail) VALUES ($1, $2, $3, $4, $5)`,
		tenant, actor, accion, objetivo, b); err != nil {
		return fmt.Errorf("social: auditar %s: %w", accion, err)
	}
	return nil
}

// autorEn dice quién escribió un comentario o un mensaje vivo de ESE vídeo.
func autorEn(ctx context.Context, tx pgx.Tx, tabla string, v Video, id uuid.UUID) (sub, nombre, cuerpo string, err error) {
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT author_sub, author_name, body FROM %s
		 WHERE id = $1 AND video_id = $2 AND tenant = $3 AND deleted_at IS NULL`, tabla), id, v.ID, v.Tenant).Scan(&sub, &nombre, &cuerpo)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNoExiste
	}
	return sub, nombre, cuerpo, err
}

// personaEn da el nombre de una persona que aparece en el vídeo: su dueño, o quien
// ha comentado o escrito en su chat. Quien no aparece no existe: así no se puede
// denunciar ni bloquear a un sub cualquiera, ni sondear cuáles hay.
func personaEn(ctx context.Context, tx pgx.Tx, v Video, sub string) (string, error) {
	if sub == "" {
		return "", ErrNoExiste
	}
	if sub == v.OwnerSub {
		return v.OwnerName, nil
	}
	var nombre string
	err := tx.QueryRow(ctx, `
		SELECT author_name FROM (
		    SELECT author_name, created_at FROM comments
		     WHERE video_id = $1 AND tenant = $2 AND author_sub = $3 AND deleted_at IS NULL
		    UNION ALL
		    SELECT author_name, created_at FROM chat_messages
		     WHERE video_id = $1 AND tenant = $2 AND author_sub = $3 AND deleted_at IS NULL
		) a ORDER BY created_at DESC LIMIT 1`, v.ID, v.Tenant, sub).Scan(&nombre)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoExiste
	}
	return nombre, err
}

// denunciar resuelve el objetivo dentro del vídeo y guarda la denuncia, todo en
// una transacción. Devuelve false si ya había una abierta igual.
func (s *store) denunciar(ctx context.Context, v Video, objetivo string, f *fila) (bool, error) {
	var nueva bool
	err := s.enTenant(ctx, v, func(tx pgx.Tx) error {
		var err error
		switch f.kind {
		case DenunciaVideo:
			f.targetID, f.targetSub, f.targetName, f.excerpt = v.ID.String(), v.OwnerSub, v.OwnerName, v.Titulo
		case DenunciaComentario, DenunciaChat:
			id, perr := uuid.Parse(objetivo)
			if perr != nil {
				return fmt.Errorf("%w: objetivo ilegible", ErrEntradaInvalida)
			}
			tabla := "comments"
			if f.kind == DenunciaChat {
				tabla = "chat_messages"
			}
			f.targetID = id.String()
			if f.targetSub, f.targetName, f.excerpt, err = autorEn(ctx, tx, tabla, v, id); err != nil {
				return err
			}
		case DenunciaPersona:
			f.targetID, f.targetSub = objetivo, objetivo
			if f.targetName, err = personaEn(ctx, tx, v, objetivo); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: tipo de denuncia desconocido", ErrEntradaInvalida)
		}
		if f.reporterSub != "" && f.reporterSub == f.targetSub {
			return fmt.Errorf("%w: no puedes denunciarte", ErrEntradaInvalida)
		}
		nueva, err = insertarDenunciaTx(ctx, tx, f)
		return err
	})
	if err != nil {
		return false, traducirPG(err, "social: guardar denuncia")
	}
	return nueva, nil
}

func (s *store) insertarDenuncia(ctx context.Context, f *fila) (bool, error) {
	var nueva bool
	err := s.enOrg(ctx, f.tenant, func(tx pgx.Tx) (err error) {
		nueva, err = insertarDenunciaTx(ctx, tx, f)
		return err
	})
	if err != nil {
		return false, traducirPG(err, "social: guardar denuncia")
	}
	return nueva, nil
}

func insertarDenunciaTx(ctx context.Context, tx pgx.Tx, f *fila) (bool, error) {
	f.excerpt = validation.TruncateRunes(f.excerpt, maxExtracto)
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO reports (tenant, kind, video_id, target_id, target_sub, target_name, excerpt,
		                     reason, details, reporter_sub, reporter_tenant, reporter_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (tenant, kind, target_id, reporter_key) WHERE status = 'open' DO NOTHING
		RETURNING id`,
		f.tenant, f.kind, f.videoID, f.targetID, f.targetSub, f.targetName, f.excerpt,
		f.reason, f.details, f.reporterSub, f.reporterTenant, f.clave).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// En la traza, quien denuncia es «account» o «anonymous»: la lee quien modera,
	// que puede ser el denunciado. Quién fue queda en `reports.reporter_sub`.
	actor := "account"
	if f.reporterSub == "" {
		actor = "anonymous"
	}
	// `target_sub` va en el detalle para que la traza no le enseñe a nadie las
	// denuncias contra sí mismo (`auditoria`), tampoco cuando la denuncia ya se
	// haya purgado.
	return true, auditar(ctx, tx, f.tenant, actor, "report.created", f.kind+":"+f.targetID,
		map[string]any{"report_id": id, "reason": f.reason, "target_sub": f.targetSub})
}

// moderadores son las personas a las que avisar en esa organización.
func (s *store) moderadores(ctx context.Context, tenant string) ([]string, error) {
	var out []string
	err := s.enOrg(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT sub FROM moderators WHERE tenant = $1 ORDER BY seen_at DESC LIMIT 50`, tenant)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	return out, err
}

// abiertas cuenta las denuncias sin resolver —menos las que son contra quien
// pregunta— y le apunta como moderador. Una transacción.
func (s *store) abiertas(ctx context.Context, tenant, sub string) (int, error) {
	var n int
	err := s.enOrg(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO moderators (tenant, sub) VALUES ($1, $2)
			ON CONFLICT (tenant, sub) DO UPDATE SET seen_at = now()`, tenant, sub); err != nil {
			return fmt.Errorf("social: apuntar moderador: %w", err)
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM reports WHERE tenant = $1 AND status = 'open' AND target_sub <> $2`, tenant, sub).Scan(&n)
	})
	return n, err
}

const columnasDenuncia = `id, kind, video_id, target_id, target_sub, target_name, excerpt, reason, details,
       reporter_sub = '', status, resolution, created_at, resolved_at, resolved_from, tenant`

func leerDenuncia(row pgx.Row) (Denuncia, error) {
	var d Denuncia
	err := row.Scan(&d.ID, &d.Kind, &d.VideoID, &d.TargetID, &d.TargetSub, &d.TargetName, &d.Excerpt, &d.Reason, &d.Details,
		&d.Anonymous, &d.Status, &d.Resolution, &d.CreatedAt, &d.ResolvedAt, &d.ResolvedFrom, &d.Tenant)
	return d, err
}

// denuncias lista la cola de `tenant` tal como la ve `quien`: sin las denuncias
// contra él. Con `tenant` vacío es la del operador: todas las organizaciones, por
// la política de mantenimiento (migración 0010).
func (s *store) denuncias(ctx context.Context, tenant, quien string, resueltas bool, desde *cursorDe, limite int) ([]Denuncia, error) {
	var out []Denuncia
	fecha, id := argumentosCursor(desde)
	abrir := func(fn func(tx pgx.Tx) error) error { return s.enOrg(ctx, tenant, fn) }
	if tenant == "" {
		abrir = func(fn func(tx pgx.Tx) error) error { return s.pool.InMaintenanceTx(ctx, fn) }
	}
	err := abrir(func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+columnasDenuncia+`
			  FROM reports
			 WHERE ($1 = '' OR tenant = $1) AND (status = 'open') <> $2 AND target_sub <> $6
			   AND ($3::timestamptz IS NULL OR (created_at, id) < ($3::timestamptz, $4::uuid))
			 ORDER BY created_at DESC, id DESC
			 LIMIT $5`, tenant, resueltas, fecha, id, limite, quien)
		if err != nil {
			return fmt.Errorf("social: listar denuncias: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			d, err := leerDenuncia(rows)
			if err != nil {
				return fmt.Errorf("social: leer denuncia: %w", err)
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}

// orgDeDenuncia lee de qué organización es una denuncia, cruzando todas.
func (s *store) orgDeDenuncia(ctx context.Context, id uuid.UUID) (string, error) {
	var tenant string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT tenant FROM reports WHERE id = $1`, id).Scan(&tenant)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoExiste
	}
	return tenant, err
}

// resolverDenuncia aplica la acción y cierra la denuncia, con su fila de
// auditoría, en una transacción. Todo va acotado a la organización de quien
// modera: la denuncia, y también lo que se borra, se retira o se bloquea. Una
// denuncia contra quien modera no existe para él.
func (s *store) resolverDenuncia(ctx context.Context, quien Quien, id uuid.UUID, accion string) (Denuncia, error) {
	var d Denuncia
	tenant := quien.Tenant
	err := s.enOrg(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		d, err = leerDenuncia(tx.QueryRow(ctx, `
			SELECT `+columnasDenuncia+` FROM reports
			 WHERE id = $1 AND tenant = $2 AND status = 'open' AND target_sub <> $3 FOR UPDATE`, id, tenant, quien.Sub))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoExiste
		}
		if err != nil {
			return fmt.Errorf("social: leer denuncia: %w", err)
		}
		if !slices.Contains(accionesDe(d, quien), accion) {
			return fmt.Errorf("%w: esa acción no vale para esta denuncia", ErrEntradaInvalida)
		}
		switch accion {
		case AccionBorrar:
			tabla := "comments"
			if d.Kind == DenunciaChat {
				tabla = "chat_messages"
			}
			// Sin filas no es un error: ya lo borró su autor, y la denuncia se cierra igual.
			_, err = tx.Exec(ctx, fmt.Sprintf(`
				UPDATE %s SET deleted_at = now()
				 WHERE id = $1::uuid AND tenant = $2 AND deleted_at IS NULL`, tabla), d.TargetID, tenant)
		case AccionRetirar:
			_, err = tx.Exec(ctx, `
				UPDATE videos SET visibility = 'unlisted', updated_at = now()
				 WHERE id = $1::uuid AND tenant = $2 AND deleted_at IS NULL AND visibility = 'public'`, d.TargetID, tenant)
		case AccionDespublicar:
			_, err = tx.Exec(ctx, `
				UPDATE videos SET visibility = 'org', updated_at = now()
				 WHERE id = $1::uuid AND tenant = $2 AND deleted_at IS NULL AND visibility IN ('public', 'unlisted')`, d.TargetID, tenant)
		case AccionBloquear:
			err = bloquearEnOrgTx(ctx, tx, tenant, d.TargetSub, d.TargetName, quien.Sub)
		}
		if err != nil {
			return fmt.Errorf("social: aplicar %s: %w", accion, err)
		}
		estado := "actioned"
		if accion == AccionDescartar {
			estado = "dismissed"
		}
		// Desde dónde: lo ve la administración de la organización en su traza, así
		// que el operador nunca actúa como si fuera ella.
		desde := "org"
		if quien.Operador {
			desde = "platform"
		}
		// Actuar resuelve TODAS las abiertas sobre lo mismo; descartar, solo esta.
		if _, err := tx.Exec(ctx, `
			UPDATE reports SET status = $3, resolution = $4, resolved_by = $5, resolved_at = now(), resolved_from = $8
			 WHERE tenant = $2 AND status = 'open'
			   AND (id = $1 OR ($3 = 'actioned' AND kind = $6 AND target_id = $7))`,
			id, tenant, estado, accion, quien.Sub, d.Kind, d.TargetID, desde); err != nil {
			return fmt.Errorf("social: cerrar denuncia: %w", err)
		}
		d.Status, d.Resolution, d.ResolvedFrom = estado, accion, desde
		return auditar(ctx, tx, tenant, quien.Sub, "report."+estado, d.Kind+":"+d.TargetID,
			map[string]any{"report_id": id, "action": accion, "target_sub": d.TargetSub, "from": desde})
	})
	return d, err
}

// auditoria es la traza de `tenant` tal como la ve `quien`: sin las filas de las
// denuncias contra él. Las nuevas llevan `target_sub` en el detalle; las de antes,
// solo el id de la denuncia, y se miran contra ella.
func (s *store) auditoria(ctx context.Context, tenant, quien string, limite int) ([]auditlog.Entrada, error) {
	var out []auditlog.Entrada
	err := s.enOrg(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.occurred_at, a.tenant, a.actor, a.action, a.target, a.detail
			  FROM moderation_audit a
			 WHERE a.tenant = $1 AND COALESCE(a.detail->>'target_sub', '') <> $3
			   AND NOT EXISTS (SELECT 1 FROM reports r
			                    WHERE r.tenant = a.tenant AND r.target_sub = $3
			                      AND r.id::text = a.detail->>'report_id')
			 ORDER BY a.occurred_at DESC, a.id DESC LIMIT $2`, tenant, limite, quien)
		if err != nil {
			return fmt.Errorf("social: leer auditoría: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var e auditlog.Entrada
			if err := rows.Scan(&e.ID, &e.OccurredAt, &e.Tenant, &e.Actor, &e.Action, &e.Target, &e.Detail); err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}

// ritmoDeDenuncias: cinco de golpe y una cada veinte segundos por persona (o por
// IP troceada). Por réplica, como el del chat.
func ritmoDeDenuncias() *httpx.TokenBucket { return httpx.NewTokenBucket(0.05, 5) }

// ── Las denuncias de una persona, para «Exportar tus datos» ─────────────────

// DenunciaPropia es una denuncia tal como la exporta quien la puso: qué denunció,
// por qué y en qué quedó. Sin la copia de lo denunciado (es de otra persona) y sin
// la acción que se tomó (es de la organización).
type DenunciaPropia struct {
	Kind       string     `json:"kind"`
	VideoID    *uuid.UUID `json:"video_id,omitempty"`
	TargetID   string     `json:"target_id"`
	Reason     string     `json:"reason"`
	Details    string     `json:"details,omitempty"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// DenunciasDe son las denuncias que puso quien pregunta, en cualquier
// organización. Nunca las que otros pusieron contra ella.
func (s *Servicio) DenunciasDe(ctx context.Context, quien Quien) ([]DenunciaPropia, error) {
	if quien.Anonimo || quien.Sub == "" {
		return nil, ErrProhibido
	}
	var out []DenunciaPropia
	err := s.store.enPersona(ctx, quien.Sub, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT kind, video_id, target_id, reason, details, status, created_at, resolved_at
			  FROM reports WHERE reporter_sub = $1
			 ORDER BY created_at DESC, id DESC LIMIT 5000`, quien.Sub)
		if err != nil {
			return fmt.Errorf("social: leer mis denuncias: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (DenunciaPropia, error) {
			var d DenunciaPropia
			return d, r.Scan(&d.Kind, &d.VideoID, &d.TargetID, &d.Reason, &d.Details, &d.Status, &d.CreatedAt, &d.ResolvedAt)
		})
		return err
	})
	return out, err
}
