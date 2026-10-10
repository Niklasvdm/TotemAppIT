package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Suggestion is a community request for a new animal, deduplicated by a
// normalized name with a running count.
type Suggestion struct {
	ID        int64
	Name      string
	Note      string
	Count     int
	Status    string
	CreatedAt string
	UpdatedAt string
}

// Report is a community report about an existing animal (wrong, obscure, poor
// description, …), deduplicated per (animal, reason) with a running count.
type Report struct {
	ID         int64
	AnimalSlug string
	AnimalName string // canonical Dutch name, for the admin list
	Reason     string
	Note       string
	Count      int
	Status     string
	CreatedAt  string
}

// ValidReportReason reports whether r is an accepted report reason. The same
// set is enforced by a CHECK constraint in migration 0003 (defence in depth).
func ValidReportReason(r string) bool {
	switch r {
	case "incorrect", "unknown", "poor_description", "other":
		return true
	default:
		return false
	}
}

// GameReport is a community report about a game (a bug, a rule implemented
// wrong, something unclear), deduplicated per (game, reason) with a count.
type GameReport struct {
	ID        int64
	Game      string
	Reason    string
	Note      string
	Count     int
	Status    string
	CreatedAt string
}

// ValidGameReportReason reports whether r is an accepted game-report reason.
// Enforced again by a CHECK constraint in migration 0004.
func ValidGameReportReason(r string) bool {
	switch r {
	case "bug", "rules", "unclear", "other":
		return true
	default:
		return false
	}
}

// ValidStatus reports whether s is an accepted moderation status.
func ValidStatus(s string) bool {
	switch s {
	case "pending", "accepted", "rejected":
		return true
	default:
		return false
	}
}

// normName lowercases and collapses whitespace so "Red  Panda" and "red panda"
// dedupe to the same suggestion.
func normName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// AddSuggestion records a new-animal suggestion. A repeat of the same name
// (case/space-insensitive) bumps the count instead of inserting a duplicate.
// name/note are expected to be validated by the caller; the SQL is fully
// parameterized regardless.
func (s *Store) AddSuggestion(ctx context.Context, name, note string) error {
	norm := normName(name)
	if norm == "" {
		return errors.New("empty suggestion name")
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO suggestion (name, name_norm, note) VALUES (?, ?, ?)
		ON CONFLICT(name_norm) DO UPDATE SET count = count + 1, updated_at = datetime('now')`,
		strings.TrimSpace(name), norm, note)
	if err != nil {
		return fmt.Errorf("add suggestion: %w", err)
	}
	return nil
}

// AddReport records a report against the animal identified by slug. A repeat of
// the same (animal, reason) bumps the count. Returns ErrNotFound if slug has no
// animal, so people can't seed reports for arbitrary strings.
func (s *Store) AddReport(ctx context.Context, slug, reason, note string) error {
	if !ValidReportReason(reason) {
		return fmt.Errorf("invalid report reason %q", reason)
	}
	var animalID int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM animal WHERE slug = ?`, slug).Scan(&animalID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve animal %s: %w", slug, err)
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO report (animal_id, reason, note) VALUES (?, ?, ?)
		ON CONFLICT(animal_id, reason) DO UPDATE SET count = count + 1, updated_at = datetime('now')`,
		animalID, reason, note)
	if err != nil {
		return fmt.Errorf("add report: %w", err)
	}
	return nil
}

// AddGameReport records a report about a game. A repeat of the same (game,
// reason) bumps the count. The game slug is validated by the caller (the API
// checks it against the game registry); reason is whitelisted here.
func (s *Store) AddGameReport(ctx context.Context, gameSlug, reason, note string) error {
	if !ValidGameReportReason(reason) {
		return fmt.Errorf("invalid game-report reason %q", reason)
	}
	if strings.TrimSpace(gameSlug) == "" {
		return errors.New("empty game slug")
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO game_report (game, reason, note) VALUES (?, ?, ?)
		ON CONFLICT(game, reason) DO UPDATE SET count = count + 1, updated_at = datetime('now')`,
		gameSlug, reason, note)
	if err != nil {
		return fmt.Errorf("add game report: %w", err)
	}
	return nil
}

// ListGameReports returns game reports with the given status (most-reported
// first). An empty status returns every report.
func (s *Store) ListGameReports(ctx context.Context, status string) ([]GameReport, error) {
	q := `SELECT id, game, reason, note, count, status, created_at FROM game_report`
	var args []any
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY count DESC, updated_at DESC`

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list game reports: %w", err)
	}
	defer rows.Close()

	var out []GameReport
	for rows.Next() {
		var x GameReport
		if err := rows.Scan(&x.ID, &x.Game, &x.Reason, &x.Note, &x.Count, &x.Status, &x.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan game report: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SetGameReportStatus updates one game report's status; false if no row matched.
func (s *Store) SetGameReportStatus(ctx context.Context, id int64, status string) (bool, error) {
	return s.setStatus(ctx, "game_report", id, status)
}

// ListSuggestions returns suggestions with the given status (most-suggested
// first). An empty status returns every suggestion.
func (s *Store) ListSuggestions(ctx context.Context, status string) ([]Suggestion, error) {
	q := `SELECT id, name, note, count, status, created_at, updated_at FROM suggestion`
	var args []any
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY count DESC, updated_at DESC`

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list suggestions: %w", err)
	}
	defer rows.Close()

	var out []Suggestion
	for rows.Next() {
		var x Suggestion
		if err := rows.Scan(&x.ID, &x.Name, &x.Note, &x.Count, &x.Status, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan suggestion: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ListReports returns reports with the given status (most-reported first),
// joined to the animal for a readable slug/name. Empty status returns all.
func (s *Store) ListReports(ctx context.Context, status string) ([]Report, error) {
	q := `SELECT r.id, a.slug, a.name_nl, r.reason, r.note, r.count, r.status, r.created_at
	      FROM report r JOIN animal a ON a.id = r.animal_id`
	var args []any
	if status != "" {
		q += ` WHERE r.status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY r.count DESC, r.updated_at DESC`

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()

	var out []Report
	for rows.Next() {
		var x Report
		if err := rows.Scan(&x.ID, &x.AnimalSlug, &x.AnimalName, &x.Reason, &x.Note, &x.Count, &x.Status, &x.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SetSuggestionStatus updates one suggestion's status; returns false if no row
// matched the id.
func (s *Store) SetSuggestionStatus(ctx context.Context, id int64, status string) (bool, error) {
	return s.setStatus(ctx, "suggestion", id, status)
}

// SetReportStatus updates one report's status; returns false if no row matched.
func (s *Store) SetReportStatus(ctx context.Context, id int64, status string) (bool, error) {
	return s.setStatus(ctx, "report", id, status)
}

func (s *Store) setStatus(ctx context.Context, table string, id int64, status string) (bool, error) {
	if !ValidStatus(status) {
		return false, fmt.Errorf("invalid status %q", status)
	}
	// table is an internal constant ("suggestion"/"report"), never user input.
	res, err := s.DB.ExecContext(ctx,
		`UPDATE `+table+` SET status = ?, updated_at = datetime('now') WHERE id = ?`, status, id)
	if err != nil {
		return false, fmt.Errorf("set %s status: %w", table, err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
