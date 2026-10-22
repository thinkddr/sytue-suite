# GitHub Security → Codex → PR borrador

`codeql.yml` analiza Go, JavaScript/TypeScript, Python, GitHub Actions, C/C++,
Java y Swift y publica los resultados en **Security → Code scanning**. Se ejecuta
en PR, pushes a `main`, los viernes a las 06:38 UTC y manualmente. `security.yml`
ejecuta Dependency review en las PR, que rechaza vulnerabilidades nuevas de
severidad alta o crítica, y las pruebas de los helpers de esta automatización.
Estas pruebas también se ejecutan en push, semanalmente y manualmente. La CI
existente conserva Gitleaks y govulncheck.

`codex-security-fix.yml` permite encargar a Codex **una alerta abierta concreta**
de Code scanning o Dependabot. Codex investiga su aplicabilidad, prepara un
arreglo y ejecuta comprobaciones; después otro runner publica un PR borrador.
La selección manual evita lanzar tareas de pago por cada resultado del scanner.

## Activación

1. Publica estos archivos en la rama predeterminada del repositorio.
2. En **Settings → Advanced Security** (o **Code security and analysis**, según
   la interfaz), activa Dependency graph y Dependabot alerts. Para CodeQL,
   selecciona **Advanced setup** y utiliza `codeql.yml`; desactiva Default
   setup si ya está activo para evitar configuraciones incompatibles.
3. En **Settings → Secrets and variables → Actions**, configura los siguientes
   **repository secrets**. No los añadas a archivos del repositorio.

| Secret | Acceso requerido |
| --- | --- |
| `OPENAI_API_KEY` | Clave del proyecto de OpenAI con acceso a Codex. La ejecución consume API; configura los controles de gasto del proyecto. |
| `CODEX_PR_TOKEN` | Token fine-grained limitado a este repositorio con **Contents: Read and write** y **Pull requests: Read and write**. Solo lo utiliza el job que publica el PR. |
| `SECURITY_ALERTS_TOKEN` | Token fine-grained limitado a este repositorio con **Dependabot alerts: Read**. Solo es obligatorio y se utiliza para Dependabot; Code scanning usa `GITHUB_TOKEN` con `security-events: read`. |

Los permisos del token no sustituyen los permisos de su propietario para acceder
a las alertas. Autoriza el token en la organización si su política lo exige.
`CODEX_PR_TOKEN` puede sustituirse por un token de GitHub App equivalente si
adaptas su generación al workflow. Se usa un token propio para que el push y el
PR **activen la CI y CodeQL**: GitHub no dispara estos workflows al publicar con
el `GITHUB_TOKEN` de otra ejecución. No se requieren permisos de administración
ni de escritura sobre alertas.

CodeQL y Dependency review están disponibles para este repositorio público.
En un repositorio privado debes comprobar que el plan tenga las funciones de
GitHub Code Security correspondientes. El workflow de corrección solo acepta
ejecuciones sobre la rama predeterminada; seleccionar otra rama en **Run
workflow** omite los jobs.

## Asignar una alerta a Codex

1. Abre **Security → Code scanning** o **Security → Dependabot** y elige una
   alerta abierta. Copia su número, el último segmento de la URL.
2. Abre **Actions → Codex security fix → Run workflow**, elige la rama
   predeterminada y rellena `finding_type` (`code-scanning` o `dependabot`) y
   `alert_number`.
3. Consulta el PR borrador enlazado en el resumen del job `pull-request`.
   Revisa el diff, el informe de Codex y las comprobaciones de CI antes de
   aprobarlo. Tras el merge, GitHub vuelve a evaluar la alerta.

También puedes lanzarlo con GitHub CLI:

```sh
gh workflow run codex-security-fix.yml --ref main \
  -f finding_type=code-scanning -f alert_number=123

gh workflow run codex-security-fix.yml --ref main \
  -f finding_type=dependabot -f alert_number=45
```

Los números son ejemplos. Cada ejecución con cambios crea una rama
`codex/security/<tipo>-<número>-<run>-<intento>` y un PR nuevo; comprueba que no
haya otro PR abierto para esa alerta antes de repetirla. Las ejecuciones de una
misma alerta se serializan.

Si Codex no genera cambios, no se crea un PR. Consulta `report.md` en el artifact
`security-fix-<run>-<intento>` para ver si la alerta era un falso positivo, ya
estaba corregida o requiere intervención manual. El informe y el parche caducan
a los siete días; el contexto seleccionado de la alerta caduca al día.

## Permisos, alcance y límites

- **Separación de credenciales:** `prepare` lee la alerta con un token de solo
  lectura; `fix` recibe sus metadatos seleccionados y la clave de OpenAI; el
  último paso de `pull-request` recibe el token de publicación. El checkout no
  conserva credenciales. Codex usa `workspace-write` y `drop-sudo`, con red para
  resolver dependencias y ejecutar pruebas en un runner alojado y desechable.
- **Datos de entrada:** no se ejecuta texto procedente de alertas como shell. El
  helper valida el tipo y número, comprueba que la alerta siga abierta y
  selecciona únicamente los campos necesarios. El prompt trata su contenido
  como evidencia no confiable.
- **Transferencia del parche:** se fija el commit base entre los tres jobs. El
  runner de publicación comprueba el manifiesto, el hash y las rutas antes de
  publicar. No ejecuta pruebas ni código del parche con el token de escritura.
  Se admiten hasta 50 archivos y 1 MiB por parche.
- **Archivos protegidos:** los helpers rechazan cambios en `.github/`, `.codex/`,
  `.agents/`, `AGENTS.md`, `.gitmodules`, enlaces simbólicos y submódulos. Las
  alertas de GitHub Actions se detectan con CodeQL, pero su arreglo requiere una
  edición y revisión manual de los workflows. Codex debe dejarlo explicado en
  el informe.
- **Cobertura:** Go analiza el módulo raíz, como la CI existente; los módulos
  Go independientes requieren builds adicionales. Swift compila los targets
  macOS `drive-archivos`, `notes-widgets` y `SytueWidgetBridge` sin firma, para
  analizar las fuentes Swift sin empaquetar las aplicaciones Go/Fyne. Dependency
  review utiliza el
  grafo de GitHub y no garantiza la cobertura transitiva de `bun.lock`; conserva
  las comprobaciones del gestor de paquetes y Renovate. Para corregir una
  alerta CodeQL debe existir una instancia abierta en la rama predeterminada.
- **Secretos y divulgación:** este flujo no importa alertas de Secret scanning.
  Revoca o rota los secretos expuestos siguiendo el procedimiento de incidentes;
  borrar una cadena del código no revoca la credencial. Las propuestas e
  informes heredan la visibilidad del repositorio: los PR de este repositorio
  son públicos. Para vulnerabilidades bajo embargo utiliza un proceso privado.

## Comprobación local

```sh
actionlint .github/workflows/codeql.yml .github/workflows/security.yml .github/workflows/codex-security-fix.yml
node --test .github/scripts/security-alert.test.mjs .github/scripts/security-patch.test.mjs
```

Estas comprobaciones validan los workflows y la gestión de alertas/parches con
respuestas simuladas y repositorios temporales. La primera ejecución en GitHub
verificará los permisos, la configuración de CodeQL y el acceso a OpenAI.

Referencias: [OpenAI Docs: Codex GitHub Action](https://learn.chatgpt.com/docs/github-action),
[REST de Code scanning](https://docs.github.com/en/rest/code-scanning/code-scanning),
[REST de Dependabot](https://docs.github.com/en/rest/dependabot/alerts),
[eventos disparados desde workflows](https://docs.github.com/en/actions/how-tos/writing-workflows/choosing-when-your-workflow-runs/triggering-a-workflow),
[ecosistemas del grafo de dependencias](https://docs.github.com/en/code-security/reference/supply-chain-security/dependency-graph-supported-package-ecosystems).
