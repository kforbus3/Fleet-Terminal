package store

import (
	"context"

	"github.com/google/uuid"
)

// CompleteSFTPTransfer finalizes a transfer record with its byte count + status.
func (s *Store) CompleteSFTPTransfer(ctx context.Context, id uuid.UUID, sizeBytes int64, status string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sftp_transfers SET size_bytes=$2, status=$3, completed_at=now() WHERE id=$1`,
		id, sizeBytes, status)
	return err
}
