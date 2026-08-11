package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"golang.org/x/oauth2"
)

// deviceChannel builds a device channel carrying (or missing) a push token.
func deviceChannel(token string) Channel {
	cfg := map[string]any{"device_id": "dev-1", "platform": "android"}
	if token != "" {
		cfg["fcm_token"] = token
	}
	return Channel{Type: "device", Config: cfg}
}

// fcmAgainst points a sender at a stub FCM server with a canned access token,
// so no service-account key or network is involved.
func fcmAgainst(srv *httptest.Server) *FCMSender {
	return &FCMSender{
		ProjectID: "test-project",
		Tokens:    oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "stub", TokenType: "Bearer"}),
		Client:    srv.Client(),
		Endpoint:  srv.URL,
	}
}

// fcmStub serves a fixed status/body and records the last request.
func fcmStub(status int, body string, hits *int32, auth, payload *string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		*auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		*payload = string(b)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// TestFCMSendDelivers is the happy path, and pins the payload shape: data-only
// (a "notification" block would be drawn by the system, bypassing our dedup and
// deep link), HIGH android priority, and every data value a string — FCM rejects
// the whole message with 400 otherwise.
func TestFCMSendDelivers(t *testing.T) {
	var hits int32
	var auth, payload string
	srv := fcmStub(http.StatusOK, `{"name":"projects/p/messages/1"}`, &hits, &auth, &payload)
	defer srv.Close()

	err := fcmAgainst(srv).Send(context.Background(), deviceChannel("tok-abc"), Message{
		Kind: "assigned", Title: "Назначена задача", Body: "B",
		Link: "https://app/x", ID: "n-1", TaskID: "t-1",
	})
	if err != nil {
		t.Fatalf("Send = %v, want nil", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
	if auth != "Bearer stub" {
		t.Fatalf("Authorization = %q", auth)
	}

	var got struct {
		Message struct {
			Token        string            `json:"token"`
			Android      map[string]any    `json:"android"`
			Data         map[string]string `json:"data"`
			Notification json.RawMessage   `json:"notification"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("payload %q: %v", payload, err)
	}
	if got.Message.Token != "tok-abc" {
		t.Fatalf("token = %q", got.Message.Token)
	}
	if len(got.Message.Notification) != 0 {
		t.Fatalf("payload must stay data-only, got notification block %s", got.Message.Notification)
	}
	if got.Message.Android["priority"] != "HIGH" {
		t.Fatalf("android.priority = %v, want HIGH", got.Message.Android["priority"])
	}
	for k, want := range map[string]string{
		"notification_id": "n-1", "kind": "assigned",
		"title": "Назначена задача", "body": "B", "task_id": "t-1", "link": "https://app/x",
	} {
		if got.Message.Data[k] != want {
			t.Fatalf("data[%q] = %q, want %q", k, got.Message.Data[k], want)
		}
	}
}

// TestFCMSendOmitsAbsentTask: a notification not tied to a task carries no
// task_id key at all (an empty string would deep-link the client to nowhere).
func TestFCMSendOmitsAbsentTask(t *testing.T) {
	var hits int32
	var auth, payload string
	srv := fcmStub(http.StatusOK, "{}", &hits, &auth, &payload)
	defer srv.Close()

	if err := fcmAgainst(srv).Send(context.Background(), deviceChannel("tok"), Message{Kind: "reminder", ID: "n-2"}); err != nil {
		t.Fatalf("Send = %v", err)
	}
	var got struct {
		Message struct {
			Data map[string]string `json:"data"`
		} `json:"message"`
	}
	_ = json.Unmarshal([]byte(payload), &got)
	if _, ok := got.Message.Data["task_id"]; ok {
		t.Fatalf("task_id should be absent, data = %v", got.Message.Data)
	}
}

// TestFCMSendNoToken: a device channel that never registered a push token (a
// browser, or a phone without Play Services) fails permanently and without a
// network call — it lives on the live-socket path only.
func TestFCMSendNoToken(t *testing.T) {
	var hits int32
	var auth, payload string
	srv := fcmStub(http.StatusOK, "{}", &hits, &auth, &payload)
	defer srv.Close()

	err := fcmAgainst(srv).Send(context.Background(), deviceChannel(""), Message{ID: "n-1"})
	if !IsPermanent(err) {
		t.Fatalf("Send = %v, want permanent", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("hits = %d, want 0 (no request without a token)", hits)
	}
}

// TestFCMSendClassifiesErrors covers the retry contract: a dead token is
// permanent *and* tagged so the caller can drop it, bad credentials/payloads are
// permanent, rate limits and FCM outages are transient.
func TestFCMSendClassifiesErrors(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		permanent    bool
		unregistered bool
	}{
		{
			name:   "unregistered token",
			status: http.StatusNotFound,
			body: `{"error":{"code":404,"status":"NOT_FOUND","details":[
				{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`,
			permanent: true, unregistered: true,
		},
		{
			name:   "sender id mismatch",
			status: http.StatusForbidden,
			body: `{"error":{"code":403,"status":"PERMISSION_DENIED","details":[
				{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"SENDER_ID_MISMATCH"}]}}`,
			permanent: true, unregistered: true,
		},
		{
			name:      "404 without a parseable code is still a dead token",
			status:    http.StatusNotFound,
			body:      `not json`,
			permanent: true, unregistered: true,
		},
		{
			name:      "invalid argument",
			status:    http.StatusBadRequest,
			body:      `{"error":{"status":"INVALID_ARGUMENT","message":"bad data"}}`,
			permanent: true,
		},
		{
			name:      "revoked key",
			status:    http.StatusUnauthorized,
			body:      `{"error":{"status":"UNAUTHENTICATED"}}`,
			permanent: true,
		},
		{
			name:   "rate limited",
			status: http.StatusTooManyRequests,
			body:   `{"error":{"status":"RESOURCE_EXHAUSTED"}}`,
		},
		{
			name:   "fcm outage",
			status: http.StatusServiceUnavailable,
			body:   `{"error":{"status":"UNAVAILABLE"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits int32
			var auth, payload string
			srv := fcmStub(tc.status, tc.body, &hits, &auth, &payload)
			defer srv.Close()

			err := fcmAgainst(srv).Send(context.Background(), deviceChannel("tok"), Message{ID: "n-1"})
			if err == nil {
				t.Fatal("Send = nil, want an error")
			}
			if got := IsPermanent(err); got != tc.permanent {
				t.Fatalf("IsPermanent = %v, want %v (err: %v)", got, tc.permanent, err)
			}
			if got := errors.Is(err, ErrPushUnregistered); got != tc.unregistered {
				t.Fatalf("Is(ErrPushUnregistered) = %v, want %v (err: %v)", got, tc.unregistered, err)
			}
		})
	}
}

// TestNewFCMSenderRejectsBadKey: an unusable key is reported at wiring time, so
// the server logs a warning and stays on the WS-only path instead of failing
// every delivery later.
func TestNewFCMSenderRejectsBadKey(t *testing.T) {
	for _, raw := range []string{
		`not json`,
		`{}`,
		`{"project_id":"p"}`, // no credentials
		`{"client_email":"a@b","private_key":"-----BEGIN..."}`, // no project
	} {
		if _, err := NewFCMSender([]byte(raw)); err == nil {
			t.Fatalf("NewFCMSender(%q) = nil error, want failure", raw)
		}
	}
}

// TestFCMEndpoint: the send URL is scoped to the project taken from the key, so
// no separate project-id env can drift out of sync with it.
func TestFCMEndpoint(t *testing.T) {
	s := &FCMSender{ProjectID: "tessera-42"}
	want := "https://fcm.googleapis.com/v1/projects/tessera-42/messages:send"
	if got := s.endpoint(); got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
}
