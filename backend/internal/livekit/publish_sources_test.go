package livekit

import (
	"context"
	"testing"
)

// TestSetPublishSourcesNarrowsTheMicrophoneAway covers the half of force-mute
// that MutePublishedTrack cannot do (#2872). Muting the live track is undone by
// the client publishing a new one a second later; taking MICROPHONE out of the
// permitted sources is what keeps it silenced. Camera and screen share must
// survive — a muted participant is still allowed to present.
func TestSetPublishSourcesNarrowsTheMicrophoneAway(t *testing.T) {
	c, got := serve(t, `{}`)
	if err := c.SetPublishSources(context.Background(), "conf_x", "u-1", SourcesWithoutMic()); err != nil {
		t.Fatalf("SetPublishSources: %v", err)
	}
	if got.Method != "UpdateParticipant" {
		t.Errorf("method = %q, want UpdateParticipant", got.Method)
	}
	perm, ok := got.Body["permission"].(map[string]any)
	if !ok {
		t.Fatalf("permission missing from %v", got.Body)
	}
	sources := sourceSet(t, perm)
	if sources[SourceMicrophone] {
		t.Error("a force-muted participant may still publish a microphone")
	}
	for _, want := range []string{SourceCamera, SourceScreenShare} {
		if !sources[want] {
			t.Errorf("force-mute also revoked %s", want)
		}
	}
	// canPublish itself must stay true. LiveKit reads an *empty* source list as
	// "everything allowed", so a blanket ban is not the safe-looking fallback it
	// appears to be — it would take the camera along with the voice.
	if perm["canPublish"] != true {
		t.Error("canPublish was revoked outright instead of narrowed")
	}
}

// TestSetPublishSourcesRestoresTheMicrophone pins the shape of lifting a
// force-mute: the whole list has to be named again, because leaving the field
// out would mean "unrestricted" only by accident of the same encoding.
func TestSetPublishSourcesRestoresTheMicrophone(t *testing.T) {
	c, got := serve(t, `{}`)
	if err := c.SetPublishSources(context.Background(), "conf_x", "u-1", AllSources()); err != nil {
		t.Fatalf("SetPublishSources: %v", err)
	}
	perm, ok := got.Body["permission"].(map[string]any)
	if !ok {
		t.Fatalf("permission missing from %v", got.Body)
	}
	if !sourceSet(t, perm)[SourceMicrophone] {
		t.Error("lifting a force-mute did not give the microphone back")
	}
}

// sourceSet reads canPublishSources out of a decoded permission object.
func sourceSet(t *testing.T, perm map[string]any) map[string]bool {
	t.Helper()
	raw, ok := perm["canPublishSources"].([]any)
	if !ok {
		t.Fatalf("canPublishSources missing from %v", perm)
	}
	out := map[string]bool{}
	for _, s := range raw {
		name, ok := s.(string)
		if !ok {
			t.Fatalf("source %v is not a string", s)
		}
		out[name] = true
	}
	return out
}
