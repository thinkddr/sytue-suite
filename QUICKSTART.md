<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr) -->

# Sytue Suite — Guía rápida de despliegue (`QUICKSTART.md`)

Pon en marcha **Sytue Suite** en pocos minutos utilizando el asistente interactivo **`./sytue-deploy`**, que genera automáticamente todas las contraseñas y claves criptográficas sin necesidad de editar ficheros manualmente.

---

## 1. Requisitos previos

- **Comunes**: `git`, `bash`, `openssl`, `make`.
- **Para Docker Compose (local o servidor único)**:
  - Docker + Docker Compose v2.
  - [`mkcert`](https://github.com/FiloSottile/mkcert) (para certificados HTTPS locales en `*.sytue.localhost`).
- **Para Kubernetes (producción / clúster)**:
  - `kubectl` y `helm` v3/v4 conectados a tu clúster.
  - Servidor PostgreSQL 16+ y almacenamiento de objetos compatible con S3.

---

## 2. Paso a paso con el asistente `./sytue-deploy`

Ejecuta desde la raíz del repositorio:

```bash
./sytue-deploy
# o bien:
make deploy
```

El asistente te preguntará:

1. **Modo de despliegue**:
   - `1) Docker Compose` (entorno local o servidor único).
   - `2) Kubernetes / Helm` (clúster de producción o staging).
2. **Perfil y dominio**:
   - En Docker Compose puedes elegir levantar la suite completa (`full`), solo ofimática y productividad (`office`), solo el núcleo de identidad (`core`), o una lista de servicios a medida (`custom`, p. ej. `mail,drive,docs,rooms,portal`).
3. **Organización y administrador inicial**:
   - Slug de tu organización (por defecto `acme`), correo del administrador (p. ej. `admin@sytue.localhost`) y contraseña (si pulsas `Enter`, se genera una contraseña aleatoria segura y se muestra en pantalla al terminar).
4. **Generación automática de secretos**:
   - El CLI crea `deploy/compose/.env` (o `deploy/helm/values-custom.yaml` + `deploy/helm/apply-secrets.sh`) con claves únicas generadas mediante `openssl rand` para las 17 bases de datos PostgreSQL, claves AES-256-GCM de cifrado en reposo, secretos OIDC y LiveKit.
5. **Arranque**:
   - Si eliges iniciar los servicios desde el propio asistente, emitirá los certificados locales con `mkcert`, construirá y levantará los contenedores, y sembrará el usuario administrador en **Sytue ID**.

---

## 3. Comandos manuales (Docker Compose)

Si prefieres ejecutar cada paso por separado tras generar la configuración con `./sytue-deploy --only-config`:

```bash
# 1) Generar deploy/compose/.env de forma no interactiva (si aún no lo has hecho):
./sytue-deploy --mode compose --profile full --non-interactive --only-config

# 2) Levantar la suite (o usar `make dev-app P=docs,mail,portal` para ahorrar RAM):
make dev-up

# 3) Crear la organización inicial y el usuario administrador en Sytue ID:
make dev-seed

# 4) Comprobar el estado de los contenedores:
make dev-ps
```

---

## 4. Acceso a las aplicaciones

Con el entorno local en marcha, abre en tu navegador:

| Aplicación | URL local |
| --- | --- |
| **Sytue One (Portal principal)** | https://one.sytue.localhost |
| **Sytue ID (Inicio de sesión / Cuenta)** | https://id.sytue.localhost |
| **Sytue Admin (Consola de organización)** | https://admin.sytue.localhost |
| **Sytue Mail** | https://mail.sytue.localhost |
| **Sytue Drive** | https://drive.sytue.localhost |
| **SyDocus / SyCalcus / SyDiapos / SyCertiPDF** | https://docus.sytue.localhost · https://calcus.sytue.localhost · https://diapos.sytue.localhost · https://certipdf.sytue.localhost |
| **Sytue Rooms (Chat + Videollamadas)** | https://rooms.sytue.localhost |
| **Sytue Calendar & Contacts** | https://calendar.sytue.localhost · https://contacts.sytue.localhost |
| **Sytue Notepads & Photos** | https://notes.sytue.localhost · https://photos.sytue.localhost |
| **SyERP** | https://erp.sytue.localhost |
| **Sytue Cast / Sytue Web / Sytue Bases** | https://cast.sytue.localhost · https://pages.sytue.localhost · https://bases.sytue.localhost |
| **Sytue Cloud (Dashboard PaaS)** | https://cloud.sytue.localhost |
| **Sitio web público (`www`)** | https://www.sytue.localhost |

> **Nota sobre segundo factor (MFA) en local**: el rol `admin` en desarrollo tiene habilitado TOTP con la semilla de prueba `JBSWY3DPEHPK3PXP` para poder validar flujos completos con *step-up MFA* desde el primer minuto.

---

## 5. Parar o reiniciar el entorno

```bash
make dev-down      # Detiene los contenedores conservando los datos
make dev-clean     # Detiene los contenedores y elimina todos los volúmenes
```

Más detalles sobre despliegue en Kubernetes (Helm), configuración de DNS de correo, copias de seguridad y referencia completa de variables de entorno en [`docs/operations.md`](docs/operations.md).
