// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package forge habla con Sytue Git (Gitea) EN NOMBRE DEL USUARIO, para la única
// cosa que Sytue Cloud no puede hacer sola: crear su repositorio.
//
// # Por qué OAuth y no un token de administrador
//
// La alternativa barata era darle a Cloud un token de admin de la forja y usar la
// cabecera `Sudo` para actuar como cualquiera —el patrón que ya usa
// `services/integrations` para migrar repositorios—. Aquí no vale, y no por
// purismo: ese token crea repositorios en la cuenta de CUALQUIER usuario sin que
// nadie lo autorice, así que un fallo en Cloud pasa a ser un fallo en la forja de
// todo el mundo. Con OAuth, lo que Cloud puede hacer es exactamente lo que ESE
// usuario acaba de consentir, y él lo ve y lo revoca desde su cuenta.
//
// Como Sytue Git ya delega el login en Sytue ID, quien tiene sesión ve la
// pantalla de consentimiento y vuelve: el «1 clic» es literal.
//
// # El token no se guarda
//
// Se canjea, se usa para crear el repositorio y se descarta. No hay tabla, ni
// cifrado en reposo, ni refresco, ni una credencial de la forja esperando a que
// alguien lea la base de datos. Si algún día Cloud necesita hablar con la forja
// más allá del alta —ramas, webhooks—, entonces habrá que guardarlo, y ese día se
// decide con lo que haga falta entonces.
package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// Client habla con la API de Sytue Git.
type Client struct {
	baseURL      string
	clientID     string
	clientSecret string
	redirectURI  string
	http         *http.Client
}

// New construye el cliente. Devuelve nil si falta configuración: sin credenciales
// de OAuth no hay forma de crear un repositorio en nombre de nadie, y fingir que
// sí la hay convertiría el botón en un error a mitad de camino.
//
// Quien lo llama tiene que enseñar esa ausencia, no esconderla: un botón que no
// aparece sin decir por qué es indistinguible de una función que no existe.
func New(baseURL, clientID, clientSecret, redirectURI string) *Client {
	if baseURL == "" || clientID == "" || clientSecret == "" || redirectURI == "" {
		return nil
	}
	return &Client{
		baseURL:      strings.TrimSuffix(baseURL, "/"),
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		// Con guardián de SSRF igual que el resto de salidas del control-plane: la
		// URL de la forja viene de la configuración, pero su DNS no lo controlamos.
		http: ssrfhttp.NewClient(15*time.Second, 3),
	}
}

// UsarHTTPParaTest presta un cliente HTTP distinto.
//
// Existe porque el servidor de pruebas escucha en 127.0.0.1 y con su propio
// certificado, que son exactamente las dos cosas que el cliente de producción
// rechaza a propósito. Sin esta costura, el camino entero —canje, usuario y
// creación— no se podría ejecutar en ningún test.
func (c *Client) UsarHTTPParaTest(h *http.Client) { c.http = h }

// AuthorizeURL es a dónde se manda al usuario a dar su consentimiento.
//
// `state` es opaco aquí a propósito: quien lo genera es el servicio, que lo ata a
// la sesión y guarda con él la intención (qué plantilla, qué nombre). Si esta
// función lo inventara, el callback no tendría forma de saber qué se estaba
// creando ni quién lo pidió.
func (c *Client) AuthorizeURL(state string) string {
	q := url.Values{
		"client_id":     {c.clientID},
		"redirect_uri":  {c.redirectURI},
		"response_type": {"code"},
		"state":         {state},
	}
	return c.baseURL + "/login/oauth/authorize?" + q.Encode()
}

// Exchange canjea el código por un token de acceso del usuario.
func (c *Client) Exchange(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {c.redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("forge: componer el canje: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.do(req, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("forge: la forja no devolvió token de acceso")
	}
	return out.AccessToken, nil
}

// Repo es el repositorio recién creado.
type Repo struct {
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
	HTMLURL  string `json:"html_url"`
}

// CurrentUser devuelve el login de quien autorizó. Hace falta porque el
// repositorio se crea EN SU CUENTA, y el nombre de usuario de la forja no tiene
// por qué parecerse al de Sytue ID.
func (c *Client) CurrentUser(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/user", http.NoBody)
	if err != nil {
		return "", fmt.Errorf("forge: componer la consulta de usuario: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)
	var out struct {
		Login string `json:"login"`
	}
	if err := c.do(req, &out); err != nil {
		return "", err
	}
	if out.Login == "" {
		return "", fmt.Errorf("forge: la forja no dijo quién autorizó")
	}
	return out.Login, nil
}

// GenerateFromTemplate crea un repositorio del usuario copiando un repositorio
// PLANTILLA de la forja.
//
// Lo copia la forja, no nosotros: `POST /repos/{owner}/{repo}/generate` deja el
// repositorio con su historia limpia y su rama por defecto en un solo paso. La
// alternativa —clonar el esqueleto en el control-plane y subir fichero a fichero—
// habría metido el contenido de un repositorio ajeno en nuestra memoria y en
// nuestro disco para acabar en el mismo sitio.
//
// `private` es TRUE siempre: el repositorio de alguien que acaba de empezar no se
// publica en internet porque el formulario no tuviera una casilla. Hacerlo público
// después es un clic en la forja; deshacerlo, imposible —ya lo indexó alguien—.
func (c *Client) GenerateFromTemplate(ctx context.Context, token, template, owner, name string) (Repo, error) {
	tOwner, tRepo, ok := strings.Cut(template, "/")
	if !ok || tOwner == "" || tRepo == "" {
		return Repo{}, fmt.Errorf("forge: plantilla %q mal formada (owner/repo)", template)
	}
	body, err := json.Marshal(map[string]any{
		"owner":       owner,
		"name":        name,
		"private":     true,
		"git_content": true,
		"description": "Creado desde Sytue Cloud",
	})
	if err != nil {
		return Repo{}, fmt.Errorf("forge: serializar la petición: %w", err)
	}
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/generate",
		c.baseURL, url.PathEscape(tOwner), url.PathEscape(tRepo))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return Repo{}, fmt.Errorf("forge: componer la creación: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")

	var repo Repo
	if err := c.do(req, &repo); err != nil {
		return Repo{}, err
	}
	if repo.CloneURL == "" {
		return Repo{}, fmt.Errorf("forge: la forja creó el repositorio pero no dijo dónde")
	}
	return repo, nil
}

// maxResponse acota lo que se lee de la forja. Ninguna de estas respuestas pasa de
// unos kilobytes.
const maxResponse = 256 * 1024

// do ejecuta y decodifica, traduciendo los errores a algo que se pueda enseñar.
//
// El cuerpo del error NO se propaga: puede llevar el `code` o fragmentos del token
// en un mensaje de la forja, y eso acabaría en un log o en una respuesta de la API.
func (c *Client) do(req *http.Request, out any) error {
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("forge: Sytue Git no responde")
	}
	defer func() { _ = res.Body.Close() }()

	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return fmt.Errorf("forge: Sytue Git rechazó la autorización")
	case res.StatusCode == http.StatusConflict:
		return fmt.Errorf("forge: ya tienes un repositorio con ese nombre")
	case res.StatusCode == http.StatusNotFound:
		return fmt.Errorf("forge: la plantilla no existe en Sytue Git")
	case res.StatusCode >= 300:
		return fmt.Errorf("forge: Sytue Git respondió %d", res.StatusCode)
	}
	// LimitReader porque al otro lado hay un servidor: «devuelve un flujo infinito»
	// es la forma barata de tumbar a quien lo lee entero en memoria.
	dec := json.NewDecoder(io.LimitReader(res.Body, maxResponse))
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("forge: respuesta ilegible de Sytue Git")
	}
	return nil
}
