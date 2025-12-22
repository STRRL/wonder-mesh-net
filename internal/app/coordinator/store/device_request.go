package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database"
)

const (
	DeviceCodeLength    = 32
	UserCodeLength      = 8
	DefaultDeviceExpiry = 15 * time.Minute
	PollInterval        = 5 * time.Second
	maxCollisionRetries = 10
)

// ErrDeviceRequestNotFound is returned when a device request is not found
var ErrDeviceRequestNotFound = errors.New("device request not found")

// DeviceStatus represents the status of a device authorization request
type DeviceStatus string

const (
	DeviceStatusPending  DeviceStatus = "pending"
	DeviceStatusApproved DeviceStatus = "approved"
	DeviceStatusExpired  DeviceStatus = "expired"
	DeviceStatusDenied   DeviceStatus = "denied"
)

// DeviceRequest represents a device authorization flow request
type DeviceRequest struct {
	DeviceCode     string
	UserCode       string
	Status         DeviceStatus
	HeadscaleUser  string
	UserID         string
	ExpiresAt      time.Time
	CreatedAt      time.Time
	Authkey        string
	HeadscaleURL   string
	CoordinatorURL string
}

// DeviceRequestStore provides database-backed storage for device authorization requests
type DeviceRequestStore struct {
	queries *database.Queries
}

// NewDeviceRequestStore creates a new database-backed device request store
func NewDeviceRequestStore(queries *database.Queries) *DeviceRequestStore {
	return &DeviceRequestStore{queries: queries}
}

// Create creates a new device authorization request
func (s *DeviceRequestStore) Create(ctx context.Context) (*DeviceRequest, error) {
	deviceCode, err := generateDeviceCode(DeviceCodeLength)
	if err != nil {
		return nil, fmt.Errorf("failed to generate device code: %w", err)
	}

	var userCode string
	for i := 0; i < maxCollisionRetries; i++ {
		code, err := generateUserCode()
		if err != nil {
			return nil, fmt.Errorf("failed to generate user code: %w", err)
		}
		exists, err := s.queries.UserCodeExists(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("failed to check user code: %w", err)
		}
		if exists == 0 {
			userCode = code
			break
		}
	}
	if userCode == "" {
		return nil, fmt.Errorf("failed to generate unique user code after %d attempts", maxCollisionRetries)
	}

	now := time.Now()
	expiresAt := now.Add(DefaultDeviceExpiry)

	err = s.queries.CreateDeviceRequest(ctx, database.CreateDeviceRequestParams{
		DeviceCode: deviceCode,
		UserCode:   userCode,
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create device request: %w", err)
	}

	return &DeviceRequest{
		DeviceCode: deviceCode,
		UserCode:   userCode,
		Status:     DeviceStatusPending,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
	}, nil
}

// GetByDeviceCode retrieves a device request by device code
func (s *DeviceRequestStore) GetByDeviceCode(ctx context.Context, deviceCode string) (*DeviceRequest, error) {
	dbReq, err := s.queries.GetDeviceRequestByDeviceCode(ctx, deviceCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeviceRequestNotFound
		}
		return nil, fmt.Errorf("failed to get device request: %w", err)
	}

	req := dbDeviceRequestToDeviceRequest(dbReq)
	if time.Now().After(req.ExpiresAt) && req.Status == DeviceStatusPending {
		req.Status = DeviceStatusExpired
	}

	return req, nil
}

// GetByUserCode retrieves a device request by user code
func (s *DeviceRequestStore) GetByUserCode(ctx context.Context, userCode string) (*DeviceRequest, error) {
	dbReq, err := s.queries.GetDeviceRequestByUserCode(ctx, userCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeviceRequestNotFound
		}
		return nil, fmt.Errorf("failed to get device request: %w", err)
	}

	req := dbDeviceRequestToDeviceRequest(dbReq)
	if time.Now().After(req.ExpiresAt) && req.Status == DeviceStatusPending {
		req.Status = DeviceStatusExpired
	}

	return req, nil
}

// Approve approves a device request and stores the approval details
func (s *DeviceRequestStore) Approve(ctx context.Context, userCode, userID, headscaleUser, authkey, headscaleURL, coordinatorURL string) error {
	return s.queries.ApproveDeviceRequest(ctx, database.ApproveDeviceRequestParams{
		UserID:         sql.NullString{String: userID, Valid: true},
		HeadscaleUser:  sql.NullString{String: headscaleUser, Valid: true},
		Authkey:        sql.NullString{String: authkey, Valid: true},
		HeadscaleUrl:   sql.NullString{String: headscaleURL, Valid: true},
		CoordinatorUrl: sql.NullString{String: coordinatorURL, Valid: true},
		UserCode:       userCode,
	})
}

// Delete removes a device request by device code
func (s *DeviceRequestStore) Delete(ctx context.Context, deviceCode string) {
	_ = s.queries.DeleteDeviceRequest(ctx, deviceCode)
}

// DeleteExpired removes all expired device requests
func (s *DeviceRequestStore) DeleteExpired(ctx context.Context) error {
	return s.queries.DeleteExpiredDeviceRequests(ctx)
}

func dbDeviceRequestToDeviceRequest(dbReq database.DeviceRequest) *DeviceRequest {
	return &DeviceRequest{
		DeviceCode:     dbReq.DeviceCode,
		UserCode:       dbReq.UserCode,
		Status:         DeviceStatus(dbReq.Status),
		HeadscaleUser:  dbReq.HeadscaleUser.String,
		UserID:         dbReq.UserID.String,
		Authkey:        dbReq.Authkey.String,
		HeadscaleURL:   dbReq.HeadscaleUrl.String,
		CoordinatorURL: dbReq.CoordinatorUrl.String,
		CreatedAt:      dbReq.CreatedAt,
		ExpiresAt:      dbReq.ExpiresAt,
	}
}

func generateDeviceCode(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func generateUserCode() (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	charsetLen := len(charset)
	maxUnbiased := 256 - (256 % charsetLen)

	code := make([]byte, UserCodeLength)
	for i := 0; i < UserCodeLength; i++ {
		for {
			var b [1]byte
			if _, err := rand.Read(b[:]); err != nil {
				return "", err
			}
			if int(b[0]) < maxUnbiased {
				code[i] = charset[int(b[0])%charsetLen]
				break
			}
		}
	}

	return string(code[:4]) + "-" + string(code[4:]), nil
}
