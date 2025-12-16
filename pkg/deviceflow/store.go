package deviceflow

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

const (
	DeviceCodeLength = 32
	UserCodeLength   = 8
	DefaultExpiry    = 15 * time.Minute
	PollInterval     = 5 * time.Second
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusExpired  Status = "expired"
	StatusDenied   Status = "denied"
)

type DeviceRequest struct {
	DeviceCode     string
	UserCode       string
	Status         Status
	HeadscaleUser  string
	UserID         string
	ExpiresAt      time.Time
	CreatedAt      time.Time
	Authkey        string
	HeadscaleURL   string
	CoordinatorURL string
}

type Store struct {
	mu       sync.RWMutex
	requests map[string]*DeviceRequest
	byUser   map[string]*DeviceRequest
}

func NewStore() *Store {
	s := &Store{
		requests: make(map[string]*DeviceRequest),
		byUser:   make(map[string]*DeviceRequest),
	}
	go s.cleanupLoop()
	return s
}

func (s *Store) Create() (*DeviceRequest, error) {
	deviceCode, err := generateCode(DeviceCodeLength)
	if err != nil {
		return nil, fmt.Errorf("failed to generate device code: %w", err)
	}

	userCode, err := generateUserCode()
	if err != nil {
		return nil, fmt.Errorf("failed to generate user code: %w", err)
	}

	req := &DeviceRequest{
		DeviceCode: deviceCode,
		UserCode:   userCode,
		Status:     StatusPending,
		ExpiresAt:  time.Now().Add(DefaultExpiry),
		CreatedAt:  time.Now(),
	}

	s.mu.Lock()
	s.requests[deviceCode] = req
	s.byUser[userCode] = req
	s.mu.Unlock()

	return req, nil
}

func (s *Store) GetByDeviceCode(deviceCode string) (*DeviceRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	req, ok := s.requests[deviceCode]
	if !ok {
		return nil, false
	}

	if time.Now().After(req.ExpiresAt) {
		req.Status = StatusExpired
	}

	return req, true
}

func (s *Store) GetByUserCode(userCode string) (*DeviceRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	req, ok := s.byUser[userCode]
	if !ok {
		return nil, false
	}

	if time.Now().After(req.ExpiresAt) {
		req.Status = StatusExpired
	}

	return req, true
}

func (s *Store) Approve(userCode, userID, headscaleUser, authkey, headscaleURL, coordinatorURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.byUser[userCode]
	if !ok {
		return fmt.Errorf("device request not found")
	}

	if time.Now().After(req.ExpiresAt) {
		req.Status = StatusExpired
		return fmt.Errorf("device request expired")
	}

	if req.Status != StatusPending {
		return fmt.Errorf("device request already processed")
	}

	req.Status = StatusApproved
	req.UserID = userID
	req.HeadscaleUser = headscaleUser
	req.Authkey = authkey
	req.HeadscaleURL = headscaleURL
	req.CoordinatorURL = coordinatorURL

	return nil
}

func (s *Store) Delete(deviceCode string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req, ok := s.requests[deviceCode]; ok {
		delete(s.byUser, req.UserCode)
		delete(s.requests, deviceCode)
	}
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanup()
	}
}

func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for deviceCode, req := range s.requests {
		if now.After(req.ExpiresAt.Add(time.Minute)) {
			delete(s.byUser, req.UserCode)
			delete(s.requests, deviceCode)
		}
	}
}

func generateCode(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func generateUserCode() (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	bytes := make([]byte, UserCodeLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	code := make([]byte, UserCodeLength)
	for i := range bytes {
		code[i] = charset[int(bytes[i])%len(charset)]
	}

	return string(code[:4]) + "-" + string(code[4:]), nil
}
