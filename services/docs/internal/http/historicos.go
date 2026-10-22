// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// ResolverDirecciones pregunta al directorio de Sytue ID (peticiones.PersonasDeOrg).
type ResolverDirecciones func(ctx context.Context, c peticiones.ConsultaDirectorio) (peticiones.Resueltas, error)

// AvisosHistoricos son los avisos de Docus, Calcus y Diapos de la organización del
// contexto que la campana no llegó a guardar, cada uno con la hora en que ocurrió:
//
//   - «compartido contigo» (notifyShare): la concesión a una persona que SIGUE en pie
//     —si se le quitó o se le bloqueó, no se le recuerda—, dada a mano (no el canje
//     de un enlace, que no avisaba), nunca al dueño ni a quien la dio;
//   - comentarios (avisarComentario): al dueño y, en una respuesta, a quienes ya
//     habían escrito en ese hilo y conservan su permiso PERSONAL vivo, nunca a quien
//     lo escribe;
//   - menciones (decidido por el dueño el 2026-09-27): a quien la dirección es HOY
//     suya y HOY puede abrir el documento (avisosDeMenciones), también si colabora
//     desde otra organización. El aviso va a esa persona, no a la dirección.
//
// Y todos, solo a quien HOY puede abrir el documento: alguien que sigue siendo miembro
// de la organización (quien se fue conserva sus filas de permiso, pero ya no abre nada
// de ella), o alguien de fuera con un permiso vivo de los que abren a los de fuera
// (`externo`, el mismo filtro que `docs_anfitrion`). Eso lo sabe Sytue ID: sin
// `resolver` no se reconstruye nada. De documentos y comentarios sin borrar, de los
// últimos 90 días.
func AvisosHistoricos(ctx context.Context, pool *pg.Pool, publicURL string, resolver ResolverDirecciones) ([]hubnotify.Historico, error) {
	if resolver == nil {
		return nil, errors.New("sin el directorio de Sytue ID no se sabe quién puede abrir hoy cada documento")
	}
	directos, menciones, err := leerAvisosDeDocs(ctx, pool, publicURL)
	if err != nil {
		return nil, err
	}
	docs := map[string]bool{}
	for _, a := range directos {
		docs[a.doc] = true
	}
	for _, m := range menciones {
		docs[m.doc.ID] = true
	}
	accesos, err := accesosDeHoy(ctx, pool, slices.Sorted(maps.Keys(docs)))
	if err != nil {
		return nil, err
	}
	// Por quién se pregunta si sigue en la organización: los destinatarios directos y
	// quienes tienen un permiso `externo` (así los que ya son de dentro no se dan como
	// candidatos de fuera).
	destinatarios := map[string]bool{}
	for _, a := range directos {
		destinatarios[a.h.Subject] = true
	}
	for _, a := range accesos {
		maps.Copy(destinatarios, a.externos)
	}
	// Primera vuelta: quién sigue en la organización y de quién son las direcciones
	// mencionadas dentro de ella. Sin esto no se puede entregar nada.
	activos, miembros, err := miembrosDeHoy(ctx, resolver, slices.Sorted(maps.Keys(destinatarios)), menciones)
	if err != nil {
		return nil, fmt.Errorf("comprobar en Sytue ID quién sigue en la organización: %w", err)
	}
	var out []hubnotify.Historico
	for _, a := range directos {
		if activos[a.h.Subject] || accesos[a.doc].externos[a.h.Subject] {
			out = append(out, a.h)
		}
	}
	// Segunda vuelta, solo si hace falta: quien colabora desde fuera. Si falla, lo
	// demás se entrega igual y el error dice qué faltó: al relanzar, lo hecho sale
	// «repetido».
	deMenciones, fallo := avisosDeMenciones(ctx, resolver, publicURL, menciones, accesos, miembros, activos)
	out = append(out, deMenciones...)
	if fallo != nil {
		return out, fmt.Errorf("menciones de fuera no reconstruidas (todas o parte): %w", fallo)
	}
	return out, nil
}

// avisoDirecto es un aviso a un `sub` ya conocido (compartido, comentario), con su
// documento para comprobar quién puede abrirlo hoy.
type avisoDirecto struct {
	h   hubnotify.Historico
	doc string
}

// mencionHistorica es un comentario con direcciones mencionadas, antes de saber de
// quién son.
type mencionHistorica struct {
	id, texto, autor string
	fecha            time.Time
	doc              domain.Document
	direcciones      []string
}

// leerAvisosDeDocs lee de la base los compartidos y comentarios, y aparte los
// comentarios con menciones.
func leerAvisosDeDocs(ctx context.Context, pool *pg.Pool, publicURL string) ([]avisoDirecto, []mencionHistorica, error) {
	t := tenancy.MustFrom(ctx)
	var out []avisoDirecto
	var menciones []mencionHistorica
	err := pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.created_at, d.id::text, d.kind::text, d.title, p.principal_id
			FROM doc_permissions p
			JOIN documents d ON d.id = p.document_id
			WHERE d.tenant = $1 AND d.deleted_at IS NULL AND p.principal_kind = 'user'
			  AND p.principal_id NOT LIKE '%@%'
			  AND p.principal_id <> d.owner_sub AND p.principal_id <> p.granted_by
			  AND p.via_link IS NULL AND p.bloqueado_desde IS NULL
			ORDER BY p.created_at`, t.Slug)
		if err != nil {
			return err
		}
		for rows.Next() {
			var h hubnotify.Historico
			var doc domain.Document
			var para string
			if err := rows.Scan(&h.Fecha, &doc.ID, &doc.Kind, &doc.Title, &para); err != nil {
				rows.Close()
				return err
			}
			h.Notification = avisoDeDocumento(publicURL, t.Slug, para, &doc, "docs.shared", tituloCompartido(&doc), "", "")
			h.Clave = "docs:shared:" + doc.ID
			out = append(out, avisoDirecto{h: h, doc: doc.ID})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		// Un comentario con quién tiene que enterarse. `quien` vacío es la fila de las
		// menciones, que se buscan en el cuerpo.
		rows, err = tx.Query(ctx, `
			SELECT c.created_at, c.id::text, c.body, c.author_sub, d.id::text, d.kind::text, d.title, q.quien
			FROM doc_comments c
			JOIN documents d ON d.id = c.document_id
			CROSS JOIN LATERAL (
			    SELECT '' AS quien
			    UNION
			    SELECT d.owner_sub
			    UNION
			    SELECT o.author_sub FROM doc_comments o
			    WHERE c.parent_id IS NOT NULL AND o.document_id = c.document_id
			      AND o.deleted_at IS NULL AND o.created_at < c.created_at
			      AND (o.id = c.parent_id OR o.parent_id = c.parent_id)
			      AND EXISTS (SELECT 1 FROM doc_permissions p
			                  WHERE p.document_id = d.id AND p.principal_kind = 'user'
			                    AND p.principal_id = o.author_sub
			                    AND docs_permiso_vivo(p.via_link, p.bloqueado_desde))
			) q
			WHERE d.tenant = $1 AND d.deleted_at IS NULL AND c.deleted_at IS NULL
			  AND q.quien <> c.author_sub
			  AND c.created_at > $2
			ORDER BY c.created_at, c.id`, t.Slug, time.Now().Add(-hubnotify.VentanaHistorica))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var fecha time.Time
			var id, texto, autor, quien string
			var doc domain.Document
			if err := rows.Scan(&fecha, &id, &texto, &autor, &doc.ID, &doc.Kind, &doc.Title, &quien); err != nil {
				return err
			}
			if quien == "" {
				if ds := hubnotify.Mencionados(texto); len(ds) > 0 {
					menciones = append(menciones, mencionHistorica{id: id, fecha: fecha, texto: texto, autor: autor, doc: doc, direcciones: ds})
				}
				continue
			}
			out = append(out, avisoDirecto{doc: doc.ID, h: hubnotify.Historico{
				Notification: avisoDeDocumento(publicURL, t.Slug, quien, &doc, "docs.comment", tituloComentario(&doc),
					validation.TruncateRunes(texto, 300), ""),
				Clave: "docs:comment:" + id,
				Fecha: fecha,
			}})
		}
		return rows.Err()
	})
	return out, menciones, err
}

// miembrosDeHoy pregunta, por tandas, cuáles de `subs` siguen siendo miembros activos
// de la organización y de quién de ella son las direcciones mencionadas.
func miembrosDeHoy(ctx context.Context, resolver ResolverDirecciones, subs []string, menciones []mencionHistorica) (activos map[string]bool, miembros map[string]string, err error) {
	unicas := map[string]bool{}
	for _, m := range menciones {
		for _, d := range m.direcciones {
			unicas[d] = true
		}
	}
	emails := slices.Sorted(maps.Keys(unicas))
	activos, miembros = map[string]bool{}, map[string]string{}
	for len(emails) > 0 || len(subs) > 0 {
		ne, ns := min(len(emails), peticiones.MaxDireccionesDirectorio), min(len(subs), peticiones.MaxSubsDirectorio)
		r, err := resolver(ctx, peticiones.ConsultaDirectorio{Emails: emails[:ne], Subs: subs[:ns]})
		if err != nil {
			return nil, nil, err
		}
		emails, subs = emails[ne:], subs[ns:]
		maps.Copy(miembros, r.Miembros)
		for _, s := range r.Activos {
			activos[s] = true
		}
	}
	// Una dirección de un miembro es de alguien que sigue en la organización.
	for _, s := range miembros {
		activos[s] = true
	}
	return activos, miembros, nil
}

// avisosDeMenciones deja cada mención solo a quien HOY puede abrir su documento:
//
//   - alguien de la organización (`miembros`): su dueño, quien tiene un permiso
//     personal vivo, o cualquiera si el documento está abierto a toda ella;
//   - quien colabora desde otra organización (ADR 0040): con un permiso vivo de los
//     que abren a los de fuera en ESE documento (`externo`: el canje de un enlace, ADR
//     0052, o la concesión a mano que lo sustituye). Sytue ID solo contesta con uno de
//     esos candidatos, y solo se le pregunta por las direcciones que no dieron ya con
//     alguien de dentro que pueda abrirlo.
//
// Los permisos de grupo no se miran: dependen de Sytue ID, y ante la duda no se avisa.
// Nunca a quien escribió el comentario.
func avisosDeMenciones(ctx context.Context, resolver ResolverDirecciones, publicURL string, menciones []mencionHistorica, accesos map[string]accesoDeHoy, miembros map[string]string, activos map[string]bool) ([]hubnotify.Historico, error) {
	t := tenancy.MustFrom(ctx)
	deFuera := map[string]string{}
	var fallo error
	for _, c := range tandasDirectorio(candidatosDeFuera(menciones, accesos, miembros, activos)) {
		r, err := resolver(ctx, c)
		if err != nil {
			fallo = err
			break
		}
		maps.Copy(deFuera, r.ConPermiso)
	}
	var out []hubnotify.Historico
	for _, m := range menciones {
		a, ok := accesos[m.doc.ID]
		if !ok {
			continue
		}
		var avisados []string
		for _, d := range m.direcciones {
			sub := ""
			if s := miembros[d]; s != "" && a.puede(s) {
				sub = s
			} else if s := deFuera[d]; s != "" && a.externos[s] {
				sub = s // de fuera: solo con permiso de los de fuera en ESTE documento
			}
			if sub == "" || sub == m.autor || slices.Contains(avisados, sub) {
				continue
			}
			avisados = append(avisados, sub)
			doc := m.doc
			out = append(out, hubnotify.Historico{
				Notification: avisoDeDocumento(publicURL, t.Slug, sub, &doc, "docs.mention", tituloMencion(&doc),
					validation.TruncateRunes(m.texto, 300), ""),
				Clave: "docs:mention:" + m.id,
				Fecha: m.fecha,
			})
		}
	}
	return out, fallo
}

// candidatosDeFuera son, por dirección mencionada, quienes tienen en ese documento un
// permiso de los que abren a los de fuera. Solo para las direcciones que no dieron ya
// con alguien de dentro que pueda abrirlo, y sin quienes ya son de la organización: a
// Sytue ID no se le pregunta de más.
func candidatosDeFuera(menciones []mencionHistorica, accesos map[string]accesoDeHoy, miembros map[string]string, activos map[string]bool) map[string]map[string]bool {
	candidatos := map[string]map[string]bool{}
	for _, m := range menciones {
		a := accesos[m.doc.ID]
		for _, d := range m.direcciones {
			if s := miembros[d]; s != "" && a.puede(s) {
				continue
			}
			for sub := range a.externos {
				if activos[sub] {
					continue
				}
				if candidatos[d] == nil {
					candidatos[d] = map[string]bool{}
				}
				candidatos[d][sub] = true
			}
		}
	}
	return candidatos
}

// tandasDirectorio parte las preguntas por candidatos en tandas que caben en los topes
// de Sytue ID: direcciones, candidatos en total y por dirección. Una dirección con más
// candidatos de los que caben va en varias tandas, cada una con una parte: así ninguno
// se queda sin preguntar.
func tandasDirectorio(candidatos map[string]map[string]bool) []peticiones.ConsultaDirectorio {
	var tandas []peticiones.ConsultaDirectorio
	actual := peticiones.ConsultaDirectorio{Candidatos: map[string][]string{}}
	enActual := 0
	cerrar := func() {
		if len(actual.Candidatos) > 0 {
			tandas = append(tandas, actual)
		}
		actual, enActual = peticiones.ConsultaDirectorio{Candidatos: map[string][]string{}}, 0
	}
	for _, d := range slices.Sorted(maps.Keys(candidatos)) {
		subs := slices.Sorted(maps.Keys(candidatos[d]))
		for len(subs) > 0 {
			hueco := min(peticiones.MaxCandidatosDirectorio-enActual, peticiones.MaxCandidatosPorDireccion)
			if hueco <= 0 || len(actual.Candidatos) >= peticiones.MaxDireccionesDirectorio || actual.Candidatos[d] != nil {
				cerrar()
				continue
			}
			n := min(hueco, len(subs))
			actual.Candidatos[d] = subs[:n]
			enActual += n
			subs = subs[n:]
		}
	}
	cerrar()
	return tandas
}

// accesoDeHoy es quién puede abrir hoy un documento, sin contar grupos.
type accesoDeHoy struct {
	dueno string
	// personas: permisos personales vivos de gente de la organización.
	personas map[string]bool
	// externos: los vivos que abren el documento a alguien de fuera (`externo`), el
	// mismo filtro que `docs_anfitrion` (0010).
	externos map[string]bool
	toda     bool // abierto a toda la organización (solo a su gente)
}

// puede es para alguien de la organización: también con un permiso `externo`, que es
// el de quien entró por un enlace siendo de fuera y después se hizo miembro.
func (a accesoDeHoy) puede(sub string) bool {
	return sub == a.dueno || a.toda || a.personas[sub] || a.externos[sub]
}

// accesosDeHoy lee, de los documentos sin borrar, quién puede abrirlos hoy.
func accesosDeHoy(ctx context.Context, pool *pg.Pool, docs []string) (map[string]accesoDeHoy, error) {
	t := tenancy.MustFrom(ctx)
	out := map[string]accesoDeHoy{}
	err := pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT d.id::text, d.owner_sub,
			       coalesce(array_agg(p.principal_id) FILTER (
			           WHERE p.principal_kind = 'user' AND p.principal_id NOT LIKE '%@%' AND NOT p.externo
			             AND docs_permiso_vivo(p.via_link, p.bloqueado_desde)), '{}'),
			       coalesce(array_agg(p.principal_id) FILTER (
			           WHERE p.principal_kind = 'user' AND p.externo
			             AND docs_permiso_vivo(p.via_link, p.bloqueado_desde)), '{}'),
			       coalesce(bool_or(p.principal_kind = 'tenant' AND p.principal_id = d.tenant
			                        AND docs_permiso_vivo(p.via_link, p.bloqueado_desde)), false)
			FROM documents d
			LEFT JOIN doc_permissions p ON p.document_id = d.id
			WHERE d.tenant = $1 AND d.deleted_at IS NULL AND d.id = ANY ($2::uuid[])
			GROUP BY d.id, d.owner_sub`, t.Slug, docs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var a accesoDeHoy
			var personas, externos []string
			if err := rows.Scan(&id, &a.dueno, &personas, &externos, &a.toda); err != nil {
				return err
			}
			a.personas, a.externos = conjunto(personas), conjunto(externos)
			out[id] = a
		}
		return rows.Err()
	})
	return out, err
}

func conjunto(v []string) map[string]bool {
	out := make(map[string]bool, len(v))
	for _, x := range v {
		out[x] = true
	}
	return out
}
