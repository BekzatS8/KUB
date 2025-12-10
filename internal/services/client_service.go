package services

import (
	"errors"
	"strings"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type ClientService struct {
	Repo *repositories.ClientRepository
}

func NewClientService(repo *repositories.ClientRepository) *ClientService {
	return &ClientService{Repo: repo}
}

func (s *ClientService) authorizeWrite(roleID int) error {
	if roleID == authz.RoleAdminStaff {
		return ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}
	return nil
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

func (s *ClientService) Create(c *models.Client, userID, roleID int) (int64, error) {
	if err := s.authorizeWrite(roleID); err != nil {
		return 0, err
	}
	if roleID == authz.RoleSales {
		c.OwnerID = userID
	}
	if c.OwnerID == 0 {
		c.OwnerID = userID
	}
	if roleID == authz.RoleSales && c.OwnerID != userID {
		return 0, ErrForbidden
	}
	if err := s.normalizeAndValidate(c); err != nil {
		return 0, err
	}
	return s.Repo.Create(c)
}

func (s *ClientService) Update(c *models.Client, userID, roleID int) error {
	if err := s.authorizeWrite(roleID); err != nil {
		return err
	}
	current, err := s.Repo.GetByID(c.ID)
	if err != nil {
		return err
	}
	if current == nil {
		return errors.New("client not found")
	}
	if roleID == authz.RoleSales && current.OwnerID != userID {
		return ErrForbidden
	}
	if roleID != authz.RoleManagement {
		c.OwnerID = current.OwnerID
	}
	if err := s.normalizeAndValidate(c); err != nil {
		return err
	}
	return s.Repo.Update(c)
}

func (s *ClientService) GetByID(id int, userID, roleID int) (*models.Client, error) {
	if roleID == authz.RoleAdminStaff {
		return nil, ErrForbidden
	}
	client, err := s.Repo.GetByID(id)
	if err != nil || client == nil {
		return client, err
	}
	if roleID == authz.RoleSales && client.OwnerID != userID {
		return nil, ErrForbidden
	}
	return client, nil
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

func (s *ClientService) ListAll(limit, offset int) ([]*models.Client, error) {
	return s.Repo.ListAll(limit, offset)
}

func (s *ClientService) ListMine(userID, limit, offset int) ([]*models.Client, error) {
	return s.Repo.ListByOwner(userID, limit, offset)
}

func (s *ClientService) ListForRole(userID, roleID, limit, offset int) ([]*models.Client, error) {
	if roleID == authz.RoleAdminStaff {
		return nil, ErrForbidden
	}
	if roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	return s.Repo.ListAll(limit, offset)
}
