// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// m365Teams lee Microsoft Teams por Graph y lo presenta de DOS maneras, porque
// Teams son dos cosas distintas:
//
//   - La ESTRUCTURA (equipos, canales, quién está en cada uno) se recrea en Sytue
//     Rooms para poder seguir trabajando el primer día: ListTeams/ListChannels/
//     ListChannelMembers.
//   - El CONTENIDO (los ficheros de SharePoint del canal y el historial de
//     conversación) aterriza en Sytue Drive, y para eso se expone como un árbol
//     sintético —equipo → canal → ficheros— que recorre el MISMO runner que copia
//     un Drive normal. No hay una segunda implementación del recorrido.
//
// Por qué el historial va a Drive y no a Rooms: la API de Rooms publica un mensaje
// como quien lo escribe AHORA, no acepta autor ni fecha de origen. Volcar allí una
// conversación ajena falsificaría la autoría o la fecha. La transcripción conserva
// ambas DENTRO del fichero, que es la única forma honesta de traérselo entero.
type m365Teams struct {
	*m365
}

// Prefijos de los ids sintéticos del árbol. Van en el id porque el runner solo
// conoce «carpetas» y «ficheros»: el tipo de cada nodo tiene que viajar con él.
const (
	prefixTeam    = "team:"
	prefixChannel = "chan:"
	prefixHistory = "hist:"
	prefixItem    = "item:"
	// transcriptName es el fichero de conversación que se deja en cada canal.
	transcriptName = "historial-de-conversacion.md"
	// messagePageSize es el tamaño de página del historial (tope de Graph: 50).
	messagePageSize = 50
)

// Team es un equipo del origen.
type Team struct {
	ID          string
	Name        string
	Description string
}

// Channel es un canal de un equipo del origen.
type Channel struct {
	ID    string
	Name  string
	Topic string
	Kind  string // standard | private | shared
	Skip  string // motivo si el canal NO se puede migrar
}

// Member es una persona de un canal del origen, tal y como la nombra el origen.
type Member struct {
	Email string
	Name  string
}

// --- Estructura (hacia Sytue Rooms) ------------------------------------------

// ListTeams devuelve los equipos de la cuenta de origen.
func (t *m365Teams) ListTeams(ctx context.Context) ([]Team, error) {
	//nolint:tagliatelle // tags = contrato de Microsoft Graph.
	var resp struct {
		Value []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Description string `json:"description"`
		} `json:"value"`
	}
	if err := getJSON(ctx, t.rt, t.userRoot()+"/joinedTeams", t.token, &resp); err != nil {
		return nil, err
	}
	out := make([]Team, 0, len(resp.Value))
	for _, v := range resp.Value {
		out = append(out, Team{ID: v.ID, Name: v.DisplayName, Description: v.Description})
	}
	return out, nil
}

// ListChannels devuelve los canales de un equipo. Los canales COMPARTIDOS con
// otras organizaciones se marcan como no migrables: su contenido no es de esta
// organización y traérselo sería llevarse datos de un tercero.
func (t *m365Teams) ListChannels(ctx context.Context, teamID string) ([]Channel, error) {
	u := t.base + "/v1.0/teams/" + url.PathEscape(teamID) + "/channels"
	//nolint:tagliatelle // tags = contrato de Microsoft Graph.
	var resp struct {
		Value []struct {
			ID             string `json:"id"`
			DisplayName    string `json:"displayName"`
			Description    string `json:"description"`
			MembershipType string `json:"membershipType"`
		} `json:"value"`
	}
	if err := getJSON(ctx, t.rt, u, t.token, &resp); err != nil {
		return nil, err
	}
	out := make([]Channel, 0, len(resp.Value))
	for _, c := range resp.Value {
		ch := Channel{ID: c.ID, Name: c.DisplayName, Topic: c.Description, Kind: c.MembershipType}
		if c.MembershipType == "shared" {
			ch.Skip = "es un canal compartido con otra organización: su contenido no pertenece a esta y no se copia"
		}
		out = append(out, ch)
	}
	return out, nil
}

// ListChannelMembers devuelve quién está en el canal. Los invitados externos se
// dejan fuera: no tienen cuenta en esta organización y añadirlos sería inventarla.
func (t *m365Teams) ListChannelMembers(ctx context.Context, teamID, channelID string) ([]Member, error) {
	u := t.base + "/v1.0/teams/" + url.PathEscape(teamID) +
		"/channels/" + url.PathEscape(channelID) + "/members"
	//nolint:tagliatelle // tags = contrato de Microsoft Graph.
	var resp struct {
		Value []struct {
			DisplayName string `json:"displayName"`
			Email       string `json:"email"`
		} `json:"value"`
	}
	if err := getJSON(ctx, t.rt, u, t.token, &resp); err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(resp.Value))
	for _, m := range resp.Value {
		if strings.TrimSpace(m.Email) == "" {
			continue // sin correo no hay a quién emparejar en Sytue.
		}
		out = append(out, Member{Email: m.Email, Name: m.DisplayName})
	}
	return out, nil
}

// --- Contenido (hacia Sytue Drive) -------------------------------------------

// ListChildren recorre el árbol sintético: raíz → equipos → canales → (historial +
// ficheros del canal) → carpetas reales de SharePoint.
func (t *m365Teams) ListChildren(ctx context.Context, folderID string) ([]Item, error) {
	switch {
	case folderID == "":
		return t.rootFolders(ctx)
	case strings.HasPrefix(folderID, prefixTeam):
		return t.channelFolders(ctx, strings.TrimPrefix(folderID, prefixTeam))
	case strings.HasPrefix(folderID, prefixChannel):
		teamID, channelID, ok := splitPair(strings.TrimPrefix(folderID, prefixChannel))
		if !ok {
			return nil, fmt.Errorf("migrate: identificador de canal inesperado: %q", folderID)
		}
		return t.channelContents(ctx, teamID, channelID)
	case strings.HasPrefix(folderID, prefixItem):
		driveID, itemID, ok := splitPair(strings.TrimPrefix(folderID, prefixItem))
		if !ok {
			return nil, fmt.Errorf("migrate: identificador de carpeta inesperado: %q", folderID)
		}
		return t.driveChildren(ctx, driveID, itemID)
	default:
		return nil, fmt.Errorf("migrate: nodo desconocido del árbol de Teams: %q", folderID)
	}
}

// rootFolders: una carpeta por equipo.
func (t *m365Teams) rootFolders(ctx context.Context) ([]Item, error) {
	teams, err := t.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(teams))
	for _, tm := range teams {
		out = append(out, Item{ID: prefixTeam + tm.ID, Name: safeFileName(tm.Name), IsFolder: true})
	}
	return out, nil
}

// channelFolders: una carpeta por canal del equipo.
func (t *m365Teams) channelFolders(ctx context.Context, teamID string) ([]Item, error) {
	chans, err := t.ListChannels(ctx, teamID)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(chans))
	for _, c := range chans {
		it := Item{ID: prefixChannel + teamID + "|" + c.ID, Name: safeFileName(c.Name), IsFolder: true}
		if c.Skip != "" {
			// Un canal que no se puede traer se marca como fichero omitido: el runner
			// no baja por él y el motivo acaba en el informe.
			it.IsFolder, it.SkipReason = false, c.Skip
		}
		out = append(out, it)
	}
	return out, nil
}

// channelContents: el historial del canal (fichero sintético) y sus ficheros
// reales de SharePoint.
func (t *m365Teams) channelContents(ctx context.Context, teamID, channelID string) ([]Item, error) {
	history := Item{
		ID:       prefixHistory + teamID + "|" + channelID,
		Name:     transcriptName,
		MimeType: "text/markdown",
		// Sin tamaño: la transcripción se genera al vuelo mientras se sube, así que
		// no se conoce hasta que termina. Los contadores de bytes del informe no la
		// incluyen — mejor que declarar un tamaño inventado.
	}

	files, err := t.channelFiles(ctx, teamID, channelID)
	if err != nil {
		// Un canal sin biblioteca de documentos (o sin permiso sobre ella) no debe
		// tumbar el historial, que sí se puede traer.
		return []Item{history}, nil //nolint:nilerr // degradación deliberada: se trae lo que sí hay.
	}
	out := make([]Item, 0, len(files)+1)
	return append(append(out, history), files...), nil
}

// channelFiles resuelve la carpeta de SharePoint del canal y lista su contenido.
func (t *m365Teams) channelFiles(ctx context.Context, teamID, channelID string) ([]Item, error) {
	u := t.base + "/v1.0/teams/" + url.PathEscape(teamID) +
		"/channels/" + url.PathEscape(channelID) + "/filesFolder"
	//nolint:tagliatelle // tags = contrato de Microsoft Graph.
	var folder struct {
		ID              string `json:"id"`
		ParentReference struct {
			DriveID string `json:"driveId"`
		} `json:"parentReference"`
	}
	if err := getJSON(ctx, t.rt, u, t.token, &folder); err != nil {
		return nil, err
	}
	if folder.ID == "" || folder.ParentReference.DriveID == "" {
		return nil, fmt.Errorf("migrate: el canal no publica su carpeta de ficheros")
	}
	return t.driveChildren(ctx, folder.ParentReference.DriveID, folder.ID)
}

// driveChildren lista una carpeta real de SharePoint siguiendo @odata.nextLink.
func (t *m365Teams) driveChildren(ctx context.Context, driveID, itemID string) ([]Item, error) {
	next := t.base + "/v1.0/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(itemID) +
		"/children?$top=200&$select=id,name,size,folder,file,lastModifiedDateTime"

	out := make([]Item, 0, 64)
	for page := 0; next != "" && page < maxPages; page++ {
		//nolint:tagliatelle // @odata.nextLink es el nombre literal que emite Graph.
		var resp struct {
			Value    []graphItem `json:"value"`
			NextLink string      `json:"@odata.nextLink"`
		}
		if err := getJSON(ctx, t.rt, next, t.token, &resp); err != nil {
			return nil, err
		}
		for _, gi := range resp.Value {
			it := gi.toItem()
			// El id lleva su drive: el árbol mezcla bibliotecas de equipos distintos.
			it.ID = prefixItem + driveID + "|" + gi.ID
			out = append(out, it)
		}
		if strings.TrimSpace(resp.NextLink) == "" {
			return out, nil
		}
		var err error
		if next, err = sameHostNext(next, resp.NextLink); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Open entrega el contenido de un nodo del árbol: la transcripción del canal
// (generada al vuelo) o un fichero real de SharePoint.
func (t *m365Teams) Open(ctx context.Context, it Item) (io.ReadCloser, error) {
	switch {
	case strings.HasPrefix(it.ID, prefixHistory):
		teamID, channelID, ok := splitPair(strings.TrimPrefix(it.ID, prefixHistory))
		if !ok {
			return nil, fmt.Errorf("migrate: identificador de historial inesperado: %q", it.ID)
		}
		return t.transcript(ctx, teamID, channelID), nil
	case strings.HasPrefix(it.ID, prefixItem):
		if it.DownloadURL != "" {
			return openStream(ctx, t.rt, it.DownloadURL, "") // URL pre-autenticada.
		}
		driveID, itemID, ok := splitPair(strings.TrimPrefix(it.ID, prefixItem))
		if !ok {
			return nil, fmt.Errorf("migrate: identificador de fichero inesperado: %q", it.ID)
		}
		u := t.base + "/v1.0/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(itemID) +
			"?$select=id,@microsoft.graph.downloadUrl"
		var meta graphItem
		if err := getJSON(ctx, t.rt, u, t.token, &meta); err != nil {
			return nil, err
		}
		if meta.DownloadURL == "" {
			return nil, fmt.Errorf("migrate: microsoft graph no entregó una URL de descarga")
		}
		return openStream(ctx, t.rt, meta.DownloadURL, "")
	default:
		return nil, fmt.Errorf("migrate: nodo del árbol de Teams que no se puede abrir: %q", it.ID)
	}
}

// --- Transcripción del canal --------------------------------------------------

// transcript devuelve la conversación del canal en Markdown, ESCRITA MIENTRAS SE
// SUBE (io.Pipe): un canal con años de historia no cabe cómodamente en memoria y
// aquí no hace falta que quepa.
//
// Autor y fecha son los ORIGINALES: es lo que hace que esto sea un archivo honesto
// y no una conversación falsificada en el destino.
func (t *m365Teams) transcript(ctx context.Context, teamID, channelID string) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		// El error del recorrido viaja por el pipe: quien lee (la subida) lo recibe y
		// el runner lo cuenta como fallo del fichero, no como éxito a medias.
		pw.CloseWithError(t.writeTranscript(ctx, pw, teamID, channelID))
	}()
	return pr
}

// writeTranscript pagina los mensajes del canal y sus respuestas, y los escribe.
func (t *m365Teams) writeTranscript(ctx context.Context, w io.Writer, teamID, channelID string) error {
	head := "# Historial de conversación\n\n" +
		"Exportado de Microsoft Teams por Sytue. **El autor y la fecha de cada mensaje son los originales**; " +
		"lo que ves aquí es el archivo fiel de la conversación, no una copia republicada.\n\n"
	if _, err := io.WriteString(w, head); err != nil {
		return err
	}

	base := t.base + "/v1.0/teams/" + url.PathEscape(teamID) + "/channels/" + url.PathEscape(channelID)
	next := base + fmt.Sprintf("/messages?$top=%d", messagePageSize)
	for page := 0; next != "" && page < maxPages; page++ {
		//nolint:tagliatelle // @odata.nextLink es el nombre literal que emite Graph.
		var resp struct {
			Value    []graphMessage `json:"value"`
			NextLink string         `json:"@odata.nextLink"`
		}
		if err := getJSON(ctx, t.rt, next, t.token, &resp); err != nil {
			return err
		}
		for _, m := range resp.Value {
			if err := writeMessage(w, m, false); err != nil {
				return err
			}
			if m.ReplyCount() == 0 {
				continue
			}
			replies, err := t.replies(ctx, base, m.ID)
			if err != nil {
				return err
			}
			for _, rep := range replies {
				if err := writeMessage(w, rep, true); err != nil {
					return err
				}
			}
		}
		if strings.TrimSpace(resp.NextLink) == "" {
			return nil
		}
		var err error
		if next, err = sameHostNext(next, resp.NextLink); err != nil {
			return err
		}
	}
	return nil
}

// replies trae las respuestas de un hilo (paginadas).
func (t *m365Teams) replies(ctx context.Context, channelBase, messageID string) ([]graphMessage, error) {
	next := channelBase + "/messages/" + url.PathEscape(messageID) + fmt.Sprintf("/replies?$top=%d", messagePageSize)
	out := make([]graphMessage, 0, 8)
	for page := 0; next != "" && page < maxPages; page++ {
		//nolint:tagliatelle // @odata.nextLink es el nombre literal que emite Graph.
		var resp struct {
			Value    []graphMessage `json:"value"`
			NextLink string         `json:"@odata.nextLink"`
		}
		if err := getJSON(ctx, t.rt, next, t.token, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Value...)
		if strings.TrimSpace(resp.NextLink) == "" {
			return out, nil
		}
		var err error
		if next, err = sameHostNext(next, resp.NextLink); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// graphMessage es lo justo de un chatMessage para transcribirlo.
//
//nolint:tagliatelle // tags = contrato de Microsoft Graph (camelCase).
type graphMessage struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdDateTime"`
	Subject   string `json:"subject"`
	Body      struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
	From struct {
		User *struct {
			DisplayName string `json:"displayName"`
		} `json:"user"`
		Application *struct {
			DisplayName string `json:"displayName"`
		} `json:"application"`
	} `json:"from"`
	Attachments []struct {
		Name string `json:"name"`
	} `json:"attachments"`
	// Graph no publica el número de respuestas al listar, así que se pide el hilo de
	// todo mensaje normal; los de sistema («fulano se unió al canal») nunca tienen.
	MessageType string `json:"messageType"`
}

// ReplyCount es una heurística deliberada: Graph no dice cuántas respuestas tiene
// un mensaje al listarlo, así que se pide el hilo de todo mensaje normal. Un
// mensaje de sistema («fulano se unió al canal») nunca tiene hilo.
func (m graphMessage) ReplyCount() int {
	if m.MessageType != "" && m.MessageType != "message" {
		return 0
	}
	return 1
}

// author es quién escribió el mensaje (persona, bot o el propio sistema).
func (m graphMessage) author() string {
	switch {
	case m.From.User != nil && strings.TrimSpace(m.From.User.DisplayName) != "":
		return m.From.User.DisplayName
	case m.From.Application != nil && strings.TrimSpace(m.From.Application.DisplayName) != "":
		return m.From.Application.DisplayName + " (aplicación)"
	default:
		return "Sistema"
	}
}

// writeMessage vuelca un mensaje. Las respuestas van citadas para que el hilo se
// lea como un hilo y no como una lista plana.
func writeMessage(w io.Writer, m graphMessage, isReply bool) error {
	body := strings.TrimSpace(plainText(m.Body.Content, m.Body.ContentType))
	if body == "" && len(m.Attachments) == 0 {
		return nil // mensaje vacío (o solo una reacción): no aporta al archivo.
	}
	when := m.CreatedAt
	if ts, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
		when = ts.UTC().Format("2006-01-02 15:04 UTC")
	}

	var b strings.Builder
	prefix := ""
	if isReply {
		prefix = "> "
	}
	if s := strings.TrimSpace(m.Subject); s != "" {
		b.WriteString(prefix + "**" + s + "**\n")
	}
	b.WriteString(prefix + "**" + m.author() + "** · " + when + "\n")
	for _, line := range strings.Split(body, "\n") {
		b.WriteString(prefix + line + "\n")
	}
	for _, a := range m.Attachments {
		if n := strings.TrimSpace(a.Name); n != "" {
			b.WriteString(prefix + "_Adjunto: " + n + "_\n")
		}
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// tagRe quita el marcado del cuerpo HTML que emite Teams.
var tagRe = regexp.MustCompile(`(?s)<[^>]*>`)

// plainText convierte a texto el cuerpo de un mensaje. Teams manda HTML casi
// siempre; se conservan los saltos de línea que el marcado representa.
func plainText(content, contentType string) string {
	if !strings.EqualFold(contentType, "html") {
		return content
	}
	s := strings.NewReplacer(
		"<br>", "\n", "<br/>", "\n", "<br />", "\n",
		"</p>", "\n", "</div>", "\n", "</li>", "\n", "<li>", "- ",
	).Replace(content)
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	// Tres saltos seguidos o más se colapsan: el HTML de Teams deja mucho hueco.
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

// splitPair parte "a|b" en sus dos mitades.
func splitPair(s string) (first, second string, ok bool) {
	first, second, ok = strings.Cut(s, "|")
	if !ok || first == "" || second == "" {
		return "", "", false
	}
	return first, second, true
}

var (
	_ Source     = (*m365Teams)(nil)
	_ TeamSource = (*m365Teams)(nil)
)
