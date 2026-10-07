package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tessera/config"
	"tessera/internal/mail"
	"tessera/internal/realtime"
)

// TestMultipartRoutesHaveBodyBudget is the ratchet behind TestDocumentPdfBudgetSurvivesRealPdf and
// TestConferenceChatBudgetSurvivesRealAttachment (limits_flow_test.go).
//
// TestBodyLimitRoutesExist checks one direction — every constant in the limits
// table names a real route. Nothing checked the other, and that is the direction
// the bug travels in: a new upload handler is written, its route is registered,
// and the blanket 1 MiB silently becomes its ceiling. Both routes fixed here
// were added that way, a year apart from the two #2733 found.
//
// So: find every handler that reads a multipart body, walk the registered routes
// to see which paths reach it, and require each of those paths to carry a budget
// of its own. The call graph is needed because the read is often one helper deep
// (PostConferenceMessage → readChatSubmission → c.MultipartForm).
func TestMultipartRoutesHaveBodyBudget(t *testing.T) {
	multipart := multipartHandlers(t)
	if len(multipart) == 0 {
		t.Fatal("found no multipart handlers at all — the AST scan is broken, not the router")
	}

	cfg := &config.Config{
		JWTSecret:     "integration-test-secret-min32chars!!",
		UploadDir:     testUploadDir,
		EncryptionKey: "integration-test-encryption-key",
		CORSOrigin:    "*",
	}
	hub := realtime.NewHub()
	go hub.Run()
	r, _ := newRouter(cfg, testQueries, testPool, hub, mail.New(mail.Config{}))

	budgeted := bodyLimitRoutes(cfg)
	for _, route := range r.Routes() {
		name := handlerFuncName(route.Handler)
		if !multipart[name] {
			continue
		}
		if _, ok := budgeted[route.Path]; !ok {
			t.Errorf("%s %s reads a multipart body through %s but has no entry in the body-limit table, "+
				"so it is capped at the blanket JSON budget", route.Method, route.Path, name)
		}
	}
}

// handlerFuncName reduces gin's fully qualified handler name
// ("tessera/handlers.(*API).UploadMedia-fm") to the method name.
func handlerFuncName(qualified string) string {
	name := strings.TrimSuffix(qualified, "-fm")
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// multipartHandlers parses package handlers and returns the set of function
// names that read a multipart body, directly or through another function in the
// same package.
func multipartHandlers(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir("handlers")
	if err != nil {
		t.Fatalf("read handlers: %v", err)
	}
	fset := token.NewFileSet()

	// calls maps a function name to the names it calls; direct holds the ones
	// that read the body themselves.
	calls := map[string][]string{}
	direct := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join("handlers", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch sel.Sel.Name {
				case "FormFile", "MultipartForm":
					direct[fn.Name.Name] = true
				default:
					calls[fn.Name.Name] = append(calls[fn.Name.Name], sel.Sel.Name)
				}
				return true
			})
		}
	}

	// Close over the call graph: a handler that calls a reader is a reader.
	reaches := map[string]bool{}
	for name := range direct {
		reaches[name] = true
	}
	for changed := true; changed; {
		changed = false
		for caller, callees := range calls {
			if reaches[caller] {
				continue
			}
			for _, callee := range callees {
				if reaches[callee] {
					reaches[caller] = true
					changed = true
					break
				}
			}
		}
	}
	return reaches
}
