package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	sqlcpostgres "github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc/postgres"
	sqlcsqlite "github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc/sqlite"
)

type WonderNet struct {
	ID            string
	OwnerID       string
	HeadscaleUser string
	DisplayName   string
	MeshType      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ApiKey struct {
	ID          string
	WonderNetID string
	Name        string
	KeyHash     string
	KeyPrefix   string
	CreatedAt   time.Time
	LastUsedAt  sql.NullTime
	ExpiresAt   sql.NullTime
}

type CreateWonderNetParams struct {
	ID            string
	OwnerID       string
	HeadscaleUser string
	DisplayName   string
	MeshType      string
}

type UpdateWonderNetParams struct {
	DisplayName string
	ID          string
}

type CreateAPIKeyParams struct {
	ID          string
	WonderNetID string
	Name        string
	KeyHash     string
	KeyPrefix   string
	ExpiresAt   sql.NullTime
}

type Queries interface {
	CreateWonderNet(ctx context.Context, arg CreateWonderNetParams) error
	GetWonderNet(ctx context.Context, id string) (WonderNet, error)
	GetWonderNetByHeadscaleUser(ctx context.Context, headscaleUser string) (WonderNet, error)
	ListWonderNetsByOwner(ctx context.Context, ownerID string) ([]WonderNet, error)
	UpdateWonderNet(ctx context.Context, arg UpdateWonderNetParams) error
	DeleteWonderNet(ctx context.Context, id string) error
	ListWonderNets(ctx context.Context) ([]WonderNet, error)

	CreateAPIKey(ctx context.Context, arg CreateAPIKeyParams) (ApiKey, error)
	GetAPIKeyByHash(ctx context.Context, keyHash string) (ApiKey, error)
	GetAPIKeyByID(ctx context.Context, id string) (ApiKey, error)
	ListAPIKeysByWonderNet(ctx context.Context, wonderNetID string) ([]ApiKey, error)
	DeleteAPIKey(ctx context.Context, id string) error
	UpdateAPIKeyLastUsed(ctx context.Context, id string) error
}

func newQueries(driver Driver, db *sql.DB) (Queries, error) {
	switch driver {
	case DriverSQLite:
		return &sqliteQueries{q: sqlcsqlite.New(db)}, nil
	case DriverPostgres:
		return &postgresQueries{q: sqlcpostgres.New(db)}, nil
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

type sqliteQueries struct {
	q *sqlcsqlite.Queries
}

func (s *sqliteQueries) CreateWonderNet(ctx context.Context, arg CreateWonderNetParams) error {
	return s.q.CreateWonderNet(ctx, sqlcsqlite.CreateWonderNetParams{
		ID:            arg.ID,
		OwnerID:       arg.OwnerID,
		HeadscaleUser: arg.HeadscaleUser,
		DisplayName:   arg.DisplayName,
		MeshType:      arg.MeshType,
	})
}

func (s *sqliteQueries) GetWonderNet(ctx context.Context, id string) (WonderNet, error) {
	row, err := s.q.GetWonderNet(ctx, id)
	if err != nil {
		return WonderNet{}, err
	}
	return sqliteWonderNet(row), nil
}

func (s *sqliteQueries) GetWonderNetByHeadscaleUser(ctx context.Context, headscaleUser string) (WonderNet, error) {
	row, err := s.q.GetWonderNetByHeadscaleUser(ctx, headscaleUser)
	if err != nil {
		return WonderNet{}, err
	}
	return sqliteWonderNet(row), nil
}

func (s *sqliteQueries) ListWonderNetsByOwner(ctx context.Context, ownerID string) ([]WonderNet, error) {
	rows, err := s.q.ListWonderNetsByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	items := make([]WonderNet, len(rows))
	for i, row := range rows {
		items[i] = sqliteWonderNet(row)
	}
	return items, nil
}

func (s *sqliteQueries) UpdateWonderNet(ctx context.Context, arg UpdateWonderNetParams) error {
	return s.q.UpdateWonderNet(ctx, sqlcsqlite.UpdateWonderNetParams{
		DisplayName: arg.DisplayName,
		ID:          arg.ID,
	})
}

func (s *sqliteQueries) DeleteWonderNet(ctx context.Context, id string) error {
	return s.q.DeleteWonderNet(ctx, id)
}

func (s *sqliteQueries) ListWonderNets(ctx context.Context) ([]WonderNet, error) {
	rows, err := s.q.ListWonderNets(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]WonderNet, len(rows))
	for i, row := range rows {
		items[i] = sqliteWonderNet(row)
	}
	return items, nil
}

func (s *sqliteQueries) CreateAPIKey(ctx context.Context, arg CreateAPIKeyParams) (ApiKey, error) {
	row, err := s.q.CreateAPIKey(ctx, sqlcsqlite.CreateAPIKeyParams{
		ID:          arg.ID,
		WonderNetID: arg.WonderNetID,
		Name:        arg.Name,
		KeyHash:     arg.KeyHash,
		KeyPrefix:   arg.KeyPrefix,
		ExpiresAt:   arg.ExpiresAt,
	})
	if err != nil {
		return ApiKey{}, err
	}
	return sqliteApiKey(row), nil
}

func (s *sqliteQueries) GetAPIKeyByHash(ctx context.Context, keyHash string) (ApiKey, error) {
	row, err := s.q.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return ApiKey{}, err
	}
	return sqliteApiKey(row), nil
}

func (s *sqliteQueries) GetAPIKeyByID(ctx context.Context, id string) (ApiKey, error) {
	row, err := s.q.GetAPIKeyByID(ctx, id)
	if err != nil {
		return ApiKey{}, err
	}
	return sqliteApiKey(row), nil
}

func (s *sqliteQueries) ListAPIKeysByWonderNet(ctx context.Context, wonderNetID string) ([]ApiKey, error) {
	rows, err := s.q.ListAPIKeysByWonderNet(ctx, wonderNetID)
	if err != nil {
		return nil, err
	}
	items := make([]ApiKey, len(rows))
	for i, row := range rows {
		items[i] = sqliteApiKey(row)
	}
	return items, nil
}

func (s *sqliteQueries) DeleteAPIKey(ctx context.Context, id string) error {
	return s.q.DeleteAPIKey(ctx, id)
}

func (s *sqliteQueries) UpdateAPIKeyLastUsed(ctx context.Context, id string) error {
	return s.q.UpdateAPIKeyLastUsed(ctx, id)
}

func sqliteWonderNet(row sqlcsqlite.WonderNet) WonderNet {
	return WonderNet{
		ID:            row.ID,
		OwnerID:       row.OwnerID,
		HeadscaleUser: row.HeadscaleUser,
		DisplayName:   row.DisplayName,
		MeshType:      row.MeshType,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func sqliteApiKey(row sqlcsqlite.ApiKey) ApiKey {
	return ApiKey{
		ID:          row.ID,
		WonderNetID: row.WonderNetID,
		Name:        row.Name,
		KeyHash:     row.KeyHash,
		KeyPrefix:   row.KeyPrefix,
		CreatedAt:   row.CreatedAt,
		LastUsedAt:  row.LastUsedAt,
		ExpiresAt:   row.ExpiresAt,
	}
}

type postgresQueries struct {
	q *sqlcpostgres.Queries
}

func (p *postgresQueries) CreateWonderNet(ctx context.Context, arg CreateWonderNetParams) error {
	return p.q.CreateWonderNet(ctx, sqlcpostgres.CreateWonderNetParams{
		ID:            arg.ID,
		OwnerID:       arg.OwnerID,
		HeadscaleUser: arg.HeadscaleUser,
		DisplayName:   arg.DisplayName,
		MeshType:      arg.MeshType,
	})
}

func (p *postgresQueries) GetWonderNet(ctx context.Context, id string) (WonderNet, error) {
	row, err := p.q.GetWonderNet(ctx, id)
	if err != nil {
		return WonderNet{}, err
	}
	return postgresWonderNet(row), nil
}

func (p *postgresQueries) GetWonderNetByHeadscaleUser(ctx context.Context, headscaleUser string) (WonderNet, error) {
	row, err := p.q.GetWonderNetByHeadscaleUser(ctx, headscaleUser)
	if err != nil {
		return WonderNet{}, err
	}
	return postgresWonderNet(row), nil
}

func (p *postgresQueries) ListWonderNetsByOwner(ctx context.Context, ownerID string) ([]WonderNet, error) {
	rows, err := p.q.ListWonderNetsByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	items := make([]WonderNet, len(rows))
	for i, row := range rows {
		items[i] = postgresWonderNet(row)
	}
	return items, nil
}

func (p *postgresQueries) UpdateWonderNet(ctx context.Context, arg UpdateWonderNetParams) error {
	return p.q.UpdateWonderNet(ctx, sqlcpostgres.UpdateWonderNetParams{
		DisplayName: arg.DisplayName,
		ID:          arg.ID,
	})
}

func (p *postgresQueries) DeleteWonderNet(ctx context.Context, id string) error {
	return p.q.DeleteWonderNet(ctx, id)
}

func (p *postgresQueries) ListWonderNets(ctx context.Context) ([]WonderNet, error) {
	rows, err := p.q.ListWonderNets(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]WonderNet, len(rows))
	for i, row := range rows {
		items[i] = postgresWonderNet(row)
	}
	return items, nil
}

func (p *postgresQueries) CreateAPIKey(ctx context.Context, arg CreateAPIKeyParams) (ApiKey, error) {
	row, err := p.q.CreateAPIKey(ctx, sqlcpostgres.CreateAPIKeyParams{
		ID:          arg.ID,
		WonderNetID: arg.WonderNetID,
		Name:        arg.Name,
		KeyHash:     arg.KeyHash,
		KeyPrefix:   arg.KeyPrefix,
		ExpiresAt:   arg.ExpiresAt,
	})
	if err != nil {
		return ApiKey{}, err
	}
	return postgresApiKey(row), nil
}

func (p *postgresQueries) GetAPIKeyByHash(ctx context.Context, keyHash string) (ApiKey, error) {
	row, err := p.q.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return ApiKey{}, err
	}
	return postgresApiKey(row), nil
}

func (p *postgresQueries) GetAPIKeyByID(ctx context.Context, id string) (ApiKey, error) {
	row, err := p.q.GetAPIKeyByID(ctx, id)
	if err != nil {
		return ApiKey{}, err
	}
	return postgresApiKey(row), nil
}

func (p *postgresQueries) ListAPIKeysByWonderNet(ctx context.Context, wonderNetID string) ([]ApiKey, error) {
	rows, err := p.q.ListAPIKeysByWonderNet(ctx, wonderNetID)
	if err != nil {
		return nil, err
	}
	items := make([]ApiKey, len(rows))
	for i, row := range rows {
		items[i] = postgresApiKey(row)
	}
	return items, nil
}

func (p *postgresQueries) DeleteAPIKey(ctx context.Context, id string) error {
	return p.q.DeleteAPIKey(ctx, id)
}

func (p *postgresQueries) UpdateAPIKeyLastUsed(ctx context.Context, id string) error {
	return p.q.UpdateAPIKeyLastUsed(ctx, id)
}

func postgresWonderNet(row sqlcpostgres.WonderNet) WonderNet {
	return WonderNet{
		ID:            row.ID,
		OwnerID:       row.OwnerID,
		HeadscaleUser: row.HeadscaleUser,
		DisplayName:   row.DisplayName,
		MeshType:      row.MeshType,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func postgresApiKey(row sqlcpostgres.ApiKey) ApiKey {
	return ApiKey{
		ID:          row.ID,
		WonderNetID: row.WonderNetID,
		Name:        row.Name,
		KeyHash:     row.KeyHash,
		KeyPrefix:   row.KeyPrefix,
		CreatedAt:   row.CreatedAt,
		LastUsedAt:  row.LastUsedAt,
		ExpiresAt:   row.ExpiresAt,
	}
}
