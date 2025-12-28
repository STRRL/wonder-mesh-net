package service

import (
	"context"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
)

// JoinCredentials contains the credentials for a worker to join the mesh.
type JoinCredentials struct {
	AuthKey      string
	HeadscaleURL string
	User         string
}

// WorkerService handles worker join token operations.
type WorkerService struct {
	tokenGenerator  *jointoken.Generator
	jwtSecret       string
	realmRepository *repository.RealmRepository
	realmService    *RealmService
}

// NewWorkerService creates a new WorkerService.
func NewWorkerService(
	tokenGenerator *jointoken.Generator,
	jwtSecret string,
	realmRepository *repository.RealmRepository,
	realmService *RealmService,
) *WorkerService {
	return &WorkerService{
		tokenGenerator:  tokenGenerator,
		jwtSecret:       jwtSecret,
		realmRepository: realmRepository,
		realmService:    realmService,
	}
}

// GenerateJoinToken creates a JWT for a worker to join the mesh.
func (s *WorkerService) GenerateJoinToken(ctx context.Context, realm *repository.Realm, ttl time.Duration) (string, error) {
	return s.tokenGenerator.Generate(realm.ID, realm.HeadscaleUser, ttl)
}

// ExchangeJoinToken validates a JWT and returns credentials for joining the mesh.
func (s *WorkerService) ExchangeJoinToken(ctx context.Context, token string) (*JoinCredentials, error) {
	validator := jointoken.NewValidator(s.jwtSecret)
	claims, err := validator.Validate(token)
	if err != nil {
		return nil, ErrInvalidToken
	}

	realm, err := s.realmRepository.Get(ctx, claims.RealmID)
	if err != nil || realm == nil {
		return nil, ErrInvalidToken
	}

	authKey, err := s.realmService.CreateAuthKey(ctx, realm, 24*time.Hour, false)
	if err != nil {
		return nil, err
	}

	return &JoinCredentials{
		AuthKey:      authKey,
		HeadscaleURL: s.realmService.GetPublicURL(),
		User:         claims.HeadscaleUser,
	}, nil
}
