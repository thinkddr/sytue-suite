# GitHub Security → Codex con ChatGPT Pro

`codeql.yml` analiza Go, JavaScript/TypeScript, Python, GitHub Actions, C/C++,
Java y Swift y publica en **Security → Code scanning**. Se ejecuta en PR, pushes
a `main`, los viernes a las 06:38 UTC y manualmente. `security.yml` ejecuta
Dependency review en las PR, rechazando vulnerabilidades nuevas altas o críticas,
y comprueba los helpers de esta automatización. La CI ejecuta Gitleaks y govulncheck.

`codex-security-fix.yml` prepara un PR borrador con el contexto de una alerta
abierta y publica un comentario `@codex` con la cuenta conectada a ChatGPT.
El conector nativo recibe el encargo, investiga la alerta, hace el arreglo y
ejecuta comprobaciones en su entorno. El PR inicial contiene una solicitud,
**todavía no una corrección**; Codex debe sustituirla por el arreglo.

Este flujo no llama a OpenAI API, no instala Codex en Actions y no necesita
`OPENAI_API_KEY`. Utiliza el acceso a Codex de la cuenta ChatGPT Pro conectada,
sujeto a sus límites de uso. No compra créditos ni configura recargas. Al agotar
la cuota incluida, espera a su renovación en lugar de comprar uso adicional.

## Activación

1. Activa Dependency graph y Dependabot alerts. Para CodeQL utiliza **Advanced
   setup** con `codeql.yml`; desactiva Default setup para evitar incompatibilidades.
   Publica los workflows en la rama predeterminada.
2. Inicia sesión en Codex con **ChatGPT Pro** y conecta `thinkddr/sytue-suite`
   mediante la integración de GitHub. Comprueba el repositorio en
   [Codex settings](https://chatgpt.com/settings/code-review). Si pide un entorno
   cloud, selecciona o crea uno para este repositorio. La integración debe poder
   leer el repositorio y publicar cambios en la rama del PR.
3. Configura estos **repository secrets** en **Settings → Secrets and variables →
   Actions**. No guardes valores en archivos del repositorio.

| Secret | Permisos y propósito |
| --- | --- |
| `CODEX_PR_TOKEN` | Token fine-grained del mismo usuario GitHub conectado a tu cuenta Pro, limitado a este repositorio, con **Contents: Read and write** y **Pull requests: Read and write**. Publica el PR y la mención con tu identidad. |
| `SECURITY_ALERTS_TOKEN` | Token fine-grained limitado a este repositorio con **Dependabot alerts: Read**. Solo se utiliza para Dependabot. Code scanning usa `GITHUB_TOKEN` con `security-events: read`. |

No requieren administración, permisos de workflows ni escritura de alertas.
Utiliza una caducidad limitada y renueva los secrets antes de que venza. La cuenta
propietaria debe poder consultar las alertas seleccionadas. No copies sesiones
de ChatGPT ni `~/.codex/auth.json` a GitHub: el conector gestiona el acceso de Codex.

## Encargar una alerta a Codex

1. Abre una alerta en **Security → Code scanning** o **Security → Dependabot** y
   copia su número, el último segmento de la URL.
2. En **Actions → Codex security fix → Run workflow**, selecciona la rama
   predeterminada y rellena `finding_type` (`code-scanning` o `dependabot`) y
   `alert_number`.
3. Abre el PR enlazado en el resumen. Comprueba que Codex reaccione al comentario
   y aparezca el encargo en tu cuenta. El éxito del workflow confirma el envío,
   no la aceptación ni la finalización del trabajo por Codex.
4. Revisa el arreglo y la CI antes de aprobarlo. Codex debe retirar el archivo
   temporal `docs/security-findings/<tipo>-<número>.md` al terminar. GitHub vuelve
   a evaluar la alerta después del merge.

Si el conector no reacciona, comprueba la conexión, la identidad del token, el
entorno y la cuota. Puedes publicar tú mismo el comentario desde la cuenta
vinculada o abrir el PR en Codex y pedirle resolverlo. No uses una API key como
alternativa.

```sh
gh workflow run codex-security-fix.yml --ref main \
  -f finding_type=code-scanning -f alert_number=123

gh workflow run codex-security-fix.yml --ref main \
  -f finding_type=dependabot -f alert_number=45
```

Los números son ejemplos. Cada ejecución crea una rama
`codex/security/<tipo>-<número>-<run>-<intento>` y un PR nuevo. Comprueba que no
exista otro PR para la alerta antes de repetirla; las ejecuciones se serializan.

## Alcance y comprobaciones

- Lectura y publicación se ejecutan en jobs separados. El token de escritura
  solo se entrega al paso que publica la solicitud. Actions no ejecuta código
  generado por modelos.
- El tipo, número y estado se validan; solo se transfieren metadatos seleccionados,
  tratados como evidencia no confiable. El contexto temporal caduca al día.
- Go analiza el módulo raíz; los módulos independientes necesitan builds propios.
  Swift compila `drive-archivos`, `notes-widgets` y `SytueWidgetBridge` sin firma ni
  empaquetado Go/Fyne. Dependency review usa el grafo de GitHub y no garantiza
  todos los transitivos de `bun.lock`; conserva Renovate y los análisis del gestor.
- No se importan alertas de Secret scanning: las credenciales expuestas requieren
  revocación o rotación. El PR y su contexto heredan la visibilidad del repositorio;
  utiliza un proceso privado para vulnerabilidades bajo embargo.
- Codex debe confirmar la aplicabilidad, hacer un cambio mínimo, añadir pruebas
  relevantes y explicar qué comprobaciones ejecutó. No debe descartar alertas ni
  desactivar análisis para aparentar una corrección.

```sh
actionlint .github/workflows/codeql.yml .github/workflows/security.yml .github/workflows/codex-security-fix.yml
node --test .github/scripts/security-alert.test.mjs .github/scripts/security-request.test.mjs
```

Referencias oficiales: [tareas con @codex en GitHub](https://learn.chatgpt.com/docs/third-party/github),
[autenticación ChatGPT o API](https://learn.chatgpt.com/docs/auth),
[REST de Code scanning](https://docs.github.com/en/rest/code-scanning/code-scanning),
[REST de Dependabot](https://docs.github.com/en/rest/dependabot/alerts).
