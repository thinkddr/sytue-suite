<a id="app-store-las-fichas-de-las-quince-apps"></a>
# App Store: the store listings of the fifteen apps

Everything App Store Connect asks for about each app (iOS and macOS) that can be kept in the
repository. It is uploaded by `apps/fyne/scripts/appstore-publicar.py` from Codemagic's
`appstore-fichas` workflow, which is triggered by `appstore-publicar.yml`; the why and the
how are in “App Store: ficha y envío por la API” (App Store: listing and submission through the API) in `docs/operations.md`. It is the
twin of [`apps/fyne/play/`](../play/README.md) and comes out of the same generator.

```
ficha.json                                 lo común y, por app, Apple ID, categorías y notas
<app>/<locale>/name.txt                    ≤ 30 caracteres (el Name de FyneApp.toml)
<app>/<locale>/subtitle.txt                ≤ 30; opcional (no está donde nada cabe)
<app>/<locale>/promotional_text.txt        ≤ 170
<app>/<locale>/description.txt             ≤ 4000 (la misma de Play)
<app>/<locale>/keywords.txt                ≤ 100, con sus comas
<app>/capturas/iphone-N.png                1179 × 2556, RGB sin alfa, de 1 a 10
<app>/capturas/ipad-N.png                  2048 × 2732
<app>/capturas/mac-N.png                   2880 × 1800
<app>/privacidad.txt                       las etiquetas de «App Privacy», para marcar en la web
```

`<app>` is the directory under `apps/fyne/cmd/`; the bundle id comes from its `FyneApp.toml`
(`eu.sytue.<app>`, and `eu.sytue.one` for the Hub) and the script checks that the Apple ID in
`ficha.json` belongs to that bundle before writing anything.

<a id="cómo-se-regenera"></a>
## How it is regenerated

- **Texts and `privacidad.txt`**: `bun apps/fyne/play/generar.ts` from the root, the same
  script that writes the Play listing. The description is the Play one; the promotional text, its
  short description; the subtitle, what the Hub shows under the app's name
  (`app.<clave>.description` from the shared catalog) if it fits in 30, otherwise Play's short one, and
  if that does not fit either, **there is no subtitle in that language** (51 of 285; rather than cutting a sentence short).
  The keywords are the acronyms and formats that the catalog names for that product plus
  the words of its short description in that language. **DECISION**: nobody has written them
  with store search in mind; they are correct and poor.
- **19 languages, not 26**: the App Store has no Bulgarian, Estonian, Irish, Icelandic, Latvian,
  Lithuanian or Maltese ([App Store localizations](https://developer.apple.com/help/app-store-connect/reference/app-store-localizations);
  Slovenian, `sl-SI`, was added on 2026-03-31). The locales are those of the API: `cs da de-DE el
en-US es-ES fi fr-FR hr hu it nl-NL no pl pt-PT ro sk sl-SI sv`. The primary one is `es-ES`:
  screenshots are only uploaded to it and the store shows them in the others.
- **Screenshots**: each app's walkthrough (`TestRecorridoConSesion`) against its fake
  service, with `SYTUE_CAPTURAS_PLAY=<dir>` and the size in `SYTUE_CAPTURAS_MEDIDA`. The usual
  route is CI; it is split by class so each collection can be reviewed separately:

  ```sh
  gh workflow run ci-desktop.yml --ref <rama> -F empaquetado_solo=true \
    -F captura_fichas=true -f apps_fichas=todas -f clase_fichas=iphone
  ```

  `clase_fichas` accepts `iphone`, `ipad` and `mac` (also `android`, for Play), and
  `apps_fichas` a space-separated list of apps or `todas`. CI imports the
  selection through `capturas.sh --seleccion <clase> <app>`, keeps the walkthrough's full index
  and retains only the chosen PNGs. If GitHub rejects the artifact because of the
  storage quota, the same PNGs are preserved in base64 blocks identified in the
  log. A missing screenshot still makes the job fail. For a walkthrough run in
  another remote environment, `capturas.sh <dir>` copies the chosen ones and, if ImageMagick is available,
  recompresses them losslessly.

  | Class    | Walkthrough size     | Pixels      | API type                | Why that one                                                                  |
  | -------- | -------------------- | ----------- | ----------------------- | ----------------------------------------------------------------------------- |
  | `iphone` | `393x852@3`          | 1179 × 2556 | `APP_IPHONE_61`         | “iPhone with Dynamic Island (medium display)”: the only mandatory iPhone size |
  | `ipad`   | `1024x1366@2`        | 2048 × 2732 | `APP_IPAD_PRO_3GEN_129` | 13" iPad: mandatory because the apps declare `UIDeviceFamily` 1 **and 2**     |
  | `mac`    | `1440x900@2`         | 2880 × 1800 | `APP_DESKTOP`           | Mac, 16:10: mandatory for the macOS listing                                   |

  The mandatory sizes are those in [Screenshot specifications](https://developer.apple.com/help/app-store-connect/reference/screenshot-specifications)
  as read on 2026-10-06; App Store Connect scales the other displays. **What these
  screenshots are not**: they do not come from an iPhone, an iPad or a Mac, but from Fyne's test
  driver at that size. They do not have the status bar, the Dynamic Island or the macOS
  window title bar, and the typeface is the one the app embeds, not the system one. Apple asks
  for screenshots to show the app in use, and these do; if App Review objects to them, they will have to be
  taken from a device or from a simulator on a Mac.

  > **Visual review completed on 2026-10-14: 125 PNGs of the fifteen apps**, 50
  > iPhone, 38 iPad and 37 Mac. The three collections were regenerated and inspected:
  > [iPhone](https://github.com/thinkddr/sytue-one/actions/runs/38607261142),
  > [iPad](https://github.com/thinkddr/sytue-one/actions/runs/38607260811) and
  > [Mac](https://github.com/thinkddr/sytue-one/actions/runs/38607262007). Rooms and Drive
  > were later replaced by those from
  > [the targeted regeneration](https://github.com/thinkddr/sytue-one/actions/runs/38613047745),
  > also reviewed: reactions and a picker with color emojis, and complete selection actions
  > on the list. The CertiPDF ones show pages drawn with a valid
  > PDF and the test render, without the drawing-failure notice. Excluded were
  > error or loading states of the fake service, text typed in by the walkthrough and
  > the fourth Cast screenshot on Mac, which corresponded to a view with thumbnails not yet
  > loaded. The final selection is in `capturas.sh`.
  >
  > This review checks the software driver's PNGs and the states chosen for
  > the listing. Validation on physical devices is still pending; it does not attest to
  > the fifteen apps working fully on iOS or macOS.

<a id="lo-que-hace-la-api-y-lo-que-no"></a>
## What the API does and what it does not

| Step                                                                       | Through the API?                                                            | Who                                                 |
| -------------------------------------------------------------------------- | --------------------------------------------------------------------------- | --------------------------------------------------- |
| Create the App ID and the app record                                       | No (creating apps is not in the API)                                        | Done by the owner on 2026-10-06, all fifteen        |
| Name, subtitle and privacy URL, per language                               | Yes, `appInfoLocalizations`                                                 | `--ficha`                                           |
| Primary and secondary category                                             | Yes, `appInfos`                                                             | `--ficha`                                           |
| Age rating                                                                 | Yes, `ageRatingDeclarations`                                                | `--ficha`                                           |
| Content rights (“does not use third-party content”)                        | Yes, `apps.contentRightsDeclaration`                                        | `--ficha`                                           |
| Description, keywords, promotional text, support and marketing URL         | Yes, `appStoreVersionLocalizations`                                         | `--ficha`                                           |
| Copyright, version number and release after approval                       | Yes, `appStoreVersions`                                                     | `--ficha`                                           |
| iPhone, iPad and Mac screenshots                                           | Yes, `appScreenshotSets` + `appScreenshots`                                 | `--ficha`                                           |
| Review notes                                                               | Yes, `appStoreReviewDetails`                                                | `--ficha` or just `--notas`                         |
| Contact and **demo account** for the reviewer                              | Yes, but **they are not in the repository**                                 | The owner: variables in Codemagic, or on the web    |
| Free price                                                                 | Yes, `appPriceSchedules` (only if none is set)                              | `--ficha`                                           |
| Countries (all except `scripts/paises.json`: France, China, sanctions)     | Yes, `appAvailabilities` v2 on creation; `territoryAvailabilities` to expand | `--ficha` (creation) and `--paises` (already created) |
| **Privacy labels (“App Privacy”)**                                         | **No**: the API does not have them                                          | The owner, with `<app>/privacidad.txt`              |
| **Encryption (export compliance)**                                         | Questionnaire via the API, authorized; documentation exemption in the plist: see below | Pipeline and review of the facts          |
| DSA trader status                                                          | No                                                                          | Done by the owner                                   |
| Agreements, tax and banking                                                | No                                                                          | The owner (the apps are free: the agreement is enough) |
| Upload the build                                                           | Yes, but not from this script                                               | Codemagic `ios-fyne` (and the macOS `.pkg`)         |
| Attach the build and **submit for review**                                 | Yes, `reviewSubmissions`                                                    | `--revision`, with the confirmation phrase          |
| Release after approval                                                     | Automatic (`releaseType: AFTER_APPROVAL`)                                   | —                                                   |

The listings of the fifteen apps and their two platforms were updated through the API on
2026-10-14. This is not the same as submitting for review or releasing: the reviewer
account, the privacy labels and the build's encryption information are still required.
The contract is additionally verified against a test API
(`deploy/production/tests/appstore-publicar-test.sh`).

## `ficha.json`

- **URL**: support and marketing `https://sytue.eu`; privacy `https://sytue.eu/privacidad`.
- **Copyright**: `2026 Sytue Projects, S.L.` (Apple prepends the ©).
- **Category** (DECISION; the brief asked for Productivity or Business):

  | App                                                                                    | Primary       | Secondary     | Why                                                                              |
  | -------------------------------------------------------------------------------------- | ------------- | ------------- | -------------------------------------------------------------------------------- |
  | Hub, Mail, Calendar, Contacts, Drive, Docus, Calcus, Diapos, CertiPDF, Notepads, Bases | Productivity  | Business      | An individual's work tools; that is where their equivalents are                  |
  | Photos                                                                                 | Productivity  | Photo & Video | It is the organization's gallery, not a consumer photo editor                    |
  | Rooms                                                                                  | Business      | Productivity  | Team messaging and meetings: in the store, the work ones are under Business      |
  | Cast                                                                                   | Business      | Photo & Video | An organization's video and live streams, not an open video network             |
  | ERP                                                                                    | Business      | Finance       | A company's accounting and invoicing                                             |

  On macOS, `empaquetar.sh` writes `LSApplicationCategoryType = productivity` in all of them; the
  store category is the one in App Store Connect, and if they do not match the upload warns. For
  Rooms, Cast and ERP it would have to write `public.app-category.business` (that belongs to packaging,
  which is not touched here).

- **Release**: `AFTER_APPROVAL` (it goes live in the store as soon as Apple approves it), as on
  Play. To release it by hand after approval, `"publicacion": "MANUAL"` in `ficha.json`.
- **Review notes**: in English, shared, plus a paragraph of its own for Rooms (where
  block and report are) and for Cast.
- **Demo account**: `revision.cuenta_de_prueba` is `true` and **the field is left empty**. The
  owner fills it in, with an account of their own in a demo organization, **without a second
  factor or mandatory passkey**, as secret variables in Codemagic's `apple-firma`
  group: `APPSTORE_DEMO_USUARIO`, `APPSTORE_DEMO_CLAVE`, `APPSTORE_CONTACTO_NOMBRE`,
  `APPSTORE_CONTACTO_APELLIDOS`, `APPSTORE_CONTACTO_TELEFONO` and `APPSTORE_CONTACTO_CORREO`.
  Or types them on the web (_App Review Information_), thirty times. Without them `--ficha`
  does not touch those fields and `--revision` refuses. `--seco` goes red if anything that looks like a
  credential appears in `ficha.json`.

The review notes are composed in a single place: `revision.notas`, each app's
`notas` and, at the end, `revision.notas_por_plataforma[IOS|MAC_OS]`.
The justification for the desktop OIDC listener lives in `MAC_OS`, so that
`--ficha` keeps it when regenerating the notes and does not send it to iOS. The limit of
4000 UTF-8 bytes includes each platform's annex and separators.
Credentials and contacts remain in App Store Connect's private fields.
The video from a physical device is added only once it exists: the notes
in the repository do not claim it has been recorded and contain no example links.

To repair only those notes, `--notas --version X.Y.Z` requires the exact version that already
exists and is editable, with its review detail already created. It first checks all
the selected targets (by default the fifteen apps on iOS and macOS) and then
only does a `PATCH` of `notes`, without changing contacts, demo account, builds,
versions or submissions. It rechecks the target before writing and re-reads the notes to
confirm they match; the receipt shows only the result and its SHA-256. A pending earlier
rejection is neither cancelled nor resubmitted. In the manual workflow, choose
`modo=notas`, `ref=vX.Y.Z` and, optionally, apps and platforms.

<a id="clasificación-por-edades-pregunta-a-pregunta"></a>
## Age rating, question by question

It goes through the API (`edades` in `ficha.json`). Consistent with what was declared to Google
(`../play/README.md`, “Clasificación de contenido” (content rating)): it comes out **4+**, and **18+ for Cast** (below).

| App Store Connect question                                                        | Answer                                                               | API key                                                                                                              |
| --------------------------------------------------------------------------------- | -------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| In-app controls: parental controls, age assurance                                 | None                                                                 | `parentalControls`, `ageAssurance` = `false`                                                                         |
| Unrestricted web access                                                           | No (login opens the system browser; there is no in-app browser)      | `unrestrictedWebAccess` = `false`                                                                                    |
| **User-generated content**                                                        | **Yes**, in all fifteen: they all share within the organization      | `userGeneratedContent` = `true`                                                                                      |
| Social media                                                                      | No                                                                   | `socialMedia`, `socialMediaAgeRestricted` = `false`                                                                  |
| **Messaging and chat**                                                            | **Yes** in Mail, Rooms and Cast; not in the other twelve             | `messagingAndChat`                                                                                                   |
| Advertising                                                                       | No                                                                   | `advertising` = `false`                                                                                              |
| Profanity or crude humor; horror or fear themes; alcohol, tobacco or drugs        | None                                                                 | `profanityOrCrudeHumor`, `horrorOrFearThemes`, `alcoholTobaccoOrDrugUseOrReferences`                                 |
| Medical or treatment information; health or wellness                              | None; No                                                             | `medicalOrTreatmentInformation`, `healthOrWellnessTopics`                                                            |
| Mature or suggestive themes; sexual content or nudity; graphic sexual content     | None                                                                 | `matureOrSuggestiveThemes`, `sexualContentOrNudity`, `sexualContentGraphicAndNudity`                                 |
| Cartoon or fantasy violence; realistic; prolonged or sadistic realistic; weapons  | None                                                                 | `violenceCartoonOrFantasy`, `violenceRealistic`, `violenceRealisticProlongedGraphicOrSadistic`, `gunsOrOtherWeapons` |
| Gambling; simulated gambling; contests; loot boxes                                | No; none; none; No                                                   | `gambling`, `gamblingSimulated`, `contests`, `lootBox`                                                               |
| Made for kids?                                                                    | No (`kidsAgeBand` is not sent)                                       | —                                                                                                                    |

- **DECISION**: the target audience declared to Google is 18+ and here the rating comes out
  4+: they do not contradict each other (one is who it is aimed at, the other what it contains), but if you want to
  raise the age on the App Store there is `ageRatingOverrideV2`; it is only used for Cast (below).
- **User-generated content and guideline 1.2** (filter, report, block and a
  published contact). **Rooms** ([ADR 0073](../../../docs/decisions.md#0073)) and **Cast**
  ([ADR 0074](../../../docs/decisions.md#0074), native app since #566) have it, and their review notes
  say where each thing is. The condition for submitting them is the same as on
  Play: that the submitted build **and** the deployed service both include the feature.
- **Cast, 18+** (DECISION, `ageRatingOverrideV2: EIGHTEEN_PLUS` only for Cast). Google was
  told that in Cast users **can** publicly share nudity or
  violence (owner's decision of 2026-10-06, `../play/README.md`): nothing prevents it and a
  video reaches the front page without prior review. Apple's questionnaire asks about what
  the app ships with, not about what others can upload, so its answers remain “none”
  and consistency with Google is achieved by raising the age, which is what the override exists
  for. If the owner meets what the Play README requires in order to answer “No” to Google,
  the override is removed here in the same change. The other fourteen, 4+.

<a id="etiquetas-de-privacidad-app-privacy-pregunta-a-pregunta"></a>
## Privacy labels (“App Privacy”), question by question

**They do not go through the API**: they are ticked on the web, under _App Privacy_ for each app, with
`<app>/privacidad.txt`, which the generator derives from the **same data** as Play's
`seguridad-datos.csv`. What is declared to Apple and to Google must not diverge.

- **Does this app collect data?** Yes, in all fifteen.
- **Is it used for tracking?** No, none: there are no third-party SDKs, ads, analytics
  or data brokers (inventory of 2026-09-14).
- Each data type: purpose **App Functionality** and nothing else (Apple has no “account
  management”: authenticating is functionality); **linked to identity: yes** (everything hangs
  off the account); **tracking: no**.
- Differences in form from Play, not in substance: search history, which on
  Play is declared as “ephemeral”, is **not declared** here (for Apple “collecting” means keeping it longer than
  it takes to serve the request); calendar, files and created content go under “Other User
  Content”; photos and videos are a single type; so are emails and other messages.
- **Privacy policy URL**: `https://sytue.eu/privacidad` (this one does go through the API).
- Account deletion (guideline 5.1.1(v)): `https://id.sytue.eu/account/delete`, the
  same one given to Google. It has to respond before submitting for review.

<a id="cifrado-cumplimiento-de-exportación"></a>
## Encryption (export compliance)

**Declaration authorized by the owner on 2026-10-14.** The questionnaire answers
and the declarations in _Distribución → Información de la app
→ Documentación de cifrado de la app_ (Distribution → App Information → App Encryption Documentation) can be recorded through the API with that authorization.
This replaces the earlier decision to leave the questionnaire to a person on every build.

**Standard encryption without distribution in France: exempt from documentation.** The owner
decided not to distribute the fifteen apps in France for now, without filing the French
ANSSI declaration. With these apps' standard algorithms and no proprietary algorithms,
[Apple's table](https://developer.apple.com/help/app-store-connect/reference/app-information/export-compliance-documentation-for-encryption)
only requires the French declaration when distributing in France. That is why the iOS and
macOS plists carry **`ITSAppUsesNonExemptEncryption=false`**, without
`ITSEncryptionExportComplianceCode`: it means exempt from documentation, **not that there is no
encryption**, and it avoids repeating the questionnaire on future uploads according to
[Apple's procedure](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation).
This decision must be reviewed before enabling France or adding proprietary
algorithms. The guard checks the exclusion of `FRA` against the listing publisher's list.

As background, the Calendar pilot of 2026-10-14 with
`ITSAppUsesNonExemptEncryption=true` and no code archived for iOS and macOS, but Apple
rejected both uploads with **90592: Invalid Export Compliance Code**. The fifteen
declarations created while France was enabled remain as history in
`CREATED`, with `codeValue=null`; they do not attest to approval and are not needed for this
documentation exemption.

If Apple requires documentation in the future, after approving it Apple issues the code for that app.
It is configured **only in that app's workflow** as
`SYTUE_APP_ENCRYPTION_EXPORT_CODE`; `ci_post_clone.sh` then adds
`ITSAppUsesNonExemptEncryption=true` together with `ITSEncryptionExportComplianceCode` in
the two shared iOS and macOS plists. Without a value, it keeps the documentation exemption.
The post-clone step rejects missing metadata, wrong types, `false` with a code or
`true` without a valid code before installing Go. It does not check
approval against Apple: whoever configures the variable must use the code of that app's
**APPROVED declaration**, never one from another app, a fictitious value or a pending one.
Creating an `appEncryptionDeclaration` in `CREATED` state is not the same as `APPROVED`, nor
does it get linked to the build automatically.

The commercial listing is still not the source for encryption: `--seco` goes red if the key appears
in `ficha.json`, and `appstore-publicar.py --revision` refuses to submit a build without the
encryption information. Before treating it as available, the
TestFlight state and its group membership are checked; `true` does not attest that Apple has approved the
documentation for submitting the app for review.

For earlier builds whose flag is still `null`, the workflow's manual mode
`cifrado-exento` can complete only `null` → `false`, with
`confirmacion_cifrado=ESTANDAR_EXENTO_SIN_FRANCIA` and the explicit tag. It reuses
[Apple's builds PATCH](https://developer.apple.com/documentation/appstoreconnectapi/patch-v1-builds-_id_).
Before the first PATCH it checks all the requested apps: bundle, valid and
current build, no active review and no linked declaration, France disabled in the
actual availability and new territories disabled. It re-reads the changes before writing
and requires a subsequent GET with `false`. A response that is already `false` is not rewritten; `true`, an
unreadable field or an existing declaration stops the operation. It does not create documents,
change countries or submit for review. A later failure is reported without reverting the builds
already confirmed; the API offers no transaction across apps.

The facts, so it can be answered truthfully:

- All **fifteen** talk to the server over TLS with the **Go** implementation
  (`crypto/tls`), not with Apple's operating system one.
- **Notepads** encrypts notebooks on the client with **AES-256-GCM**.
- **Rooms and Cast** include their own **DTLS-SRTP** for voice and video.
- None uses proprietary or non-standard algorithms.

| Question                                                                                                    | Truthful answer                                                                                          |
| ----------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Does the app use encryption (implement it, incorporate it or access it)?                                    | **Yes**, all fifteen                                                                                     |
| Does it use proprietary algorithms or ones not accepted as standard by international bodies (IEEE, IETF, ITU)? | **No**                                                                                                |
| Does it use standard algorithms **instead of, or in addition to**, the encryption in Apple's operating system? | **Yes**, all fifteen: TLS is Go's; plus AES-256-GCM in Notepads and DTLS-SRTP in Rooms and Cast       |
| Is it limited to the operating system's encryption (`URLSession` HTTPS, etc.)?                              | **No**: ticking this would be false, even if it is the answer that avoids paperwork                      |
| Will it be available in France?                                                                             | **No**, by the owner's decision of 2026-10-14; France is excluded from the distribution of the fifteen apps |

What follows from those answers (whether US export documentation is needed,
the annual self-classification report or the French declaration) is determined by Apple in the
questionnaire itself and is a legal declaration by the owner. The exact wording of the questions
is whatever App Store Connect shows on the day it is answered; the meaning is given above. The values
**Worldwide distribution (owner's decision, 2026-11-01).** All App Store
territories except those in [`scripts/paises.json`](../scripts/paises.json): France (ANSSI
pending), mainland China (ICP license) and the countries sanctioned by the EU (Russia,
Belarus and, should they appear, Iran, North Korea, Syria and Cuba). Automatic additions
of new territories remain disabled: the list is computed with `/v1/territories` and
checked. `--ficha` creates it when there is none; `--paises` (workflow `modo=paises`,
`confirmacion_paises=GLOBAL_SALVO_EXCLUIDOS`, without `ref`) brings the existing one in line with the policy
with `PATCH /v1/territoryAvailabilities`, re-reads it and goes red if Apple rejects any territory.
Outside the EU, US export control still applies: the **annual
self-classification report** to the BIS (mass-market encryption) is an obligation of the owner.
The French-language texts do not change.

<a id="lo-que-le-queda-al-dueño-en-este-orden"></a>
## What is left for the owner, in this order

1. **Variables in Codemagic**, group `apple-firma`: the six for the demo account and the
   contact (above).
2. `appstore-publicar.yml` with `modo=sondear`: it reports, without changing anything, whether each Apple ID
   belongs to its bundle and which versions exist on iOS and macOS.
3. **Review the iPad and Mac screenshots** (`<app>/capturas/ipad-*.png` and `mac-*.png`): they
   were chosen without looking at them. Changing one means changing its number in `capturas.sh`.
4. `modo=ficha` with the tag of the version to be released.
5. On the web, per app: **App Privacy** with `<app>/privacidad.txt`, and review what the
   listing upload sent.
6. Upload the builds: a `tienda-vX.Y.Z` tag on the release
   (`git push origin 'vX.Y.Z^{commit}:refs/tags/tienda-vX.Y.Z'`); Xcode Cloud builds and uploads
   all fifteen on iOS and macOS. Not with every release: 30 uploads per tag exhaust the account's daily
   quota (“Upload limit reached”, 90383). See “Xcode Cloud” in `docs/operations.md`.
7. Check each app's **export compliance**: standard encryption, France
   excluded and documentation exemption in the plist; review before changing any of
   those facts. If Apple requires documents, obtain its approval and use its code.
8. `modo=revision` with the same tag and the phrase `ENVIAR_<n>_APPS_A_REVISION`.
