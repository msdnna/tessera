package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
)

// oauthConfigView is the admin-facing OAuth config. The client secret is never
// returned; has_secret only reports whether one is stored.
type oauthConfigView struct {
	Provider        string          `json:"provider"`
	ClientID        string          `json:"client_id"`
	GlBaseURL       string          `json:"gl_base_url"`
	Enabled         bool            `json:"enabled"`
	OrgMap          json.RawMessage `json:"org_map"`
	HasSecret       bool            `json:"has_secret"`
	HasServiceToken bool            `json:"has_service_token"`
}

// GetOAuthConfig returns the GitLab OAuth app config for the admin panel.
func (h *API) GetOAuthConfig(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	p, err := h.q.GetOAuthProvider(c, "gitlab")
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusOK, oauthConfigView{Provider: "gitlab", OrgMap: json.RawMessage("{}")})
		return
	}
	if err != nil {
		fail(c)
		return
	}
	om := p.OrgMap
	if len(om) == 0 {
		om = []byte("{}")
	}
	c.JSON(http.StatusOK, oauthConfigView{
		Provider: "gitlab", ClientID: p.ClientID, GlBaseURL: p.GlBaseUrl,
		Enabled: p.Enabled, OrgMap: om, HasSecret: p.ClientSecretEnc != "",
		HasServiceToken: p.ServiceTokenEnc != "",
	})
}

// SetOAuthConfig upserts the GitLab OAuth app config. An empty client_secret keeps
// the stored one (so the secret needn't be re-entered on every edit).
func (h *API) SetOAuthConfig(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	var req struct {
		ClientID     string          `json:"client_id"`
		ClientSecret string          `json:"client_secret"`
		GlBaseURL    string          `json:"gl_base_url"`
		Enabled      bool            `json:"enabled"`
		OrgMap       json.RawMessage `json:"org_map"`
		ServiceToken string          `json:"service_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Keep the existing encrypted secret/service-token when none is supplied.
	secretEnc, serviceEnc := "", ""
	if existing, err := h.q.GetOAuthProvider(c, "gitlab"); err == nil {
		secretEnc = existing.ClientSecretEnc
		serviceEnc = existing.ServiceTokenEnc
	}
	if s := strings.TrimSpace(req.ClientSecret); s != "" {
		enc, err := h.sealer.Encrypt(s)
		if err != nil {
			fail(c)
			return
		}
		secretEnc = enc
	}
	if s := strings.TrimSpace(req.ServiceToken); s != "" {
		enc, err := h.sealer.Encrypt(s)
		if err != nil {
			fail(c)
			return
		}
		serviceEnc = enc
	}

	orgMap := req.OrgMap
	if len(orgMap) == 0 || string(orgMap) == "null" {
		orgMap = []byte("{}")
	}
	// Validate org_map shape (map of group → {workspace_id, admins, users}).
	var probe map[string]orgMapEntry
	if err := json.Unmarshal(orgMap, &probe); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "org_map: invalid JSON shape"})
		return
	}

	p, err := h.q.UpsertOAuthProvider(c, db.UpsertOAuthProviderParams{
		Provider:        "gitlab",
		ClientID:        strings.TrimSpace(req.ClientID),
		ClientSecretEnc: secretEnc,
		GlBaseUrl:       strings.TrimRight(strings.TrimSpace(req.GlBaseURL), "/"),
		Enabled:         req.Enabled,
		OrgMap:          orgMap,
		ServiceTokenEnc: serviceEnc,
	})
	if err != nil {
		fail(c)
		return
	}
	om := p.OrgMap
	if len(om) == 0 {
		om = []byte("{}")
	}
	c.JSON(http.StatusOK, oauthConfigView{
		Provider: "gitlab", ClientID: p.ClientID, GlBaseURL: p.GlBaseUrl,
		Enabled: p.Enabled, OrgMap: om, HasSecret: p.ClientSecretEnc != "",
		HasServiceToken: p.ServiceTokenEnc != "",
	})
}
