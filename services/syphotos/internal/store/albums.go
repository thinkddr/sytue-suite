package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"git.sytue.eu/sytue/one/platform/pkg/pg"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// colsAlbum: la portada efectiva y el recuento se calculan al leer. Solo
// cuentan las fotos que no están en la papelera, que es lo que se ve.
const colsAlbum = `
	al.id, al.tenant, al.owner_sub, al.title,
	COALESCE(al.cover_asset_id, (
		SELECT aa.asset_id FROM album_assets aa JOIN assets a ON a.id = aa.asset_id
		WHERE aa.album_id = al.id AND a.deleted_at IS NULL AND a.owner_sub = al.owner_sub
		ORDER BY aa.position, aa.added_at LIMIT 1)),
	(SELECT count(*) FROM album_assets aa JOIN assets a ON a.id = aa.asset_id
		WHERE aa.album_id = al.id AND a.deleted_at IS NULL AND a.owner_sub = al.owner_sub),
	al.created_at, al.updated_at, al.public_token_hash <> ''`

// `extra` son destinos para las columnas que la consulta ponga DETRÁS de colsAlbum.
func scanAlbum(row pgx.Row, extra ...any) (domain.Album, error) {
	var al domain.Album
	err := row.Scan(append([]any{
		&al.ID, &al.Tenant, &al.Duenyo, &al.Titulo, &al.PortadaID, &al.Fotos,
		&al.CreadoEn, &al.ActualizadoEn, &al.Compartido,
	}, extra...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return al, domain.ErrNotFound
	}
	return al, err
}

// CrearAlbum crea un álbum vacío de esa persona.
func (s *Store) CrearAlbum(ctx context.Context, duenyo, titulo string) (domain.Album, error) {
	titulo, err := domain.TituloDeAlbum(titulo)
	if err != nil {
		return domain.Album{}, err
	}
	var al domain.Album
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			WITH nuevo AS (
				INSERT INTO albums (tenant, owner_sub, title) VALUES ($1, $2, $3) RETURNING *
			) SELECT `+colsAlbum+` FROM nuevo al`, tenant, duenyo, titulo)
		al, err = scanAlbum(row)
		return err
	})
	return al, err
}

// Albumes lista los de esa persona, el tocado más recientemente primero.
func (s *Store) Albumes(ctx context.Context, duenyo string) ([]domain.Album, error) {
	var out []domain.Album
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+colsAlbum+` FROM albums al
			WHERE al.tenant = $1 AND al.owner_sub = $2
			ORDER BY al.updated_at DESC, al.id DESC`, tenant, duenyo)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			al, err := scanAlbum(rows)
			if err != nil {
				return err
			}
			out = append(out, al)
		}
		return rows.Err()
	})
	return out, err
}

// Album devuelve uno de esa persona; de otra, no existe.
func (s *Store) Album(ctx context.Context, id uuid.UUID, duenyo string) (domain.Album, error) {
	var al domain.Album
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		al, err = scanAlbum(tx.QueryRow(ctx, `
			SELECT `+colsAlbum+` FROM albums al
			WHERE al.tenant = $1 AND al.id = $2 AND al.owner_sub = $3`, tenant, id, duenyo))
		return err
	})
	return al, err
}

// RenombrarAlbum cambia el título y, si se pasa, la portada (que tiene que
// ser una foto del álbum: una portada de fuera es un enlace roto en potencia).
// `quien` es su dueña o alguien con edición (0010), comprobado en la MISMA transacción.
func (s *Store) RenombrarAlbum(ctx context.Context, id uuid.UUID, quien, titulo string, portada *uuid.UUID) (domain.Album, error) {
	titulo, err := domain.TituloDeAlbum(titulo)
	if err != nil {
		return domain.Album{}, err
	}
	var al domain.Album
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		duenyo, err := duenyaEditable(ctx, tx, tenant, id, quien)
		if err != nil {
			return err
		}
		if portada != nil {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM album_assets
				WHERE tenant = $1 AND album_id = $2 AND asset_id = $3)`, tenant, id, *portada).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("%w: la portada tiene que ser una foto del álbum", domain.ErrInvalidInput)
			}
		}
		row := tx.QueryRow(ctx, `
			WITH tocado AS (
				UPDATE albums SET title = $4, cover_asset_id = COALESCE($5, cover_asset_id), updated_at = now()
				WHERE tenant = $1 AND id = $2 AND owner_sub = $3 RETURNING *
			) SELECT `+colsAlbum+` FROM tocado al`, tenant, id, duenyo, titulo, portada)
		al, err = scanAlbum(row)
		return err
	})
	return al, err
}

// BorrarAlbum borra el álbum y sus pertenencias (CASCADE). Las fotos no se tocan.
func (s *Store) BorrarAlbum(ctx context.Context, id uuid.UUID, duenyo string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM albums WHERE tenant = $1 AND id = $2 AND owner_sub = $3`,
			tenant, id, duenyo)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// AnyadirAlAlbum añade fotos al final, en el orden dado; las que ya estaban
// se ignoran (un arrastre repetido no duplica ni reordena). Solo fotos DE SU
// DUEÑA y no borradas: las demás se saltan en silencio, que es lo que hace un
// «añadir» con una selección donde algo ya no existe. Una foto de otra persona
// en un álbum sería servírsela a quien lo abre por su enlace o se lo compartieron.
//
// `quien` es su dueña o alguien con edición (0010), comprobado en la MISMA transacción;
// las fotos tienen que ser de la dueña (las de quien edita llegan ya copiadas a ella).
func (s *Store) AnyadirAlAlbum(ctx context.Context, album uuid.UUID, quien string, fotos []uuid.UUID) (int, error) {
	fotos = sinRepetidas(fotos)
	if len(fotos) == 0 {
		return 0, nil
	}
	n := 0
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		duenyo, err := duenyaEditable(ctx, tx, tenant, album, quien)
		if err != nil {
			return err
		}
		// UNA sentencia para toda la selección, y no una por foto. Con un INSERT
		// por foto, añadir doscientas eran doscientos viajes Y doscientos
		// `max(position)` sobre una tabla que crece con cada uno; aquí el máximo
		// se calcula una vez —la subconsulta no mira la fila de fuera, así que
		// Postgres la resuelve como InitPlan— y `row_number()` reparte el resto.
		//
		// El `NOT EXISTS` va ANTES de numerar, que es lo que mantiene las
		// posiciones contiguas: una foto que ya estaba no debe gastar un número.
		// El `ON CONFLICT` se queda como red por si dos peticiones a la vez
		// añaden la misma.
		tag, err := tx.Exec(ctx, `
			INSERT INTO album_assets (tenant, album_id, asset_id, position)
			SELECT $1, $2, a.id,
			       (SELECT COALESCE(max(position), -1) FROM album_assets WHERE album_id = $2)
			       + row_number() OVER (ORDER BY t.ord)
			  FROM unnest($3::uuid[]) WITH ORDINALITY AS t(fid, ord)
			  JOIN assets a ON a.tenant = $1 AND a.id = t.fid AND a.deleted_at IS NULL AND a.owner_sub = $4
			 WHERE NOT EXISTS (
			       SELECT 1 FROM album_assets x WHERE x.album_id = $2 AND x.asset_id = a.id)
			ON CONFLICT (album_id, asset_id) DO NOTHING`, tenant, album, fotos, duenyo)
		if err != nil {
			return err
		}
		n = int(tag.RowsAffected())
		if n > 0 {
			_, err = tx.Exec(ctx, `UPDATE albums SET updated_at = now() WHERE id = $1`, album)
		}
		return err
	})
	return n, err
}

// sinRepetidas quita ids duplicados CONSERVANDO el orden de la primera
// aparición. Antes daba igual —cada foto iba en su propia sentencia y la
// segunda chocaba con el ON CONFLICT—, pero al numerar las posiciones de una
// vez un id repetido gastaría un número y dejaría un hueco.
func sinRepetidas(ids []uuid.UUID) []uuid.UUID {
	vistas := make(map[uuid.UUID]struct{}, len(ids))
	out := ids[:0]
	for _, id := range ids {
		if _, ya := vistas[id]; ya {
			continue
		}
		vistas[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// QuitarDelAlbum saca una foto del álbum. La foto sigue en la galería.
// `quien` es su dueña o alguien con edición (0010), comprobado en la MISMA transacción.
func (s *Store) QuitarDelAlbum(ctx context.Context, album uuid.UUID, quien string, foto uuid.UUID) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		if _, err := duenyaEditable(ctx, tx, tenant, album, quien); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM album_assets WHERE tenant = $1 AND album_id = $2 AND asset_id = $3`,
			tenant, album, foto)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		_, err = tx.Exec(ctx, `UPDATE albums SET updated_at = now() WHERE id = $1`, album)
		return err
	})
}

// FotosDelAlbum devuelve las fotos en el orden del álbum, sin las de la papelera.
func (s *Store) FotosDelAlbum(ctx context.Context, album uuid.UUID, duenyo string) ([]domain.Activo, error) {
	var out []domain.Activo
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		if err := s.albumDe(ctx, tx, tenant, album, duenyo); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+cols+` FROM album_assets aa JOIN assets a ON a.id = aa.asset_id
			JOIN albums al ON al.id = aa.album_id AND al.owner_sub = a.owner_sub
			WHERE aa.tenant = $1 AND aa.album_id = $2 AND a.deleted_at IS NULL
			ORDER BY aa.position, aa.added_at`, tenant, album)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanActivo(rows, tenant)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// duenyaEditable es la dueña del álbum si `quien` puede editarlo —es ella o se lo
// compartió con edición—, dentro de la transacción de lo que se va a hacer: retirar el
// permiso a mitad no deja pasar lo que llegue después. ErrNotFound si no.
func duenyaEditable(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, quien string) (string, error) {
	var duenya string
	err := tx.QueryRow(ctx, `
		SELECT al.owner_sub FROM albums al
		 WHERE al.tenant = $1 AND al.id = $2
		   AND (al.owner_sub = $3 OR EXISTS (SELECT 1 FROM album_personas p
		        WHERE p.album_id = al.id AND p.sujeto = $3 AND p.puede_editar))`,
		tenant, id, quien).Scan(&duenya)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && quien == "") {
		return "", domain.ErrNotFound
	}
	return duenya, err
}

// albumDe comprueba dentro de la transacción que el álbum es de esa persona.
func (s *Store) albumDe(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, duenyo string) error {
	// El id que devuelve el SELECT es el mismo que se pasa: lo que se comprueba
	// aquí es que EXISTE y que es de quien dice, no cuál es. Por eso se descarta.
	var got uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM albums WHERE tenant = $1 AND id = $2 AND owner_sub = $3`,
		tenant, id, duenyo).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

// FotoPublica es una foto tal como se sirve por un enlace: el activo y la
// huella de la receta de su dueña, que es lo que nombra la derivada. Huella
// vacía significa «sin editar».
type FotoPublica struct {
	Activo domain.Activo
	Huella string
}

// bytesDelToken: 32 bytes de azar. El token ES la credencial del enlace, así
// que tiene que resistir que alguien pruebe tokens a millones; con 256 bits no
// hay nada que probar.
const bytesDelToken = 32

func nuevoToken() (string, error) {
	b := make([]byte, bytesDelToken)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("syphotos: no hay azar para el enlace: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// huellaDelToken es lo que se guarda del token del enlace: su SHA-256 en hex,
// el mismo cálculo que la migración 0012 hizo con los que ya había.
func huellaDelToken(token string) string {
	suma := sha256.Sum256([]byte(token))
	return hex.EncodeToString(suma[:])
}

// CompartirAlbum genera un enlace NUEVO y lo devuelve; el anterior, si lo había,
// deja de valer (FILE-10, decisión del dueño). Del token solo se guarda la
// huella, así que no hay forma de devolver el de antes.
func (s *Store) CompartirAlbum(ctx context.Context, id uuid.UUID, duenyo string) (string, error) {
	token, err := nuevoToken()
	if err != nil {
		return "", err
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE albums SET public_token_hash = $4, updated_at = now()
			WHERE tenant = $1 AND id = $2 AND owner_sub = $3`, tenant, id, duenyo, huellaDelToken(token))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// DejarDeCompartirAlbum apaga el enlace. Los que ya se mandaron dejan de valer,
// que es exactamente lo que se pide al pulsarlo.
func (s *Store) DejarDeCompartirAlbum(ctx context.Context, id uuid.UUID, duenyo string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE albums SET public_token_hash = '', updated_at = now()
			WHERE tenant = $1 AND id = $2 AND owner_sub = $3`, tenant, id, duenyo)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// AlbumPorToken resuelve el álbum de un enlace, SIN sesión y sin organización.
//
// `app.public_link='on'` es lo que lo hace posible: con RLS forzada y sin
// organización fijada, la política de aislamiento no devuelve ni una fila —ni
// siquiera la del token correcto—. La política que abre esta puerta solo ve
// álbumes CON token, así que la credencial sigue siendo el token.
//
// Es la ÚNICA función del servicio que fija esa variable.
func (s *Store) AlbumPorToken(ctx context.Context, token string) (domain.Album, error) {
	var al domain.Album
	if token == "" {
		return al, domain.ErrNotFound
	}
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.public_link', 'on', true)"); err != nil {
			return fmt.Errorf("syphotos: abrir la lectura del enlace: %w", err)
		}
		var err error
		al, err = scanAlbum(tx.QueryRow(ctx, `
			SELECT `+colsAlbum+` FROM albums al WHERE al.public_token_hash = $1`, huellaDelToken(token)))
		return err
	})
	if err != nil {
		return domain.Album{}, err
	}
	// El dueño no se publica: es un dato personal que no aporta nada a quien
	// abre el enlace.
	al.Duenyo = ""
	return al, nil
}

// FotosDelAlbumPublico lee las fotos de un álbum ya resuelto por su token.
//
// Va con la organización del álbum FIJADA y la política de aislamiento de
// siempre: aquí no se relaja nada, porque a estas alturas ya se sabe de quién
// es lo que se está leyendo.
func (s *Store) FotosDelAlbumPublico(ctx context.Context, al domain.Album) ([]FotoPublica, error) {
	var out []FotoPublica
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, al.Tenant); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+cols+`, coalesce(e.fingerprint, '')
			FROM album_assets aa
			JOIN assets a ON a.id = aa.asset_id
			-- Solo las fotos de la dueña del álbum: una ajena no sale ni por el enlace ni
			-- por «Compartido conmigo», aunque alguien la hubiera metido.
			JOIN albums al ON al.id = aa.album_id AND al.owner_sub = a.owner_sub
			LEFT JOIN edits e
			  ON e.tenant = a.tenant AND e.asset_id = a.id AND e.owner_sub = a.owner_sub
			WHERE aa.tenant = $1 AND aa.album_id = $2 AND a.deleted_at IS NULL
			ORDER BY aa.position, aa.added_at
			LIMIT 500`, al.Tenant, al.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var f FotoPublica
			f.Activo, f.Huella, err = scanPublica(rows, al.Tenant)
			if err != nil {
				return err
			}
			out = append(out, f)
		}
		return rows.Err()
	})
	return out, err
}

// FotoDelAlbumPublico resuelve UNA foto del álbum. Que la foto esté en el
// álbum es lo que autoriza a servirla: sin esa comprobación, un enlace válido
// sería una licencia para pedir cualquier identificador de la organización.
func (s *Store) FotoDelAlbumPublico(ctx context.Context, al domain.Album, id uuid.UUID) (FotoPublica, error) {
	var f FotoPublica
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, al.Tenant); err != nil {
			return err
		}
		var err error
		f.Activo, f.Huella, err = scanPublica(tx.QueryRow(ctx, `
			SELECT `+cols+`, coalesce(e.fingerprint, '')
			FROM album_assets aa
			JOIN assets a ON a.id = aa.asset_id
			-- Solo las fotos de la dueña del álbum: una ajena no sale ni por el enlace ni
			-- por «Compartido conmigo», aunque alguien la hubiera metido.
			JOIN albums al ON al.id = aa.album_id AND al.owner_sub = a.owner_sub
			LEFT JOIN edits e
			  ON e.tenant = a.tenant AND e.asset_id = a.id AND e.owner_sub = a.owner_sub
			WHERE aa.tenant = $1 AND aa.album_id = $2 AND a.id = $3 AND a.deleted_at IS NULL`,
			al.Tenant, al.ID, id), al.Tenant)
		return err
	})
	return f, err
}

// scanPublica lee un activo y, pegada, la huella de su receta.
func scanPublica(row pgx.Row, tenant string) (domain.Activo, string, error) {
	var a domain.Activo
	var orient int16
	var huella string
	// LA LISTA VA PEGADA A `cols` Y SE ROMPE SI SE SEPARAN. Al añadir la
	// descripción y las etiquetas al SELECT compartido, este Scan se quedó dos
	// columnas corto y los álbumes compartidos dejaron de resolver — con un
	// error de pgx que no dice qué falta. Si vuelve a pasar, es esto.
	err := row.Scan(&a.ID, &a.Duenyo, &a.DriveID, &a.Nombre, &a.Tipo, &a.Bytes,
		&a.Ancho, &a.Alto, &orient, &a.TomadaEn, &a.SubidaEn, &a.Favorita, &a.Camara,
		&a.Objetivo, &a.Lat, &a.Lon, &a.Descripcion, &a.Etiquetas, &huella)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Activo{}, "", domain.ErrNotFound
	}
	if err != nil {
		return domain.Activo{}, "", err
	}
	a.Orientacion = domain.OrientacionDe(int(orient))
	a.Tenant = tenant
	return a, huella, nil
}
