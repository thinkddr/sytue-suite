// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

// Las cadenas del relevo cuando lo pide el puente de una app web con su clave
// (desbloqueo.go, entregaSinTeclear): decir que no hay ninguna clave que entregar
// en vez de volver a la app con las manos vacías. El título de ese aviso es `e2e.lost_title`.
func init() {
	for k, v := range catalogoDesbloqueoPuente {
		catalogo[k] = v
	}
}

var catalogoDesbloqueoPuente = map[string]map[string]string{
	"e2e.failed_body": {
		"en": "This browser could not get the key that protects your encrypted data. Sign in again typing your password: it is what opens that key.",
		"es": "Este navegador no ha podido obtener la clave que protege tus datos cifrados. Vuelve a iniciar sesión escribiendo tu contraseña: es lo que abre esa clave.",
		"fr": "Ce navigateur n’a pas pu obtenir la clé qui protège vos données chiffrées. Reconnectez-vous en saisissant votre mot de passe : c’est lui qui ouvre cette clé.",
		"de": "Dieser Browser konnte den Schlüssel, der deine verschlüsselten Daten schützt, nicht erhalten. Melde dich erneut mit deinem Passwort an: Es öffnet diesen Schlüssel.",
		"it": "Questo browser non è riuscito a ottenere la chiave che protegge i tuoi dati crittografati. Accedi di nuovo digitando la password: è ciò che apre quella chiave.",
		"bg": "Този браузър не можа да получи ключа, който защитава шифрованите ви данни. Влезте отново, като въведете паролата си: тя отваря този ключ.",
		"cs": "Tento prohlížeč nedokázal získat klíč, který chrání vaše šifrovaná data. Přihlaste se znovu zadáním hesla: právě to tento klíč otevírá.",
		"da": "Denne browser kunne ikke hente den nøgle, der beskytter dine krypterede data. Log ind igen ved at skrive din adgangskode: Det er den, der åbner nøglen.",
		"el": "Αυτό το πρόγραμμα περιήγησης δεν μπόρεσε να λάβει το κλειδί που προστατεύει τα κρυπτογραφημένα δεδομένα σας. Συνδεθείτε ξανά πληκτρολογώντας τον κωδικό σας: αυτός ανοίγει το κλειδί.",
		"et": "See brauser ei saanud kätte võtit, mis kaitseb teie krüpteeritud andmeid. Logige uuesti sisse, sisestades oma parooli: just see avab selle võtme.",
		"fi": "Tämä selain ei saanut avainta, joka suojaa salattuja tietojasi. Kirjaudu uudelleen kirjoittamalla salasanasi: se avaa tämän avaimen.",
		"ga": "Níorbh fhéidir leis an mbrabhsálaí seo an eochair a chosnaíonn do shonraí criptithe a fháil. Logáil isteach arís agus do phasfhocal á chlóscríobh agat: is é a osclaíonn an eochair sin.",
		"hr": "Ovaj preglednik nije uspio dobiti ključ koji štiti vaše šifrirane podatke. Prijavite se ponovno upisivanjem lozinke: ona otvara taj ključ.",
		"hu": "Ez a böngésző nem tudta megszerezni a titkosított adatait védő kulcsot. Jelentkezzen be újra a jelszava beírásával: az nyitja meg ezt a kulcsot.",
		"is": "Þessi vafri gat ekki sótt lykilinn sem verndar dulkóðuðu gögnin þín. Skráðu þig inn aftur með því að slá inn lykilorðið þitt: það opnar lykilinn.",
		"lt": "Ši naršyklė negalėjo gauti rakto, saugančio jūsų šifruotus duomenis. Prisijunkite dar kartą įvesdami slaptažodį: būtent jis atveria šį raktą.",
		"lv": "Šis pārlūks nevarēja iegūt atslēgu, kas aizsargā jūsu šifrētos datus. Pierakstieties vēlreiz, ievadot paroli: tieši tā atver šo atslēgu.",
		"mt": "Dan il-browser ma setax jikseb iċ-ċavetta li tipproteġi d-data kriptata tiegħek. Erġa’ idħol billi tikteb il-password tiegħek: hija dik li tiftaħ iċ-ċavetta.",
		"nb": "Denne nettleseren klarte ikke å hente nøkkelen som beskytter de krypterte dataene dine. Logg på igjen ved å skrive inn passordet ditt: Det er det som åpner nøkkelen.",
		"nl": "Deze browser kon de sleutel die je versleutelde gegevens beschermt niet ophalen. Meld je opnieuw aan door je wachtwoord te typen: dat opent die sleutel.",
		"pl": "Ta przeglądarka nie mogła uzyskać klucza, który chroni Twoje zaszyfrowane dane. Zaloguj się ponownie, wpisując hasło: to ono otwiera ten klucz.",
		"pt": "Este navegador não conseguiu obter a chave que protege os seus dados encriptados. Inicie sessão novamente escrevendo a sua palavra-passe: é ela que abre essa chave.",
		"ro": "Acest browser nu a putut obține cheia care îți protejează datele criptate. Autentifică-te din nou tastând parola: ea deschide această cheie.",
		"sk": "Tento prehliadač nedokázal získať kľúč, ktorý chráni vaše šifrované údaje. Prihláste sa znova zadaním hesla: práve ono tento kľúč otvára.",
		"sl": "Ta brskalnik ni mogel pridobiti ključa, ki ščiti vaše šifrirane podatke. Znova se prijavite z vnosom gesla: prav to odpre ta ključ.",
		"sv": "Den här webbläsaren kunde inte hämta nyckeln som skyddar dina krypterade data. Logga in igen genom att skriva ditt lösenord: det är det som öppnar nyckeln.",
	},
	"e2e.retry": {
		"en": "Sign in with your password",
		"es": "Iniciar sesión con tu contraseña",
		"fr": "Se connecter avec votre mot de passe",
		"de": "Mit deinem Passwort anmelden",
		"it": "Accedi con la tua password",
		"bg": "Вход с вашата парола",
		"cs": "Přihlásit se heslem",
		"da": "Log ind med din adgangskode",
		"el": "Σύνδεση με τον κωδικό σας",
		"et": "Logi sisse parooliga",
		"fi": "Kirjaudu salasanallasi",
		"ga": "Logáil isteach le do phasfhocal",
		"hr": "Prijava lozinkom",
		"hu": "Bejelentkezés jelszóval",
		"is": "Skrá inn með lykilorði",
		"lt": "Prisijungti su slaptažodžiu",
		"lv": "Pierakstīties ar paroli",
		"mt": "Idħol bil-password tiegħek",
		"nb": "Logg på med passordet ditt",
		"nl": "Aanmelden met je wachtwoord",
		"pl": "Zaloguj się hasłem",
		"pt": "Iniciar sessão com a sua palavra-passe",
		"ro": "Autentifică-te cu parola",
		"sk": "Prihlásiť sa heslom",
		"sl": "Prijava z geslom",
		"sv": "Logga in med ditt lösenord",
	},
}
