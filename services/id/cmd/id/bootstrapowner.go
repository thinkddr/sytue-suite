// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Alta del PRIMER propietario — OPERACIÓN DE DESPLIEGUE, idempotente.
//
// El hueco que cubre: Sytue ID no tiene registro público. Un usuario nace por
// INVITACIÓN (`invitations.go`) o por login federado, y ambas cosas exigen que ya
// exista alguien dentro. En dev ese alguien lo crea `id seed`, que aborta fuera de
// dev (`seedAllowed`). Resultado: una producción recién instalada no tenía NINGÚN
// usuario, así que la suite era inalcanzable y nadie podía invitar a nadie.
//
// Crea tres cosas de una vez, porque las tres son necesarias para que alguien
// pueda entrar y operar: la organización que opera la plataforma, su propietario, y
// la inscripción de ese propietario en `platform_admins` (mig 0031). El alcance de
// plataforma NO viene del rol `owner`: ningún rol de organización ve más de una
// organización, y eso sigue siendo cierto. Son dos permisos distintos que este
// alta concede a la vez a la misma persona.
//
// El segundo factor NO se provisiona aquí, y eso es deliberado: se enrola por
// autoservicio en `id.sytue.eu/account` («Activar verificación en dos pasos», con
// QR). Con solo la contraseña, este usuario abre sesión EN SYTUE ID y nada más:
// `owner` exige step-up MFA para completar `authorize` (`requiresStepUp`), así que
// Sytue Admin y el resto de la suite siguen cerrados hasta que enrole su factor.
// Manejar aquí un secreto TOTP obligaría al despliegue a custodiar —y a alguien a
// leer de un log— la semilla de un segundo factor: peor, y para nada.
const (
	// Valores por defecto: una producción nueva DEBE quedar con un admin global
	// utilizable aunque el despliegue no pase ninguna variable. Se pueden
	// sobreescribir, pero no dejar en blanco.
	bootstrapDefaultEmail = "admin@sytue.eu"
	bootstrapDefaultName  = "Daniel Delgado"
	// El slug ES la dirección de la organización: cada una vive en
	// <slug>.sytue.eu, así que la de Sytue es corporate.sytue.eu. No se usa
	// `sytue` para no acabar en sytue.sytue.eu.
	bootstrapDefaultOrgSlug = "corporate"
	bootstrapDefaultOrgName = "Sytue"

	bootstrapEmailEnvVar    = "SYTUE_ID_BOOTSTRAP_EMAIL"
	bootstrapNameEnvVar     = "SYTUE_ID_BOOTSTRAP_NAME"
	bootstrapOrgSlugEnvVar  = "SYTUE_ID_BOOTSTRAP_ORG_SLUG"
	bootstrapOrgNameEnvVar  = "SYTUE_ID_BOOTSTRAP_ORG_NAME"
	bootstrapPasswordEnvVar = "SYTUE_ID_BOOTSTRAP_PASSWORD" //nolint:gosec // G101: nombre de variable de entorno, no un secreto.
	// Slug ANTERIOR de la organización de plataforma. Si se indica y todavía
	// existe, el bootstrap la RENOMBRA (datos incluidos) en vez de crear una
	// segunda. Es el camino de una instalación que ya se desplegó con otro slug:
	// sin esto habría que ejecutar `id rename-org` a mano y en el orden correcto,
	// antes de que este mismo bootstrap creara la duplicada.
	bootstrapRenameFromEnvVar = "SYTUE_ID_BOOTSTRAP_ORG_RENAME_FROM"
)

// bootstrapOwnerInput son los datos del propietario inicial, ya validados.
type bootstrapOwnerInput struct {
	Email    string
	Name     string
	OrgSlug  string
	OrgName  string
	Password string
	// RenameFrom es el slug anterior a renombrar (vacío = no hay nada que migrar).
	RenameFrom string
}

func runBootstrapOwner() int {
	in, err := bootstrapOwnerInputFromEnv(os.Getenv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	dsn := os.Getenv("SYTUE_ID_DATABASE_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "falta SYTUE_ID_DATABASE_DSN")
		return 2
	}

	ctx := context.Background()
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 2})
	if err != nil {
		fmt.Fprintf(os.Stderr, "conexión: %v\n", err)
		return 1
	}
	defer pool.Close()

	// store.New sin cipher: en producción el secreto TOTP se cifra con el KEK del
	// servidor, pero este proceso no lo tiene. Se guarda por la ruta "en claro
	// heredado" de keycrypto, la misma que lee GetTOTPSecret. ponytail: sin cipher
	// — el alta inicial no debe exigir el material de firma del IdP.
	st := store.New(pool)
	if err := configureCommandCallIdentity(ctx, st); err != nil {
		fmt.Fprintf(os.Stderr, "encrypted call identity guard: %v\n", err)
		return 1
	}
	if err := bootstrapOwner(ctx, st, dsn, in); err != nil {
		fmt.Fprintf(os.Stderr, "alta del propietario inicial: %v\n", err)
		return 1
	}
	fmt.Printf("propietario y admin de plataforma listos: %s en %s\n", in.Email, in.OrgSlug)
	fmt.Println("siguiente paso humano: entrar en /account y activar la verificación en dos pasos;")
	fmt.Println("hasta entonces el rol owner mantiene cerrada Sytue Admin y el resto de la suite.")
	return 0
}

// bootstrapOwnerInputFromEnv lee y valida la entrada. Función pura sobre un
// lector de entorno para poder probar cada rechazo sin tocar el proceso.
//
// La contraseña se valida con la MISMA política que el formulario de login
// (DefaultPasswordPolicy): descubrir en el primer intento de acceso que la
// contraseña generada no cumple la política sería un fallo mudo del despliegue.
func bootstrapOwnerInputFromEnv(getenv func(string) string) (bootstrapOwnerInput, error) {
	in := bootstrapOwnerInput{
		Email:      strings.ToLower(strings.TrimSpace(getenv(bootstrapEmailEnvVar))),
		Name:       strings.TrimSpace(getenv(bootstrapNameEnvVar)),
		OrgSlug:    strings.ToLower(strings.TrimSpace(getenv(bootstrapOrgSlugEnvVar))),
		OrgName:    strings.TrimSpace(getenv(bootstrapOrgNameEnvVar)),
		Password:   getenv(bootstrapPasswordEnvVar),
		RenameFrom: strings.ToLower(strings.TrimSpace(getenv(bootstrapRenameFromEnvVar))),
	}
	if in.Email == "" {
		in.Email = bootstrapDefaultEmail
	}
	if in.OrgSlug == "" {
		in.OrgSlug = bootstrapDefaultOrgSlug
	}
	// La contraseña NO tiene valor por defecto: la genera y custodia el gestor de
	// secretos del despliegue (02b). Un default aquí sería una contraseña conocida
	// en producción.
	if in.Password == "" {
		return bootstrapOwnerInput{}, fmt.Errorf("falta %s", bootstrapPasswordEnvVar)
	}
	if !strings.Contains(in.Email, "@") {
		return bootstrapOwnerInput{}, fmt.Errorf("%s no es una dirección de correo", bootstrapEmailEnvVar)
	}
	if in.Name == "" {
		in.Name = bootstrapDefaultName
		if in.Email != bootstrapDefaultEmail {
			in.Name = in.Email
		}
	}
	if in.OrgName == "" {
		in.OrgName = bootstrapDefaultOrgName
		if in.OrgSlug != bootstrapDefaultOrgSlug {
			in.OrgName = in.OrgSlug
		}
	}
	if err := domain.ValidatePassword(domain.DefaultPasswordPolicy, in.Password); err != nil {
		return bootstrapOwnerInput{}, fmt.Errorf("%s no cumple la política: %w", bootstrapPasswordEnvVar, err)
	}
	return in, nil
}

// bootstrapOwner crea (o reafirma) organización, usuario, contraseña, rol de
// propietario y alcance de plataforma. Idempotente y en UNA transacción: un
// despliegue interrumpido no deja al propietario a medias —sin rol o sin
// plataforma—, que es el estado en el que no puede entrar ni arreglarlo.
func bootstrapOwner(ctx context.Context, st *store.Store, dsn string, in bootstrapOwnerInput) error {
	hash, err := domain.HashPassword(in.Password)
	if err != nil {
		return fmt.Errorf("hash de contraseña: %w", err)
	}
	// Renombrado previo, si la instalación se desplegó con otro slug. Va ANTES de
	// todo lo demás y arrastra los datos de las nueve bases de servicio: el slug
	// es el tenant de la suite, no una etiqueta (ver renameorg.go). Idempotente:
	// si ya se hizo, no toca nada.
	if in.RenameFrom != "" && in.RenameFrom != in.OrgSlug {
		movidas, err := renameOrg(ctx, st, dsn, in.RenameFrom, in.OrgSlug)
		switch {
		case errors.Is(err, errRenameYaHecho):
			// Nada que migrar: el renombrado ya se aplicó en un despliegue anterior.
		case err != nil:
			return fmt.Errorf("renombrar %s → %s: %w", in.RenameFrom, in.OrgSlug, err)
		default:
			fmt.Printf("organización renombrada: %s → %s (%d filas reetiquetadas)\n",
				in.RenameFrom, in.OrgSlug, movidas)
		}
	}

	// Guard contra la SEGUNDA organización de plataforma. Si el propietario ya
	// existe y es owner de otra organización, crear una nueva con este slug
	// dejaría dos —y el despliegue no diría nada—: la suite quedaría partida en
	// dos tenants, con los datos en el viejo y la sesión apuntando al nuevo. Pasa
	// al cambiar el slug configurado, que es exactamente lo que hay que hacer con
	// `id rename-org` para que los datos viajen con él.
	if err := comprobarOrgUnica(ctx, st, in); err != nil {
		return err
	}
	return st.InTx(ctx, func(q pg.Querier) error {
		org, err := ensureOrg(ctx, st, q, in.OrgSlug, in.OrgName)
		if err != nil {
			return err
		}
		// mfa=false: el factor lo enrola la persona en /account. Marcarlo aquí
		// como habilitado sin secreto la dejaría fuera incluso de Sytue ID.
		user, err := ensureUser(ctx, st, q, hash, in.Email, in.Name, false)
		if err != nil {
			return err
		}
		if err := st.UpsertMembership(ctx, q, domain.Membership{
			OrgID:  org.ID,
			UserID: user.ID,
			Role:   domain.RoleOwner,
		}); err != nil {
			return fmt.Errorf("fijar rol de propietario: %w", err)
		}
		// granted_by vacío: no hay un admin anterior al primero. Que la
		// procedencia sea NULL y no «se lo dio a sí mismo» es lo que hace
		// distinguible el alta del despliegue de una concesión humana.
		if err := st.UpsertPlatformAdmin(ctx, q, user.ID, "",
			"alta inicial del despliegue"); err != nil {
			return fmt.Errorf("inscribir como admin de plataforma: %w", err)
		}
		return nil
	})
}

// comprobarOrgUnica rechaza crear una organización nueva si el propietario ya lo
// es de otra: el caso real es un slug configurado distinto del que se desplegó, y
// la respuesta correcta no es duplicar sino renombrar.
func comprobarOrgUnica(ctx context.Context, st *store.Store, in bootstrapOwnerInput) error {
	if _, err := st.GetOrgBySlug(ctx, in.OrgSlug); err == nil {
		return nil // La organización de destino ya existe: alta idempotente normal.
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	user, err := st.GetUserByEmail(ctx, in.Email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil // Instalación nueva: no hay nada que duplicar.
		}
		return err
	}
	memberships, err := st.ListMembershipsForUser(ctx, user.ID)
	if err != nil {
		return err
	}
	for _, m := range memberships {
		if m.Role != domain.RoleOwner {
			continue
		}
		org, err := st.GetOrgByID(ctx, m.OrgID)
		if err != nil || org.Slug == in.OrgSlug {
			continue
		}
		return fmt.Errorf(
			"%s ya es propietario de la organización %q y se pidió crear %q: "+
				"renombra la existente en vez de duplicarla "+
				"(`id rename-org %s %s`, que reetiqueta también sus datos)",
			in.Email, org.Slug, in.OrgSlug, org.Slug, in.OrgSlug,
		)
	}
	return nil
}
