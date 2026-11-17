<a id="google-play-las-fichas-de-las-quince-apps"></a>
# Google Play: the store listings of the fifteen apps

Everything Play Console asks for about each app that can be kept in the repository. It is uploaded by
`apps/fyne/scripts/play-publicar.sh` (the `play-publicar.yml` workflow, and the optional step
in `release-mobile.yml`); the why and how it works are in «Google Play: ficha y subida
por la API» (Google Play: listing and upload through the API) in `docs/operations.md`.

```
<app>/<idioma de Play>/title.txt               ≤ 30 caracteres (el Name de FyneApp.toml)
<app>/<idioma de Play>/short_description.txt   ≤ 80
<app>/<idioma de Play>/full_description.txt    ≤ 4000
<app>/graficos/icono.png                       512 × 512, RGBA de 8 bits
<app>/graficos/destacado.png                   1024 × 500
<app>/graficos/telefono-N.png                  1080 × 1920, de 2 a 8
<app>/seguridad-datos.csv                      «Seguridad de los datos», formato de exportación de Play
```

`<app>` is the directory under `apps/fyne/cmd/`; the package comes from its `FyneApp.toml`
(`eu.sytue.<app>`, and `eu.sytue.one` for the Hub).

<a id="cómo-se-regenera"></a>
## How it is regenerated

- **Texts and CSV**: `bun apps/fyne/play/generar.ts` from the root. The texts come from the
  www catalog (`product.<clave>.*`), which is already in the 26 languages: the listing says the
  same as the website and there is no other translation to maintain. What the phone app does not
  do is left out key by key, in `FICHAS`, with the reason next to it.
  **24 languages, not 26**: Play Console has no Irish (`ga`) or Maltese (`mt`).
  The default language is `es-ES`: images are only uploaded to it, and Play shows them in
  the others.
- **Icon and feature graphic**: `bash apps/fyne/play/graficos.sh` (it needs `magick`, so
  on the Mac). The icon is the app's `Icon.png`; the feature graphic, the mark and the name
  on the suite's paper (`--sytue-neutral-50`, ink `--sytue-ink`, Figtree Bold).
- **Screenshots**: each app's walkthrough (`TestRecorridoConSesion`) against its fake
  service, with `SYTUE_CAPTURAS_PLAY=<dir>`: instead of checking, it sets the window to
  360 × 640 at ×3 (1080 × 1920, phone layout) and leaves a screenshot of every
  screen and every layer reached by tapping. The listing ones are chosen from there and
  copied with `graficos.sh <dir>`, which reads the `CAPTURAS` list in that script. The usual way
  is the CI, which imports that selection with `graficos.sh --seleccion <app>`:

    ```sh
  gh workflow run ci-desktop.yml --ref <rama> -F empaquetado_solo=true \
    -F captura_fichas=true -f apps_fichas=todas -f clase_fichas=android
  ```

  `apps_fichas` also accepts a space-separated list. The job keeps the full
  index and only the selected PNGs, as described in
  [`../appstore/README.md`](../appstore/README.md). The images must be looked at again
  after regenerating them: the walkthrough numbers change when the interface changes.

  **Visual review completed on 2026-10-14: 45 phone PNGs of the fifteen apps**,
  at 1080 × 1920, RGB without alpha. The review covered
  [the full Android regeneration](https://github.com/thinkddr/sytue-one/actions/runs/38608561176)
  and Rooms and Drive were replaced with the images from
  [the targeted regeneration](https://github.com/thinkddr/sytue-one/actions/runs/38613047745),
  also reviewed. Of the initial 54, two from Calcus, one from Calendar and
  one from Docus with error notices from the fake service, one from Drive and two from Rooms,
  and the optional views of Photos and Diapos with clipped controls were removed. `graficos.sh`
  keeps the final selection; the PNGs keep the original render.
  The review is of screenshots from the software driver at phone size. Validation
  on physical devices is still pending and the images do not prove that the apps
  work fully on Android.

<a id="lo-que-hace-la-api-y-lo-que-no"></a>
## What the API does and what it does not

| Step                                                                                    | Through the API?                                                                | Who                                                   |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- | ----------------------------------------------------- |
| Create the app in Play Console                                                          | No                                                                              | The owner (thirteen on 25/09; **Cast, pending**)      |
| Listing (title, descriptions) in 24 languages                                           | Yes, `edits.listings.update`                                                    | `play-publicar.sh`                                    |
| Icon, feature graphic and screenshots                                                   | Yes, `edits.images.upload`                                                      | `play-publicar.sh`                                    |
| Contact website (`https://sytue.eu`) and default language                               | Yes, `edits.details.patch`                                                      | `play-publicar.sh`                                    |
| Contact email                                                                           | Yes, but **we do not make it up**                                               | The owner's DECISION                                  |
| Privacy policy (`https://sytue.eu/privacidad`)                                          | **No**: `AppDetails` does not have it                                           | The owner, in _Contenido de la app_ (App content)     |
| Data safety                                                                             | Yes, `applications.dataSafety` (outside the edit)                               | `play-publicar.sh`                                    |
| AAB to “internal” (as a draft)                                                          | Yes, `edits.bundles.upload` + `edits.tracks.update`                             | `play-publicar.sh --aab`                              |
| “internal” testers                                                                      | Only **Google groups** (`edits.testers`); email lists, in the console           | The owner                                             |
| Countries (all except `scripts/paises.json`)                                            | **No**: `countryAvailability` is read-only; `--paises` checks it                | The owner; DECISION below                             |
| Content rating, target audience, ads, access, declarations, advertising ID             | **No**                                                                          | The owner, with the answers below                     |
| Send for review and to production                                                       | Not done                                                                        | The owner                                             |

<a id="respuestas-para-contenido-de-la-app-pregunta-a-pregunta"></a>
## Answers for _Contenido de la app_ (App content), question by question

They apply to all fifteen except where stated otherwise. They come from the code and from the inventory of
what goes out to third parties (2026-09-14): no trackers, no third-party SDKs, no ads,
no analytics in the apps; Android notifications arrive through the Hub's SSE (no Firebase).

<a id="política-de-privacidad"></a>
### Privacy policy

`https://sytue.eu/privacidad`.

<a id="acceso-a-la-app"></a>
### App access

«**Toda o parte de la funcionalidad está restringida**» (“All or some functionality is restricted”). All of them require signing in with
Sytue ID (the system browser opens at `id.sytue.eu`). Instructions for the
reviewer, which **the owner writes** with a test account of their own (nobody else has it, nor should
it be in the repository):

> Tap «Iniciar sesión» (“Sign in”): the browser opens at id.sytue.eu. Username: `<la del dueño>`.
> Password: `<la del dueño>`. The account belongs to a demo organization with
> sample email, files, calendar and channels. It does not ask for a second factor.

The account must have **no second factor and no mandatory passkey**, or the reviewer is stuck
at the door and Play rejects the app. A single account works for all fifteen.

<a id="anuncios"></a>
### Ads

«**No, mi app no contiene anuncios**» (“No, my app does not contain ads”).

<a id="id-de-publicidad"></a>
### Advertising ID

«**No**»: no app uses it (there is no ads or analytics SDK, and no `AD_ID` in the
manifest).

<a id="clasificación-de-contenido-iarc"></a>
### Content rating (IARC)

- Contact email: the owner's.
- Category: «**Todas las demás tipos de apps**» (“All other app types”) (neither a game nor a store).
- Violence, sexuality, crude language, substances, gambling, sensitive content: **No** to everything, as far as the app
  ships out of the box. **Sytue Cast is the exception** on the question of whether users can publicly share nudity or
  violence: there the answer is **Yes** (owner's decision, 2026-10-06; the why and what would be needed for a “No” are
  further down in “Block and report”).
- Can users interact or exchange content? **Yes** in all fifteen: they all
  share within the organization (Mail, Rooms and Cast also with others; Cast, with a
  public front page). It does not change the age rating
  (it comes out PEGI 3 / USK 0 / IARC 3+) and only adds the «Los usuarios interactúan» (“Users interact”) notice.
- Does it share the user's location? **No**. Digital purchases inside the app? **No**.
- Is it a web browser or a search engine? **No**.

<a id="bloquear-y-denunciar-contenido-generado-por-usuarios"></a>
### Block and report (user-generated content)

The rating asks whether the app lets users **block** users or content and **report**
users or content, and the user-generated content policy requires it where the
main content comes from users.

**Sytue Rooms** ([ADR 0073](../../../docs/decisions.md#0073)). What can be declared, and where
it is in the app:

| Console question                               | Answer    | Where, in the app                                                                                                                                                                                                                                  |
| ---------------------------------------------- | --------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Can other users be blocked?                    | **Yes**   | «…» on a message → «Bloquear a…» (Block…); «Personas» (People) panel → the person's «…». It is undone in the channel menu → «Personas bloqueadas» (Blocked people)                                                                                 |
| Can other users be reported?                   | **Yes**   | «Personas» panel → the person's «…» → «Denunciar a…» (Report…)                                                                                                                                                                                     |
| Can content be reported?                       | **Yes**   | «…» on a message → «Denunciar mensaje» (Report message), with a reason (harassment, sexual content, violence, spam, other) and an optional note                                                                                                     |
| Is user content moderated?                     | **Yes**   | Team and organization administrators receive the report and can dismiss it, delete the message or remove the person from the channel; and the platform operator (Sytue), in its own queue                                                         |

- **What blocking does**, in case the review asks: whoever blocks stops receiving that person's
  direct messages and notifications —enforced by the server, not the app— and sees
  their messages collapsed in the channels they share. Nothing is deleted.
- **Condition for declaring it.** It is true when the version uploaded to Play and the deployed
  service both include this feature: the app without the service shows the entries and
  fails when they are used, and the service without the app has no way in. Before ticking it, check
  both things in the version being submitted.

**Sytue Cast** ([ADR 0074](../../../docs/decisions.md#0074)). What can be declared, and where
it is in the app:

| Console question                               | Answer    | Where, in the app                                                                                                                                                                                                                                                                                                                                                       |
| ---------------------------------------------- | --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Can other users be blocked?                    | **Yes**   | «Denunciar» (Report) next to the video → «Bloquear a…» (Block…); «Bloquear a…» under each comment; «…» on a chat message. It is undone in «Denuncias y bloqueos» (Reports and blocks)                                                                                                                                                                                  |
| Can other users be reported?                   | **Yes**   | «Denunciar» next to the video → «Denunciar a…» (Report…)                                                                                                                                                                                                                                                                                                                |
| Can content be reported?                       | **Yes**   | «Denunciar» next to the video, under each comment, in the «…» of each chat message and in a channel's header; with a reason (harassment, sexual content or nudity, violence, spam or deception, copyright, other) and optional text. **Also without an account**, on public content                                                                                     |
| Is user content moderated?                     | **Yes**   | Those who administer the organization that owns the content see the queue in «Denuncias y bloqueos» and can dismiss, delete the comment or the message, remove the video from the public front page, unpublish it or block the person; nobody sees reports against themselves. And the platform operator (Sytue) has its own queue, with the same actions                 |

- **What blocking does**, in case the review asks: whoever blocks stops receiving that person's comments, chat messages
  and live notifications, and that person can no longer comment or write on the videos of whoever
  blocked them. It is enforced by the server. Nothing is deleted.
- **Condition for declaring it**, the same as in Rooms: the version uploaded to Play and the deployed service must
  both include this feature. In addition, reporting without an account goes through the Cast website only if that website is deployed with it.
- **The operator queue** ([ADR 0075](../../../docs/decisions.md#0075)) belongs to the platform console, web only: the
  app does not change. To count it in the review, Sytue ID (the
  `platform/moderation/access` route), Cast (migrations 0010-0012) and Rooms (migration 0039), and the platform console, must all be deployed at the same time; and
  the owner must have reviewed the terms and the DPA, because it is Sytue access to customers' content.

**Nudity and violence in Cast: today we declare that they CAN be shared publicly.** That is the owner's decision
(2026-10-06) and it is still the truthful answer: nothing prevents it technically, anyone with an account can publish
a video and it appears on the front page without review. To be able to answer “**No**”, all of the following would be needed (status as of 2026-10-12):

1. **Terms of use that forbid it**, accepted by whoever publishes, and linked from the listing and from the app.
   **Missing**: it is legal text, the owner's decision.
2. **Takedown after a report, carried out by Sytue**, not only by the publishing organization. **Done in code, not
   deployed**: the operator queue ([ADR 0075](../../../docs/decisions.md#0075)) receives every report —including
   those against whoever administers, which that person no longer sees— and can remove from the front page, unpublish, delete and
   block. **Missing**: committing to a response time and someone watching the queue: the operator receives no notifications.
3. **Ability to act against repeat offenders.** **Partly**: suspending an organization already exists in the platform
   console (with a second person), and blocking within the organization shuts the door on a person inside it.
   There is **no** way to suspend a person across the whole platform, nor to take away public publishing from an account or
   organization without suspending it entirely.
4. **A decision about the front page**: that a video does not reach «Explorar» (Explore) without some filter (prior review, account
   age or automatic detection), or accepting in writing that takedown happens after publication.
   **Missing**: the owner's decision.
5. **A published abuse contact** for those who do not use the app (an email address), that is attended to. **Missing.** (Reporting without an account
   from the public website already exists and reaches both queues.)

With 1, 2 —deployed and with a response time— and 5, “it is not allowed and it is taken down” can be upheld. Until then the honest
answer remains “yes”.

**What can be declared today, in one line.** Blocking users, reporting users and content, and moderation: **Yes** in
Rooms and in Cast, as soon as the version uploaded and the deployed services include the feature (the operator queue
adds Sytue moderation once deployed). Nudity and violence in Cast: they **can** be shared.

<a id="público-objetivo-y-contenido"></a>
### Target audience and content

- Target audience age: **18 or over in all fifteen apps**. An explicit decision
  by the owner, confirmed on **2026-10-15** for Google Play.
- Could it unintentionally appeal to children? **No**.

<a id="seguridad-de-los-datos"></a>
### Data safety

It is uploaded through the API from `seguridad-datos.csv`. Summary of what is declared:

- Data is **collected** (it goes to our servers) and **not shared** with anyone: Scaleway, Mollie
  and Sweego are processors, which Play does not count as “sharing”.
- Everything encrypted in transit (TLS 1.3). Data deletion can be requested
  (`PSL_SUPPORT_DATA_DELETION_BY_USER` = yes).
- In all of them: name, email, user ID (the account: app functionality and account management;
  required) and in-app search history (ephemeral).
- In addition: Hub → phone number (optional; the guided sign-up verifies it by SMS); Mail → emails and
  files; Drive → files, photos and videos; Docus, Calcus, Diapos and CertiPDF → files
  and documents; Calendar → calendar; Contacts → contacts; Notepads → user-generated content
  and files; Photos → photos; Rooms → other messages, audio and videos (the
  voice and the camera or screen of a call; not ephemeral, because the meeting can be
  recorded from the phone), photos and files; Cast → videos and their audio (the recording of
  a live stream stays as a video), photos (the custom poster), user-generated
  content (title and description) and other messages (chat and comments), all optional, and
  app interactions (who is watching a live stream, stored while it lasts); ERP → “other financial info”
  (the organization's books); Bases → user-generated content (what the
  organization writes in its tables; attachments are not uploaded from the app).
- **Account creation**: in Sytue ID, with **username and password** (`PSL_ACM_USER_ID_PASSWORD`);
  the mobile SMS and organization invitations do not count as another method. Without
  this answer the API rejects the entire CSV (“Response missing for
  PSL_SUPPORTED_ACCOUNT_CREATION_METHODS”).
- **Account deletion**: the web link Play asks for is `https://id.sytue.eu/account/delete`
  (`PSL_ACCOUNT_DELETION_URL`), in all fifteen. It must work before submitting for review:
  Play rejects the app with “Invalid account/data deletion link” if it does not load.
- **Deleting data without deleting the account** (`PSL_DATA_DELETION_URL`): the privacy
  policy, `https://sytue.eu/privacidad`, which explains how to request erasure. Without this
  answer the API also rejects the CSV (“Response missing for PSL_DATA_DELETION_URL”).

<a id="apps-gubernamentales"></a>
### Government apps

«**No**» in all fifteen.

<a id="funciones-financieras"></a>
### Financial features

- Thirteen apps: «**Mi app no ofrece ninguna función financiera**» (“My app doesn't provide any financial features”).
- **ERP**: double-entry accounting, invoicing and VERI*FACTU for the organization; it does not
  move money, give loans or store cards. **DECISION**: declare it under
  «Servicios de soporte y gestión» (“Support and management services”) (financial/accounting management tools) for
  transparency, or “none” if the console does not have that checkbox. If it is ever connected to the
  bank (PSD2, pending the owner), the answer changes.

<a id="apps-de-salud"></a>
### Health apps

«**No**» in all fifteen (none processes health data).

<a id="apps-de-noticias"></a>
### News apps

«**No**» in all fifteen.

<a id="permisos"></a>
### Permissions

- All: `INTERNET`, `POST_NOTIFICATIONS` (notifications, Android 13+) and `USE_BIOMETRIC` (the
  session lock). None of them needs a separate declaration.
- Hub: `FOREGROUND_SERVICE` and `FOREGROUND_SERVICE_SPECIAL_USE` (keeping the notification
  stream open without Firebase).
- Rooms and Cast (`llamadas = "si"` in their `FyneApp.toml`): additionally `RECORD_AUDIO`,
  `MODIFY_AUDIO_SETTINGS`, `CAMERA`, `FOREGROUND_SERVICE` and
  `FOREGROUND_SERVICE_MEDIA_PROJECTION` (voice and video of a call in Rooms; streaming
  live with camera, microphone or screen in Cast). They are requested when used, not at
  install time. None asks for location, SMS, call log or “all files”.
- Photos (`fototeca = "si"`): additionally `READ_MEDIA_IMAGES`, `READ_MEDIA_VIDEO` and
  `READ_MEDIA_VISUAL_USER_SELECTED` (`READ_EXTERNAL_STORAGE` up to Android 12),
  `ACCESS_MEDIA_LOCATION` (without it, Android strips the GPS from photos when reading them and the
  «Cerca» (Nearby) view stays empty), `ACCESS_NETWORK_STATE` (backup, by default, only on Wi-Fi) and
  `RECEIVE_BOOT_COMPLETED` (rescheduling the backup after a reboot). The gallery ones
  are requested when backup is turned on, not at install time.
- **Photo and video permissions** (_Contenido de la app → Permisos de fotos y vídeos_, App content → Photo and video permissions): Play
  requires anyone requesting `READ_MEDIA_IMAGES` or `READ_MEDIA_VIDEO` to declare them, and only accepts them
  if reading the gallery is a CORE feature; otherwise the system photo picker must be used.
  In Photos it is: backup reads the whole gallery, continuously and
  with the app closed, which the picker does not allow. The owner submits it; the text, ready
  to paste (the console asks for it in English):

  > Sytue Photos is a photo gallery and backup app. Its core feature is the automatic
  > backup of the device's photo and video library to the user's own Sytue Drive: when the
  > user turns backup on, the app reads every photo and video in the device library, uploads
  > the ones that are not backed up yet, and keeps doing so for new ones in the background
  > (JobScheduler, only while the device is unlocked and, by default, on Wi-Fi). This needs
  > persistent access to the whole library, which the Android photo picker cannot provide.
  > Location metadata (ACCESS_MEDIA_LOCATION) is kept so the gallery can sort and map photos
  > by place. Nothing is read until the user turns backup on.

  And the short video it asks for: turning on «Copia de seguridad» (Backup) in the gallery's cloud,
  accepting the permission and seeing «Copiando 1 de N…» (Copying 1 of N…) with photos and videos. The owner records it.

- **Foreground services** (_Contenido de la app → Permisos de servicios en primer
  plano_, App content → Foreground service permissions): Play asks apps targeting Android 14+ to declare them. Rooms and Cast, «Proyección
  multimedia» (Media projection) (sharing the screen in a call or streaming it live; the
  person starts it and sees it in the notification); Hub, «Uso especial» (Special use) (receiving the
  organization's notifications without Firebase). Each declaration asks for a short video of the use: the
  owner records it.

<a id="por-app-categoría-y-lo-que-queda"></a>
## Per app: category and what remains

**Category** (DECISION; in the console, _Presencia en tienda → Configuración de la tienda_, Store presence → Store settings):

| App               | Package             | Proposed category       | Tags                 |
| ----------------- | ------------------- | ----------------------- | -------------------- |
| Sytue Hub (`hub`) | `eu.sytue.one`      | Productivity            | —                    |
| Sytue Mail        | `eu.sytue.mail`     | Communication           | Email                |
| Sytue Rooms       | `eu.sytue.rooms`    | Communication           | Team messaging       |
| Sytue Contacts    | `eu.sytue.contacts` | Communication           | Contacts             |
| Sytue Calendar    | `eu.sytue.calendar` | Productivity            | Calendar             |
| Sytue Drive       | `eu.sytue.drive`    | Productivity            | Storage              |
| Sytue Docus       | `eu.sytue.docus`    | Productivity            | Word processor       |
| Sytue Calcus      | `eu.sytue.calcus`   | Productivity            | Spreadsheet          |
| Sytue Diapos      | `eu.sytue.diapos`   | Productivity            | Presentations        |
| Sytue CertiPDF    | `eu.sytue.certipdf` | Productivity            | PDF                  |
| Sytue Notepads    | `eu.sytue.notes`    | Productivity            | Notes                |
| Sytue Photos      | `eu.sytue.photos`   | Photography             | Gallery              |
| Sytue Cast        | `eu.sytue.cast`     | Video Players & Editors | Video                |
| Sytue ERP         | `eu.sytue.erp`      | Business                | Accounting           |
| Sytue Bases       | `eu.sytue.bases`    | Productivity            | Databases            |

**Countries** (the account holder's DECISION, **2026-11-01**): **all** those Play offers except
those in [`scripts/paises.json`](../scripts/paises.json): France (excluded on
2026-10-14, until the ANSSI procedure is completed), China (Play does not operate there; it is made explicit) and those
sanctioned by the EU (Russia and Belarus; Iran, North Korea, Syria and Cuba in case they
appear). It replaces the list of 30 (EU + EEA + CH) saved on 2026-10-25.

The API does not allow writing them. In **each of the fifteen**: _Prueba y publicación →
Producción → Países o regiones → Añadir países o regiones_ (Test and release → Production → Countries / regions → Add countries / regions), tick **all**, untick
France, Russia and Belarus (and any other from `paises.json` that it offers), save and
send the changes with the next release. Do not enable automatic availability
in new countries. Afterwards, `play-publicar` with `modo=paises` checks it (read-only):
it fails if an excluded one appears, if “rest of the world” is enabled or if there are fewer than
`minimo_play` countries.

**What remains for the owner in EACH of the fifteen**, in this order:

0. The fifteen apps, including Cast and Bases, **exist and allow editing with the service
   account**, confirmed on 2026-10-14 by
   [the Play probe](https://github.com/thinkddr/sytue-one/actions/runs/38608638019).
   `modo=produccion` requires, in all fifteen, the exact `versionCode` of the requested `tag` in
   `internal`; a different version stops the preparation before modifying releases.
   **Bases, in addition, has not been seen on a phone**: its screenshots are from the walkthrough at 360 × 640 against the
   fake service (the header and the dialogs were adjusted that day; the grid
   scrolls sideways). Before promoting it to production, open it on a real phone.

1. _Contenido de la app_ (App content): privacy policy, access (with the test account),
   ads, advertising ID, rating, target audience, government, financial,
   health and news, with the answers above.
2. _Presencia en tienda_ (Store presence): category, tags and contact email.
3. Check that `https://id.sytue.eu/account/delete` responds: the CSV already carries the link.
4. Run the `play-publicar.yml` workflow with `modo=sondear`, then `modo=ficha`, and
   `modo=todo` with the tag when there are signed AABs (Codemagic `android-fyne` +
   `release-mobile.yml`).
5. In “internal”: the testers list and **publish the draft** that the API leaves.
6. Production: prepare the drafts with `play-publicar.yml`, `modo=produccion`,
   empty `apps`, `tag=vX.Y.Z`, `estado_produccion=draft` and the exact confirmation
   `PUBLICAR_15_APPS`. It reuses the AABs already signed for that tag in `internal`, without
   uploading them again or changing countries or listing, and leaves drafts with no review and no
   public release. It checks all fifteen before modifying the first one; it keeps
   any other production release, and repeating the same draft does not overwrite it.
   **The fifteen apps are still in Draft state and their internal releases are drafts**:
   do not repeat `completed` in that state. The
   [first release is completed in Play Console](https://developers.google.com/android-publisher/edits),
   with its forms and consents; if you start with internal testing, its release
   must also be activated in the Console. Only when the app's state allows
   publishing through the API and the listing, demo account, countries and deletion are complete
   can `estado_produccion=completed` request review of the same AAB. If an app is already
   in review, it fails without cancelling it; a Google error does not trigger any other
   automatic state. A prepared draft does not prove that Play has published or reviewed the app.
