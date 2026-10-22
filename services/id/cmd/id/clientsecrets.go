// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Reconciliación de los client secrets de las superficies propias — OPERACIÓN
// DE DESPLIEGUE, en cualquier entorno.
//
// El hueco que cubre: `clientauth.go` deja DESHABILITADO todo client
// confidencial sin `secret_hash` (deny-by-default, security.md §4.5), y la
// migración 0002 los siembra con `secret_hash` NULL a propósito. En dev los
// activa `seed` con un secreto compartido conocido, pero `seedAllowed` lo
// prohíbe fuera de dev — así que en producción NADIE los activaba: cada BFF
// recibía su secreto de Kubernetes y el token endpoint lo rechazaba con
// `invalid_client`. security.md §4.5 ya prometía «hasta que el deploy fije su
// secreto»; esto es ese paso del deploy.
//
// Por qué en el binario y no en un endpoint: fijar el secreto de un client de
// primera parte es exactamente la credencial que permite suplantar a una
// superficie entera. Igual que `delegated-client`, exige acceso al despliegue
// (el control de cambios del operador), no una sesión de administración.
//
// Entrada: un directorio con un fichero por client, cuyo NOMBRE es el client_id
// y cuyo contenido es el secreto. Es la forma natural de un Secret de Kubernetes
// montado con `items` (clave → ruta), así que el mapeo app→client_id vive en el
// chart, que ya lo declara, y no se duplica aquí.
const clientSecretsDirEnvVar = "SYTUE_ID_CLIENT_SECRETS_DIR" //nolint:gosec // G101: nombre de variable de entorno, no un secreto.

func runReconcileClientSecrets(args []string) int {
	dir := os.Getenv(clientSecretsDirEnvVar)
	if len(args) > 0 {
		dir = args[0]
	}
	if dir == "" {
		fmt.Fprintf(os.Stderr, "uso: id reconcile-client-secrets <directorio> (o %s)\n",
			clientSecretsDirEnvVar)
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

	// store.New sin cipher: solo se escriben hashes Argon2id, que no pasan por el
	// KEK. ponytail: sin cipher — no se toca material de firma ni secretos TOTP.
	changed, err := reconcileClientSecrets(ctx, store.New(pool), dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reconciliar client secrets: %v\n", err)
		return 1
	}
	fmt.Printf("client secrets reconciliados: %d cambiados\n", changed)
	return 0
}

// clientSecretsFromDir lee los pares client_id → secreto de un directorio. Los
// ficheros que empiezan por punto se ignoran: un Secret de Kubernetes montado
// expone `..data` y enlaces internos que NO son clients. Un secreto vacío es un
// error, no un client sin secreto: dejarlo pasar reproduciría en silencio el
// mismo deny-by-default que esto viene a arreglar.
func clientSecretsFromDir(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("leer %s: %w", dir, err)
	}
	secrets := make(map[string]string)
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // G304: ruta derivada de ReadDir del directorio pedido.
		if err != nil {
			return nil, fmt.Errorf("leer secreto de %s: %w", name, err)
		}
		// El salto de línea final es habitual al materializar un secreto a mano;
		// el BFF envía el valor tal cual lo recibe de Kubernetes, sin él.
		secret := strings.TrimRight(string(raw), "\r\n")
		if secret == "" {
			return nil, fmt.Errorf("el secreto de %s está vacío", name)
		}
		secrets[name] = secret
	}
	if len(secrets) == 0 {
		return nil, fmt.Errorf("%s no contiene ningún secreto", dir)
	}
	return secrets, nil
}

// reconcileClientSecrets fija el secret_hash de cada client cuyo secreto se
// aporta. Idempotente por VERIFICACIÓN, no por comparación: dos hashes Argon2id
// del mismo secreto difieren (sal aleatoria), así que se comprueba con
// VerifyPassword y solo se re-hashea lo que de verdad cambió. Eso hace que la
// operación valga igual para el primer despliegue y para una rotación.
//
// Un client ausente en la base es un ERROR: significa que el chart pide un
// client_id que Sytue ID no conoce (p. ej. `sytue-rooms` cuando el client de
// Rooms se llama `sytue-chat`), y el login de esa superficie fallaría con
// `invalid_client` tras un despliegue aparentemente correcto.
func reconcileClientSecrets(ctx context.Context, st *store.Store, dir string) (int, error) {
	secrets, err := clientSecretsFromDir(dir)
	if err != nil {
		return 0, err
	}
	clients, err := st.ListClients(ctx)
	if err != nil {
		return 0, fmt.Errorf("listar clients: %w", err)
	}
	byID := make(map[string]domain.Client, len(clients))
	for _, c := range clients {
		byID[c.ID] = c
	}

	// Orden estable: el log de un despliegue debe ser comparable entre ejecuciones.
	ids := make([]string, 0, len(secrets))
	for id := range secrets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var missing []string
	for _, id := range ids {
		if _, ok := byID[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return 0, fmt.Errorf("clients inexistentes en Sytue ID: %s", strings.Join(missing, ", "))
	}

	changed := 0
	for _, id := range ids {
		client := byID[id]
		if client.IsPublic() {
			return changed, fmt.Errorf("%s es un client público: no admite secreto", id)
		}
		if client.SecretHash != "" {
			ok, err := domain.VerifyPassword(secrets[id], client.SecretHash)
			if err != nil {
				return changed, fmt.Errorf("verificar secreto vigente de %s: %w", id, err)
			}
			if ok {
				continue
			}
		}
		hash, err := domain.HashPassword(secrets[id])
		if err != nil {
			return changed, fmt.Errorf("hash del secreto de %s: %w", id, err)
		}
		if err := st.SetClientSecretHash(ctx, nil, id, hash); err != nil {
			return changed, fmt.Errorf("fijar secreto de %s: %w", id, err)
		}
		fmt.Printf("  · %s\n", id)
		changed++
	}
	return changed, nil
}
