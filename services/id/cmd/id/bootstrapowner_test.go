// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import "testing"

// entorno construye un lector de entorno de mentira a partir de un mapa.
func entorno(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

// El requisito es explícito: una producción nueva tiene que quedar con
// admin@sytue.eu dado de alta aunque el despliegue no pase ninguna
// variable de identidad. Solo las credenciales son obligatorias.
func TestBootstrapOwnerPorDefectoEsElAdminGlobal(t *testing.T) {
	in, err := bootstrapOwnerInputFromEnv(entorno(map[string]string{
		bootstrapPasswordEnvVar: "contraseña-de-despliegue-larga",
	}))
	if err != nil {
		t.Fatalf("entrada mínima rechazada: %v", err)
	}
	if in.Email != "admin@sytue.eu" {
		t.Errorf("correo por defecto inesperado: %q", in.Email)
	}
	if in.Name != "Daniel Delgado" {
		t.Errorf("nombre por defecto inesperado: %q", in.Name)
	}
	// El slug ES la dirección: la organización de Sytue vive en
	// corporate.sytue.eu, como cualquier cliente en <slug>.sytue.eu.
	if in.OrgSlug != "corporate" || in.OrgName != "Sytue" {
		t.Errorf("organización por defecto inesperada: %q / %q", in.OrgSlug, in.OrgName)
	}
}

func TestBootstrapOwnerNormalizaYExigeCredenciales(t *testing.T) {
	base := map[string]string{
		bootstrapPasswordEnvVar: "contraseña-de-despliegue-larga",
	}
	con := func(extra map[string]string) map[string]string {
		vars := map[string]string{}
		for k, v := range base {
			vars[k] = v
		}
		for k, v := range extra {
			vars[k] = v
		}
		return vars
	}

	t.Run("correo y slug se normalizan", func(t *testing.T) {
		in, err := bootstrapOwnerInputFromEnv(entorno(con(map[string]string{
			bootstrapEmailEnvVar:   "  Otro.Admin@Sytue.COM ",
			bootstrapOrgSlugEnvVar: " ACME ",
		})))
		if err != nil {
			t.Fatalf("rechazado: %v", err)
		}
		// users_email_lower_key impone unicidad en minúsculas: si el bootstrap
		// guardara «Otro.Admin@…» crearía un duplicado inaccesible del mismo buzón.
		if in.Email != "otro.admin@sytue.com" || in.OrgSlug != "acme" {
			t.Errorf("sin normalizar: %q / %q", in.Email, in.OrgSlug)
		}
		// Con un correo distinto del de por defecto, el nombre por defecto no aplica.
		if in.Name != "otro.admin@sytue.com" {
			t.Errorf("nombre inesperado: %q", in.Name)
		}
	})

	// Sin contraseña no hay alta: un default aquí sería una credencial conocida en
	// producción.
	t.Run("falta la contraseña", func(t *testing.T) {
		vars := con(nil)
		delete(vars, bootstrapPasswordEnvVar)
		if _, err := bootstrapOwnerInputFromEnv(entorno(vars)); err == nil {
			t.Fatalf("se aceptó un alta sin %s", bootstrapPasswordEnvVar)
		}
	})

	t.Run("contraseña que no cumple la política", func(t *testing.T) {
		// La política del formulario de login exige 12 caracteres. Descubrirlo en el
		// primer intento de acceso, y no en el despliegue, sería un fallo mudo.
		if _, err := bootstrapOwnerInputFromEnv(entorno(con(map[string]string{
			bootstrapPasswordEnvVar: "corta",
		}))); err == nil {
			t.Fatal("se aceptó una contraseña que el login rechazaría")
		}
	})

	t.Run("correo sin arroba", func(t *testing.T) {
		if _, err := bootstrapOwnerInputFromEnv(entorno(con(map[string]string{
			bootstrapEmailEnvVar: "no-es-un-correo",
		}))); err == nil {
			t.Fatal("se aceptó un correo inválido")
		}
	})
}
