package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/fleet-terminal/backend/internal/models"
)

// CreateEnrollmentJob opens an enrollment job for a host.
func (s *Store) CreateEnrollmentJob(ctx context.Context, hostID uuid.UUID, target, osHint string, createdBy *uuid.UUID) (*models.EnrollmentJob, error) {
	var j models.EnrollmentJob
	var steps []byte
	err := s.pool.QueryRow(ctx, `
		INSERT INTO enrollment_jobs (host_id, target, os_hint, status, created_by, started_at)
		VALUES ($1,$2,$3,'running',$4, now())
		RETURNING id, host_id, target, os_hint, status, steps, error, created_by, created_at, started_at, finished_at`,
		hostID, target, osHint, createdBy).
		Scan(&j.ID, &j.HostID, &j.Target, &j.OSHint, &j.Status, &steps, &j.Error, &j.CreatedBy, &j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(steps, &j.Steps)
	return &j, nil
}

// AppendEnrollmentStep records one step result on a job.
func (s *Store) AppendEnrollmentStep(ctx context.Context, jobID uuid.UUID, step models.EnrollmentStep) error {
	b, _ := json.Marshal(step)
	_, err := s.pool.Exec(ctx,
		`UPDATE enrollment_jobs SET steps = steps || $2::jsonb WHERE id=$1`, jobID, b)
	return err
}

// FinishEnrollmentJob marks a job succeeded/failed/rolled_back with an optional error.
func (s *Store) FinishEnrollmentJob(ctx context.Context, jobID uuid.UUID, status, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE enrollment_jobs SET status=$2, error=$3, finished_at=now() WHERE id=$1`, jobID, status, errMsg)
	return err
}

// GetEnrollmentJob loads a job by id.
func (s *Store) GetEnrollmentJob(ctx context.Context, id uuid.UUID) (*models.EnrollmentJob, error) {
	var j models.EnrollmentJob
	var steps []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, host_id, target, os_hint, status, steps, error, created_by, created_at, started_at, finished_at
		FROM enrollment_jobs WHERE id=$1`, id).
		Scan(&j.ID, &j.HostID, &j.Target, &j.OSHint, &j.Status, &steps, &j.Error, &j.CreatedBy, &j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	if err != nil {
		return nil, mapNotFound(err)
	}
	_ = json.Unmarshal(steps, &j.Steps)
	return &j, nil
}

// ListEnrollmentJobs returns recent enrollment jobs.
func (s *Store) ListEnrollmentJobs(ctx context.Context, limit int) ([]models.EnrollmentJob, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, host_id, target, os_hint, status, steps, error, created_by, created_at, started_at, finished_at
		FROM enrollment_jobs ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.EnrollmentJob
	for rows.Next() {
		var j models.EnrollmentJob
		var steps []byte
		if err := rows.Scan(&j.ID, &j.HostID, &j.Target, &j.OSHint, &j.Status, &steps, &j.Error,
			&j.CreatedBy, &j.CreatedAt, &j.StartedAt, &j.FinishedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(steps, &j.Steps)
		out = append(out, j)
	}
	return out, rows.Err()
}

// UsedWGAddresses returns the set of WireGuard addresses already assigned, so
// enrollment can allocate a free one.
func (s *Store) UsedWGAddresses(ctx context.Context) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT host(wg_address) FROM hosts WHERE wg_address IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	used := map[string]bool{}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		used[a] = true
	}
	return used, rows.Err()
}

// SetHostWGAddress records the assigned overlay address for a host.
func (s *Store) SetHostWGAddress(ctx context.Context, hostID uuid.UUID, wgAddr string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE hosts SET wg_address=NULLIF($2,'')::inet, updated_at=now() WHERE id=$1`, hostID, wgAddr)
	return err
}
