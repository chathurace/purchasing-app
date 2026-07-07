package repository

import (
	"context"
	"errors"
	"time"
)

// ErrDuplicateValue is returned when a list already contains the given value.
var ErrDuplicateValue = errors.New("this value already exists in the list")

// ConfigOption is a single value within a configurable dropdown list.
type ConfigOption struct {
	ID        int64     `json:"id"`
	ListKey   string    `json:"list_key"`
	Value     string    `json:"value"`
	SortOrder int       `json:"sort_order"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConfigOptionInput is the writable shape for create/update.
type ConfigOptionInput struct {
	ListKey   string
	Value     string
	SortOrder int
	IsActive  bool
}

const configOptionCols = `id, list_key, value, sort_order, is_active, created_at, updated_at`

func scanConfigOption(row interface {
	Scan(dest ...any) error
}, o *ConfigOption) error {
	return row.Scan(&o.ID, &o.ListKey, &o.Value, &o.SortOrder, &o.IsActive, &o.CreatedAt, &o.UpdatedAt)
}

// ListConfigOptions returns every option (active and inactive) ordered for the
// Settings page.
func (r *Repository) ListConfigOptions(ctx context.Context) ([]*ConfigOption, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+configOptionCols+`
		FROM config_options ORDER BY list_key, sort_order, lower(value)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ConfigOption{}
	for rows.Next() {
		o := &ConfigOption{}
		if err := scanConfigOption(rows, o); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ConfigOptionsLookup returns the active option values grouped by list_key, for
// populating the requisition-form dropdowns. Readable by any authenticated user.
func (r *Repository) ConfigOptionsLookup(ctx context.Context) (map[string][]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT list_key, value
		FROM config_options WHERE is_active ORDER BY list_key, sort_order, lower(value)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = append(out[key], value)
	}
	return out, rows.Err()
}

func (r *Repository) GetConfigOption(ctx context.Context, id int64) (*ConfigOption, error) {
	o := &ConfigOption{}
	if err := scanConfigOption(r.pool.QueryRow(ctx, `SELECT `+configOptionCols+`
		FROM config_options WHERE id = $1`, id), o); err != nil {
		return nil, err
	}
	return o, nil
}

func (r *Repository) CreateConfigOption(ctx context.Context, in ConfigOptionInput) (*ConfigOption, error) {
	var id int64
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO config_options (list_key, value, sort_order, is_active)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		in.ListKey, in.Value, in.SortOrder, in.IsActive).Scan(&id); err != nil {
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
			return nil, ErrDuplicateValue
		}
		return nil, err
	}
	return r.GetConfigOption(ctx, id)
}

func (r *Repository) UpdateConfigOption(ctx context.Context, id int64, in ConfigOptionInput) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE config_options
		SET value = $2, sort_order = $3, is_active = $4, updated_at = NOW()
		WHERE id = $1`, id, in.Value, in.SortOrder, in.IsActive)
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
		return ErrDuplicateValue
	}
	return err
}

func (r *Repository) DeleteConfigOption(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM config_options WHERE id = $1`, id)
	return err
}
