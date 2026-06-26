// Package app defines the shared dependency container passed to every HTTP
// module. Modules depend only on this struct (and leaf packages like store /
// models), which keeps wiring mechanical and avoids import cycles.
package app

import (
	"log/slog"

	"github.com/fleet-terminal/backend/internal/auth"
	"github.com/fleet-terminal/backend/internal/config"
	"github.com/fleet-terminal/backend/internal/store"
)

// Deps is the application's shared service container.
type Deps struct {
	Store *store.Store
	Cfg   *config.Config
	Log   *slog.Logger
	Auth  *auth.Service

	// SSH services are populated once the gateway/CA are constructed. Modules
	// that need them (terminal, sftp, enrollment, monitor) read these fields.
	CA      CAIssuer
	Gateway Dialer
}

// CAIssuer issues and manages ephemeral SSH user certificates. The concrete
// implementation lives in internal/ca + internal/identity.
type CAIssuer interface {
	// IssueForSession mints an ephemeral keypair + signed user certificate bound
	// to a browser session, returning an opaque handle id for later lookup.
	IssueForSession(sessionID, userID, username string, principals []string) (handle string, err error)
}

// Dialer opens an SSH connection to a managed host through the jump host. The
// concrete implementation lives in internal/sshgw.
type Dialer interface {
	// DialHost establishes an SSH client connection to host:port via the jump
	// host using the session's ephemeral credentials referenced by handle.
	DialHost(handle, host string, port int, user string) (any, error)
}
