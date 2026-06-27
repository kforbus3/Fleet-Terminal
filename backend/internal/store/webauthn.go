package store

import (
	"context"

	"github.com/google/uuid"
)

// WebAuthnCredential is a stored passkey (serialized webauthn.Credential JSON).
type WebAuthnCredential struct {
	ID   uuid.UUID
	JSON []byte
}

// AddWebAuthnCredential stores a confirmed passkey for a user.
func (s *Store) AddWebAuthnCredential(ctx context.Context, userID uuid.UUID, label string, credJSON []byte) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO mfa_methods (user_id, kind, label, secret, confirmed)
		VALUES ($1,'webauthn',$2,$3,true) RETURNING id`, userID, label, credJSON).Scan(&id)
	return id, err
}

// WebAuthnCredentials returns all of a user's stored passkeys.
func (s *Store) WebAuthnCredentials(ctx context.Context, userID uuid.UUID) ([]WebAuthnCredential, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, secret FROM mfa_methods WHERE user_id=$1 AND kind='webauthn' AND confirmed=true`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WebAuthnCredential
	for rows.Next() {
		var c WebAuthnCredential
		if err := rows.Scan(&c.ID, &c.JSON); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateWebAuthnCredential rewrites a stored passkey (e.g. updated sign count).
func (s *Store) UpdateWebAuthnCredential(ctx context.Context, id uuid.UUID, credJSON []byte) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE mfa_methods SET secret=$2, last_used_at=now() WHERE id=$1`, id, credJSON)
	return err
}
