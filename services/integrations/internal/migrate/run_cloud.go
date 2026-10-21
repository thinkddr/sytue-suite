// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// cloud recrea en Sytue Cloud la infraestructura de aplicaciones del origen:
// cada servicio de Cloud Run o sitio de Firebase Hosting llega como un proyecto
// con sus variables, sus secretos, su escala y sus dominios; cada bucket llega
// con sus objetos dentro; y cada tarea programada vuelve a estar programada.
//
// Orden deliberado —aplicaciones, luego buckets, luego tareas, y las bases de
// datos al final—: si el trabajo se interrumpe a la mitad, lo que ya está es lo
// que hace falta para que la aplicación arranque. Las bases de datos van al
// final porque NO se copian: se enumeran con lo que hay que hacer con ellas.
func (r *run) cloud(ctx context.Context) error {
	src, ok := r.d.Src.(CloudSource)
	if !ok {
		return fmt.Errorf("migrate: %s no tiene aplicaciones que migrar", r.d.Src.Label())
	}
	if r.d.Cloud == nil {
		return fmt.Errorf("migrate: este despliegue no tiene Sytue Cloud configurado, así que no puede recibir aplicaciones")
	}

	if err := r.cloudApps(ctx, src); err != nil {
		return err
	}
	if err := r.cloudBuckets(ctx, src); err != nil {
		return err
	}
	if err := r.cloudJobs(ctx, src); err != nil {
		return err
	}
	if err := r.cloudDatabases(ctx, src); err != nil {
		return err
	}
	return r.checkpoint(ctx, "")
}

// cloudApps crea un proyecto por aplicación, con sus variables y sus dominios.
func (r *run) cloudApps(ctx context.Context, src CloudSource) error {
	apps, err := src.ListApps(ctx)
	if err != nil {
		return fmt.Errorf("migrate: listar aplicaciones en %s: %w", r.d.Src.Label(), err)
	}
	for _, a := range apps {
		if r.seen >= maxItems {
			return fmt.Errorf("migrate: el origen supera el tope de %d entidades por trabajo", maxItems)
		}
		r.seen++
		if _, done := r.done["app:"+a.ID]; done {
			continue
		}
		if a.Skip != "" {
			r.skip("app:"+a.ID, domain.KindCloudApp, a.Name, a.Skip)
			continue
		}
		projectID, err := r.d.Cloud.EnsureProject(ctx, a)
		switch {
		case errors.Is(err, ErrUnimportable):
			r.skip("app:"+a.ID, domain.KindCloudApp, a.Name, err.Error())
			continue
		case err != nil:
			r.counts.Errors++
			r.job.LastError = err.Error()
			continue
		}
		r.record(domain.MigrationItem{
			SourceID: "app:" + a.ID, Kind: domain.KindCloudApp, SytueID: projectID, Path: a.Name,
		})

		r.cloudVars(ctx, a, projectID)
		r.cloudDomains(ctx, a, projectID)
		if err := r.checkpoint(ctx, ""); err != nil {
			return err
		}
	}
	return nil
}

// cloudVars traslada las variables de entorno y los secretos de la aplicación.
//
// Las que no se pudieron leer viajan igual pero VACÍAS y con su motivo: a una
// aplicación le falta antes el nombre de una variable que su valor, y una
// variable que no existe falla de una forma mucho más difícil de diagnosticar
// que una vacía.
func (r *run) cloudVars(ctx context.Context, a CloudApp, projectID string) {
	if len(a.Vars) == 0 {
		return
	}
	if err := r.d.Cloud.SetEnv(ctx, projectID, a.Vars); err != nil {
		r.counts.Errors++
		r.job.LastError = err.Error()
		return
	}
	for _, v := range a.Vars {
		if !v.Secret {
			continue
		}
		id := "secret:" + a.ID + ":" + v.Name
		if _, done := r.done[id]; done {
			continue
		}
		if v.Skip != "" {
			r.skip(id, domain.KindCloudSecret, a.Name+"/"+v.Name, v.Skip)
			continue
		}
		r.record(domain.MigrationItem{
			SourceID: id, Kind: domain.KindCloudSecret, SytueID: projectID, Path: a.Name + "/" + v.Name,
		})
	}
}

// cloudDomains añade al proyecto los dominios propios que servía el origen.
//
// El dominio NO empieza a servir por añadirlo aquí: hay que apuntar el DNS a
// Sytue, y eso solo puede hacerlo quien tenga el dominio. Añadirlo antes es lo
// que permite preparar el certificado y cambiar el DNS con la app ya en pie.
func (r *run) cloudDomains(ctx context.Context, a CloudApp, projectID string) {
	for _, host := range a.Domains {
		id := "domain:" + a.ID + ":" + host
		if _, done := r.done[id]; done {
			continue
		}
		err := r.d.Cloud.AddDomain(ctx, projectID, host)
		switch {
		case errors.Is(err, ErrUnimportable):
			r.skip(id, domain.KindCloudApp, host, err.Error())
		case err != nil:
			r.counts.Errors++
			r.job.LastError = err.Error()
		default:
			r.record(domain.MigrationItem{
				SourceID: id, Kind: domain.KindCloudApp, SytueID: projectID, Path: host,
			})
		}
	}
}

// cloudBuckets copia los buckets del origen con sus objetos.
func (r *run) cloudBuckets(ctx context.Context, src CloudSource) error {
	buckets, err := src.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("migrate: listar buckets en %s: %w", r.d.Src.Label(), err)
	}
	for _, b := range buckets {
		r.seen++
		if b.Skip != "" {
			if _, done := r.done["bucket:"+b.Name]; !done {
				r.skip("bucket:"+b.Name, domain.KindBucket, b.Name, b.Skip)
			}
			continue
		}
		// El bucket se aprovisiona en un proyecto propio: en el origen los buckets
		// no cuelgan de un servicio concreto, y adivinar de cuál sería inventarse
		// una relación que nadie declaró.
		projectID, err := r.d.Cloud.EnsureProject(ctx, CloudApp{
			ID: "bucket-" + b.Name, Name: b.Name, Kind: "storage", Region: b.Location,
			Description: "Almacenamiento migrado desde " + r.d.Src.Label(),
		})
		if err != nil {
			r.counts.Errors++
			r.job.LastError = err.Error()
			continue
		}
		bucketID, err := r.d.Cloud.EnsureBucket(ctx, projectID, b)
		if err != nil {
			r.counts.Errors++
			r.job.LastError = err.Error()
			continue
		}
		if _, done := r.done["bucket:"+b.Name]; !done {
			r.record(domain.MigrationItem{
				SourceID: "bucket:" + b.Name, Kind: domain.KindBucket, SytueID: bucketID, Path: b.Name,
			})
		}
		if err := r.cloudObjects(ctx, src, b, projectID); err != nil {
			return err
		}
	}
	return nil
}

// cloudObjects copia los objetos de UN bucket, página a página.
func (r *run) cloudObjects(ctx context.Context, src CloudSource, b CloudBucket, projectID string) error {
	cursor := ""
	for {
		objects, next, err := src.ListObjects(ctx, b.Name, cursor)
		if err != nil {
			// Un bucket ilegible no tumba la tanda: cuenta como fallo y reanudar lo
			// reintenta, porque no está en el mapa.
			r.counts.Errors++
			r.job.LastError = err.Error()
			return nil //nolint:nilerr // degradación deliberada, no un error tragado.
		}
		for _, o := range objects {
			if r.seen >= maxItems {
				return fmt.Errorf("migrate: el origen supera el tope de %d entidades por trabajo", maxItems)
			}
			r.seen++
			id := "object:" + b.Name + "/" + o.Key
			if _, done := r.done[id]; done {
				continue
			}
			path := b.Name + "/" + o.Key
			switch {
			case o.Skip != "":
				r.skip(id, domain.KindObject, path, o.Skip)
				continue
			case o.Size > maxFileBytes:
				r.skip(id, domain.KindObject, path, fmt.Sprintf(
					"el objeto ocupa %d bytes y supera el tope de %d por fichero", o.Size, int64(maxFileBytes),
				))
				continue
			}
			if err := r.copyObject(ctx, src, o, projectID); err != nil {
				r.counts.Errors++
				r.job.LastError = err.Error()
				continue
			}
			r.counts.Bytes += o.Size
			r.record(domain.MigrationItem{
				SourceID: id, Kind: domain.KindObject, SytueID: o.Key, Path: path,
			})
			if len(r.batch) >= batchItems {
				if err := r.checkpoint(ctx, ""); err != nil {
					return err
				}
			}
		}
		if next == "" {
			return r.checkpoint(ctx, "")
		}
		cursor = next
		if err := r.checkpoint(ctx, ""); err != nil {
			return err
		}
	}
}

// copyObject transmite UN objeto del origen al Storage del proyecto. El cuerpo
// se cierra aquí y no se carga en memoria: un bucket puede tener objetos de
// gigabytes y son cientos de miles.
func (r *run) copyObject(ctx context.Context, src CloudSource, o CloudObject, projectID string) error {
	body, err := src.OpenObject(ctx, o)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	return r.d.Cloud.UploadObject(ctx, projectID, o, io.LimitReader(body, maxFileBytes))
}

// cloudJobs vuelve a programar las tareas del origen.
func (r *run) cloudJobs(ctx context.Context, src CloudSource) error {
	jobs, err := src.ListJobs(ctx)
	if err != nil {
		// Cloud Scheduler puede no estar habilitado en el proyecto de origen, y eso
		// no es un fallo de la migración: no había tareas que traer.
		r.counts.Errors++
		r.job.LastError = err.Error()
		return nil //nolint:nilerr // degradación deliberada, no un error tragado.
	}
	if len(jobs) == 0 {
		return nil
	}
	// Todas las tareas caen en UN proyecto: en el origen tampoco pertenecen a un
	// servicio concreto, y repartirlas por afinidad sería adivinar.
	projectID, err := r.d.Cloud.EnsureProject(ctx, CloudApp{
		ID: "scheduler", Name: "tareas-programadas", Kind: "cron",
		Description: "Tareas programadas migradas desde " + r.d.Src.Label(),
	})
	if err != nil {
		r.counts.Errors++
		r.job.LastError = err.Error()
		return nil //nolint:nilerr // degradación deliberada, no un error tragado.
	}
	for _, j := range jobs {
		r.seen++
		id := "job:" + j.Name
		if _, done := r.done[id]; done {
			continue
		}
		if j.Skip != "" {
			r.skip(id, domain.KindCloudJob, j.Name, j.Skip)
			continue
		}
		sytueID, err := r.d.Cloud.EnsureCronJob(ctx, projectID, j)
		switch {
		case errors.Is(err, ErrUnimportable):
			r.skip(id, domain.KindCloudJob, j.Name, err.Error())
		case err != nil:
			r.counts.Errors++
			r.job.LastError = err.Error()
		default:
			r.record(domain.MigrationItem{
				SourceID: id, Kind: domain.KindCloudJob, SytueID: sytueID, Path: j.Name,
			})
		}
	}
	return r.checkpoint(ctx, "")
}

// cloudDatabases NO migra nada: enumera las bases de datos del origen con lo que
// hay que hacer con cada una.
//
// Copiar una base de datos por su API de control es imposible y fingirlo sería
// peor que no intentarlo: hay que exportarla y restaurarla, con la aplicación
// parada o asumiendo la escritura que ocurra mientras. Esa decisión es de quien
// es dueño de los datos, no de un asistente. Lo que sí se puede hacer —y se
// hace— es que ninguna quede fuera del informe.
func (r *run) cloudDatabases(ctx context.Context, src CloudSource) error {
	dbs, err := src.ListDatabases(ctx)
	if err != nil {
		r.counts.Errors++
		r.job.LastError = err.Error()
		return nil //nolint:nilerr // degradación deliberada, no un error tragado.
	}
	for _, db := range dbs {
		r.seen++
		id := "db:" + db.Name
		if _, done := r.done[id]; done {
			continue
		}
		r.skip(id, domain.KindDatabase, db.Name, db.Skip)
	}
	return r.checkpoint(ctx, "")
}
