package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database"
)

// WonderNet represents a wonder net (project/namespace) in the coordinator.
type WonderNet struct {
	ID            string
	OwnerID       string
	HeadscaleUser string
	DisplayName   string
	MeshType      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// WonderNetRepository provides wonder net storage operations.
type WonderNetRepository struct {
	queries database.Queries
}

// NewWonderNetRepository creates a new WonderNetRepository.
func NewWonderNetRepository(queries database.Queries) *WonderNetRepository {
	return &WonderNetRepository{queries: queries}
}

// Create creates a new wonder net.
func (r *WonderNetRepository) Create(ctx context.Context, wn *WonderNet) error {
	return r.queries.CreateWonderNet(ctx, database.CreateWonderNetParams{
		ID:            wn.ID,
		OwnerID:       wn.OwnerID,
		HeadscaleUser: wn.HeadscaleUser,
		DisplayName:   wn.DisplayName,
		MeshType:      wn.MeshType,
	})
}

// Get retrieves a wonder net by ID.
func (r *WonderNetRepository) Get(ctx context.Context, id string) (*WonderNet, error) {
	row, err := r.queries.GetWonderNet(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbWonderNetToWonderNet(row), nil
}

// GetByHeadscaleUser retrieves a wonder net by Headscale user.
func (r *WonderNetRepository) GetByHeadscaleUser(ctx context.Context, headscaleUser string) (*WonderNet, error) {
	row, err := r.queries.GetWonderNetByHeadscaleUser(ctx, headscaleUser)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbWonderNetToWonderNet(row), nil
}

// ListByOwner lists all wonder nets owned by a user.
func (r *WonderNetRepository) ListByOwner(ctx context.Context, ownerID string) ([]*WonderNet, error) {
	rows, err := r.queries.ListWonderNetsByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	wonderNets := make([]*WonderNet, len(rows))
	for i, row := range rows {
		wonderNets[i] = dbWonderNetToWonderNet(row)
	}
	return wonderNets, nil
}

// Update updates a wonder net.
func (r *WonderNetRepository) Update(ctx context.Context, wn *WonderNet) error {
	return r.queries.UpdateWonderNet(ctx, database.UpdateWonderNetParams{
		DisplayName: wn.DisplayName,
		ID:          wn.ID,
	})
}

// Delete deletes a wonder net.
func (r *WonderNetRepository) Delete(ctx context.Context, id string) error {
	return r.queries.DeleteWonderNet(ctx, id)
}

// List lists all wonder nets.
func (r *WonderNetRepository) List(ctx context.Context) ([]*WonderNet, error) {
	rows, err := r.queries.ListWonderNets(ctx)
	if err != nil {
		return nil, err
	}
	wonderNets := make([]*WonderNet, len(rows))
	for i, row := range rows {
		wonderNets[i] = dbWonderNetToWonderNet(row)
	}
	return wonderNets, nil
}

func dbWonderNetToWonderNet(row database.WonderNet) *WonderNet {
	return &WonderNet{
		ID:            row.ID,
		OwnerID:       row.OwnerID,
		HeadscaleUser: row.HeadscaleUser,
		DisplayName:   row.DisplayName,
		MeshType:      row.MeshType,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
