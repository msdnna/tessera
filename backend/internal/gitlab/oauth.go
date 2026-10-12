package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// OAuthUserInfo is the subset of GitLab's /user we need to provision an account.
type OAuthUserInfo struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// ExchangeOAuthCode swaps an authorization code for an access token against a
// self-hosted GitLab's /oauth/token (confidential app: client_id + client_secret,
// no PKCE). Returns the access token.
func ExchangeOAuthCode(ctx context.Context, baseURL, clientID, clientSecret, code, redirectURI string) (string, error) {
	g, err := ExchangeOAuthGrant(ctx, baseURL, clientID, clientSecret, code, redirectURI)
	return g.AccessToken, err
}

// OAuthGrant is what /oauth/token hands out: the access token, the refresh token
// that renews it, and how long the access token lives (GitLab: two hours).
type OAuthGrant struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    *time.Time // nil when GitLab names no lifetime
}

// ExchangeOAuthGrant is ExchangeOAuthCode keeping the whole grant — what a GitLab
// connection made through OAuth (#3083) stores to keep itself alive.
func ExchangeOAuthGrant(ctx context.Context, baseURL, clientID, clientSecret, code, redirectURI string) (OAuthGrant, error) {
	return oauthToken(ctx, baseURL, url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	})
}

// RefreshOAuthGrant renews an access token with its refresh token. GitLab rotates
// the refresh token too: the old one is spent, the grant carries the new pair.
func RefreshOAuthGrant(ctx context.Context, baseURL, clientID, clientSecret, refreshToken, redirectURI string) (OAuthGrant, error) {
	return oauthToken(ctx, baseURL, url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
		"redirect_uri":  {redirectURI},
	})
}

// oauthToken posts one /oauth/token request.
func oauthToken(ctx context.Context, baseURL string, form url.Values) (OAuthGrant, error) {
	base := strings.TrimRight(baseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return OAuthGrant{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := NewHTTPClient().Do(req)
	if err != nil {
		return OAuthGrant{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadRequest {
		// invalid_grant: the refresh token was revoked or already spent.
		return OAuthGrant{}, fmt.Errorf("%w: %s", ErrUnauthorized, strings.TrimSpace(string(body)))
	}
	if resp.StatusCode != http.StatusOK {
		return OAuthGrant{}, fmt.Errorf("gitlab token exchange: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		CreatedAt    int64  `json:"created_at"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return OAuthGrant{}, err
	}
	if tok.AccessToken == "" {
		return OAuthGrant{}, fmt.Errorf("gitlab token exchange: %s %s", tok.Error, tok.ErrorDesc)
	}
	g := OAuthGrant{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken}
	if tok.ExpiresIn > 0 {
		from := time.Now()
		if tok.CreatedAt > 0 {
			from = time.Unix(tok.CreatedAt, 0)
		}
		at := from.Add(time.Duration(tok.ExpiresIn) * time.Second)
		g.ExpiresAt = &at
	}
	return g, nil
}

// OAuthUser fetches the authenticated GitLab user with an OAuth access token.
func OAuthUser(ctx context.Context, baseURL, accessToken string) (OAuthUserInfo, error) {
	base := strings.TrimRight(baseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v4/user", nil)
	if err != nil {
		return OAuthUserInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := NewHTTPClient().Do(req)
	if err != nil {
		return OAuthUserInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return OAuthUserInfo{}, fmt.Errorf("gitlab /user: %s", resp.Status)
	}
	var u OAuthUserInfo
	if err := json.Unmarshal(body, &u); err != nil {
		return OAuthUserInfo{}, err
	}
	return u, nil
}

// OAuthUserGroupPaths returns the set of GitLab group full-paths the user belongs
// to (any access level), used to match the org_map. Paginated; capped defensively.
func OAuthUserGroupPaths(ctx context.Context, baseURL, accessToken string) (map[string]bool, error) {
	base := strings.TrimRight(baseURL, "/")
	out := map[string]bool{}
	client := NewHTTPClient()
	for page := 1; page <= 20; page++ {
		u := base + "/api/v4/groups?per_page=100&min_access_level=10&page=" + strconv.Itoa(page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return out, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := client.Do(req)
		if err != nil {
			return out, err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		next := resp.Header.Get("X-Next-Page")
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return out, fmt.Errorf("gitlab /groups: %s", resp.Status)
		}
		var groups []struct {
			FullPath string `json:"full_path"`
		}
		if err := json.Unmarshal(body, &groups); err != nil {
			return out, err
		}
		for _, g := range groups {
			if g.FullPath != "" {
				out[g.FullPath] = true
			}
		}
		if next == "" || len(groups) == 0 {
			break
		}
	}
	return out, nil
}
