package auth_test

import (
	"net/http"
	"strings"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
)

// identityFrom liest die Identitaet im Testhandler.
func identityFrom(r *http.Request) (httpx.Identity, bool) {
	return httpx.IdentityFrom(r.Context())
}

// joinRoles verbindet Rollen fuer Header-Assertions.
func joinRoles(roles []string) string {
	return strings.Join(roles, ",")
}
