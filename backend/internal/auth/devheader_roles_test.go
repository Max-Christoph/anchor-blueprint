package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/auth"
)

func TestDevHeaderRolesFromHeaderAndFallback(t *testing.T) {
	verifier, err := auth.NewDevHeaderVerifier(auth.DevHeaderConfig{
		AllowedEmailDomains: []string{"example.com"},
		DefaultRole:         "User",
		ModeName:            auth.ModeDevHeader,
	})
	if err != nil {
		t.Fatalf("NewDevHeaderVerifier: %v", err)
	}

	tests := []struct {
		name      string
		roles     string
		wantRoles []string
	}{
		{name: "Fallback User", roles: "", wantRoles: []string{"User"}},
		{name: "eine Rolle", roles: "Operator", wantRoles: []string{"Operator"}},
		{name: "mehrere Rollen", roles: "Admin,Operator", wantRoles: []string{"Admin", "Operator"}},
		{name: "Duplikate und Leerraum", roles: " Admin , admin ,Operator ", wantRoles: []string{"Admin", "Operator"}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			identity, err := verifier.Verify(context.Background(), auth.Credentials{
				UserEmail: "max@example.com",
				UserRoles: testCase.roles,
			})
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if len(identity.Roles) != len(testCase.wantRoles) {
				t.Fatalf("Roles = %v, erwartet %v", identity.Roles, testCase.wantRoles)
			}
			for i, want := range testCase.wantRoles {
				if identity.Roles[i] != want {
					t.Fatalf("Roles = %v, erwartet %v", identity.Roles, testCase.wantRoles)
				}
			}
		})
	}
}

func TestDevHeaderRejectsForeignDomainRegardlessOfRoles(t *testing.T) {
	verifier, err := auth.NewDevHeaderVerifier(auth.DevHeaderConfig{
		AllowedEmailDomains: []string{"example.com"},
		DefaultRole:         "User",
	})
	if err != nil {
		t.Fatalf("NewDevHeaderVerifier: %v", err)
	}

	if _, err := verifier.Verify(context.Background(), auth.Credentials{
		UserEmail: "attacker@evil.example",
		UserRoles: "Admin",
	}); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Fehler = %v, erwartet ErrInvalidToken", err)
	}
}

func TestMiddlewareForwardsRoleHeader(t *testing.T) {
	verifier, err := auth.NewDevHeaderVerifier(auth.DevHeaderConfig{
		AllowedEmailDomains: []string{"example.com"},
		DefaultRole:         "User",
	})
	if err != nil {
		t.Fatalf("NewDevHeaderVerifier: %v", err)
	}

	chain := auth.Middleware(auth.MiddlewareConfig{Verifier: verifier}, testLogger())(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, _ := identityFrom(r)
			w.Header().Set("X-Roles", joinRoles(identity.Roles))
			w.WriteHeader(http.StatusOK)
		}))

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/me", http.NoBody)
	request.Header.Set("X-User-Email", "max@example.com")
	request.Header.Set("X-User-Roles", "Admin,Operator")

	recorder := httptest.NewRecorder()
	chain.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Status = %d, erwartet 200", recorder.Code)
	}
	if roles := recorder.Header().Get("X-Roles"); roles != "Admin,Operator" {
		t.Fatalf("Rollen im Context = %q, erwartet Admin,Operator", roles)
	}
}
