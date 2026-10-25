// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Account es el autoservicio de la persona contra Sytue ID (services/id, `/api/v1/me*`):
// lo que en la web es la app «Mi cuenta». Solo trae lo que se puede ENSEÑAR: los flujos
// que manejan secretos (contraseña, alta de TOTP, códigos de recuperación, alta de
// passkeys) no tienen método aquí a propósito, así que ninguno puede acabar en un log.
// La excepción es [Account.CreateAppPassword], que no se enseña: va directa al llavero.
type Account struct {
	c *Client
	// drive es opcional: de ahí sale el espacio GASTADO, que Sytue ID no sabe.
	drive *Client
}

// NewAccount crea el cliente sobre la raíz del servicio (ServiceURL(apex, "id")). El
// segundo servicio es OPCIONAL y es la raíz de Drive: de ahí sale cuánto espacio se ha
// gastado, que Sytue ID no sabe —la web hace lo mismo, PlanCard.tsx se lo pide a Drive—.
// Sin él, el plan se enseña con lo que concede pero SIN la barra de uso, que es mejor
// que una barra con un número inventado.
func NewAccount(servicio string, s Session, h *http.Client, drive ...string) *Account {
	a := &Account{c: New(servicio+"/api/v1", s, h)}
	if len(drive) > 0 && drive[0] != "" {
		a.drive = New(drive[0]+"/api/v1", s, h)
	}
	return a
}

// Profile es la ficha propia (`meProfileWire`).
type Profile struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Username      string `json:"username"`
	EmailVerified bool   `json:"email_verified"`
	// Phone y RecoveryEmail son de SOLO LECTURA aquí, como en la web: el móvil se
	// verificó por SMS al crear la cuenta y el correo de recuperación se cambia con un
	// enlace de confirmación y la contraseña delante (ADR 0041). Se declaran porque la
	// pantalla de la cuenta los ENSEÑA, no porque se puedan tocar por aquí.
	Phone         string `json:"phone"`
	RecoveryEmail string `json:"recovery_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Title         string `json:"title"`
	Department    string `json:"department"`
	Location      string `json:"location"`
	Locale        string `json:"locale"`
	MFAEnabled    bool   `json:"mfa_enabled"`
	// HasAvatar y AvatarVersion los añade el servicio aparte de `meProfileWire`
	// (`setAvatarFields`). La versión es el `updated_at` en base 36 y sirve para no
	// servir una foto rancia de la caché.
	HasAvatar     bool   `json:"has_avatar"`
	AvatarVersion string `json:"avatar_version"`
	// ActiveOrg es el slug de la organización ACTIVA, que es lo que la web enseña como
	// «Organización: …». Lo pone `GET /me` a partir del claim `tenant` DE ESTE TOKEN
	// (`handleMe`), o sea la misma fuente que lee la web (`userFromClaims`), resuelta
	// en el servicio. Solo viaja en `GET /me`: la respuesta de `PATCH /me` no lo trae,
	// así que quien guarde la ficha editada no debe machacar con ella lo que sabía.
	ActiveOrg string `json:"active_org"`
}

// ProfilePatch son los siete campos que edita su dueño. PUNTEROS, como en el servicio
// (`updateProfileRequest`): nil no viaja y el servicio conserva lo que hay; "" vacía el
// campo a propósito. Mandar la ficha entera pisaría lo que otra ventana acabe de tocar.
type ProfilePatch struct {
	Name       *string `json:"name,omitempty"`
	GivenName  *string `json:"given_name,omitempty"`
	FamilyName *string `json:"family_name,omitempty"`
	Title      *string `json:"title,omitempty"`
	Department *string `json:"department,omitempty"`
	Location   *string `json:"location,omitempty"`
	Locale     *string `json:"locale,omitempty"`
}

// AccountSession es una sesión abierta en Sytue ID. El servicio manda también `ip`, pero
// SIEMPRE vacía —la dirección no se persiste (mig 0039)—, así que aquí no se declara.
type AccountSession struct {
	ID        string    `json:"id"`
	Device    string    `json:"device"` // «Chrome · macOS», ya resumido por el servicio
	UserAgent string    `json:"user_agent"`
	AuthTime  time.Time `json:"auth_time"`
	ExpiresAt time.Time `json:"expires_at"`
	// Current solo lo marca el servicio si le llega SU cookie de sesión. Por el proxy de
	// la web en Gio y en nativo no viaja: aquí llega siempre en falso.
	Current bool `json:"current"`
}

// AccountDevice es un dispositivo reconocido (`deviceWire`).
type AccountDevice struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Trusted  bool      `json:"trusted"`
	LastSeen time.Time `json:"last_seen"`
	Current  bool      `json:"current"`
}

// Passkey es una clave de acceso dada de alta: su nombre y cuándo, nada del autenticador.
type Passkey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// AppPassword son los METADATOS de una contraseña de aplicación: el secreto no se guarda
// en el servicio y no viaja en el listado.
type AppPassword struct {
	// OrgID y Tenant son la organización fijada al crear la credencial.
	OrgID      string     `json:"org_id,omitempty"`
	Tenant     string     `json:"tenant,omitempty"`
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// Me devuelve la ficha propia.
func (a *Account) Me(ctx context.Context) (Profile, error) {
	return JSON[Profile](ctx, a.c, Request{Path: "/me"})
}

// UpdateMe guarda los campos que vengan y devuelve la ficha como ha quedado.
func (a *Account) UpdateMe(ctx context.Context, p ProfilePatch) (Profile, error) {
	datos, err := json.Marshal(p)
	if err != nil {
		return Profile{}, err
	}
	return JSON[Profile](ctx, a.c, Request{
		Method: http.MethodPatch, Path: "/me", Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// Sessions devuelve las sesiones abiertas, la más reciente primero.
func (a *Account) Sessions(ctx context.Context) ([]AccountSession, error) {
	out, err := JSON[struct {
		Sessions []AccountSession `json:"sessions"`
	}](ctx, a.c, Request{Path: "/me/sessions"})
	return out.Sessions, err
}

// RevokeSession cierra UNA sesión. El servicio contesta 204 también si no era de esta
// persona: no confirma si existe.
func (a *Account) RevokeSession(ctx context.Context, id string) error {
	return a.sinCuerpo(ctx, "/me/sessions/"+url.PathEscape(id))
}

// RevokeOtherSessions cierra todas menos la actual. «La actual» la reconoce el servicio
// por SU cookie; sin ella —nativo, o la web en Gio, cuyo proxy no la reenvía— su
// manejador dice que las cierra todas, la que emitió este token incluida.
func (a *Account) RevokeOtherSessions(ctx context.Context) error {
	return a.sinCuerpo(ctx, "/me/sessions")
}

func (a *Account) sinCuerpo(ctx context.Context, ruta string) error {
	return a.vacia(ctx, http.MethodDelete, ruta)
}

// vacia manda una petición sin cuerpo y tira la respuesta. Las RETIRADAS de esta
// pantalla —una sesión, un dispositivo, una clave de acceso, una contraseña de
// aplicación— no devuelven nada (204) y ninguna maneja un secreto: son la mitad
// que SÍ puede vivir fuera del navegador.
func (a *Account) vacia(ctx context.Context, metodo, ruta string) error {
	resp, err := a.c.Do(ctx, Request{Method: metodo, Path: ruta})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Devices devuelve los dispositivos reconocidos, el más reciente primero.
func (a *Account) Devices(ctx context.Context) ([]AccountDevice, error) {
	out, err := JSON[struct {
		Devices []AccountDevice `json:"devices"`
	}](ctx, a.c, Request{Path: "/me/devices"})
	return out.Devices, err
}

// TrustDevice marca de confianza un dispositivo. El servicio solo lo acepta para el
// dispositivo DESDE EL QUE se pide —exige la posesión de su cookie firmada—, así que
// ofrecerlo para los demás es prometer un fallo: uno robado no se marca a distancia,
// se olvida.
func (a *Account) TrustDevice(ctx context.Context, id string) error {
	return a.vacia(ctx, http.MethodPost, "/me/devices/"+url.PathEscape(id)+"/trust")
}

// ForgetDevice olvida un dispositivo reconocido.
func (a *Account) ForgetDevice(ctx context.Context, id string) error {
	return a.sinCuerpo(ctx, "/me/devices/"+url.PathEscape(id))
}

// Passkeys devuelve las claves de acceso dadas de alta.
func (a *Account) Passkeys(ctx context.Context) ([]Passkey, error) {
	out, err := JSON[struct {
		Credentials []Passkey `json:"credentials"`
	}](ctx, a.c, Request{Path: "/webauthn/credentials"})
	return out.Credentials, err
}

// DeletePasskey retira una clave de acceso. Retirar NO es dar de alta: el alta pasa
// por el autenticador y su reto, y por eso no tiene método aquí; quitar una es un
// DELETE con el identificador que ya salió en el listado.
//
// El servicio rechaza la última si es el único factor con el que se cumple la
// verificación en dos pasos, y su mensaje explica cómo salir del paso: quien llame
// enseña ESE y no uno propio.
func (a *Account) DeletePasskey(ctx context.Context, id string) error {
	return a.sinCuerpo(ctx, "/webauthn/credentials/"+url.PathEscape(id))
}

// AppPasswords devuelve las contraseñas de aplicación vivas: etiqueta y fechas.
func (a *Account) AppPasswords(ctx context.Context) ([]AppPassword, error) {
	out, err := JSON[struct {
		AppPasswords []AppPassword `json:"app_passwords"`
	}](ctx, a.c, Request{Path: "/me/app-passwords"})
	return out.AppPasswords, err
}

// AppPasswordNueva es una contraseña de aplicación recién creada: la ÚNICA vez que
// viaja el secreto.
type AppPasswordNueva struct {
	AppPassword
	Password string `json:"password"`
}

// CreateAppPassword crea una contraseña de aplicación con `etiqueta`. El secreto no se
// registra nunca: lo guarda la credencial de Drive en el almacén seguro del equipo
// (`documentos.Asegurar`), o lo enseña UNA vez el botón de un clic de Dispositivos.
func (a *Account) CreateAppPassword(ctx context.Context, etiqueta string) (AppPasswordNueva, error) {
	out, err := PostJSON[AppPasswordNueva](ctx, a.c, "/me/app-passwords", map[string]string{"label": etiqueta})
	if err == nil && (out.ID == "" || out.Password == "") {
		err = errors.New("api: Sytue ID no ha devuelto la contraseña de aplicación")
	}
	return out, err
}

// RevokeAppPassword revoca una contraseña de aplicación. No devuelve nada.
func (a *Account) RevokeAppPassword(ctx context.Context, id string) error {
	return a.sinCuerpo(ctx, "/me/app-passwords/"+url.PathEscape(id))
}

// MaxPerfilApple acota lo que se lee del perfil: son unos pocos kilobytes, más el
// certificado de la organización si lo tiene.
const MaxPerfilApple = 1 << 20

// PerfilApple pide el perfil de Apple (.mobileconfig) de este dispositivo: Sytue ID crea
// una contraseña de aplicación con `etiqueta` y la mete en las cuentas de correo,
// calendario y contactos, así que el sistema las instala sin pedir nada. La contraseña
// solo va dentro del perfil; se revoca en Mi cuenta como cualquier otra.
func (a *Account) PerfilApple(ctx context.Context, etiqueta string) ([]byte, error) {
	cuerpo, err := json.Marshal(map[string]string{"label": etiqueta})
	if err != nil {
		return nil, err
	}
	resp, err := a.c.Do(ctx, Request{
		Method: http.MethodPost, Path: "/me/apple-profile", Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}, "Accept": {"application/x-apple-aspen-config"}},
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, MaxPerfilApple+1))
	if err != nil {
		return nil, err
	}
	if len(datos) > MaxPerfilApple {
		return nil, ErrRespuestaGrande
	}
	return datos, nil
}

// EnlacePerfilApple pide el enlace de UN solo uso al perfil de Apple, para el iPhone:
// iOS solo instala perfiles que se abren en Safari, y Safari no tiene la sesión de la app.
// Vale unos minutos; al abrirlo, Sytue ID crea la contraseña de aplicación del perfil.
func (a *Account) EnlacePerfilApple(ctx context.Context, etiqueta string) (string, error) {
	out, err := PostJSON[struct {
		URL string `json:"url"`
	}](ctx, a.c, "/me/apple-profile/link", map[string]string{"label": etiqueta})
	if err == nil && out.URL == "" {
		err = errors.New("api: Sytue ID no ha devuelto el enlace del perfil")
	}
	return out.URL, err
}

// Organizacion es una de MIS organizaciones, como la sirve el BFF de la web
// (`/auth/organizations`): identificador, slug, nombre y si es la activa.
type Organizacion struct {
	ID     string
	Slug   string
	Nombre string
	Activa bool
}

// Organizaciones son las organizaciones de quien ha entrado en las que su membresía
// está activa, con la ACTIVA marcada por el claim `tenant` de este token (`active_org`
// de `GET /me`). Es exactamente el filtro del BFF: sin slug o sin estar activa, fuera.
func (a *Account) Organizaciones(ctx context.Context) ([]Organizacion, error) {
	me, err := JSON[struct {
		ActiveOrg   string `json:"active_org"`
		Memberships []struct {
			OrgID  string `json:"org_id"`
			Slug   string `json:"slug"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"memberships"`
	}](ctx, a.c, Request{Path: "/me"})
	if err != nil {
		return nil, err
	}
	var out []Organizacion
	for _, m := range me.Memberships {
		if m.Status != "active" || m.OrgID == "" || m.Slug == "" {
			continue
		}
		nombre := m.Name
		if nombre == "" {
			nombre = m.Slug
		}
		out = append(out, Organizacion{ID: m.OrgID, Slug: m.Slug, Nombre: nombre, Activa: m.Slug == me.ActiveOrg})
	}
	return out, nil
}

// CambiarOrganizacion hace activa otra de MIS organizaciones
// (`PUT /api/v1/me/active-organization`). El token que ya se tiene sigue llevando la
// anterior —el `tenant` va firmado dentro— y el servicio revoca los refresh tokens, así
// que quien llama tiene que volver a entrar: es el `reauthorize` de la respuesta.
func (a *Account) CambiarOrganizacion(ctx context.Context, id string) error {
	cuerpo, err := json.Marshal(map[string]string{"org_id": id})
	if err != nil {
		return err
	}
	resp, err := a.c.Do(ctx, Request{
		Method: http.MethodPut, Path: "/me/active-organization", Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Subscription es el contrato de la organización activa tal y como lo cuenta Sytue ID
// (`handleSubscriptionStatus`): qué plan rige HOY, cuánto espacio concede y hasta cuándo
// está pagado. `Plan` es el vigente (`EffectivePlan`) y `ContractedPlan` el contratado:
// se diferencian mientras un cambio no ha entrado en vigor.
type Subscription struct {
	Status         string     `json:"status"`
	Plan           string     `json:"plan"`
	ContractedPlan string     `json:"contracted_plan"`
	StorageBytes   int64      `json:"storage_bytes"`
	MailboxBytes   int64      `json:"mailbox_bytes"`
	PaidUntil      *time.Time `json:"paid_until"`
	Entitled       bool       `json:"entitled"`
	Personal       bool       `json:"personal"`
	Education      bool       `json:"education"`
	// UpgradeAvailable lo decide el servicio, no una constante de aquí: abrir los
	// planes de pago a las cuentas personales tiene que ser UN solo cambio.
	UpgradeAvailable bool `json:"upgrade_available"`
}

// Subscription devuelve el contrato de la organización activa.
func (a *Account) Subscription(ctx context.Context) (Subscription, error) {
	return JSON[Subscription](ctx, a.c, Request{Path: "/onboarding/subscription"})
}

// AltaCompletada dice si quien ha entrado ya terminó el alta guiada de Sytue ID
// (`GET /onboarding`, el mismo que pide el portal antes de mandar a `/bienvenida`).
func (a *Account) AltaCompletada(ctx context.Context) (bool, error) {
	r, err := JSON[struct {
		Completed bool `json:"completed"`
	}](ctx, a.c, Request{Path: "/onboarding"})
	return r.Completed, err
}

// AvatarMaxBytes acota lo que se baja del avatar. El servicio solo guarda un data URI
// raster ya validado y acotado (validation.RasterImageDataURI), así que esto es un
// cinturón por si el que contesta no es el que se cree.
const AvatarMaxBytes = 2 << 20

// Quota es el espacio de Drive: cuánto hay y cuánto se ha gastado. `hay` en falso es
// que este cliente no sabe dónde está Drive, no que la organización no tenga cuota.
func (a *Account) Quota(ctx context.Context) (q DriveQuota, hay bool, err error) {
	if a.drive == nil {
		return DriveQuota{}, false, nil
	}
	q, err = JSON[DriveQuota](ctx, a.drive, Request{Path: "/quota"})
	return q, err == nil, err
}

// Avatar baja la foto propia y su tipo. Devuelve (nil, "", nil) —y NO un error— cuando
// la persona no tiene: quedarse en las iniciales no es un fallo que contar.
func (a *Account) Avatar(ctx context.Context) ([]byte, string, error) {
	resp, err := a.c.Do(ctx, Request{Path: "/me/avatar", Header: http.Header{"Accept": {"image/*"}}})
	if err != nil {
		var httpErr *Error
		if errors.As(err, &httpErr) && (httpErr.Status == http.StatusNotFound || httpErr.Status == http.StatusNoContent) {
			return nil, "", nil
		}
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, AvatarMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(datos) > AvatarMaxBytes {
		return nil, "", errors.New("api: el avatar pesa más de lo que Sytue ID admite")
	}
	return datos, resp.Header.Get("Content-Type"), nil
}
