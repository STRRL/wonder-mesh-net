package service

import (
	"context"
	"regexp"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
)

var (
	userCodePattern   = regexp.MustCompile(`^[A-Z0-9]{4}-[A-Z0-9]{4}$`)
	deviceCodePattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

// DeviceFlowInit contains the initial device flow response.
type DeviceFlowInit struct {
	DeviceCode      string
	UserCode        string
	VerificationURL string
	ExpiresIn       int
	Interval        int
}

// DeviceFlowResult contains the device token poll result.
type DeviceFlowResult struct {
	Status       string
	AuthKey      string
	HeadscaleURL string
	User         string
}

// DeviceFlowService handles RFC 8628 device authorization flow.
type DeviceFlowService struct {
	deviceRequestRepository *repository.DeviceRequestRepository
	wonderNetService        *WonderNetService
	publicURL               string
}

// NewDeviceFlowService creates a new DeviceFlowService.
func NewDeviceFlowService(
	deviceRequestRepository *repository.DeviceRequestRepository,
	wonderNetService *WonderNetService,
	publicURL string,
) *DeviceFlowService {
	return &DeviceFlowService{
		deviceRequestRepository: deviceRequestRepository,
		wonderNetService:        wonderNetService,
		publicURL:               publicURL,
	}
}

// InitiateDeviceFlow starts the device authorization flow.
func (s *DeviceFlowService) InitiateDeviceFlow(ctx context.Context) (*DeviceFlowInit, error) {
	req, err := s.deviceRequestRepository.Create(ctx)
	if err != nil {
		return nil, err
	}

	return &DeviceFlowInit{
		DeviceCode:      req.DeviceCode,
		UserCode:        req.UserCode,
		VerificationURL: s.publicURL + "/coordinator/device/verify",
		ExpiresIn:       int(time.Until(req.ExpiresAt).Seconds()),
		Interval:        int(repository.PollInterval.Seconds()),
	}, nil
}

// ValidateUserCode checks if the user code format is valid.
func (s *DeviceFlowService) ValidateUserCode(userCode string) bool {
	return userCodePattern.MatchString(userCode)
}

// ValidateDeviceCode checks if the device code format is valid.
func (s *DeviceFlowService) ValidateDeviceCode(deviceCode string) bool {
	return deviceCodePattern.MatchString(deviceCode)
}

// ApproveDevice approves a device request with the given user code.
func (s *DeviceFlowService) ApproveDevice(ctx context.Context, userCode string, wonderNet *repository.WonderNet) error {
	deviceReq, err := s.deviceRequestRepository.GetByUserCode(ctx, userCode)
	if err != nil {
		return err
	}

	if deviceReq.Status != repository.DeviceStatusPending {
		return ErrCodeAlreadyUsed
	}

	authKey, err := s.wonderNetService.CreateAuthKey(ctx, wonderNet, 24*time.Hour, false)
	if err != nil {
		return err
	}

	return s.deviceRequestRepository.Approve(
		ctx,
		userCode,
		wonderNet.ID,
		wonderNet.HeadscaleUser,
		authKey,
		s.publicURL,
		s.publicURL,
	)
}

// PollDeviceToken polls the device authorization status.
func (s *DeviceFlowService) PollDeviceToken(ctx context.Context, deviceCode string) (*DeviceFlowResult, error) {
	deviceReq, err := s.deviceRequestRepository.GetByDeviceCode(ctx, deviceCode)
	if err != nil {
		return nil, err
	}

	result := &DeviceFlowResult{}

	switch deviceReq.Status {
	case repository.DeviceStatusPending:
		result.Status = "authorization_pending"

	case repository.DeviceStatusApproved:
		result.Status = "approved"
		result.AuthKey = deviceReq.Authkey
		result.HeadscaleURL = deviceReq.HeadscaleURL
		result.User = deviceReq.HeadscaleUser
		s.deviceRequestRepository.Delete(ctx, deviceCode)

	case repository.DeviceStatusExpired:
		result.Status = "expired_token"
		s.deviceRequestRepository.Delete(ctx, deviceCode)

	case repository.DeviceStatusDenied:
		result.Status = "access_denied"
		s.deviceRequestRepository.Delete(ctx, deviceCode)
	}

	return result, nil
}
