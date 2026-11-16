// SPDX-License-Identifier: Apache-2.0
// Package web binds the Mail domain to the common verified product lifecycle.
package web

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/application"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
	"git.sytue.eu/sytue/one/apps/nucleo/webport/service"
)

type (
	Ports       = webport.ProductPorts
	mailCommand func(context.Context, *privateMail.Repository) error
	Session     struct {
		core *webport.ManagedSession[mailCommand]
	}
)
type Manager struct{ core *webport.Manager[mailCommand] }

func NewManager(sources *webport.Sources, policies *webport.Policies) (*Manager, error) {
	core, err := webport.NewManager[mailCommand](sources, policies, "mail", buildMailProduct)
	if err != nil {
		return nil, err
	}
	return &Manager{core: core}, nil
}

func (m *Manager) Open(ctx context.Context, ports Ports) (*Session, error) {
	value, err := m.core.Open(ctx, ports)
	if err != nil {
		return nil, err
	}
	return &Session{value}, nil
}

func (m *Manager) Lookup(ctx context.Context, id, port string) (*Session, error) {
	value, err := m.core.Lookup(ctx, id, port)
	if err != nil {
		return nil, err
	}
	return &Session{value}, nil
}

func (m *Manager) Get(ctx context.Context, id, port string) (*Session, error) {
	value, err := m.core.Get(ctx, id, port)
	if err != nil {
		return nil, err
	}
	return &Session{value}, nil
}

func (m *Manager) ClosePort(ctx context.Context, port string) error {
	return m.core.ClosePort(ctx, port)
}
func (s *Session) ID() string           { return s.core.ID() }
func (s *Session) Scope() storage.Scope { return s.core.Scope() }
func (s *Session) Pending(ctx context.Context) (offline.PendingWork, error) {
	return s.core.Pending(ctx)
}
func (s *Session) Pause(ctx context.Context) error  { return s.core.Pause(ctx) }
func (s *Session) Resume(ctx context.Context) error { return s.core.Resume(ctx) }
func (s *Session) run(ctx context.Context, command mailCommand) error {
	return s.core.Run(ctx, command)
}

type mailManagedProduct struct{ *application.Application }

func (p *mailManagedProduct) Ready(ctx context.Context) error {
	_, err := p.ReadyRepository(ctx)
	return err
}

func (p *mailManagedProduct) Execute(ctx context.Context, command mailCommand) error {
	if command == nil {
		return webport.ErrRejected
	}
	repository, err := p.ReadyRepository(ctx)
	if err != nil {
		return err
	}
	return command(ctx, repository)
}

func (p *mailManagedProduct) RunReceiver(ctx context.Context) error {
	return p.Application.RunReceiver(ctx, nil)
}

// Only Mail composes its original HTTP authority/cutover, migration copier,
// ingress enrollment and SMTP engine. The lifecycle is imported from webport.
func buildMailProduct(ctx context.Context, input webport.ProductBuildContext) (webport.ManagedProduct[mailCommand], error) {
	isolated, resources, journal := input.Isolated, input.Resources, input.Journal
	reader, writer := input.LegacyReader, input.MigrationWriter
	roomTransport, err := service.NewServiceTransport(isolated, "rooms")
	if err != nil {
		return nil, err
	}
	rooms := roomTransport.Rooms()
	scope := isolated.FileScope()
	authority, err := rooms.MatrixStorageAuthority(ctx, scope)
	if err != nil {
		return nil, err
	}
	if err := isolated.RequireStorageAuthority(ctx, authority); err != nil {
		return nil, err
	}
	cutover, err := rooms.MatrixStorageMigration(ctx, scope)
	if err != nil {
		return nil, err
	}
	requireLegacy := cutover.Phase != "isolated"
	if requireLegacy && (reader == nil || cutover.Phase == "legacy" && writer == nil) {
		return nil, webport.ErrRejected
	}
	mailTransport, err := service.NewServiceTransport(isolated, "mail")
	if err != nil {
		return nil, err
	}
	source := mailTransport.Mail()
	driveTransport, err := service.NewServiceTransport(isolated, "drive")
	if err != nil {
		return nil, err
	}
	cfg := application.Config{
		Files: isolated, Source: source, Control: source.PrivateIngress(), Rooms: rooms,
		Metadata: source, Usage: driveTransport.Drive(),
		Authority: authority, Journal: journal, Isolated: isolated, RequireLegacy: requireLegacy,
	}
	if requireLegacy {
		previous, err := webport.NewMigrationStorage(reader, writer)
		if err != nil {
			return nil, err
		}
		cfg.Legacy = previous
		cfg.LegacyFiles = previous
		cfg.CopyFile = func(ctx context.Context, target storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
			request, err := reader.Info(ctx, ref, object, revision)
			if err != nil {
				return storage.Reference{}, err
			}
			return reader.CopyTo(ctx, isolated, ref, object, revision, job, target, request)
		}
	}
	app, err := application.New(input.Lifetime, resources, cfg) //nolint:contextcheck // the application is bound to input.Lifetime, deliberately detached from the request ctx
	if err != nil {
		return nil, err
	}
	return &mailManagedProduct{app}, nil
}

// ComposeInputLimit frames base64 attachment JSON using the original admitted
// repository capability. Unsupported larger Wasm frames fail explicitly;
// configured Mail limits themselves remain unchanged.
func (s *Session) ComposeInputLimit(ctx context.Context) (int, error) {
	var maximumInput int
	err := s.run(ctx, func(_ context.Context, repository *privateMail.Repository) error {
		limit := repository.MessageByteLimit()
		maximum := int64(int(^uint(0) >> 1))
		if limit <= 0 || limit > (maximum-(2<<20))/4*3 {
			return api.ErrMailTooLarge
		}
		maximumInput = int((limit+2)/3*4 + (2 << 20))
		return nil
	})
	if err != nil {
		return 0, err
	}
	return maximumInput, nil
}
