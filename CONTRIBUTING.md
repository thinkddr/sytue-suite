# Contribuir a Sytue One

Gracias por contribuir. Sytue One es una suite **100 % UE y soberana**, reimplementada como
**producto original** con licencia Apache-2.0 (clean-room sobre estándares abiertos). Antes
de empezar, lee el contrato canónico [`docs/CONVENTIONS.md`](docs/CONVENTIONS.md): es la
**fuente de verdad** de nombres, dominios, puertos, versiones, clients OIDC y licencias.
Cualquier cambio debe coincidir con él.

## Stack único

`Go + Astro + Preact + PostgreSQL` (+ S3 para blobs). **Backend 100 % Go** (módulo único
`git.sytue.eu/sytue/one`, un binario por servicio). **Frontend** Astro + Preact. **Único
datastore** PostgreSQL (estado, colas y caché). **Sin Redis/Valkey** y **sin ningún motor
AGPL de terceros** (Keycloak, Nextcloud, OnlyOffice, Mattermost, Stalwart están fuera). La
única app de terceros es **LiveKit** (Apache-2.0), SFU de medios de Sytue Meet.

**Las versiones no se citan aquí.** La política es **siempre la última estable** y los números
vivos están en la tabla de
[`docs/CONVENTIONS.md` §3](docs/CONVENTIONS.md#3-versiones-y-stack--politica-ultima-estable--cadena-de-suministro),
que es su fuente de verdad, y `make toolchain-check` comprueba que el entorno coincide. Este
párrafo decía «Astro 5» y «PostgreSQL 16» cuando la tabla ya iba por 7 y por 18: un número
repetido en prosa envejece sin que nadie lo note.

## Principios que rigen toda contribución

1. **Diseño primero y transversal.** Usa los componentes de `@sytue/ui` y monta
   `@sytue/shell`. No crees UI ad-hoc ni variantes de componentes.
2. **El IdP es nuclear.** Toda autenticación pasa por **Sytue ID** (servicio Go propio) vía
   OIDC + PKCE con BFF y cookie httpOnly `.sytue.eu`; cada servicio valida los JWT vía JWKS.
   Ningún producto gestiona credenciales propias.
3. **Integración seamless.** Reutiliza Sytue Hub (búsqueda, notificaciones, presencia,
   intents) en lugar de duplicar integración.
4. **100 % UE.** Solo regiones UE de Scaleway. No introduzcas dependencias de proveedores
   US para datos personales.
5. **Clean-room y licencia Apache-2.0.** El código de producto es **Apache-2.0, © 2026 Daniel Delgado
   Ramos (thinkddr)**. Solo se permiten **dependencias de terceros permisivas**
   (MIT/BSD/Apache-2.0/ISC/PostgreSQL/MPL-2.0); **prohibido** copiar/portar código AGPL/GPL o
   _source-available_. Quien implementa trabaja desde RFCs/specs, **nunca** desde el código de
   motores AGPL. Ver [`docs/security.md#4-politica-de-reimplementacion-clean-room`](docs/security.md#4-politica-de-reimplementacion-clean-room) y
   [`docs/security.md#5-licencias-propiedad-y-cumplimiento`](docs/security.md#5-licencias-propiedad-y-cumplimiento).
6. **Seguridad y privacidad por defecto.** RLS multi-tenant, consultas parametrizadas,
   deny-by-default, secretos solo en gestor de secretos (placeholders en git).

## Entorno de desarrollo

```bash
cp .env.example .env
cp deploy/compose/.env.example deploy/compose/.env
make dev-up          # levanta toda la suite en local (*.sytue.localhost)

make go-build        # compila todos los servicios Go
make go-test         # tests del módulo

make fe-install      # dependencias del frontend (Bun; versión en §3 de CONVENTIONS)
make fe-dev          # apps Astro en caliente (4321-4331)
```

Más detalle en el [README](README.md) y en [`docs/operations.md#1-runbook-de-operacion`](docs/operations.md#1-runbook-de-operacion).

### El editor y los gates tienen que decir lo mismo

`.vscode/` no se versiona (los ajustes personales son personales), pero hay cuatro que no son
gusto sino hechos del repositorio. Sin ellos el editor sale lleno de rojos y amarillos que
la consola no ve, y se acaba desconfiando de los dos. En VS Code, `.vscode/settings.json`:

```jsonc
{
  // El TypeScript DEL REPOSITORIO, no el que trae el editor: con una versión anterior,
  // opciones que solo existen en la nuestra salen mal en pantalla y bien en consola.
  "typescript.tsdk": "apps/frontend/node_modules/typescript/lib",
  "typescript.enablePromptUseWorkspaceTsdk": true,
  // Tailwind 4 usa `@theme` y `@source`; el validador de CSS del editor no los conoce.
  "css.lint.unknownAtRules": "ignore",
  // shellcheck tiene que seguir los `source` (-x) y resolverlos desde el directorio del
  // script (-P SCRIPTDIR); si no, inventa 24 avisos de «no encuentro common.sh».
  "shellcheck.customArgs": ["-x", "-P", "SCRIPTDIR"],
  // Los tests de integración de Go viven detrás de `//go:build integration`; sin la
  // etiqueta, gopls los da por excluidos del build.
  "gopls": { "build.buildFlags": ["-tags=integration"] },
}
```

En otros editores, el equivalente: SDK de TypeScript del workspace, validación de CSS sin
quejarse de at-rules desconocidas y etiqueta de build `integration` para el servidor de
lenguaje de Go.

Y al revés: **si el editor marca algo que el gate no marca, suele faltar cobertura en el
gate**, no sobrar razón al editor. Pasó con 139 ficheros de test (`49c0bd2d`): estaban
fuera de los `include` de sus `tsconfig`, así que el editor los miraba y la consola no.

### La documentación no la formatea Prettier

Los `.md` de la raíz y de `docs/` **quedan fuera de Prettier a propósito**, y no por
descuido: reordena las tablas alineando cada celda, y con celdas de 300 caracteres —las hay
a puñados— una sola fila se convierte en una línea de 500 que nadie lee en un diff.
Cambiaría diez mil líneas sin mejorar una coma de lo que ve el lector.

Lo que sí se comprueba es lo que puede mentir: `make docs-links` verifica que **todos los
enlaces internos llevan a algún sitio** —el fichero existe y el ancla existe dentro de él—.
Entra en `make gates`. En una documentación que es la fuente de verdad, un enlace roto hace
más daño que una tabla mal alineada.

## Convenciones de código

- **Go:** módulo único `git.sytue.eu/sytue/one`; un binario por servicio
  (`services/<svc>/cmd/<svc>`); capas `internal/{http,domain,store}`; errores envueltos;
  `context` en todo I/O; `slog` para logs; migraciones versionadas; consultas
  **parametrizadas** (sin SQL dinámico). `gofmt`, `go vet`, `golangci-lint`, tests
  (`make go-fmt`, `make go-lint`, `make go-test`). Cabecera SPDX (`Apache-2.0`) y aviso de copyright por archivo.
- **Frontend:** **TypeScript estricto**; **oxlint + Prettier** (`make lint`, `make fmt`);
  ESLint solo cubre los `*.astro`, que oxlint todavía no parsea.
  Nada de UI fuera de `@sytue/ui`.
- **Terraform** formateado (`make tf-fmt`), módulos reutilizables, estado en backend S3
  Scaleway.
- **Documentación de producto en español**; identificadores de código en inglés.
- Respeta la estructura del monorepo (§7 de CONVENTIONS.md). No dupliques utilidades que
  ya viven en `platform/pkg/*` (Go) o en los paquetes `@sytue/*` (frontend).

## Flujo de trabajo

1. Crea una rama desde `main`: `feat/<área>-<descripción>` o `fix/<...>`.
2. Haz cambios pequeños y enfocados; añade/actualiza documentación si procede.
3. Ejecuta `make fmt` y `make lint` antes de subir.
4. Abre un Pull Request describiendo el qué y el porqué, enlazando a la sección de
   CONVENTIONS.md o al ADR relevante. Si introduces una decisión de arquitectura, añade un
   ADR en [`docs/decisions.md`](docs/decisions.md).
5. Asegúrate de que el cambio mantiene la **paridad dev/prod** y no rompe el contrato.

## Definition of Done (resumen)

- Backend en Go; datos en PostgreSQL (multi-tenant por RLS); blobs en S3.
- Monta `@sytue/shell` + usa `@sytue/ui` (si es superficie de usuario).
- Autentica vía OIDC + PKCE contra Sytue ID, valida JWT por JWKS, cookie `.sytue.eu`.
- Funciona en dev local y es desplegable en Scaleway (Helm/Terraform).
- 100 % UE, sin secretos en git, **sin Redis/Valkey, sin dependencias AGPL**.
- Documentación actualizada en español.

## Licencia de las contribuciones

Las **pull requests son bienvenidas** de cualquier persona. El único mantenedor es
[@thinkddr](https://github.com/thinkddr) (Daniel Delgado Ramos): toda PR requiere su revisión y
aprobación, y solo él puede fusionarla. Al contribuir aceptas que tu aportación se publica bajo
**Apache-2.0** (© 2026 Daniel Delgado Ramos, ver [`LICENSE`](LICENSE)). No incluyas nunca secretos,
credenciales ni valores reales de variables de entorno. Las vulnerabilidades se notifican por
*Private vulnerability reporting*, no en issues públicas.
