package application

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// fakeRepo is an in-memory Repository used by pathfinder/service tests.
type fakeRepo struct {
	contacts    map[string]sqlcgen.Contact
	connections []sqlcgen.ContactConnection
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{contacts: map[string]sqlcgen.Contact{}}
}

func newUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}

func (f *fakeRepo) addContact(name, company string) sqlcgen.Contact {
	id := newUUID()
	var companyPtr *string
	if company != "" {
		companyPtr = &company
	}
	c := sqlcgen.Contact{ID: id, Name: name, Company: companyPtr, Source: "csv"}
	f.contacts[idKey(id)] = c
	return c
}

func (f *fakeRepo) connect(a, b sqlcgen.Contact, strength float32) {
	f.connections = append(f.connections, sqlcgen.ContactConnection{
		ID:               newUUID(),
		FromContactId:    a.ID,
		ToContactId:      b.ID,
		RelationshipType: "professional",
		Strength:         strength,
	})
}

func idKey(id pgtype.UUID) string {
	return string(id.Bytes[:])
}

func (f *fakeRepo) InsertContact(ctx context.Context, arg sqlcgen.InsertContactParams) (sqlcgen.Contact, error) {
	c := sqlcgen.Contact{ID: newUUID(), Name: arg.Name, Email: arg.Email, Company: arg.Company, Role: arg.Role,
		LinkedinUrl: arg.LinkedinUrl, GithubUsername: arg.GithubUsername, Source: arg.Source}
	f.contacts[idKey(c.ID)] = c
	return c, nil
}

func (f *fakeRepo) GetContactByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Contact, error) {
	c, ok := f.contacts[idKey(id)]
	if !ok {
		return sqlcgen.Contact{}, errNotFound
	}
	return c, nil
}

func (f *fakeRepo) ListContacts(ctx context.Context) ([]sqlcgen.Contact, error) {
	out := make([]sqlcgen.Contact, 0, len(f.contacts))
	for _, c := range f.contacts {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeRepo) ListContactsByCompany(ctx context.Context, company *string) ([]sqlcgen.Contact, error) {
	return f.FindContactsAtCompany(ctx, company)
}

func (f *fakeRepo) FindContactsByGithubUsername(ctx context.Context, githubUsername *string) ([]sqlcgen.Contact, error) {
	var out []sqlcgen.Contact
	for _, c := range f.contacts {
		if c.GithubUsername != nil && githubUsername != nil && *c.GithubUsername == *githubUsername {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeRepo) DeleteAllContacts(ctx context.Context) (int64, error) {
	n := int64(len(f.contacts))
	f.contacts = map[string]sqlcgen.Contact{}
	return n, nil
}

func (f *fakeRepo) InsertContactConnection(ctx context.Context, arg sqlcgen.InsertContactConnectionParams) (sqlcgen.ContactConnection, error) {
	conn := sqlcgen.ContactConnection{ID: newUUID(), FromContactId: arg.FromContactId, ToContactId: arg.ToContactId,
		RelationshipType: arg.RelationshipType, Strength: arg.Strength}
	f.connections = append(f.connections, conn)
	return conn, nil
}

func (f *fakeRepo) ListConnections(ctx context.Context) ([]sqlcgen.ContactConnection, error) {
	return f.connections, nil
}

func (f *fakeRepo) GetConnectionsFrom(ctx context.Context, fromContactID pgtype.UUID) ([]sqlcgen.ContactConnection, error) {
	var out []sqlcgen.ContactConnection
	for _, c := range f.connections {
		if c.FromContactId == fromContactID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeRepo) GetConnectionsTo(ctx context.Context, toContactID pgtype.UUID) ([]sqlcgen.ContactConnection, error) {
	var out []sqlcgen.ContactConnection
	for _, c := range f.connections {
		if c.ToContactId == toContactID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeRepo) DeleteAllConnections(ctx context.Context) (int64, error) {
	n := int64(len(f.connections))
	f.connections = nil
	return n, nil
}

func (f *fakeRepo) FindContactsAtCompany(ctx context.Context, company *string) ([]sqlcgen.Contact, error) {
	var out []sqlcgen.Contact
	if company == nil {
		return out, nil
	}
	for _, c := range f.contacts {
		if c.Company != nil && *c.Company == *company {
			out = append(out, c)
		}
	}
	return out, nil
}

type notFoundError struct{}

func (notFoundError) Error() string { return "not found" }

var errNotFound = notFoundError{}

func TestPathFinder_FindPathsToCompany_DirectHop(t *testing.T) {
	repo := newFakeRepo()
	me := repo.addContact("Me", "")
	target := repo.addContact("Target", "Acme")
	repo.connect(me, target, 0.9)

	pf := NewPathFinder(repo)
	paths, err := pf.FindPathsToCompany(context.Background(), "Acme", 3)
	if err != nil {
		t.Fatalf("FindPathsToCompany() error = %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("got %d paths, want 1", len(paths))
	}
	if paths[0].Length != 2 {
		t.Errorf("path length = %d, want 2 (me -> target)", paths[0].Length)
	}
	if paths[0].Score <= 0 {
		t.Errorf("path score = %v, want > 0", paths[0].Score)
	}
}

func TestPathFinder_FindPathsToCompany_MultiHop(t *testing.T) {
	repo := newFakeRepo()
	me := repo.addContact("Me", "")
	bridge := repo.addContact("Bridge", "")
	target := repo.addContact("Target", "Acme")
	repo.connect(me, bridge, 0.6)
	repo.connect(bridge, target, 0.8)

	pf := NewPathFinder(repo)
	paths, err := pf.FindPathsToCompany(context.Background(), "Acme", 3)
	if err != nil {
		t.Fatalf("FindPathsToCompany() error = %v", err)
	}
	// The finder walks from every non-target contact, so both "bridge"
	// (direct hop) and "me" (via bridge) produce a path into Acme.
	if len(paths) != 2 {
		t.Fatalf("got %d paths, want 2 (from Me and from Bridge)", len(paths))
	}

	var sawThreeHop bool
	for _, p := range paths {
		if p.Length == 3 {
			sawThreeHop = true
		}
	}
	if !sawThreeHop {
		t.Errorf("no 3-hop path found in %+v, want Me -> Bridge -> Target", paths)
	}
}

func TestPathFinder_FindPathsToCompany_NoTargetCompany(t *testing.T) {
	repo := newFakeRepo()
	repo.addContact("Me", "")

	pf := NewPathFinder(repo)
	paths, err := pf.FindPathsToCompany(context.Background(), "Nonexistent", 3)
	if err != nil {
		t.Fatalf("FindPathsToCompany() error = %v", err)
	}
	if paths != nil {
		t.Errorf("got %+v, want nil (no contacts at that company)", paths)
	}
}

func TestPathFinder_FindPathsToCompany_RespectsMaxDepth(t *testing.T) {
	repo := newFakeRepo()
	me := repo.addContact("Me", "")
	a := repo.addContact("A", "")
	b := repo.addContact("B", "")
	target := repo.addContact("Target", "Acme")
	repo.connect(me, a, 0.5)
	repo.connect(a, b, 0.5)
	repo.connect(b, target, 0.5)

	pf := NewPathFinder(repo)
	// me -> a -> b -> target is 4 nodes; with maxDepth 2 that full path must
	// not be found (only the shorter a->b->target and b->target paths do).
	paths, err := pf.FindPathsToCompany(context.Background(), "Acme", 2)
	if err != nil {
		t.Fatalf("FindPathsToCompany() error = %v", err)
	}
	for _, p := range paths {
		if p.Length >= 4 {
			t.Errorf("path %+v has length %d, want < 4 under maxDepth=2", p, p.Length)
		}
	}
	if len(paths) != 2 {
		t.Fatalf("got %d paths within maxDepth=2, want 2 (from A and from B)", len(paths))
	}
}
