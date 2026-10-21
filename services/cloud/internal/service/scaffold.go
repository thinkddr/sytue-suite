// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/forge"
)

// ---------------------------------------------------------------------------
// Crear el repositorio nuevo, la tercera vía del alta
// ---------------------------------------------------------------------------
//
// Las otras dos vías parten de código que ya existe: conectar un repositorio o
// aplicar una plantilla de ajustes sobre uno. Esta es para quien todavía no tiene
// nada, que es justo quien menos ayuda tiene hoy: «pega la URL de tu repositorio»
// es una puerta cerrada si no hay repositorio.
//
// El alta entera son dos pasos y un rodeo por la forja:
//
//  1. `StartScaffold` guarda QUÉ se va a crear y manda al usuario a consentir.
//  2. `FinishScaffold` recoge el consentimiento, crea el repositorio copiando el
//     esqueleto del stack y aplica la plantilla sobre él —proyecto, servicios,
//     variables y primer despliegue, que ya sabía hacer `ApplyTemplate`—.
//
// Lo que NO hay: ni un árbol de ficheros dentro del binario (el esqueleto es un
// repositorio plantilla de la forja, ver `TemplateManifest.StarterRepo`), ni un
// token de la forja guardado en ninguna parte.

// scaffoldTTL es lo que se conserva la intención mientras el usuario consiente.
// Diez minutos: lo que tarda alguien en leer una pantalla de permisos y decidirse,
// y lo bastante corto para que un `state` filtrado no valga mañana.
const scaffoldTTL = 10 * time.Minute

// ScaffoldIntent es lo que el usuario pidió antes de irse a la forja.
//
// Viaja por la caché efímera y NO por el `state` de OAuth, que es la otra forma de
// hacerlo. La razón es que un `state` autocontenido lo escribe el navegador: quien
// supiera formarlo elegiría plantilla, slug y —lo que importa— el TENANT en el que
// se crea el proyecto. Aquí el `state` es un identificador opaco y aleatorio, y lo
// que significa solo lo sabe el servidor.
type ScaffoldIntent struct {
	Tenant     string `json:"tenant"`
	ActorSub   string `json:"actor_sub"`
	TemplateID string `json:"template_id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	RepoName   string `json:"repo_name"`
}

// Forge devuelve el cliente de la forja, o nil si Sytue Cloud no tiene
// credenciales para hablar con ella.
func (s *Service) Forge() *forge.Client { return s.forge }

// StartScaffold valida lo que se pide, lo guarda y devuelve a dónde mandar al
// usuario a dar su consentimiento.
//
// Se valida TODO aquí y no al volver: el usuario está delante. Descubrir después
// del rodeo por la forja que el nombre estaba cogido —con el repositorio ya
// creado— dejaría basura en su cuenta por un error que se podía ver antes.
func (s *Service) StartScaffold(ctx context.Context, in ScaffoldIntent) (string, error) {
	if s.forge == nil {
		return "", fmt.Errorf("%w: Sytue Cloud no está conectado a Sytue Git", domain.ErrInvalidInput)
	}
	if !s.cache.Enabled() {
		// Sin caché no hay dónde dejar la intención mientras el usuario consiente, y
		// meterla en el `state` sería dejar que la escriba el navegador.
		return "", fmt.Errorf("%w: el alta con repositorio nuevo no está disponible ahora", domain.ErrInvalidState)
	}
	tpl, ok := domain.StarterTemplateByID(in.TemplateID)
	if !ok {
		return "", fmt.Errorf("%w: no existe la plantilla %q", domain.ErrInvalidInput, in.TemplateID)
	}
	if tpl.StarterRepo == "" {
		return "", fmt.Errorf(
			"%w: la plantilla %q configura ajustes pero no trae esqueleto de proyecto",
			domain.ErrInvalidInput, in.TemplateID,
		)
	}
	if !domain.ValidSlug(in.Slug) {
		return "", fmt.Errorf("%w: nombre de proyecto inválido", domain.ErrInvalidInput)
	}
	if in.RepoName == "" {
		in.RepoName = in.Slug
	}
	if !domain.ValidSlug(in.RepoName) {
		return "", fmt.Errorf("%w: nombre de repositorio inválido", domain.ErrInvalidInput)
	}

	t := tenancy.MustFrom(ctx)
	in.Tenant = t.Slug
	raw, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("guardar la intención del alta: %w", err)
	}
	state, err := randomState()
	if err != nil {
		return "", err
	}
	if err := s.cache.SetTTL(ctx, scaffoldKey(state), string(raw), scaffoldTTL); err != nil {
		return "", fmt.Errorf("guardar la intención del alta: %w", err)
	}
	return s.forge.AuthorizeURL(state), nil
}

// FinishScaffold recoge el consentimiento y deja el proyecto creado y desplegando.
//
// `actorSub` es quien vuelve del rodeo, y tiene que ser el MISMO que se fue: sin
// esa comprobación, un `state` robado permitiría que otro completase el alta —y
// crease el repositorio en SU cuenta de la forja— dentro del tenant de la víctima.
func (s *Service) FinishScaffold(ctx context.Context, state, code, actorSub string) (TemplateApplied, string, error) {
	if s.forge == nil || !s.cache.Enabled() {
		return TemplateApplied{}, "", fmt.Errorf("%w: el alta con repositorio nuevo no está disponible", domain.ErrInvalidState)
	}
	raw, found, err := s.cache.Get(ctx, scaffoldKey(state))
	if err != nil {
		return TemplateApplied{}, "", fmt.Errorf("recuperar la intención del alta: %w", err)
	}
	if !found {
		return TemplateApplied{}, "", fmt.Errorf(
			"%w: la autorización caducó o ya se usó; vuelve a empezar", domain.ErrInvalidInput,
		)
	}
	// De un solo uso, y se consume ANTES de crear nada: si el usuario recarga el
	// callback, no se crean dos repositorios.
	_ = s.cache.Del(ctx, scaffoldKey(state))

	var intent ScaffoldIntent
	if err := json.Unmarshal([]byte(raw), &intent); err != nil {
		return TemplateApplied{}, "", fmt.Errorf("intención del alta ilegible: %w", err)
	}
	if intent.ActorSub != actorSub {
		return TemplateApplied{}, "", fmt.Errorf(
			"%w: esta autorización la empezó otra persona", domain.ErrPermission,
		)
	}
	t := tenancy.MustFrom(ctx)
	if intent.Tenant != t.Slug {
		return TemplateApplied{}, "", fmt.Errorf(
			"%w: esta autorización era de otra organización", domain.ErrPermission,
		)
	}
	tpl, ok := domain.StarterTemplateByID(intent.TemplateID)
	if !ok || tpl.StarterRepo == "" {
		return TemplateApplied{}, "", fmt.Errorf("%w: la plantilla ya no está disponible", domain.ErrInvalidInput)
	}

	token, err := s.forge.Exchange(ctx, code)
	if err != nil {
		return TemplateApplied{}, "", err
	}
	login, err := s.forge.CurrentUser(ctx, token)
	if err != nil {
		return TemplateApplied{}, "", err
	}
	repo, err := s.forge.GenerateFromTemplate(ctx, token, tpl.StarterRepo, login, intent.RepoName)
	if err != nil {
		return TemplateApplied{}, "", err
	}

	// Y a partir de aquí el alta es la de siempre: proyecto, servicios, variables y
	// primer despliegue en UNA transacción.
	//
	// Si esto falla, el repositorio se queda creado en la cuenta del usuario. Se
	// deja a propósito en vez de borrarlo: es SUYO, ya tiene el esqueleto dentro y
	// puede volver a intentarlo conectándolo por la vía normal. Borrar el trabajo
	// de alguien para dejar limpio un error nuestro sería la peor forma de fallar.
	return s.ApplyTemplate(ctx, ApplyTemplateInput{
		TemplateID: intent.TemplateID,
		Slug:       intent.Slug,
		Name:       intent.Name,
		RepoURL:    repo.CloneURL,
		ActorSub:   actorSub,
	})
}

func scaffoldKey(state string) string { return "cloud:scaffold:" + state }

// randomState son 256 bits de aleatoriedad. Es a la vez el identificador de la
// intención y la defensa CSRF del retorno, así que adivinarlo tiene que ser
// imposible, no improbable.
func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generar el estado de autorización: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
