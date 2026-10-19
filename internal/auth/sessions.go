package auth

import "context"

// Resolve returns who holds a session token, if it is still alive and its PIN need not change.
func (s *Service) Resolve(ctx context.Context, token Token) (Identity, error) {
	who, err := s.ResolveForPINChange(ctx, token)
	if err != nil {
		return Identity{}, err
	}

	if who.MustChangePIN {
		return Identity{}, ErrMustChangePIN
	}

	return who, nil
}

// ResolveForPINChange is Resolve for the PIN-change page alone: it also accepts a session
// whose PIN an admin set and must be changed.
func (s *Service) ResolveForPINChange(ctx context.Context, token Token) (Identity, error) {
	owner, ok := token.owner()
	if !ok {
		return Identity{}, ErrNoSession
	}

	who, found, err := s.store.SessionAccount(ctx, s.keys.TokenDigest(token), sessionIdle)
	if err != nil {
		return Identity{}, err
	}

	if !found || who.Account != owner {
		return Identity{}, ErrNoSession
	}

	return who, nil
}

func (s *Service) SignOut(ctx context.Context, token Token) error {
	return s.store.DeleteSession(ctx, s.keys.TokenDigest(token))
}

// DeleteExpiredSessions clears out sessions that can no longer be used; the server runs it now and then.
func (s *Service) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	return s.store.DeleteExpiredSessions(ctx, sessionIdle)
}
