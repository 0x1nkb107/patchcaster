package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-faster/jx"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/auth"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// setupActor names the setup in the audit log: before the first
// administrator exists, the setup link is all that acts.
var setupActor = store.Actor{Label: "setup link"}

// GetInstance describes this instance.
func (s *Service) GetInstance(ctx context.Context) (*v1gen.Instance, error) {
	admins, err := s.store.AdministratorCount(ctx)
	if err != nil {
		return nil, err
	}
	return &v1gen.Instance{
		Version:       s.version,
		SetupRequired: admins == 0,
	}, nil
}

// SetUp stores the identity provider a setup link's holder names. The session
// remembers the link, so that the login which follows makes the person
// logging in the first administrator (SYSREQ-038).
func (s *Service) SetUp(ctx context.Context, req *v1gen.SetupRequest) (*v1gen.SetupStarted, error) {
	linkHash := auth.SetupTokenHash(req.SetupToken)
	if err := s.checkSetupLink(ctx, linkHash); err != nil {
		return nil, err
	}
	if err := s.checkProvider(ctx, req.Issuer); err != nil {
		return nil, err
	}
	sealed, err := s.sealSecret(req.ClientSecret)
	if err != nil {
		return nil, err
	}
	settings := store.LoginSettings{
		Issuer:        req.Issuer,
		ClientID:      req.ClientID,
		ClientSecret:  sealed,
		GroupsClaim:   req.GroupsClaim.Or(""),
		AllowedGroups: req.AllowedGroups,
	}
	if err := s.store.PutSetupLoginSettings(ctx, setupActor, linkHash, settings, s.now()); err != nil {
		return nil, storeProblem(err)
	}
	auth.RememberSetupLink(ctx, s.sessions, linkHash)
	return &v1gen.SetupStarted{LoginURL: s.login.URL()}, nil
}

// checkSetupLink refuses a setup once it is complete or without a valid
// link. It runs before anything reaches out to a provider, so that only the
// link's holder can make the instance do so.
func (s *Service) checkSetupLink(ctx context.Context, linkHash []byte) error {
	admins, err := s.store.AdministratorCount(ctx)
	if err != nil {
		return err
	}
	if admins > 0 {
		return storeProblem(store.ErrAdministratorExists)
	}
	valid, err := s.store.SetupLinkValid(ctx, linkHash, s.now())
	if err != nil {
		return err
	}
	if !valid {
		return storeProblem(store.ErrSetupLinkSpent)
	}
	return nil
}

// checkProvider refuses an issuer at which no OpenID Connect provider
// answers, naming the field for the form.
func (s *Service) checkProvider(ctx context.Context, issuer string) error {
	err := s.login.Check(ctx, issuer)
	if err == nil {
		return nil
	}
	s.log.WarnContext(ctx, "identity provider check failed", "issuer", issuer, "err", err)
	return fieldProblem(http.StatusUnprocessableEntity, err.Error(),
		"issuer", "no OpenID Connect provider answers at this issuer")
}

// sealSecret seals a client secret for the login settings (SYSREQ-042).
func (s *Service) sealSecret(secret string) ([]byte, error) {
	sealed, err := s.box.Seal([]byte(secret), auth.ClientSecretBinding)
	if err != nil {
		return nil, fmt.Errorf("seal client secret: %w", err)
	}
	return sealed, nil
}

// GetSession describes the person logged in.
func (s *Service) GetSession(ctx context.Context) (*v1gen.Person, error) {
	person := personOf(caller(ctx))
	return &person, nil
}

// EndSession logs out (SYSREQ-037).
func (s *Service) EndSession(ctx context.Context) error {
	if err := s.sessions.Destroy(ctx); err != nil {
		return fmt.Errorf("end session: %w", err)
	}
	return nil
}

func personOf(p *store.Person) v1gen.Person {
	return v1gen.Person{
		ID:            p.ID,
		Issuer:        p.Issuer,
		Subject:       p.Subject,
		DisplayName:   p.DisplayName,
		Email:         p.Email,
		Administrator: p.Administrator,
		Etag:          etagOf(p.Version),
	}
}

// ListPersons pages through people, for administrators.
func (s *Service) ListPersons(ctx context.Context, params v1gen.ListPersonsParams) (*v1gen.PersonPage, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	afterID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, err
	}
	limit := params.Limit.Or(defaultPageSize)
	people, err := s.store.People(ctx, afterID, limit+1)
	if err != nil {
		return nil, err
	}
	people, next := pageOf(people, limit, func(p store.Person) int64 { return p.ID })
	items := make([]v1gen.Person, 0, len(people))
	for i := range people {
		items = append(items, personOf(&people[i]))
	}
	return &v1gen.PersonPage{Items: items, NextCursor: next}, nil
}

// UpdatePerson changes a person's role, for administrators.
func (s *Service) UpdatePerson(ctx context.Context, req *v1gen.PersonUpdate, params v1gen.UpdatePersonParams) (*v1gen.PersonHeaders, error) {
	admin, err := administrator(ctx)
	if err != nil {
		return nil, err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return nil, err
	}
	person, err := s.store.SetAdministrator(ctx, store.ActorOf(admin), params.ID, req.Administrator, version, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	return &v1gen.PersonHeaders{
		ETag:     v1gen.NewOptString(etagOf(person.Version)),
		Response: personOf(person),
	}, nil
}

func loginSettingsHeaders(s *store.LoginSettings) *v1gen.LoginSettingsHeaders {
	groups := s.AllowedGroups
	if groups == nil {
		groups = []string{}
	}
	return &v1gen.LoginSettingsHeaders{
		ETag: v1gen.NewOptString(etagOf(s.Version)),
		Response: v1gen.LoginSettings{
			Issuer:        s.Issuer,
			ClientID:      s.ClientID,
			GroupsClaim:   s.GroupsClaim,
			AllowedGroups: groups,
		},
	}
}

// GetLoginSettings reads the identity provider settings, without the client
// secret, for administrators.
func (s *Service) GetLoginSettings(ctx context.Context) (*v1gen.LoginSettingsHeaders, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	settings, err := s.store.LoginSettings(ctx)
	if err != nil {
		return nil, storeProblem(err)
	}
	return loginSettingsHeaders(settings), nil
}

// PutLoginSettings changes the identity provider settings, for
// administrators. Without a new client secret, the stored one stays.
func (s *Service) PutLoginSettings(ctx context.Context, req *v1gen.LoginSettingsUpdate, params v1gen.PutLoginSettingsParams) (*v1gen.LoginSettingsHeaders, error) {
	admin, err := administrator(ctx)
	if err != nil {
		return nil, err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return nil, err
	}
	if err := s.checkProvider(ctx, req.Issuer); err != nil {
		return nil, err
	}
	settings := store.LoginSettings{
		Issuer:        req.Issuer,
		ClientID:      req.ClientID,
		GroupsClaim:   req.GroupsClaim,
		AllowedGroups: req.AllowedGroups,
		Version:       version,
	}
	if secret, ok := req.ClientSecret.Get(); ok {
		settings.ClientSecret, err = s.sealSecret(secret)
		if err != nil {
			return nil, err
		}
	}
	updated, err := s.store.UpdateLoginSettings(ctx, store.ActorOf(admin), settings, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	return loginSettingsHeaders(updated), nil
}

// ListAudit pages through the audit log, newest first, for administrators
// (SYSREQ-041).
func (s *Service) ListAudit(ctx context.Context, params v1gen.ListAuditParams) (*v1gen.AuditPage, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	beforeID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, err
	}
	limit := params.Limit.Or(defaultPageSize)
	entries, err := s.store.AuditEntries(ctx, beforeID, limit+1)
	if err != nil {
		return nil, err
	}
	entries, next := pageOf(entries, limit, func(e store.AuditEntry) int64 { return e.ID })
	items := make([]v1gen.AuditEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, auditEntryOf(entry))
	}
	return &v1gen.AuditPage{Items: items, NextCursor: next}, nil
}

func auditEntryOf(e store.AuditEntry) v1gen.AuditEntry {
	entry := v1gen.AuditEntry{
		ID:         e.ID,
		At:         e.At,
		Actor:      e.ActorLabel,
		Action:     e.Action,
		TargetKind: e.TargetKind,
		TargetID:   e.TargetID,
		Before:     jx.Raw(e.Before),
		After:      jx.Raw(e.After),
	}
	if e.ActorPersonID != 0 {
		entry.ActorPersonID = v1gen.NewOptInt64(e.ActorPersonID)
	}
	return entry
}
