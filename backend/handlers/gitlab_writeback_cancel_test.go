package handlers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
)

// A push whose lookups fail only because the worker's ctx ended must stay
// retryable (#3103): the row is fine, the process is going away. Burying it as
// "integration gone" lost the change and wrote a false «ошибка» into the push
// journal on every deploy mid-drain — and made TestGitlabWritebackPushFlow flaky,
// since its drain helper cancels the worker it spawned.
func TestPermanentLookupKeepsCancellationRetryable(t *testing.T) {
	for name, err := range map[string]error{
		"canceled":         context.Canceled,
		"deadline":         context.DeadlineExceeded,
		"wrapped canceled": fmt.Errorf("query: %w", context.Canceled),
	} {
		got := permanentLookup("integration gone", err)
		if isPermanentWriteback(got) {
			t.Errorf("%s: classified permanent: %v", name, got)
		}
		if !errors.Is(got, context.Canceled) && !errors.Is(got, context.DeadlineExceeded) {
			t.Errorf("%s: lost the ctx error: %v", name, got)
		}
	}
	// A row that really is gone stays permanent — retrying it can never help.
	if got := permanentLookup("integration gone", pgx.ErrNoRows); !isPermanentWriteback(got) {
		t.Errorf("no rows should be permanent: %v", got)
	}
}
