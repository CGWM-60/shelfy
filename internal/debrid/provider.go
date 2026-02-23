package debrid

import "context"

type DebridProvider interface {
	Name() string
	AuthType() AuthType
	StartAuth(ctx context.Context) (AuthSession, error)
	PollAuth(ctx context.Context, session AuthSession) (Token, bool, error)
	ValidateToken(ctx context.Context, token Token) error
	Unrestrict(ctx context.Context, link string, opts UnrestrictOptions) (DebridResult, error)
	ListFolder(ctx context.Context, link string) (FolderInfo, error)
	GetStreamURL(ctx context.Context, fileID string) (string, error)
}

// PasswordAuthProvider is optional and implemented only by providers that
// support OAuth2 password grant.
type PasswordAuthProvider interface {
	AuthWithPassword(ctx context.Context, username, password string) (Token, error)
}
