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

// нормализация + базовая валидация
func (s *ClientService) normalizeAndValidate(c *models.Client) error {
	trim := func(s string) string { return strings.TrimSpace(s) }

	c.Name = trim(c.Name)
	c.LastName = trim(c.LastName)
	c.FirstName = trim(c.FirstName)
	c.MiddleName = trim(c.MiddleName)
	c.IIN = trim(c.IIN)
	c.BinIin = trim(c.BinIin)
	c.Phone = trim(c.Phone)
	c.Email = trim(c.Email)

	// если Name пустой, но есть ФИО — собираем отображаемое имя
	if c.Name == "" && (c.LastName != "" || c.FirstName != "") {
		full := strings.TrimSpace(
			strings.Join([]string{c.LastName, c.FirstName, c.MiddleName}, " "),
		)
		c.Name = full
	}

	if c.Name == "" {
		return errors.New("name or (last_name + first_name) is required")
	}

	if c.IIN != "" && len(c.IIN) != 12 {
		return errors.New("iin must be 12 digits")
	}

	if c.Email != "" && !strings.Contains(c.Email, "@") {
		return errors.New("invalid email")
	}

	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}

	return nil
}

func (s *ClientService) Create(c *models.Client) (int64, error) {
	if err := s.normalizeAndValidate(c); err != nil {
		return 0, err
	}
	return s.Repo.Create(c)
}

func (s *ClientService) Update(c *models.Client) error {
	if err := s.normalizeAndValidate(c); err != nil {
		return err
	}
	return s.Repo.Update(c)
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

	if err := s.normalizeAndValidate(fallback); err != nil {
		return nil, err
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
