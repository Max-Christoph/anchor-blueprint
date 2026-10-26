package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/auth"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
)

// withIdentity legt eine Identitaet in den Request-Context (simuliert die Auth-Middleware).
func withIdentity(request *http.Request, identity httpx.Identity) *http.Request {
	return request.WithContext(httpx.WithIdentity(request.Context(), identity))
}

func TestRequireRole(t *testing.T) {
	guard := auth.RequireRole("Admin", "Operator")(okHandler())

	tests := []struct {
		name       string
		identity   *httpx.Identity
		wantStatus int
	}{
		{name: "keine Identitaet", identity: nil, wantStatus: http.StatusUnauthorized},
		{name: "ohne Rollen", identity: &httpx.Identity{Subject: "u1"}, wantStatus: http.StatusForbidden},
		{name: "fremde Rolle", identity: &httpx.Identity{Subject: "u1", Roles: []string{"Viewer"}}, wantStatus: http.StatusForbidden},
		{name: "Rolle Operator", identity: &httpx.Identity{Subject: "u1", Roles: []string{"Operator"}}, wantStatus: http.StatusOK},
		{name: "Rolle Admin", identity: &httpx.Identity{Subject: "u1", Roles: []string{"Admin"}}, wantStatus: http.StatusOK},
		{name: "mehrere Rollen", identity: &httpx.Identity{Subject: "u1", Roles: []string{"Viewer", "Operator"}}, wantStatus: http.StatusOK},
		{name: "Schreibweise egal", identity: &httpx.Identity{Subject: "u1", Roles: []string{"operator"}}, wantStatus: http.StatusOK},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/protected/operator", http.NoBody)
			if testCase.identity != nil {
				request = withIdentity(request, *testCase.identity)
			}

			recorder := httptest.NewRecorder()
			guard.ServeHTTP(recorder, request)

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("Status = %d, erwartet %d", recorder.Code, testCase.wantStatus)
			}
			if recorder.Code == http.StatusForbidden && recorder.Body.Len() == 0 {
				t.Error("403 ohne Fehlerkoerper")
			}
		})
	}
}

func TestRequireGroup(t *testing.T) {
	guard := auth.RequireGroup("group-platform")(okHandler())

	tests := []struct {
		name       string
		identity   httpx.Identity
		wantStatus int
	}{
		{name: "Gruppe vorhanden", identity: httpx.Identity{Subject: "u1", Groups: []string{"group-platform"}}, wantStatus: http.StatusOK},
		{name: "andere Gruppe", identity: httpx.Identity{Subject: "u1", Groups: []string{"group-other"}}, wantStatus: http.StatusForbidden},
		{name: "keine Gruppe", identity: httpx.Identity{Subject: "u1"}, wantStatus: http.StatusForbidden},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := withIdentity(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", http.NoBody), testCase.identity)

			recorder := httptest.NewRecorder()
			guard.ServeHTTP(recorder, request)

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("Status = %d, erwartet %d", recorder.Code, testCase.wantStatus)
			}
		})
	}
}

func TestIdentityRoleHelpers(t *testing.T) {
	identity := httpx.Identity{Roles: []string{"Admin", " operator "}, Groups: []string{"g1"}}

	if !identity.HasRole("admin") {
		t.Error("HasRole soll Gross-/Kleinschreibung ignorieren")
	}
	if !identity.HasRole("Operator") {
		t.Error("HasRole soll Whitespace trimmen")
	}
	if identity.HasRole("Viewer") {
		t.Error("HasRole darf fremde Rollen nicht akzeptieren")
	}
	if !identity.HasAnyRole("Viewer", "Admin") {
		t.Error("HasAnyRole soll Treffer liefern")
	}
	if !identity.HasGroup("G1") {
		t.Error("HasGroup soll Gross-/Kleinschreibung ignorieren")
	}
	if identity.HasAnyGroup("g2", "g3") {
		t.Error("HasAnyGroup darf ohne Treffer false liefern")
	}
}
