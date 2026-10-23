# GitHub Security → Codex con ChatGPT Pro

`codeql.yml` analiza Go, JavaScript/TypeScript, Python, GitHub Actions, C/C++,
Java y Swift y publica en **Security → Code scanning**. Se ejecuta en PR, pushes
a `main`, los viernes a las 06:38 UTC y manualmente. `security.yml` ejecuta
Dependency review en las PR, rechazando vulnerabilidades nuevas altas o críticas,
y comprueba los helpers de esta automatización. La CI ejecuta Gitleaks y govulncheck.

`codex-security-fix.yml` busca automáticamente alertas abiertas de Code scanning
en la rama predeterminada y de Dependabot, prepara un PR borrador con el contexto
de una de ellas y publica un comentario `@codex` con la cuenta conectada a ChatGPT.
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

## Asignación automática

No hace falta pulsar **Run workflow**. La cola se revisa:

- Al terminar correctamente **CodeQL Advanced** en `main`, para ejecuciones
  iniciadas por push, horario o ejecución manual; ignora análisis de PR y forks.
- Cada seis horas, a las 00:37, 06:37, 12:37 y 18:37 UTC, para recoger también
  alertas nuevas de Dependabot. GitHub puede retrasar ejecuciones programadas.
- Al publicar cambios del workflow o de sus scripts en `main`, para comprobar
  la configuración y arrancar la cola.

Cada ejecución selecciona **una** alerta y mantiene como máximo **dos PR de
encargo abiertas**. Ordena por gravedad (crítica, alta, media, baja y otras) y
después por número de alerta. Lee ambas listas completas con paginación acotada;
si falla el acceso, la configuración o se supera el límite, falla sin publicar
una selección parcial. Las ejecuciones se serializan para evitar carreras.

El historial de PR guarda qué alertas ya se encargaron, mediante la rama y un
marcador en el cuerpo. No repite automáticamente encargos ya cerrados o fusionados;
cerrar una PR de encargo sin fusionarla deja esa alerta fuera de la cola. También
excluye alertas referenciadas en otras PR abiertas cuya rama pertenece a este
repositorio, con su URL canónica o con un título explícito
`code scanning alerts #N to #M`. Las PR de forks no reservan alertas. No elimina
ramas ni fusiona PR.
Cuando revises y fusiones o cierres un encargo, la siguiente ejecución podrá
utilizar ese hueco. El resumen de Actions indica la selección o el motivo de espera.

Si la PR se publica pero falla el comentario, la siguiente ejecución retoma esa
misma PR antes de abrir otra. Comprueba si el comentario ya existe para evitar
una segunda mención y solo marca el envío como completado después de confirmarlo.

Un workflow correcto confirma el envío. Comprueba la reacción del conector en la
PR y el encargo en tu cuenta para confirmar que Codex lo aceptó. La cuota del plan
Pro sigue aplicándose; esta cola no consulta ni amplía tu saldo o tus límites.
Si Codex no acepta el encargo por falta de cuota, el PR abierto conserva la tarea
sin generar solicitudes nuevas para la misma alerta. Se puede retomar desde esa
PR cuando se renueve la cuota.

## Selección manual opcional y revisión

1. Abre una alerta en **Security → Code scanning** o **Security → Dependabot** y
   copia su número, el último segmento de la URL.
2. Para priorizar una alerta concreta o reintentar un encargo cerrado, en
   **Actions → Codex security fix → Run workflow**, selecciona la rama
   predeterminada y rellena `finding_type` (`code-scanning` o `dependabot`) y
   `alert_number`.
3. La selección manual también respeta el límite de dos encargos y evita repetir
   una PR abierta. Abre el PR enlazado en el resumen. Comprueba que Codex reaccione al comentario
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

Los números son ejemplos. Cada ejecución que selecciona una alerta crea una rama
`codex/security/<tipo>-<número>-<run>-<intento>` y un PR nuevo. Comprueba que no
exista otro encargo cerrado que quieras retomar antes de reintentarlo.

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
node --test .github/scripts/security-*.test.mjs
```

Referencias oficiales: [tareas con @codex en GitHub](https://learn.chatgpt.com/docs/third-party/github),
[autenticación ChatGPT o API](https://learn.chatgpt.com/docs/auth),
[REST de Code scanning](https://docs.github.com/en/rest/code-scanning/code-scanning),
[REST de Dependabot](https://docs.github.com/en/rest/dependabot/alerts),
[eventos de GitHub Actions](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows).
