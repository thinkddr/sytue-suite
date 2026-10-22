// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // GitHub firma con HMAC-SHA1 (X-Hub-Signature); se acepta por compatibilidad además de SHA-256.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// VerifyWebhookSignature valida la firma HMAC de un webhook git. Soporta el
// esquema de GitHub (sha256=... / sha1=...) y GitLab (token en cabecera).
// Conforme a CONVENTIONS §10 (validación de entrada, anti-spoofing).
//
// SIN SECRETO SE RECHAZA. Antes se aceptaba «para entornos de dev», y esa línea
// era la puerta: el endpoint de webhooks es PÚBLICO y sin autenticar, así que un
// secreto vacío convertía cualquier POST con un payload fabricado en un build y
// un despliegue del proyecto al que apuntara la URL del repo. Y el secreto
// estaba vacío en producción, porque nadie lo materializaba. La comodidad de dev
// no puede vivir en una función que también corre en producción: quien no quiera
// firmar, que configure un secreto (`config.validate` lo EXIGE en prod).
func VerifyWebhookSignature(secret string, body []byte, githubSig, gitlabToken string) error {
	if secret == "" {
		return domain.ErrSignatureInvalid
	}
	// GitLab: token compartido en X-Gitlab-Token (comparación en tiempo constante).
	if gitlabToken != "" {
		if hmac.Equal([]byte(gitlabToken), []byte(secret)) {
			return nil
		}
		return domain.ErrSignatureInvalid
	}
	// GitHub: HMAC del cuerpo con el secreto.
	if githubSig != "" {
		if verifyGitHubSig(secret, body, githubSig) {
			return nil
		}
		return domain.ErrSignatureInvalid
	}
	return domain.ErrSignatureInvalid
}

// VerifyWebhook resuelve el proyecto destino por la URL del repo (extraída del
// payload) y verifica la firma HMAC con el secreto DE ESE PROYECTO (descifrado en
// reposo), no con un secreto global compartido: así un tenant no puede falsificar
// entregas de otro. La URL del repo del payload no se "confía" pre-verificación —
// solo selecciona el secreto; sin él, un cuerpo falsificado no produce una firma
// válida. Para proyectos anteriores a los secretos por proyecto (enc nil) cae al
// secreto global de config (continuidad; el vector cross-tenant se cierra para los
// proyectos nuevos y para los antiguos al rotar). Ese global ya no puede estar
// vacío en producción: `config.RequireWebhookSecret` lo exige al arrancar la API
// —la única que sirve esta ruta— y `02b-secretos.sh` lo materializa, porque un
// fallback vacío equivalía a no exigir firma.
//
// Varias organizaciones pueden tener un proyecto con la misma URL: se devuelve el
// primero (el más antiguo) cuya firma valida, y ESE es el que se despliega —
// HandleGitEvent ya no vuelve a resolver la URL, que era una segunda consulta que
// podía devolver otro proyecto (CLOUD-05).
//
// Devuelve ErrNotFound si el repo no está conectado a ningún proyecto (el llamante
// responde 202) y ErrSignatureInvalid si la firma no valida con ninguno (401).
func (s *Service) VerifyWebhook(ctx context.Context, repoURL string, body []byte, githubSig, githubSigLegacy, gitlabToken string) (store.ResolvedRepo, error) {
	rrs, err := s.store.ResolveRepos(ctx, repoURL)
	if err != nil {
		return store.ResolvedRepo{}, err // ErrNotFound => 202 matched:false
	}
	for _, rr := range rrs {
		secret := s.cfg.WebhookSecret
		if len(rr.WebhookSecretEnc) > 0 {
			secret, err = s.sealer.OpenString(rr.WebhookSecretEnc)
			if err != nil {
				return store.ResolvedRepo{}, fmt.Errorf("service: descifrar secreto de webhook: %w", err)
			}
		}
		// Esquema sha256 preferente; sha1 legado de GitHub como alternativa.
		if VerifyWebhookSignature(secret, body, githubSig, gitlabToken) == nil ||
			VerifyWebhookSignature(secret, body, githubSigLegacy, gitlabToken) == nil {
			return rr, nil
		}
	}
	return store.ResolvedRepo{}, domain.ErrSignatureInvalid
}

func verifyGitHubSig(secret string, body []byte, sig string) bool {
	sig = strings.TrimSpace(sig)
	switch {
	case strings.HasPrefix(sig, "sha256="):
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		return hmac.Equal([]byte(expected), []byte(sig))
	case strings.HasPrefix(sig, "sha1="):
		mac := hmac.New(sha1.New, []byte(secret))
		mac.Write(body)
		expected := "sha1=" + hex.EncodeToString(mac.Sum(nil))
		return hmac.Equal([]byte(expected), []byte(sig))
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Parsing de payloads de webhook (clean-room: sobre la forma pública de los
// payloads de push/PR; sin código de terceros).
// ---------------------------------------------------------------------------

type ghPush struct {
	Ref     string `json:"ref"`
	After   string `json:"after"`
	Deleted bool   `json:"deleted"`
	Repo    struct {
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
		SSHURL   string `json:"ssh_url"`
	} `json:"repository"`
	Pusher struct {
		Name string `json:"name"`
	} `json:"pusher"`
	// El asunto del commit que se despliega. GitHub y Gitea lo mandan en
	// head_commit; en un push de borrado de rama viene null, y ahí queda vacío.
	HeadCommit struct {
		Message string `json:"message"`
	} `json:"head_commit"`
	// Rutas tocadas por el push. GitHub y Gitea las mandan por commit; sirven para
	// no recompilar un proyecto de un monorepo al que el push no le afecta. GitHub
	// TRUNCA `commits` en pushes grandes, y por eso la decisión es fail-open (ver
	// domain.GitEvent.TouchesDir): una lista corta nunca puede impedir un
	// despliegue que sí tocaba.
	Commits []struct {
		Added    []string `json:"added"`
		Modified []string `json:"modified"`
		Removed  []string `json:"removed"`
	} `json:"commits"`
}

// changedPaths junta las rutas de todos los commits del push.
func (p ghPush) changedPaths() []string {
	var out []string
	for _, c := range p.Commits {
		out = append(out, c.Added...)
		out = append(out, c.Modified...)
		out = append(out, c.Removed...)
	}
	return out
}

type ghPR struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		// Lo que describe una preview es el TÍTULO del PR, no el último commit:
		// la preview representa la propuesta entera, no su cabeza.
		Title string `json:"title"`
		Head  struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
			// Repo es el repositorio DESDE el que se propone el cambio. En un PR
			// de fork no es el nuestro, y llega a null si el fork se borró.
			Repo *ghRepoRef `json:"repo"`
		} `json:"head"`
	} `json:"pull_request"`
	Repo struct {
		ghRepoRef
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
}

// ghRepoRef es lo que identifica a un repositorio en el payload de GitHub, Gitea
// y Forgejo: el id numérico y el nombre completo `dueño/repo`.
type ghRepoRef struct {
	ID int64 `json:"id"`
	//nolint:tagliatelle // el nombre del campo lo fija la forja, no nuestra API
	FullName string `json:"full_name"`
}

// prDeFork dice si el PR viene de un repositorio que no es el que recibe el
// webhook.
//
// FALLA CERRADO: sin poder demostrar que la cabeza es el propio repositorio
// —`head.repo` ausente o a null, que es como llega cuando el fork se borró, o
// sin ningún identificador que comparar— se trata como fork. Detrás de un «no es
// fork» equivocado hay código de alguien sin permiso de escritura ejecutándose en
// el proyecto del cliente, así que la duda no se resuelve a favor de desplegar.
func prDeFork(head *ghRepoRef, base ghRepoRef) bool {
	if head == nil {
		return true
	}
	if head.ID != 0 && base.ID != 0 {
		return head.ID != base.ID
	}
	if head.FullName != "" && base.FullName != "" {
		return !strings.EqualFold(head.FullName, base.FullName)
	}
	return true
}

type glPush struct {
	Ref         string `json:"ref"`
	CheckoutSHA string `json:"checkout_sha"`
	UserName    string `json:"user_name"`
	Project     struct {
		GitHTTPURL string `json:"git_http_url"`
		WebURL     string `json:"web_url"`
	} `json:"project"`
	// GitLab no manda head_commit: manda la lista, en la que el ÚLTIMO es el que
	// apunta checkout_sha.
	Commits []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"commits"`
}

type glMR struct {
	ObjectAttributes struct {
		Action     string `json:"action"`
		IID        int    `json:"iid"`
		Title      string `json:"title"`
		LastCommit struct {
			ID string `json:"id"`
		} `json:"last_commit"`
		SourceBranch string `json:"source_branch"`
		// En GitLab un MR de fork tiene proyecto de origen distinto del de
		// destino. Punteros: su ausencia no es un cero que comparar.
		SourceProjectID *int64 `json:"source_project_id"`
		TargetProjectID *int64 `json:"target_project_id"`
	} `json:"object_attributes"`
	Project struct {
		GitHTTPURL string `json:"git_http_url"`
		WebURL     string `json:"web_url"`
	} `json:"project"`
}

// glPushMessage devuelve el asunto del commit desplegado. Se busca por ID en vez
// de coger el último a ciegas: el orden de `commits` no está garantizado por el
// contrato del webhook, solo que checkout_sha es el que se despliega.
func glPushMessage(p glPush) string {
	for _, c := range p.Commits {
		if c.ID == p.CheckoutSHA {
			return c.Message
		}
	}
	if n := len(p.Commits); n > 0 {
		return p.Commits[n-1].Message
	}
	return ""
}

// ParseWebhook normaliza un payload de webhook a un domain.GitEvent. provider y
// eventType vienen de las cabeceras (X-GitHub-Event / X-Gitlab-Event).
func ParseWebhook(provider domain.GitProvider, eventType string, body []byte) (domain.GitEvent, error) {
	switch provider {
	case domain.GitHub:
		return parseGitHub(eventType, body)
	case domain.GitLab:
		return parseGitLab(eventType, body)
	default:
		return domain.GitEvent{}, fmt.Errorf("%w: proveedor de webhook no soportado", domain.ErrInvalidInput)
	}
}

func parseGitHub(eventType string, body []byte) (domain.GitEvent, error) {
	switch eventType {
	case "push":
		var p ghPush
		if err := json.Unmarshal(body, &p); err != nil {
			return domain.GitEvent{}, fmt.Errorf("%w: payload push inválido", domain.ErrInvalidInput)
		}
		return domain.GitEvent{
			Provider:      domain.GitHub,
			RepoURL:       firstNonEmpty(p.Repo.CloneURL, p.Repo.HTMLURL),
			Ref:           p.Ref,
			CommitSHA:     p.After,
			CommitMessage: p.HeadCommit.Message,
			Pusher:        p.Pusher.Name,
			Deleted:       p.Deleted,
			ChangedPaths:  p.changedPaths(),
		}, nil
	case "pull_request":
		var p ghPR
		if err := json.Unmarshal(body, &p); err != nil {
			return domain.GitEvent{}, fmt.Errorf("%w: payload PR inválido", domain.ErrInvalidInput)
		}
		return domain.GitEvent{
			Provider:      domain.GitHub,
			RepoURL:       firstNonEmpty(p.Repo.CloneURL, p.Repo.HTMLURL),
			Ref:           p.PullRequest.Head.Ref,
			CommitSHA:     p.PullRequest.Head.SHA,
			CommitMessage: p.PullRequest.Title,
			PRNumber:      p.Number,
			Action:        p.Action,
			FromFork:      prDeFork(p.PullRequest.Head.Repo, p.Repo.ghRepoRef),
		}, nil
	default:
		return domain.GitEvent{}, fmt.Errorf("%w: evento GitHub ignorado", domain.ErrInvalidState)
	}
}

func parseGitLab(eventType string, body []byte) (domain.GitEvent, error) {
	switch eventType {
	case "Push Hook", "push":
		var p glPush
		if err := json.Unmarshal(body, &p); err != nil {
			return domain.GitEvent{}, fmt.Errorf("%w: payload push inválido", domain.ErrInvalidInput)
		}
		return domain.GitEvent{
			Provider:      domain.GitLab,
			RepoURL:       firstNonEmpty(p.Project.GitHTTPURL, p.Project.WebURL),
			Ref:           p.Ref,
			CommitSHA:     p.CheckoutSHA,
			CommitMessage: glPushMessage(p),
			Pusher:        p.UserName,
			Deleted:       p.CheckoutSHA == "" && strings.HasPrefix(p.Ref, "refs/heads/"),
		}, nil
	case "Merge Request Hook", "merge_request":
		var p glMR
		if err := json.Unmarshal(body, &p); err != nil {
			return domain.GitEvent{}, fmt.Errorf("%w: payload MR inválido", domain.ErrInvalidInput)
		}
		return domain.GitEvent{
			Provider:      domain.GitLab,
			RepoURL:       firstNonEmpty(p.Project.GitHTTPURL, p.Project.WebURL),
			Ref:           p.ObjectAttributes.SourceBranch,
			CommitSHA:     p.ObjectAttributes.LastCommit.ID,
			CommitMessage: p.ObjectAttributes.Title,
			PRNumber:      p.ObjectAttributes.IID,
			Action:        p.ObjectAttributes.Action,
			// Mismo criterio que en GitHub: sin los dos ids, es fork.
			FromFork: p.ObjectAttributes.SourceProjectID == nil ||
				p.ObjectAttributes.TargetProjectID == nil ||
				*p.ObjectAttributes.SourceProjectID != *p.ObjectAttributes.TargetProjectID,
		}, nil
	default:
		return domain.GitEvent{}, fmt.Errorf("%w: evento GitLab ignorado", domain.ErrInvalidState)
	}
}

// HandleGitEvent procesa un evento git normalizado: resuelve el proyecto por URL
// de repo, deduplica por deliveryID, y dispara build+deploy (producción si el
// push es a la rama de producción; preview si es un PR abierto/actualizado). Es
// un punto de entrada SIN tenant en el contexto (lo resuelve por la URL).
//
// Devuelve el deployment creado (si lo hubo) y si fue ignorado.
func (s *Service) HandleGitEvent(ctx context.Context, rr store.ResolvedRepo, ev domain.GitEvent, deliveryID string, rawBody []byte) (*domain.Deployment, bool, error) {
	dup, err := s.store.RecordWebhook(ctx, rr.Tenant, rr.Project.ID,
		string(ev.Provider), deliveryID, eventKind(ev), rawBody)
	if err != nil {
		return nil, false, err
	}
	if dup {
		return nil, true, nil // ya procesado (idempotencia)
	}

	// Construimos el contexto con el tenant resuelto para operar bajo RLS.
	tctx, err := s.withTenant(ctx, rr.Tenant)
	if err != nil {
		return nil, false, err
	}

	switch {
	case ev.IsPullRequest():
		if ev.Action == "closed" || ev.Deleted {
			// El cierre del PR retira la preview (lo hace el controller al ver el
			// deployment cancelado). Aquí cancelamos el preview activo del PR.
			return s.retirePreview(tctx, rr.Project.ID, ev.PRNumber)
		}
		// Un PR de fork NO genera vista previa, nunca (decisión del dueño,
		// 2026-10-06). Abrir un PR contra un repositorio público no exige permiso
		// alguno, y una vista previa es el código de ese PR construido y ejecutado
		// dentro del proyecto. Se responde «procesado» y no un error: el webhook
		// hizo su trabajo, y un 4xx haría reintentar a la forja.
		if ev.FromFork {
			return nil, true, nil
		}
		pr := ev.PRNumber
		dep, _, err := s.Deploy(tctx, DeployInput{
			ProjectID:     rr.Project.ID,
			CommitSHA:     ev.CommitSHA,
			CommitMessage: ev.CommitMessage,
			GitRef:        ev.Ref,
			Target:        domain.TargetPreview,
			PRNumber:      &pr,
			CreatedBy:     ev.Pusher,
		})
		if err != nil {
			return nil, false, err
		}
		return &dep, false, nil

	case branchName(ev.Ref) == rr.Project.ProductionRef && !ev.Deleted:
		// Monorepo: si el push no toca el subdirectorio de ESTE proyecto, no hay
		// nada que construir. Sin esto, tocar la documentación de un repo con seis
		// proyectos dentro lanza seis compilaciones que producen seis imágenes
		// idénticas a las que ya había, y se cobran los minutos de las seis.
		//
		// Se responde «procesado» (true), no un error: el webhook hizo su trabajo y
		// la decisión de no desplegar es correcta, no un fallo que reintentar.
		if !ev.TouchesDir(rr.Project.RootDir) {
			return nil, true, nil
		}
		dep, _, err := s.Deploy(tctx, DeployInput{
			ProjectID:     rr.Project.ID,
			CommitSHA:     ev.CommitSHA,
			CommitMessage: ev.CommitMessage,
			GitRef:        ev.Ref,
			Target:        domain.TargetProduction,
			CreatedBy:     ev.Pusher,
		})
		if err != nil {
			return nil, false, err
		}
		return &dep, false, nil

	default:
		return nil, true, nil // push a otra rama sin PR: se ignora
	}
}

// retirePreview cancela el preview activo de un PR (cierre/borrado). Consulta
// dirigida en lugar de escanear el listado (que ahora pagina por keyset y no
// garantiza ver previews antiguas en la primera página).
func (s *Service) retirePreview(ctx context.Context, projectID uuid.UUID, pr int) (*domain.Deployment, bool, error) {
	d, err := s.store.GetActivePreview(ctx, projectID, pr)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := s.store.CancelDeployment(ctx, d.ID); err != nil {
		return nil, false, err
	}
	return &d, false, nil
}

// withTenant deriva un contexto con el tenant resuelto (para operaciones que
// llegan sin tenant, como los webhooks).
func (s *Service) withTenant(ctx context.Context, tenant string) (context.Context, error) {
	t, err := tenancy.New(tenant)
	if err != nil {
		return nil, err
	}
	return tenancy.Into(ctx, t), nil
}

func eventKind(ev domain.GitEvent) string {
	if ev.IsPullRequest() {
		return "pull_request"
	}
	return "push"
}

// branchName extrae el nombre de rama de un ref (refs/heads/main -> main).
func branchName(ref string) string {
	const prefix = "refs/heads/"
	if strings.HasPrefix(ref, prefix) {
		return strings.TrimPrefix(ref, prefix)
	}
	return ref
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
