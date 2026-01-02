package service

import (
	"context"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
	"github.com/strrl/wonder-mesh-net/pkg/meshbackend"
)

// JoinCredentials contains the credentials for a worker to join the mesh.
type JoinCredentials struct {
	MeshType string
	Metadata map[string]any
}

// WorkerService handles worker join token operations.
type WorkerService struct {
	tokenGenerator      *jointoken.Generator
	jwtSecret           string
	wonderNetRepository *repository.WonderNetRepository
	meshBackend         meshbackend.MeshBackend
}

// NewWorkerService creates a new WorkerService.
func NewWorkerService(
	tokenGenerator *jointoken.Generator,
	jwtSecret string,
	wonderNetRepository *repository.WonderNetRepository,
	meshBackend meshbackend.MeshBackend,
) *WorkerService {
	return &WorkerService{
		tokenGenerator:      tokenGenerator,
		jwtSecret:           jwtSecret,
		wonderNetRepository: wonderNetRepository,
		meshBackend:         meshBackend,
	}
}

// GenerateJoinToken creates a JWT for a worker to join the mesh.
func (s *WorkerService) GenerateJoinToken(ctx context.Context, wonderNet *repository.WonderNet, ttl time.Duration) (string, error) {
	return s.tokenGenerator.Generate(wonderNet.ID, ttl)
}

// ExchangeJoinToken validates a JWT and returns credentials for joining the mesh.
func (s *WorkerService) ExchangeJoinToken(ctx context.Context, token string) (*JoinCredentials, error) {
	validator := jointoken.NewValidator(s.jwtSecret)
	claims, err := validator.Validate(token)
	if err != nil {
		return nil, ErrInvalidToken
	}

	wonderNet, err := s.wonderNetRepository.Get(ctx, claims.WonderNetID)
	if err != nil || wonderNet == nil {
		return nil, ErrInvalidToken
	}

	metadata, err := s.meshBackend.CreateJoinCredentials(ctx, wonderNet.HeadscaleUser, meshbackend.JoinOptions{
		TTL:       24 * time.Hour,
		Reusable:  false,
		Ephemeral: false,
	})
	if err != nil {
		return nil, err
	}

	return &JoinCredentials{
		MeshType: string(s.meshBackend.MeshType()),
		Metadata: metadata,
	}, nil
}
