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
	base := strings.TrimRight(baseURL, "/")
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := NewHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gitlab token exchange: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("gitlab token exchange: %s %s", tok.Error, tok.ErrorDesc)
	}
	return tok.AccessToken, nil
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

// FetchAvatar downloads a user's avatar image (GitLab-hosted or gravatar). A
// relative/instance URL is resolved against baseURL; the access token is sent only
// to the GitLab instance host (never to gravatar/external). Returns content-type + bytes.
func FetchAvatar(ctx context.Context, baseURL, avatarURL, accessToken string) (string, []byte, error) {
	if avatarURL == "" {
		return "", nil, fmt.Errorf("no avatar url")
	}
	u := avatarURL
	if !strings.HasPrefix(u, "http") {
		u = strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(avatarURL, "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", nil, err
	}
	if gb, e := url.Parse(strings.TrimRight(baseURL, "/")); e == nil && accessToken != "" {
		if tu, e2 := url.Parse(u); e2 == nil && tu.Host == gb.Host {
			req.Header.Set("Authorization", "Bearer "+accessToken)
		}
	}
	resp, err := NewHTTPClient().Do(req)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("avatar http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", nil, err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" || !strings.HasPrefix(ct, "image/") {
		ct = "image/png"
	}
	return ct, data, nil
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
