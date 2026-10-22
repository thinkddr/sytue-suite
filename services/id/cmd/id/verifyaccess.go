// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
	"git.sytue.eu/sytue/one/services/id/internal/totp"
)

// Verificación de ACCESO — operación de despliegue, se ejecuta desde
// `04-verificar.sh`.
//
// Lo que cubre y ninguna otra comprobación cubría: que alguien pueda ENTRAR de
// verdad. Un 302 en la raíz de una superficie solo demuestra que hay un guard;
// la cadena real —formulario del IdP, contraseña, segundo factor, código de
// autorización, token endpoint con el client secret reconciliado y cookie del
// BFF— no la ejercitaba nada, y es donde estaban los fallos: `invalid_client`
// por un secreto sin reconciliar, un `redirect_uri` mal registrado, un
// post-logout que devolvía al bucle, y una consola de plataforma colgada de un
// rol de organización.
//
// Y comprueba la autorización que MÁS importa, en las dos direcciones: que
// `platform` deje entrar a un admin global (aunque no administre ninguna
// organización) y que RECHACE al `owner` de una organización cliente. Ese es el
// contrato de `security.md` §4.9 —allowlist `platform_admins` consultada en la
// base en cada petición, más segundo factor— y la única forma honesta de
// probarlo es con dos sesiones reales.
//
// «Entrar» en la consola global son TRES candados desde la mig 0043, no dos, y
// darlo por uno solo tenía al despliegue en rojo permanente: estar en
// `platform_admins` ya no autoriza nada por sí solo —dice quién PUEDE elevarse—,
// y lo que autoriza es una ELEVACIÓN viva, con motivo y ventana, que caduca sola.
// Un admin global recién inscrito recibe por diseño un 403 `activation_required`
// hasta que se eleva. Así que el recorrido que se verifica es el que hace una
// persona: pedir sin elevar y comprobar que niega POR ESE motivo (si negara con
// el código parco `forbidden` el fallo sería otro y estaría en otro sitio),
// elevarse, y solo entonces exigir el 200. Comprobar únicamente el 200 final
// habría dejado sin ejercitar la elevación, que es hoy el único camino de entrada
// y que hasta ahora no la recorría nadie en producción.
//
// Las dos cuentas son EFÍMERAS: se crean con contraseña y semilla TOTP
// aleatorias al empezar y se borran al terminar, pase lo que pase. No se puede
// reutilizar la del propietario inicial porque su segundo factor lo enrola una
// persona a mano y el despliegue no custodia esa semilla a propósito
// (`bootstrapowner.go`); y no se puede probar la denegación sin una
// organización cliente de verdad, porque el owner de la organización de
// plataforma no distingue «no eres admin global» de «no eres de esta casa».
const (
	// verifyPrefix marca TODO lo efímero que crea este comando. Es también el
	// criterio de barrido: una ejecución interrumpida (pod desalojado) deja
	// cuentas vivas, y la siguiente las retira antes de crear las suyas.
	verifyPrefix = "sytue-verify-"

	verifyDomainEnvVar = "SYTUE_VERIFY_DOMAIN"
	// verifyIngressEnvVar redirige TODAS las conexiones a esa dirección
	// (normalmente la ClusterIP de Traefik) conservando el nombre en el SNI y en
	// la validación del certificado. Evita depender de que el balanceador público
	// admita tráfico desde dentro del propio clúster (hairpin), que es una
	// propiedad del proveedor y no del producto.
	verifyIngressEnvVar = "SYTUE_VERIFY_INGRESS"
)

// verifySuperficies son las superficies con BFF de sesión. Quedan fuera `hub`
// (API sin sesión), `www` y `corporate` (estáticas y redirección).
var verifySuperficies = []string{
	"one", "mail", "drive", "docs", "docus", "calcus", "diapos", "certipdf", "calendar",
	"contacts", "rooms", "cloud", "admin", "platform",
}

// verifyIdentidad es una cuenta efímera de verificación con todo lo necesario
// para completar un login real: contraseña y semilla del segundo factor.
type verifyIdentidad struct {
	Email    string
	Password string
	Secret   []byte
	// OrgSlug vacío = miembro de la organización de plataforma existente.
	OrgSlug string
	// UserID lo rellena el alta; sirve para leer su rastro de auditoría.
	UserID string
}

func runVerifyAccess() int {
	dominio := strings.TrimSpace(os.Getenv(verifyDomainEnvVar))
	if dominio == "" {
		fmt.Fprintf(os.Stderr, "falta %s\n", verifyDomainEnvVar)
		return 2
	}
	dsn := os.Getenv("SYTUE_ID_DATABASE_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "falta SYTUE_ID_DATABASE_DSN")
		return 2
	}
	orgPlataforma := strings.TrimSpace(os.Getenv(bootstrapOrgSlugEnvVar))
	if orgPlataforma == "" {
		orgPlataforma = bootstrapDefaultOrgSlug
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 2})
	if err != nil {
		fmt.Fprintf(os.Stderr, "conexión: %v\n", err)
		return 1
	}
	defer pool.Close()

	// store.New sin cipher, por lo mismo que el bootstrap: este proceso no tiene
	// el KEK del servidor. La semilla TOTP se guarda por la ruta «en claro
	// heredado» de keycrypto, que es la que lee GetTOTPSecret al validar el
	// código. ponytail: la semilla vive minutos y se borra al terminar.
	st := store.New(pool)
	if err := configureCommandCallIdentity(ctx, st); err != nil {
		fmt.Fprintf(os.Stderr, "encrypted call identity guard: %v\n", err)
		return 1
	}

	if err := st.InTx(ctx, func(q pg.Querier) error { return retirarVerificadores(ctx, q) }); err != nil {
		fmt.Fprintf(os.Stderr, "barrido previo: %v\n", err)
		return 1
	}
	admin, inquilino, err := provisionarVerificadores(ctx, st, dominio, orgPlataforma)
	// El borrado va SIEMPRE, también si el provisionamiento quedó a medias: dejar
	// una cuenta con contraseña conocida viva en producción es peor que un fallo.
	defer func() {
		if err := st.InTx(context.WithoutCancel(ctx), func(q pg.Querier) error { return retirarVerificadores(context.WithoutCancel(ctx), q) }); err != nil {
			fmt.Fprintf(os.Stderr,
				"AVISO: no se pudieron retirar las cuentas de verificación (%s*): %v\n",
				verifyPrefix, err)
		}
	}()
	if err != nil {
		fmt.Fprintf(os.Stderr, "provisionar las cuentas de verificación: %v\n", err)
		return 1
	}

	return verificarAcceso(ctx, pool, dominio, os.Getenv(verifyIngressEnvVar), admin, inquilino)
}

// verificarAcceso ejecuta las comprobaciones y devuelve el código de salida.
//
//nolint:gocyclo // la secuencia es lineal: entrar, recorrer superficies y contrastar los dos veredictos de plataforma.
func verificarAcceso(
	ctx context.Context, q pg.Querier, dominio, ingress string, admin, inquilino verifyIdentidad,
) int {
	fallos := 0
	ok := func(msg string) { fmt.Printf("  ✓ %s\n", msg) }
	mal := func(msg string) { fmt.Printf("  ✗ %s\n", msg); fallos++ }

	clAdmin, err := clienteNavegador(ingress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cliente HTTP: %v\n", err)
		return 1
	}

	// Un solo login interactivo: lo demás es SSO, que es como entra una persona.
	// Repetirlo por superficie además agotaría el lockout suave por cuenta del
	// IdP (5/min por email) y convertiría un despliegue sano en un fallo.
	primera := "one." + dominio
	if err := entrarConMFA(ctx, clAdmin, dominio, primera, admin); err != nil {
		mal("no se puede entrar en la suite: " + err.Error())
		return 1
	}
	ok("login real en " + primera + ": contraseña + segundo factor EXIGIDO")

	for _, sub := range verifySuperficies {
		host := sub + "." + dominio
		if err := comprobarSuperficie(ctx, clAdmin, host); err != nil {
			mal(host + ": " + err.Error())
			continue
		}
		ok(host + " completa su ronda OIDC y reconoce la sesión")
	}

	plataforma := "platform." + dominio
	if err := alcanzarConsolaGlobal(ctx, clAdmin, plataforma); err != nil {
		// Se dice CON QUÉ cuenta se intentó. El IdP registra el `user_id` que
		// rechaza; sin este dato hay que adivinar si el que provisionó el
		// verificador y el que viaja en el token son el mismo, que es justo la
		// pregunta que decide dónde está el fallo.
		mal(fmt.Sprintf("la consola global es inalcanzable para el admin global "+
			"con user_id=%s (%s): %v", admin.UserID, admin.Email, err))
	} else {
		ok("platform AUTORIZA al admin global (allowlist platform_admins + MFA + elevación PIM)")
	}

	// La otra dirección, que es la que abre un agujero si falla: el owner de una
	// organización cliente NO administra la plataforma.
	clInquilino, err := clienteNavegador(ingress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cliente HTTP: %v\n", err)
		return 1
	}
	if err := entrarConMFA(ctx, clInquilino, dominio, plataforma, inquilino); err != nil {
		mal("el owner de la organización cliente no pudo iniciar sesión: " + err.Error())
		return salida(fallos)
	}
	switch codigo, motivo, err := estadoPlataforma(ctx, clInquilino, plataforma); {
	case err != nil:
		mal("consultar la API de plataforma como owner de organización: " + err.Error())
	case codigo != http.StatusForbidden:
		mal(fmt.Sprintf(
			"platform NO deniega al owner de una organización cliente (%d): "+
				"la administración global cuelga de un rol de organización", codigo,
		))
	case motivo != codigoForbidden:
		// El 403 tiene que venir del PRIMER candado. `activation_required` y
		// `mfa_required` significan que este owner ya pasó la allowlist —solo le
		// faltaba elevarse o repetir el MFA—, y eso es un agujero, no una
		// denegación: quedarse en «devolvió 403» lo daría por bueno.
		mal(fmt.Sprintf(
			"platform deniega al owner de una organización cliente por el motivo "+
				"equivocado (%q): ese código dice que SÍ está en platform_admins", motivo,
		))
	default:
		ok("platform DENIEGA al owner de una organización cliente")
	}

	if err := comprobarCorreoTransaccional(ctx, q, clInquilino, dominio, inquilino); err != nil {
		mal("el correo transaccional no sale: " + err.Error())
	} else {
		ok("el reseteo de contraseña sale de verdad por el MSA de Sytue Mail")
	}
	return salida(fallos)
}

func salida(fallos int) int {
	if fallos > 0 {
		fmt.Printf("\n%d comprobación(es) de acceso fallida(s)\n", fallos)
		return 1
	}
	fmt.Println("\nel acceso funciona de extremo a extremo y la plataforma autoriza a quien debe")
	return 0
}

// --- Cuentas efímeras -------------------------------------------------------

// provisionarVerificadores crea las dos cuentas en una transacción: el admin
// global (owner de la organización de plataforma e inscrito en platform_admins)
// y el owner de una organización CLIENTE recién creada, sin alcance global.
func provisionarVerificadores(
	ctx context.Context, st *store.Store, dominio, orgPlataforma string,
) (verifyIdentidad, verifyIdentidad, error) {
	sufijo, err := aleatorioHex(4)
	if err != nil {
		return verifyIdentidad{}, verifyIdentidad{}, err
	}
	admin, err := nuevaIdentidad("platform", sufijo, dominio, "")
	if err != nil {
		return verifyIdentidad{}, verifyIdentidad{}, err
	}
	inquilino, err := nuevaIdentidad("tenant", sufijo, dominio, verifyPrefix+sufijo)
	if err != nil {
		return verifyIdentidad{}, verifyIdentidad{}, err
	}

	orgP, err := st.GetOrgBySlug(ctx, orgPlataforma)
	if err != nil {
		return verifyIdentidad{}, verifyIdentidad{}, fmt.Errorf(
			"la organización de plataforma %q no existe (¿falta el bootstrap del propietario?): %w",
			orgPlataforma, err,
		)
	}

	err = st.InTx(ctx, func(q pg.Querier) error {
		if admin.UserID, err = altaVerificador(ctx, st, q, admin, orgP.ID); err != nil {
			return err
		}
		// granted_by vacío: lo concede el despliegue, no una persona. Se retira
		// con la cuenta (platform_admins cascadea con users).
		if err := st.UpsertPlatformAdmin(ctx, q, admin.UserID, "",
			"verificación de acceso del despliegue (cuenta efímera)"); err != nil {
			return fmt.Errorf("inscribir el verificador como admin de plataforma: %w", err)
		}
		orgC, err := ensureOrg(ctx, st, q, inquilino.OrgSlug, "Verificación de acceso")
		if err != nil {
			return err
		}
		inquilino.UserID, err = altaVerificador(ctx, st, q, inquilino, orgC.ID)
		return err
	})
	if err != nil {
		return verifyIdentidad{}, verifyIdentidad{}, err
	}
	return admin, inquilino, nil
}

// altaVerificador crea el usuario, lo hace owner de la organización indicada y
// le fija el segundo factor ya confirmado. Owner —y no member— porque la mitad
// de las superficies exige rol elevado, y ese rol exige a su vez step-up MFA:
// justo el camino que hay que ejercitar.
func altaVerificador(
	ctx context.Context, st *store.Store, q pg.Querier, id verifyIdentidad, orgID string,
) (string, error) {
	hash, err := domain.HashPassword(id.Password)
	if err != nil {
		return "", fmt.Errorf("hash de contraseña: %w", err)
	}
	user, err := ensureUser(ctx, st, q, hash, id.Email, "Verificación de acceso", true)
	if err != nil {
		return "", err
	}
	if err := st.UpsertTOTPSecret(ctx, q, user.ID, id.Secret, true); err != nil {
		return "", fmt.Errorf("fijar el segundo factor: %w", err)
	}
	if err := st.UpsertMembership(ctx, q, domain.Membership{
		OrgID:  orgID,
		UserID: user.ID,
		Role:   domain.RoleOwner,
	}); err != nil {
		return "", fmt.Errorf("fijar el rol de propietario: %w", err)
	}
	return user.ID, nil
}

// nuevaIdentidad genera una cuenta efímera con contraseña y semilla aleatorias.
// La contraseña son 32 hex (la política son 12 caracteres, sin complejidad).
func nuevaIdentidad(rol, sufijo, dominio, orgSlug string) (verifyIdentidad, error) {
	secret, err := totp.GenerateSecret()
	if err != nil {
		return verifyIdentidad{}, fmt.Errorf("generar la semilla TOTP: %w", err)
	}
	pass, err := aleatorioHex(16)
	if err != nil {
		return verifyIdentidad{}, err
	}
	id := verifyIdentidad{
		Email:    verifyPrefix + rol + "-" + sufijo + "@" + dominio,
		Password: pass,
		Secret:   secret,
		OrgSlug:  orgSlug,
	}
	// La política es la MISMA que aplica el alta: descubrirlo en el login sería
	// un fallo mudo que parecería un problema de despliegue.
	if err := domain.ValidatePassword(domain.DefaultPasswordPolicy, id.Password); err != nil {
		return verifyIdentidad{}, fmt.Errorf("contraseña efímera inválida: %w", err)
	}
	return id, nil
}

// retirarVerificadores borra TODO lo efímero, por prefijo. Borrado físico (no el
// lógico de DeleteUser): estas cuentas no tienen historia que preservar, y una
// fila `deleted` con contraseña conocida seguiría acumulándose en cada
// despliegue. La auditoría de sus logins sobrevive: audit_log no tiene FK.
func retirarVerificadores(ctx context.Context, q pg.Querier) error {
	if _, err := q.Exec(ctx, `DELETE FROM users WHERE email LIKE $1`, verifyPrefix+"%"); err != nil {
		return fmt.Errorf("borrar usuarios de verificación: %w", err)
	}
	if _, err := q.Exec(ctx, `DELETE FROM organizations WHERE slug LIKE $1`, verifyPrefix+"%"); err != nil {
		return fmt.Errorf("borrar organizaciones de verificación: %w", err)
	}
	return nil
}

func aleatorioHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("aleatoriedad: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// --- Flujo de acceso --------------------------------------------------------

// clienteNavegador es un cliente HTTP que se comporta como un navegador para lo
// único que importa aquí: guarda cookies y sigue redirecciones.
func clienteNavegador(ingress string) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	if ingress != "" {
		tr.DialContext = func(ctx context.Context, red, dir string) (net.Conn, error) {
			_, puerto, err := net.SplitHostPort(dir)
			if err != nil {
				return nil, err
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(
				ctx, red, net.JoinHostPort(ingress, puerto),
			)
		}
	}
	return &http.Client{
		Jar:       jar,
		Transport: tr,
		Timeout:   45 * time.Second,
		// La cadena completa (guard → authorize → formulario → MFA → callback)
		// encadena más saltos que el máximo por defecto de 10.
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 20 {
				return errors.New("demasiadas redirecciones (¿bucle de login?)")
			}
			return nil
		},
	}, nil
}

// entrarConMFA completa un login interactivo real desde una superficie: guard →
// IdP → contraseña → segundo factor → vuelta a la superficie con sesión.
func entrarConMFA(ctx context.Context, cl *http.Client, dominio, host string, id verifyIdentidad) error {
	idHost := "id." + dominio
	// /auth/login es el arranque del BFF y es idéntico en las diez superficies;
	// la raíz de cada app no lo es (unas redirigen, otras pintan algo público).
	resp, err := pedir(ctx, cl, http.MethodGet, "https://"+host+"/auth/login?returnTo=%2F", nil)
	if err != nil {
		return fmt.Errorf("arrancar el login: %w", err)
	}
	if resp.URL.Host != idHost {
		return fmt.Errorf("el login no llegó al IdP (acabó en %s, %d)", resp.URL, resp.Status)
	}
	returnTo := resp.URL.Query().Get("return_to")

	resp, err = pedir(ctx, cl, http.MethodPost, "https://"+idHost+"/login/", url.Values{
		"email":     {id.Email},
		"password":  {id.Password},
		"return_to": {returnTo},
	})
	if err != nil {
		return fmt.Errorf("enviar credenciales: %w", err)
	}
	// La contraseña sola NO abre sesión: el IdP debe reclamar el segundo factor.
	// Si aquí ya hubiéramos vuelto a la superficie, el MFA no se estaría
	// exigiendo y eso es un fallo de seguridad, no un atajo.
	if resp.URL.Query().Get("step_up") != "1" {
		return fmt.Errorf("el IdP no exigió el segundo factor tras la contraseña (acabó en %s, %d)",
			resp.URL, resp.Status)
	}
	returnTo = resp.URL.Query().Get("return_to")

	resp, err = pedir(ctx, cl, http.MethodPost, "https://"+idHost+"/login/mfa", url.Values{
		"code":      {totp.Code(id.Secret, time.Now())},
		"return_to": {returnTo},
	})
	if err != nil {
		return fmt.Errorf("enviar el segundo factor: %w", err)
	}
	if resp.URL.Host != host || resp.Status != http.StatusOK {
		return fmt.Errorf("el segundo factor no devolvió a la superficie (acabó en %s, %d)",
			resp.URL, resp.Status)
	}
	return nil
}

// comprobarSuperficie ejercita la ronda OIDC de una superficie con la sesión SSO
// ya abierta y confirma que su BFF emitió cookie propia. Es donde se rompe un
// client secret sin reconciliar (invalid_client) o un redirect_uri mal
// registrado, y ninguna de las dos cosas se ve pidiendo la raíz.
func comprobarSuperficie(ctx context.Context, cl *http.Client, host string) error {
	resp, err := pedir(ctx, cl, http.MethodGet, "https://"+host+"/auth/login?returnTo=%2F", nil)
	if err != nil {
		return fmt.Errorf("ronda OIDC: %w", err)
	}
	if resp.URL.Host != host {
		return fmt.Errorf("el SSO no devolvió a la superficie (acabó en %s, %d)",
			resp.URL, resp.Status)
	}
	resp, err = pedir(ctx, cl, http.MethodGet, "https://"+host+"/auth/session", nil)
	if err != nil {
		return fmt.Errorf("leer la sesión: %w", err)
	}
	if resp.Status != http.StatusOK || !strings.Contains(resp.Cuerpo, `"authenticated":true`) {
		return fmt.Errorf("el BFF no reconoce la sesión (%d)", resp.Status)
	}
	return nil
}

// comprobarCorreoTransaccional dispara un reseteo de contraseña real y espera al
// rastro que SOLO se escribe si el correo salió: `dispatchResetEmail` audita
// `auth.password.reset_requested` únicamente cuando el envío al MSA terminó bien
// (services/id/internal/http/forgot_password.go). Todo lo demás en ese camino es
// deliberadamente mudo —la respuesta es uniforme para no enumerar cuentas y el
// envío ocurre en segundo plano—, así que un SMTP inalcanzable o un MSA que
// rechaza el token de Sytue ID no se notaba hasta que alguien perdía su
// contraseña. Es el ÚNICO camino de recuperación de una cuenta.
//
// El mensaje va a la cuenta efímera, que se borra a continuación: la entrega
// acaba en «no such user» y no deja buzón ni cola viva. Lo que se está probando
// es que el IdP SABE ENTREGAR al MSA, no el buzón de destino.
func comprobarCorreoTransaccional(
	ctx context.Context, q pg.Querier, cl *http.Client, dominio string, id verifyIdentidad,
) error {
	if id.UserID == "" {
		return errors.New("la cuenta efímera no tiene id: no se puede leer su auditoría")
	}
	resp, err := pedir(ctx, cl, http.MethodPost, "https://id."+dominio+"/forgot-password",
		url.Values{"email": {id.Email}})
	if err != nil {
		return fmt.Errorf("pedir el enlace de reseteo: %w", err)
	}
	if resp.Status != http.StatusOK {
		return fmt.Errorf("el formulario de reseteo respondió %d", resp.Status)
	}
	// El envío ocurre en una goroutine desligada de la respuesta (para que la
	// latencia del correo no filtre si la cuenta existe), así que se espera.
	const sql = `SELECT EXISTS (
		SELECT 1 FROM audit_log
		WHERE actor_id = $1::uuid AND action = 'auth.password.reset_requested')`
	fin := time.Now().Add(45 * time.Second)
	for {
		var enviado bool
		if err := q.QueryRow(ctx, sql, id.UserID).Scan(&enviado); err != nil {
			return fmt.Errorf("leer la auditoría del envío: %w", err)
		}
		if enviado {
			return nil
		}
		if time.Now().After(fin) {
			return errors.New(
				"no hay rastro de envío 45 s después: revisa SMTP_ADDR de Sytue ID, " +
					"que el MSA (<release>-mail-mx:587) esté vivo y que acepte el token del IdP",
			)
		}
		time.Sleep(2 * time.Second)
	}
}

// codigoForbidden es el `code` del 403 parco de Sytue ID: «no estás en la
// allowlist». Los otros dos candados de la API de plataforma tienen código
// propio (`mfa_required`, `activation_required`) precisamente para poder
// distinguirlos, y este verificador es el que se lo cree.
const codigoForbidden = "forbidden"

// elevacionMinutos es la ventana que pide el verificador: el MÍNIMO que admite
// la API (5). El recorrido dura segundos y la cuenta se borra a continuación;
// pedir más solo dejaría una elevación global viva el día que el barrido falle.
const elevacionMinutos = 5

// alcanzarConsolaGlobal recorre lo que hace una persona con alcance global:
// tropezar con el gate sin estar elevado, elevarse con un motivo y entrar. El
// error que devuelve dice en cuál de los tres pasos se rompió, que es lo que
// decide dónde mirar.
func alcanzarConsolaGlobal(ctx context.Context, cl *http.Client, host string) error {
	codigo, motivo, err := estadoPlataforma(ctx, cl, host)
	if err != nil {
		return fmt.Errorf("consultar la API de plataforma: %w", err)
	}
	switch {
	case codigo == http.StatusOK:
		// Una elevación viva de antes (una ejecución anterior interrumpida). No es
		// el recorrido que se quería probar, pero lo que se afirma —que la consola
		// responde a un admin global— es cierto y decir lo contrario sería mentir.
		return nil
	case codigo != http.StatusForbidden:
		return fmt.Errorf("respondió %d sin elevar, y el contrato es 403 activation_required", codigo)
	case motivo == "mfa_required":
		return errors.New("el token no acredita segundo factor: revisa el claim `amr` " +
			"y el step-up de /authorize para el alcance de plataforma")
	case motivo != "activation_required":
		return fmt.Errorf("403 %q: el admin global no pasa la allowlist `platform_admins` "+
			"(la inscripción del verificador no llegó a la base o se consulta otra)", motivo)
	}

	if err := elevarPlataforma(ctx, cl, host); err != nil {
		return err
	}

	codigo, motivo, err = estadoPlataforma(ctx, cl, host)
	if err != nil {
		return fmt.Errorf("consultar la API de plataforma ya elevado: %w", err)
	}
	if codigo != http.StatusOK {
		return fmt.Errorf("sigue negando (%d %q) DESPUÉS de una elevación aceptada: "+
			"el gate no ve la activación que acaba de escribirse", codigo, motivo)
	}
	return nil
}

// elevarPlataforma pide la elevación temporal del alcance global. Va por el
// proxy de la consola como todo lo demás: es el único camino que tiene un
// navegador, y probar contra Sytue ID directamente dejaría sin verificar
// precisamente la allowlist de rutas del proxy.
func elevarPlataforma(ctx context.Context, cl *http.Client, host string) error {
	cuerpo := []byte(`{"justification":"verificación de acceso del despliegue",` +
		`"duration_minutes":` + strconv.Itoa(elevacionMinutos) + `}`)
	resp, err := pedirCuerpo(ctx, cl, http.MethodPost,
		"https://"+host+"/api/platform/activate", "application/json", cuerpo)
	if err != nil {
		return fmt.Errorf("pedir la elevación de plataforma: %w", err)
	}
	if resp.Status != http.StatusOK {
		return fmt.Errorf("la elevación de plataforma respondió %d (%s): "+
			"sin ella la consola global no la abre nadie",
			resp.Status, primeraLinea(resp.Cuerpo))
	}
	return nil
}

// estadoPlataforma devuelve el código con que la consola global responde a esta
// sesión y el `code` estable del error, si lo hubo. El proxy propaga los dos tal
// cual: 200 = autorizado; 403 = uno de los tres candados, y CUÁL lo dice el
// código, no el número.
func estadoPlataforma(ctx context.Context, cl *http.Client, host string) (int, string, error) {
	resp, err := pedir(ctx, cl, http.MethodGet, "https://"+host+"/api/platform/usage", nil)
	if err != nil {
		return 0, "", err
	}
	return resp.Status, codigoDeError(resp.Cuerpo), nil
}

// codigoDeError saca el `code` que el BFF de la consola copia del error de Sytue
// ID (`{"error":"upstream","status":403,"code":"activation_required",...}`). Un
// cuerpo que no sea ese JSON —una página de error de Traefik, por ejemplo— da
// cadena vacía y el llamador lo trata como «motivo desconocido».
func codigoDeError(cuerpo string) string {
	var v struct {
		Code string `json:"code"`
	}
	if json.Unmarshal([]byte(cuerpo), &v) != nil {
		return ""
	}
	return v.Code
}

// primeraLinea acota un cuerpo de error para meterlo en un mensaje de una línea.
func primeraLinea(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// respuesta es lo que queda de una petición ya consumida: su código, la URL final
// tras las redirecciones (lo que dice dónde acabó el login) y el cuerpo leído.
//
// Devolver esto en vez de un *http.Response es deliberado: el cuerpo se cierra
// dentro de pedir, y entregar una respuesta con el cuerpo ya cerrado es una trampa
// para quien la reciba —y lo que hacía que cada llamada pareciera una fuga.
type respuesta struct {
	Status int
	URL    *url.URL
	Cuerpo string
}

// pedir hace la petición con un formulario (o sin cuerpo, si form es nil).
func pedir(ctx context.Context, cl *http.Client, metodo, destino string, form url.Values) (respuesta, error) {
	if form == nil {
		return pedirCuerpo(ctx, cl, metodo, destino, "", nil)
	}
	return pedirCuerpo(ctx, cl, metodo, destino,
		"application/x-www-form-urlencoded", []byte(form.Encode()))
}

// pedirCuerpo hace la petición, agota y cierra el cuerpo, y devuelve lo anterior.
// El tipo va explícito porque las APIs de la consola hablan JSON y los
// formularios del IdP no.
func pedirCuerpo(
	ctx context.Context, cl *http.Client, metodo, destino, tipo string, datos []byte,
) (respuesta, error) {
	var cuerpo io.Reader
	if datos != nil {
		cuerpo = bytes.NewReader(datos)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, destino, cuerpo)
	if err != nil {
		return respuesta{}, err
	}
	if tipo != "" {
		req.Header.Set("Content-Type", tipo)
	}
	req.Header.Set("User-Agent", "sytue-verify-access")
	resp, err := cl.Do(req)
	if err != nil {
		return respuesta{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return respuesta{Status: resp.StatusCode, URL: resp.Request.URL, Cuerpo: string(b)}, err
}
