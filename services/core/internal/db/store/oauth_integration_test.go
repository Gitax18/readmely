//go:build integration

package store_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
)

func (e *env) identity(t *testing.T, user store.User, provider, account string) store.OauthIdentity {
	t.Helper()
	id, err := e.q.CreateIdentity(ctxT(t), store.CreateIdentityParams{
		UserID: user.ID, Provider: provider, ProviderUserID: account, Email: user.Email,
	})
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}
	return id
}

func TestIdentityLookupByProviderAccount(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)
	ada := e.user(t, "ada@example.invalid")
	e.identity(t, ada, "google", "g-123")

	got, err := e.q.GetIdentityByProviderAccount(ctx, store.GetIdentityByProviderAccountParams{Provider: "google", ProviderUserID: "g-123"})
	if err != nil || got.User.ID != ada.ID || got.OauthIdentity.UserID != ada.ID {
		t.Fatalf("lookup = %+v, %v", got, err)
	}
	// The same id at another provider is a different account.
	_, err = e.q.GetIdentityByProviderAccount(ctx, store.GetIdentityByProviderAccountParams{Provider: "github", ProviderUserID: "g-123"})
	wantNoRows(t, err)
	// Lookup is by the provider's id, never by email.
	_, err = e.q.GetIdentityByProviderAccount(ctx, store.GetIdentityByProviderAccountParams{Provider: "google", ProviderUserID: "ada@example.invalid"})
	wantNoRows(t, err)
}

func TestIdentityUniqueness(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)
	ada, bob := e.user(t, "ada@example.invalid"), e.user(t, "bob@example.invalid")
	e.identity(t, ada, "google", "g-1")

	// A provider account cannot belong to two users.
	_, err := e.q.CreateIdentity(ctx, store.CreateIdentityParams{UserID: bob.ID, Provider: "google", ProviderUserID: "g-1", Email: bob.Email})
	wantCode(t, err, "23505", "oauth_identities_provider_account_key")
	// A user holds one account per provider.
	_, err = e.q.CreateIdentity(ctx, store.CreateIdentityParams{UserID: ada.ID, Provider: "google", ProviderUserID: "g-2", Email: ada.Email})
	wantCode(t, err, "23505", "oauth_identities_user_provider_key")
	// But one account at each provider is fine, and so is the same id at another provider.
	e.identity(t, ada, "github", "g-1")
}

func TestIdentityRules(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)
	ada := e.user(t, "ada@example.invalid")
	for name, p := range map[string]store.CreateIdentityParams{
		"unknown provider": {UserID: ada.ID, Provider: "facebook", ProviderUserID: "x", Email: "ada@example.invalid"},
		"empty account id": {UserID: ada.ID, Provider: "google", ProviderUserID: "", Email: "ada@example.invalid"},
		"upper-case email": {UserID: ada.ID, Provider: "google", ProviderUserID: "x", Email: "Ada@example.invalid"},
		"email without @":  {UserID: ada.ID, Provider: "google", ProviderUserID: "x", Email: "nope"},
		"long account id":  {UserID: ada.ID, Provider: "google", ProviderUserID: fmt.Sprintf("%0300d", 1), Email: "ada@example.invalid"},
	} {
		if _, err := e.q.CreateIdentity(ctx, p); err == nil {
			t.Errorf("%s was accepted", name)
		} else {
			wantCode(t, err, "23514", "")
		}
	}
	_, err := e.q.CreateIdentity(ctx, store.CreateIdentityParams{UserID: ada.ID, Provider: "google", ProviderUserID: "x", Email: "ada@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDisconnectIsScopedToTheOwner(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)
	ada, bob := e.user(t, "ada@example.invalid"), e.user(t, "bob@example.invalid")
	e.identity(t, ada, "google", "g-1")
	e.identity(t, ada, "github", "h-1")

	if n, err := e.q.DeleteUserIdentity(ctx, store.DeleteUserIdentityParams{UserID: bob.ID, Provider: "google"}); err != nil || n != 0 {
		t.Fatalf("bob disconnected ada's account: %d, %v", n, err)
	}
	list, err := e.q.ListUserIdentities(ctx, bob.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("bob sees %d identities, %v", len(list), err)
	}
	list, _ = e.q.ListUserIdentities(ctx, ada.ID)
	if len(list) != 2 || list[0].Provider != "github" || list[1].Provider != "google" {
		t.Fatalf("ada's identities = %+v", list)
	}
	if n, err := e.q.DeleteUserIdentity(ctx, store.DeleteUserIdentityParams{UserID: ada.ID, Provider: "google"}); err != nil || n != 1 {
		t.Fatalf("ada's disconnect = %d, %v", n, err)
	}
	if _, err := e.q.GetUserIdentity(ctx, store.GetUserIdentityParams{UserID: ada.ID, Provider: "google"}); err == nil {
		t.Error("the identity is still there")
	}
	// Once disconnected the provider account is free to be connected elsewhere.
	e.identity(t, bob, "google", "g-1")
}

func TestIdentitiesGoWithTheirUser(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ada := e.user(t, "ada@example.invalid")
	e.identity(t, ada, "google", "g-1")
	if _, err := e.q.DeleteUser(ctxT(t), ada.ID); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM oauth_identities`); n != 0 {
		t.Errorf("identities left after deleting the user: %d", n)
	}
}

func TestTouchIdentityLogin(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)
	ada := e.user(t, "ada@example.invalid")
	id := e.identity(t, ada, "google", "g-1")
	if err := e.q.TouchIdentityLogin(ctx, store.TouchIdentityLoginParams{ID: id.ID, Email: "ada.new@example.invalid"}); err != nil {
		t.Fatal(err)
	}
	got, err := e.q.GetUserIdentity(ctx, store.GetUserIdentityParams{UserID: ada.ID, Provider: "google"})
	if err != nil || got.Email != "ada.new@example.invalid" || !got.LastLoginAt.After(id.LastLoginAt) && !got.LastLoginAt.Equal(id.LastLoginAt) {
		t.Fatalf("after touch: %+v, %v", got, err)
	}
	// The provider's email changing does not move the account to another user.
	if got.UserID != ada.ID {
		t.Error("touch changed the owner")
	}
}

func TestUpsertVerifiedUserWithName(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)

	u, err := e.q.UpsertVerifiedUserWithName(ctx, store.UpsertVerifiedUserWithNameParams{Email: "new@example.invalid", DisplayName: "New Person"})
	if err != nil || u.DisplayName != "New Person" || u.EmailVerifiedAt == nil {
		t.Fatalf("new user = %+v, %v", u, err)
	}
	// An existing name is kept; the other spelling of the address is the same account.
	again, err := e.q.UpsertVerifiedUserWithName(ctx, store.UpsertVerifiedUserWithNameParams{Email: "New@Example.invalid", DisplayName: "Someone Else"})
	if err != nil || again.ID != u.ID || again.DisplayName != "New Person" {
		t.Fatalf("existing user = %+v, %v", again, err)
	}
	// An empty name is filled in; an unverified address becomes verified.
	bare := e.user(t, "bare@example.invalid")
	filled, err := e.q.UpsertVerifiedUserWithName(ctx, store.UpsertVerifiedUserWithNameParams{Email: "bare@example.invalid", DisplayName: "Bare"})
	if err != nil || filled.ID != bare.ID || filled.DisplayName != "Bare" || filled.EmailVerifiedAt == nil {
		t.Fatalf("filled user = %+v, %v", filled, err)
	}
	if n := e.count(t, `SELECT count(*) FROM users`); n != 2 {
		t.Errorf("users = %d, want 2", n)
	}
}

// Two people cannot claim one provider account: of 20 simultaneous connects by 20 different
// users, exactly one wins.
func TestOneProviderAccountGoesToOneUser(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	users := make([]store.User, 20)
	for i := range users {
		users[i] = e.user(t, fmt.Sprintf("user%d@example.invalid", i))
	}
	var wg sync.WaitGroup
	wins := make(chan int, len(users))
	for i := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.q.CreateIdentity(ctxT(t), store.CreateIdentityParams{UserID: users[i].ID, Provider: "github", ProviderUserID: "contested", Email: users[i].Email})
			if err == nil {
				wins <- i
			}
		}()
	}
	wg.Wait()
	close(wins)
	if n := len(wins); n != 1 {
		t.Errorf("%d users got the same provider account, want 1", n)
	}
	if n := e.count(t, `SELECT count(*) FROM oauth_identities`); n != 1 {
		t.Errorf("identities = %d, want 1", n)
	}
}

func TestIdentityTablePrivileges(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)
	ai := e.db.AsRole(t, "readmely_ai")
	if _, err := ai.Exec(ctx, `SELECT 1 FROM oauth_identities`); err == nil {
		t.Error("readmely_ai may read oauth_identities")
	} else {
		wantCode(t, err, "42501", "")
	}
	ada := e.user(t, "ada@example.invalid")
	app := e.db.AsRole(t, "readmely_app")
	if _, err := store.New(app).CreateIdentity(ctx, store.CreateIdentityParams{UserID: ada.ID, Provider: "google", ProviderUserID: "g", Email: ada.Email}); err != nil {
		t.Fatalf("readmely_app cannot connect an account: %v", err)
	}
	if _, err := app.Exec(ctx, `ALTER TABLE oauth_identities ADD COLUMN x int`); err == nil {
		t.Error("readmely_app may alter oauth_identities")
	}
}
