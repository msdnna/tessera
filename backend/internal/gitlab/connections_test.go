package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// An OAuth access token (#3083) rides REST calls as a Bearer header — GitLab
// refuses it as PRIVATE-TOKEN — while a PAT keeps PRIVATE-TOKEN; GraphQL takes
// Bearer for both. The mark that tells them apart never reaches GitLab.
func TestOAuthTokenAuth(t *testing.T) {
	var bearer, private string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer, private = r.Header.Get("Authorization"), r.Header.Get("PRIVATE-TOKEN")
		_, _ = fmt.Fprint(w, `{"expires_at":"2027-01-12"}`)
	}))
	t.Cleanup(srv.Close)

	if _, err := New(srv.URL, MarkOAuthToken("acc")).restGet(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if bearer != "Bearer acc" || private != "" {
		t.Fatalf("oauth REST auth: Authorization=%q PRIVATE-TOKEN=%q", bearer, private)
	}
	exp, err := New(srv.URL, "glpat").TokenExpiry(context.Background())
	if err != nil || private != "glpat" || bearer != "" {
		t.Fatalf("PAT REST auth: %v Authorization=%q PRIVATE-TOKEN=%q", err, bearer, private)
	}
	if exp == nil || exp.Format("2006-01-02") != "2027-01-12" {
		t.Fatalf("expiry = %v", exp)
	}
	if exp, err := New(srv.URL, MarkOAuthToken("acc")).TokenExpiry(context.Background()); exp != nil || err != nil {
		t.Fatalf("an OAuth token has no PAT expiry: %v %v", exp, err)
	}
}

// A refused credential reads as unauthorized on both surfaces (#3084).
func TestIsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "dead")
	if _, err := c.CurrentUser(context.Background()); !IsUnauthorized(err) {
		t.Fatalf("GraphQL 401: %v", err)
	}
	if _, err := c.restGet(context.Background(), "x"); !IsUnauthorized(err) {
		t.Fatalf("REST 401: %v", err)
	}
	if IsUnauthorized(&APIError{Status: http.StatusForbidden}) {
		t.Fatal("403 is not a dead credential")
	}
}

// Refreshing a grant (#3083) posts the refresh token and keeps the rotated pair;
// a spent refresh token reads as unauthorized, so the binding pauses.
func TestRefreshOAuthGrant(t *testing.T) {
	var form url.Values
	spent := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		if spent {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"access_token":"a2","refresh_token":"r2","expires_in":7200,"created_at":1760000000}`)
	}))
	t.Cleanup(srv.Close)

	g, err := RefreshOAuthGrant(context.Background(), srv.URL, "id", "sec", "r1", "https://t/cb")
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "r1" || form.Get("redirect_uri") != "https://t/cb" {
		t.Fatalf("form = %v", form)
	}
	want := time.Unix(1760000000, 0).Add(2 * time.Hour)
	if g.AccessToken != "a2" || g.RefreshToken != "r2" || g.ExpiresAt == nil || !g.ExpiresAt.Equal(want) {
		t.Fatalf("grant = %+v", g)
	}
	spent = true
	if _, err := RefreshOAuthGrant(context.Background(), srv.URL, "id", "sec", "r1", "https://t/cb"); !IsUnauthorized(err) {
		t.Fatalf("spent refresh token: %v", err)
	}
}
