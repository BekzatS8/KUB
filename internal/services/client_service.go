package services

import (
	"errors"
	"strings"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type ClientService struct {
	Repo *repositories.ClientRepository
}

func NewClientService(repo *repositories.ClientRepository) *ClientService {
	return &ClientService{Repo: repo}
}

func (s *ClientService) Create(client *models.Client) (int64, error) {
	if strings.TrimSpace(client.Name) == "" {
		return 0, errors.New("name is required")
	}
	if client.CreatedAt.IsZero() {
		client.CreatedAt = time.Now()
	}
	return s.Repo.Create(client)
}

func (s *ClientService) Update(client *models.Client) error {
	if strings.TrimSpace(client.Name) == "" {
		return errors.New("name is required")
	}
	return s.Repo.Update(client)
}

func (s *ClientService) GetByID(id int) (*models.Client, error) {
	return s.Repo.GetByID(id)
}

func (s *ClientService) GetOrCreateByBIN(bin string, fallback *models.Client) (*models.Client, error) {
	if strings.TrimSpace(bin) != "" {
		existing, err := s.Repo.GetByBIN(bin)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}
	if fallback == nil {
		return nil, errors.New("client data is required")
	}
	if strings.TrimSpace(fallback.Name) == "" {
		return nil, errors.New("client name is required")
	}
	if fallback.CreatedAt.IsZero() {
		fallback.CreatedAt = time.Now()
	}
	id, err := s.Repo.Create(fallback)
	if err != nil {
		return nil, err
	}
	fallback.ID = int(id)
	return fallback, nil
}

func (s *ClientService) List(limit, offset int) ([]*models.Client, error) {
	return s.Repo.List(limit, offset)
}
