// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/github"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// DeployNotifier es el ÚNICO consumidor del outbox del canal "cloud" (el outbox
// es de un solo consumidor lógico: quien procesa un evento lo marca; dos
// Dispatchers competirían). Por eso entrega DOS cosas en un solo Handle:
//  1. el aviso legado por notify_webhook_url de proyecto/organización cuando un
//     deployment pasa a ready/error (dedupe por (deployment_id, status));
//  2. el enrutado por alert_policies de los 4 eventos de alerta (deploy
//     ready/failed + uptime up/down), con dedupe por (policy_id, event_id).
//
// El cuerpo JSON genérico lo consume o transforma cualquier endpoint
// (Slack/Discord/genérico), que es lo de mayor cobertura.
//
// El canal de una alerta puede ser un webhook o un `mailto:`. El correo entra por
// ahí —y no por un campo nuevo— porque el canal YA es una URL: sin columna, sin
// migración y sin un segundo camino que mantener. Lo entrega `net/smtp` (stdlib),
// que es todo lo que hace falta para un aviso de veinte líneas: el helper de
// correo de platform/ que este comentario llevaba pidiendo desde la Ola B habría
// sido tocar un paquete compartido por siete servicios.
type DeployNotifier struct {
	st     *store.Store
	client *http.Client
	// gh es el cliente de GitHub App (nil = sin App: el comentario de preview es
	// no-op). Lo comparte el Service.
	gh *github.Client
	// cfg lleva la configuración SMTP: un canal de alerta puede ser un `mailto:`,
	// y entonces la entrega es un correo en vez de un POST.
	cfg config.Config
	// hub lleva el despliegue al CENTRO DE NOTIFICACIONES de la suite (topBar y
	// apps nativas), a quien lo lanzó. nil = Hub no configurado: solo webhooks.
	hub *hubnotify.Client
}

// RunDeployNotifier arranca el worker: un Dispatcher del outbox sobre el canal
// "cloud" que entrega los eventos a DeployNotifier.Handle (at-least-once; los
// dedupes evitan reenviar la misma transición/alerta). Bloquea hasta que se
// cancele ctx. Lo arranca cmd/cloud-api.
func (s *Service) RunDeployNotifier(ctx context.Context) error {
	n := &DeployNotifier{
		st:     s.pstore,
		client: newWebhookClient(),
		gh:     s.gh,
		cfg:    s.cfg,
		hub:    hubnotify.New(s.cfg.HubBaseURL, s.cfg.HubInternalSecret),
	}
	disp := events.NewDispatcher(s.pstore.Pool(), n.Handle, events.DispatcherConfig{
		Channels: []events.Channel{EventChannel},
		// Acota el radio de un endpoint muerto (black-hole). El drain reordena por
		// created_at, así que el evento fallido más antiguo se re-selecciona en cada
		// ciclo hasta agotar intentos; con el default (10 × ~10s) eso son ~100s de
		// estancamiento que se acoplan a la entrega de OTROS tenants. 3 × ~4s (Timeout
		// del cliente, newWebhookClient) ≈ 12s antes de marcarlo fallido permanente.
		MaxAttempts: 3,
	})
	return disp.Run(ctx)
}

// Handle procesa un evento del outbox. Ejecuta SIEMPRE las dos entregas
// independientes e idempotentes —(1) el aviso legado por notify_webhook_url de
// proyecto/org (handleDeployNotify) y (2) el enrutado por alert_policies
// (routeAlerts, que cubre también uptime.up/down)— y combina sus errores con
// errors.Join. No las encadena: un webhook legado muerto NO puede impedir el
// enrutado de alertas (ni viceversa); si se serializasen, una notify_webhook_url
// obsoleta agotaría los reintentos del Dispatcher y descartaría en silencio todas
// las alert_policies del mismo deploy. Devuelve error solo en fallos reintentables
// (BD o envío); el Dispatcher reintenta y los dedupes por (deployment_id,status) y
// (policy_id,event_id) evitan reenviar en el reintento lo ya entregado.
//
// ponytail: ambos envíos ocurren EN LÍNEA dentro de la tx del Dispatcher (que
// mantiene el lock de la fila del outbox mientras dura), acotados por el timeout
// del cliente. Volumen bajo; si creciera, desacoplar el envío (cola dedicada).
func (n *DeployNotifier) Handle(ctx context.Context, ev events.Event) error {
	// Los tres caminos corren SIEMPRE, independientes; cada uno reintenta bajo su
	// propio dedup. El comentario de preview en el PR (GitHub App) es no-op si la
	// App no está configurada o el deployment no es una preview con instalación.
	return errors.Join(
		n.handleDeployNotify(ctx, ev),
		n.routeAlerts(ctx, ev),
		n.commentPreview(ctx, ev),
	)
}

// previewCommentMarker es un marcador HTML oculto (invisible en el render de
// GitHub) al inicio del cuerpo: documenta la autoría e identifica el comentario
// para una futura reconciliación por marcador si hiciera falta.
const previewCommentMarker = "<!-- sytue-cloud-preview -->"

// commentPreview publica —o edita, si ya existe— el comentario de preview en el
// PR cuando un deployment de PREVIEW pasa a ready y el proyecto tiene una GitHub
// App conectada. Idempotente por (proyecto, PR): recuerda el id del comentario y
// lo EDITA en los re-deploys del mismo PR, sin duplicar. Degrada a no-op si la
// App no está configurada, el deployment no es preview, no hay instalación o el
// repo no es de GitHub.
func (n *DeployNotifier) commentPreview(ctx context.Context, ev events.Event) error {
	if n.gh == nil || ev.Type != store.EventDeployReady {
		return nil // App no configurada, o evento que no es un ready
	}
	var payload struct {
		DeploymentID uuid.UUID `json:"deployment_id"`
	}
	if err := json.Unmarshal(ev.Payload, &payload); err != nil || payload.DeploymentID == uuid.Nil {
		return nil //nolint:nilerr // payload ilegible/ajeno: no reintentable
	}
	pv, ok, err := n.st.LoadGitHubPreview(ctx, ev.Tenant, payload.DeploymentID)
	if err != nil {
		return err
	}
	if !ok || pv.Target != domain.TargetPreview || pv.PRNumber == nil || pv.InstallationID == 0 {
		return nil // no es una preview con App conectada: nada que comentar
	}
	owner, repo, ok := github.ParseRepo(pv.RepoURL)
	if !ok {
		return nil // el repo del proyecto no es de github.com: no-op
	}
	// Token efímero de la instalación (no se persiste); reintentable si falla.
	token, _, err := n.gh.InstallationToken(ctx, pv.InstallationID)
	if err != nil {
		return err
	}
	commentID, err := n.gh.UpsertPRComment(ctx, token, owner, repo, *pv.PRNumber, pv.ExistingComment, previewCommentBody(pv.URL))
	if err != nil {
		return err
	}
	if commentID != pv.ExistingComment {
		// ponytail: dedupe por comment_id persistido. Si el POST crea el comentario
		// pero este guardado del id falla, el reintento del Dispatcher podría crear
		// un duplicado (ventana mínima y transitoria); el caso principal —re-deploy
		// del mismo PR— queda deduplicado porque el id sobrevive entre deployments.
		// Si molestara: dedupe adicional por el marcador oculto (list+find).
		if err := n.st.SaveGitHubPRComment(ctx, ev.Tenant, pv.ProjectID, *pv.PRNumber, commentID); err != nil {
			return err
		}
	}
	log.From(ctx).Info("comentario de preview publicado en el PR",
		"tenant", ev.Tenant, "pr", *pv.PRNumber, "project_id", pv.ProjectID)
	return nil
}

// previewCommentBody arma el cuerpo del comentario de preview: sobrio, sin
// adornos (design-system: nada de AI slop), con el marcador oculto delante.
func previewCommentBody(url string) string {
	return fmt.Sprintf("%s\n### Sytue Cloud — Preview del PR\n\nPreview desplegada: %s\n\nSe actualiza en cada push a este PR.",
		previewCommentMarker, url)
}

// handleDeployNotify entrega el aviso legado por notify_webhook_url (proyecto >
// organización). Ignora todo lo que no sea una transición de deploy.
func (n *DeployNotifier) handleDeployNotify(ctx context.Context, ev events.Event) error {
	var status string
	switch ev.Type {
	case store.EventDeployReady:
		status = "ready"
	case store.EventDeployError:
		status = "error"
	default:
		return nil // otros eventos del canal "cloud": no es asunto del notifier legado
	}

	var payload struct {
		DeploymentID uuid.UUID `json:"deployment_id"`
	}
	if err := json.Unmarshal(ev.Payload, &payload); err != nil || payload.DeploymentID == uuid.Nil {
		return nil //nolint:nilerr // payload ilegible/ajeno = mensaje envenenado: se descarta (marcar procesado), no se reintenta.
	}

	target, ok, err := n.st.LoadDeployNotify(ctx, ev.Tenant, payload.DeploymentID)
	if err != nil {
		return err
	}
	if !ok {
		return nil // deployment purgado
	}
	// AVISO EN EL HUB, a quien lanzó el despliegue: es la integración con el
	// centro de notificaciones de la suite (topBar web y apps nativas) y va
	// ANTES del filtro de webhook — el webhook es un extra por proyecto y el
	// aviso a su dueño no depende de que esté configurado. At-least-once como
	// todo el Dispatcher: si el webhook de abajo falla y el evento se reintenta,
	// puede duplicarse una notificación; benigno frente a perderla.
	n.notifyHub(ctx, ev.Tenant, target, status, payload.DeploymentID)
	if target.WebhookURL == "" {
		return nil // sin URL de aviso configurada: solo el aviso de la suite
	}
	// Defensa en profundidad: la URL se validó al fijarla; revalidar antes de
	// contactarla (además del guard anti-SSRF del propio dial).
	if err := domain.ValidateWebhookURL(target.WebhookURL); err != nil {
		log.From(ctx).Warn("URL de aviso de deploy inválida; se omite",
			"tenant", ev.Tenant, "deployment_id", payload.DeploymentID, "error", err)
		return nil // URL inválida: no reintentar
	}

	// Idempotencia: reclama (deployment_id, status) antes de enviar. Si ya estaba
	// reclamada, otra entrega ya lo mandó (o está en curso) → nada que hacer.
	claimed, err := n.st.ClaimDeployNotification(ctx, ev.Tenant, payload.DeploymentID, status, target.WebhookURL)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	if err := n.send(ctx, target, status, payload.DeploymentID); err != nil {
		// Libera la reclamación para que el reintento del Dispatcher reenvíe.
		_ = n.st.ReleaseDeployNotification(ctx, ev.Tenant, payload.DeploymentID, status)
		return err
	}
	log.From(ctx).Info("aviso de deploy enviado",
		"tenant", ev.Tenant, "project", target.ProjectSlug, "status", status)
	return nil
}

// notifyHub entrega la transición del despliegue al centro de notificaciones de
// la suite. Un fallo NO reintenta el evento: el aviso es un extra y su pérdida
// se registra, igual que hace el resto de la suite con los avisos del Hub.
func (n *DeployNotifier) notifyHub(ctx context.Context, tenant string, target store.DeployNotifyTarget, status string, deploymentID uuid.UUID) {
	if n.hub == nil || target.CreatedBy == "" {
		return
	}
	nivel, verbo := "success", "listo"
	if status != "ready" {
		nivel, verbo = "error", "fallido"
	}
	cuerpo := target.CommitSHA
	if len(cuerpo) > 7 {
		cuerpo = cuerpo[:7]
	}
	err := n.hub.Send(ctx, hubnotify.Notification{
		Tenant:  tenant,
		Subject: target.CreatedBy,
		Source:  "cloud",
		Type:    "cloud.deploy",
		Level:   nivel,
		Title:   "Despliegue " + verbo + ": " + target.ProjectSlug,
		Body:    cuerpo,
		URL:     target.URL,
	})
	if err != nil {
		log.From(ctx).Info("hub: despliegue sin notificar", "error", err, "deployment_id", deploymentID)
	}
}

// deployWebhookPayload es el cuerpo JSON del aviso saliente. Formato genérico:
// Slack/Discord y cualquier endpoint lo consumen o transforman.
type deployWebhookPayload struct {
	Event      string `json:"event"`
	Project    string `json:"project"`
	Deployment string `json:"deployment"`
	Status     string `json:"status"`
	URL        string `json:"url"`
	Commit     string `json:"commit"`
}

func (n *DeployNotifier) send(ctx context.Context, target store.DeployNotifyTarget, status string, deploymentID uuid.UUID) error {
	return n.postJSON(ctx, target.WebhookURL, "sytue-cloud/deploy-notifier", deployWebhookPayload{
		Event:      "deployment." + status,
		Project:    target.ProjectSlug,
		Deployment: deploymentID.String(),
		Status:     status,
		URL:        target.URL,
		Commit:     target.CommitSHA,
	})
}

// deliver entrega una alerta por el canal que sea: un POST al webhook o un correo
// si el canal es un `mailto:`.
//
// El correo va en TEXTO plano y no en JSON: lo lee una persona en su bandeja, no
// un endpoint que lo transforma. Es la única diferencia real entre los dos
// caminos; el resto —dedupe, reintentos, liberación de la reserva— es idéntico.
func (n *DeployNotifier) deliver(ctx context.Context, channel string, payload alertPayload) error {
	if addr, ok := domain.MailChannelAddress(channel); ok {
		subject, body := alertMailText(payload)
		if err := sendAlertMail(ctx, n.cfg, addr, subject, body); err != nil {
			return err
		}
		return nil
	}
	return n.postJSON(ctx, channel, "sytue-cloud/alerts", payload)
}

// alertMailText convierte la alerta en un asunto y un cuerpo legibles.
//
// Se escribe a mano y no con una plantilla: son seis líneas, y lo que importa es
// que el asunto diga QUÉ pasa sin abrir el correo —que es lo que se ve en el
// móvil a las tres de la mañana—.
func alertMailText(p alertPayload) (subject, body string) {
	quien := p.Project
	if quien == "" {
		quien = p.ProjectID
	}
	subject = "Sytue Cloud · " + p.Event
	if quien != "" {
		subject += " · " + quien
	}

	var b strings.Builder
	b.WriteString("Evento: " + p.Event + "\n")
	if quien != "" {
		b.WriteString("Proyecto: " + quien + "\n")
	}
	if p.Status != "" {
		b.WriteString("Estado: " + p.Status + "\n")
	}
	if p.URL != "" {
		b.WriteString("URL: " + p.URL + "\n")
	}
	if p.Commit != "" {
		b.WriteString("Commit: " + p.Commit + "\n")
	}
	if p.Check != "" {
		b.WriteString("Sonda: " + p.Check + " (" + p.TargetURL + ")\n")
	}
	if p.ThresholdPct > 0 {
		fmt.Fprintf(&b, "Gasto: %.2f € de %.2f € (%d %% del límite)\n",
			p.SpendEur, p.LimitEur, p.ThresholdPct)
	}
	if p.ToSize != "" {
		fmt.Fprintf(&b, "Tamaño: %s → %s (%s)\n", p.FromSize, p.ToSize, p.Reason)
	}
	if p.Resource != "" && p.UsedPercent > 0 {
		recurso := "CPU"
		if p.Resource == "memory" {
			recurso = "memoria"
		}
		fmt.Fprintf(&b, "%s: %d %% del límite de %s\n", recurso, p.UsedPercent, p.FromSize)
	}
	b.WriteString("\n— Sytue Cloud\n")
	return subject, b.String()
}

// postJSON envía payload como POST JSON a url con el cliente anti-SSRF. Es el
// emisor genérico compartido por el aviso de deploy y las alertas. Devuelve error
// (reintentable) ante fallo de red o respuesta no 2xx.
func (n *DeployNotifier) postJSON(ctx context.Context, url, userAgent string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook %s: %w", url, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook %s devolvió %d", url, resp.StatusCode)
	}
	return nil
}

// alertPayload es el cuerpo JSON genérico de una alerta por evento (deploy,
// uptime o coste). Campos omitempty: solo van los relevantes al evento.
type alertPayload struct {
	Event     string `json:"event"`
	Project   string `json:"project,omitempty"`    // slug (eventos de deploy)
	ProjectID string `json:"project_id,omitempty"` // uuid del proyecto afectado
	Status    string `json:"status,omitempty"`
	URL       string `json:"url,omitempty"`        // URL de la app/preview (deploy)
	Commit    string `json:"commit,omitempty"`     // commit desplegado (deploy)
	Check     string `json:"check,omitempty"`      // nombre del uptime check
	TargetURL string `json:"target_url,omitempty"` // URL sondeada (uptime)
	LatencyMS int    `json:"latency_ms,omitempty"` // latencia del sondeo (uptime)
	// Aviso de coste: el receptor no debería tener que consultar nada para saber
	// de qué le están avisando, así que van las tres cifras.
	ThresholdPct int     `json:"threshold_pct,omitempty"` // % del límite cruzado
	SpendEur     float64 `json:"spend_eur,omitempty"`     // gasto del periodo
	LimitEur     float64 `json:"limit_eur,omitempty"`     // límite configurado
	// Aviso de tamaño: qué se propone, por qué y cuánto cambia la factura. Con
	// esto el receptor decide sin abrir el panel.
	Action           string  `json:"action,omitempty"`            // up | down
	Reason           string  `json:"reason,omitempty"`            // near_limit, over_provisioned…
	FromSize         string  `json:"from_size,omitempty"`         // tamaño actual
	ToSize           string  `json:"to_size,omitempty"`           // tamaño propuesto
	MonthlyDeltaEur  float64 `json:"monthly_delta_eur,omitempty"` // negativo = ahorro
	P95MemoryPercent int     `json:"p95_memory_percent,omitempty"`
	// Aviso de consumo al límite: qué recurso y cuánto se está usando del techo.
	Resource    string `json:"resource,omitempty"`     // cpu | memory
	UsedPercent int    `json:"used_percent,omitempty"` // % del límite del tamaño
}

// alertEventForBusType mapea un tipo de evento del bus a su evento CORTO de
// alert_policies. ok=false para eventos que no alimentan alertas.
func alertEventForBusType(busType string) (string, bool) {
	switch busType {
	case store.EventDeployReady:
		return domain.AlertEventDeployReady, true
	case store.EventDeployError:
		return domain.AlertEventDeployFailed, true
	case store.EventUptimeDown:
		return domain.AlertEventUptimeDown, true
	case store.EventUptimeUp:
		return domain.AlertEventUptimeUp, true
	case store.EventSpendThreshold:
		return domain.AlertEventSpendThreshold, true
	case store.EventSizingAdvice:
		return domain.AlertEventSizingAdvice, true
	case store.EventMetricCPUHigh:
		return domain.AlertEventCPUHigh, true
	case store.EventMetricMemoryHigh:
		return domain.AlertEventMemoryHigh, true
	default:
		return "", false
	}
}

// routeAlerts entrega el evento a las alert_policies que casan (mismo evento,
// alcance de proyecto o de organización). Idempotente por (policy_id, event_id):
// reclama antes de enviar y libera si el envío falla, para reenviar en el
// reintento del Dispatcher. Doble barrera anti-SSRF: revalida channel_url por
// host (ValidateWebhookURL) además del guard del dial.
func (n *DeployNotifier) routeAlerts(ctx context.Context, ev events.Event) error {
	alertEvent, ok := alertEventForBusType(ev.Type)
	if !ok {
		return nil
	}
	projectID, payload, ok, err := n.resolveAlertPayload(ctx, ev, alertEvent)
	if err != nil {
		return err
	}
	if !ok {
		return nil // deployment purgado o payload ilegible: nada que enrutar
	}
	eventID, err := uuid.Parse(ev.ID)
	if err != nil {
		return nil //nolint:nilerr // id de evento ilegible: no reintentable
	}
	policies, err := n.st.MatchAlertPolicies(ctx, ev.Tenant, alertEvent, projectID)
	if err != nil {
		return err
	}
	for _, p := range policies {
		// Defensa en profundidad: el canal se validó al fijarlo; revalidar aquí
		// (por host en un webhook, por forma en un `mailto:`).
		if err := domain.ValidateAlertChannel(p.ChannelURL); err != nil {
			log.From(ctx).Warn("channel_url de alerta inválida; se omite",
				"tenant", ev.Tenant, "policy", p.ID, "error", err)
			continue
		}
		claimed, err := n.st.ClaimAlertDelivery(ctx, ev.Tenant, p.ID, eventID)
		if err != nil {
			return err
		}
		if !claimed {
			continue // ya entregada (otra réplica o reintento previo)
		}
		if err := n.deliver(ctx, p.ChannelURL, payload); err != nil {
			_ = n.st.ReleaseAlertDelivery(ctx, ev.Tenant, p.ID, eventID)
			return err
		}
		log.From(ctx).Info("alerta enviada", "tenant", ev.Tenant, "event", alertEvent, "policy", p.ID)
	}
	return nil
}

// resolveAlertPayload extrae del evento el proyecto afectado y el cuerpo de la
// alerta. ok=false si el recurso ya no existe (purgado) o el payload es ilegible.
// metricAlertPayload traduce el aviso de consumo al payload del canal.
//
// Sale del switch de resolveAlertPayload porque ese switch ya estaba al borde
// del techo de complejidad, y un case más lo cruzaba. Cortar por el case es la
// división natural: cada evento del bus tiene su propia forma de payload.
func metricAlertPayload(ev events.Event, alertEvent string) (uuid.UUID, alertPayload, bool, error) {
	// Aviso de PROYECTO, como el de tamaño: casan sus políticas y las de la
	// organización.
	var pl store.MetricAlertPayload
	if err := json.Unmarshal(ev.Payload, &pl); err != nil || pl.Percent <= 0 {
		//nolint:nilerr // payload ilegible ⇒ el evento no aplica, no es un fallo del notificador.
		return uuid.Nil, alertPayload{}, false, nil
	}
	projectID, err := uuid.Parse(pl.ProjectID)
	if err != nil {
		return uuid.Nil, alertPayload{}, false, nil //nolint:nilerr // ídem: id ilegible, no reintentable.
	}
	return projectID, alertPayload{
		Event:       alertEvent,
		ProjectID:   pl.ProjectID,
		Resource:    pl.Resource,
		UsedPercent: pl.Percent,
		// FromSize, no ThresholdPct: ese campo significa «% del límite de GASTO» y
		// el correo lo imprime como tal, así que reusarlo aquí escribiría una línea
		// de factura con ceros.
		FromSize: pl.InstanceSze,
	}, true, nil
}

func (n *DeployNotifier) resolveAlertPayload(ctx context.Context, ev events.Event, alertEvent string) (uuid.UUID, alertPayload, bool, error) {
	switch ev.Type {
	case store.EventDeployReady, store.EventDeployError:
		var pl struct {
			DeploymentID uuid.UUID `json:"deployment_id"`
		}
		if err := json.Unmarshal(ev.Payload, &pl); err != nil || pl.DeploymentID == uuid.Nil {
			//nolint:nilerr // un payload ilegible no es un fallo del notificador: el evento simplemente no aplica.
			return uuid.Nil, alertPayload{}, false, nil
		}
		target, ok, err := n.st.LoadDeployNotify(ctx, ev.Tenant, pl.DeploymentID)
		if err != nil {
			return uuid.Nil, alertPayload{}, false, err
		}
		if !ok {
			return uuid.Nil, alertPayload{}, false, nil
		}
		status := "ready"
		if ev.Type == store.EventDeployError {
			status = "error"
		}
		return target.ProjectID, alertPayload{
			Event:     alertEvent,
			Project:   target.ProjectSlug,
			ProjectID: target.ProjectID.String(),
			Status:    status,
			URL:       target.URL,
			Commit:    target.CommitSHA,
		}, true, nil
	case store.EventUptimeDown, store.EventUptimeUp:
		var pl store.UptimeEventPayload
		if err := json.Unmarshal(ev.Payload, &pl); err != nil || pl.ProjectID == uuid.Nil {
			//nolint:nilerr // ídem: payload ilegible ⇒ el evento no aplica, no es un error del notificador.
			return uuid.Nil, alertPayload{}, false, nil
		}
		return pl.ProjectID, alertPayload{
			Event:     alertEvent,
			ProjectID: pl.ProjectID.String(),
			Status:    pl.Status,
			Check:     pl.Name,
			TargetURL: pl.TargetURL,
			LatencyMS: pl.LatencyMS,
		}, true, nil
	case store.EventSpendThreshold:
		// Aviso de ORGANIZACIÓN, no de proyecto: se devuelve uuid.Nil, y
		// MatchAlertPolicies casa entonces solo las políticas sin project_id
		// («project_id IS NULL»), que es exactamente el alcance correcto.
		var pl domain.SpendAlertPayload
		if err := json.Unmarshal(ev.Payload, &pl); err != nil || pl.ThresholdPct <= 0 {
			//nolint:nilerr // payload ilegible ⇒ el evento no aplica, no es un fallo del notificador.
			return uuid.Nil, alertPayload{}, false, nil
		}
		return uuid.Nil, alertPayload{
			Event:        alertEvent,
			ThresholdPct: pl.ThresholdPct,
			SpendEur:     pl.SpendEur,
			LimitEur:     pl.LimitEur,
		}, true, nil
	case store.EventSizingAdvice:
		// Aviso de PROYECTO: se devuelve su id, así que casan tanto las políticas
		// acotadas a ese proyecto como las de organización.
		var pl domain.SizingAlertPayload
		if err := json.Unmarshal(ev.Payload, &pl); err != nil || pl.ToSize == "" {
			//nolint:nilerr // payload ilegible ⇒ el evento no aplica, no es un fallo del notificador.
			return uuid.Nil, alertPayload{}, false, nil
		}
		projectID, err := uuid.Parse(pl.ProjectID)
		if err != nil {
			return uuid.Nil, alertPayload{}, false, nil //nolint:nilerr // ídem: id ilegible, no reintentable.
		}
		return projectID, alertPayload{
			Event:            alertEvent,
			ProjectID:        pl.ProjectID,
			Action:           string(pl.Action),
			Reason:           string(pl.Reason),
			FromSize:         pl.FromSize,
			ToSize:           pl.ToSize,
			MonthlyDeltaEur:  pl.MonthlyDeltaEur,
			P95MemoryPercent: pl.P95MemoryPercent,
		}, true, nil
	case store.EventMetricCPUHigh, store.EventMetricMemoryHigh:
		return metricAlertPayload(ev, alertEvent)
	default:
		return uuid.Nil, alertPayload{}, false, nil
	}
}

// newWebhookClient construye el cliente HTTP del emisor con defensa anti-SSRF en
// el DIAL: Control se ejecuta tras resolver DNS y rechaza cualquier IP interna
// (domain.IsInternalIP), cerrando el rebinding de un hostname público a una IP
// interna que la validación por host no puede ver. No sigue redirecciones (un
// 3xx a un destino interno se trata como fallo). Timeouts acotados.
func newWebhookClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: safeDialControl}
	return &http.Client{
		Timeout: 4 * time.Second,
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// safeDialControl rechaza la conexión si la IP destino (ya resuelta) es interna.
func safeDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("webhook: destino no resuelto a IP: %q", host)
	}
	if domain.IsInternalIP(ip) {
		return fmt.Errorf("webhook: destino interno bloqueado (SSRF): %s", ip)
	}
	return nil
}
