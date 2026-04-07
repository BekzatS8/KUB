package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type chatDirectoryRepoStub struct {
	items      []*models.ChatUserDirectoryItem
	chats      []*models.Chat
	info       *models.ChatInfoResponse
	infoErr    error
	profiles   map[int]*models.ChatVisibleProfile
	statusByID map[int]struct {
		online   bool
		lastSeen time.Time
	}
}

func (s *chatDirectoryRepoStub) ListChatDirectoryUsers(viewerUserID int, query string, limit, offset int) ([]*models.ChatUserDirectoryItem, int, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	filtered := make([]*models.ChatUserDirectoryItem, 0, len(s.items))
	for _, it := range s.items {
		if it == nil {
			continue
		}
		if q != "" {
			hay := strings.ToLower(it.DisplayName + " " + it.Email + " " + it.RoleCode + " " + it.RoleName)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		cp := *it
		filtered = append(filtered, &cp)
	}
	if offset >= len(filtered) {
		return []*models.ChatUserDirectoryItem{}, len(filtered), nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], len(filtered), nil
}

func (s *chatDirectoryRepoStub) ListUserChats(int) ([]*models.Chat, error) {
	out := make([]*models.Chat, 0, len(s.chats))
	for _, ch := range s.chats {
		cp := *ch
		cp.Members = append([]int(nil), ch.Members...)
		out = append(out, &cp)
	}
	return out, nil
}
func (s *chatDirectoryRepoStub) ListMessages(int, int, int) ([]*models.ChatMessage, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) CreateMessage(int, int, string, []string) (*models.ChatMessage, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) IsMember(int, int) (bool, error) { return false, nil }
func (s *chatDirectoryRepoStub) CreateChat(name string, isGroup bool, creatorID int, memberIDs []int) (*models.Chat, error) {
	id := len(s.chats) + 100
	chat := &models.Chat{ID: id, IsGroup: isGroup, Name: name, CreatorID: creatorID, Members: append([]int(nil), memberIDs...)}
	s.chats = append(s.chats, chat)
	return chat, nil
}
func (s *chatDirectoryRepoStub) AddMembers(int, []int) error { return nil }
func (s *chatDirectoryRepoStub) RemoveMember(int, int) error { return nil }
func (s *chatDirectoryRepoStub) DeleteChat(int) error        { return nil }
func (s *chatDirectoryRepoStub) GetMemberRole(int, int) (string, error) {
	return "", nil
}
func (s *chatDirectoryRepoStub) GetChatInfo(int, int) (*models.ChatInfoResponse, error) {
	if s.infoErr != nil {
		return nil, s.infoErr
	}
	if s.info == nil {
		return &models.ChatInfoResponse{}, nil
	}
	return s.info, nil
}
func (s *chatDirectoryRepoStub) GetChatByID(id int) (*models.Chat, error) {
	for _, ch := range s.chats {
		if ch.ID == id {
			cp := *ch
			cp.Members = append([]int(nil), ch.Members...)
			return &cp, nil
		}
	}
	return nil, sql.ErrNoRows
}
func (s *chatDirectoryRepoStub) LastMessage(int) (*models.ChatMessage, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) SetOnline(int, bool) error { return nil }
func (s *chatDirectoryRepoStub) GetOnlineStatus(userID int) (bool, time.Time, error) {
	st, ok := s.statusByID[userID]
	if !ok {
		return false, time.Time{}, nil
	}
	return st.online, st.lastSeen, nil
}
func (s *chatDirectoryRepoStub) UpdateLastRead(int, int, int) error { return nil }
func (s *chatDirectoryRepoStub) MarkChatRead(int, int, *int) (int, time.Time, error) {
	return 0, time.Time{}, nil
}
func (s *chatDirectoryRepoStub) CountUnread(int, int) (int, error) { return 0, nil }
func (s *chatDirectoryRepoStub) SearchChats(userID int, query string) ([]*models.Chat, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []*models.Chat{}, nil
	}
	matched := make([]*models.Chat, 0)
	for _, ch := range s.chats {
		name := strings.ToLower(ch.Name)
		lastText := strings.ToLower(ch.LastMessageText)
		if strings.Contains(name, q) || strings.Contains(lastText, q) {
			cp := *ch
			cp.Members = append([]int(nil), ch.Members...)
			matched = append(matched, &cp)
			continue
		}
		if !ch.IsGroup {
			for _, uid := range ch.Members {
				if uid == userID {
					continue
				}
				p := s.profiles[uid]
				if p == nil {
					continue
				}
				hay := strings.ToLower(p.DisplayName + " " + p.Email)
				if strings.Contains(hay, q) {
					cp := *ch
					cp.Members = append([]int(nil), ch.Members...)
					matched = append(matched, &cp)
					break
				}
			}
		}
	}
	return matched, nil
}
func (s *chatDirectoryRepoStub) SearchMessagesFTS(int, int, string, int, int) ([]*models.ChatMessage, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) SearchMessagesILIKE(int, int, string, int, int) ([]*models.ChatMessage, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) CreateAttachment(int, int, string, string, int64, string) (*models.Attachment, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) AttachToMessage([]string, int, int, int) error { return nil }
func (s *chatDirectoryRepoStub) GetAttachmentsByMessageIDs([]int) (map[int][]models.AttachmentResponse, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) GetAttachmentForDownload(string) (*models.Attachment, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) EditMessage(int, int, int, string) (*models.ChatMessage, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) DeleteMessage(int, int, int) (*models.ChatMessage, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) PinMessage(int, int, int) (*models.PinResponse, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) UnpinMessage(int, int, int) error { return nil }
func (s *chatDirectoryRepoStub) FavoriteMessage(int, int, int) (*models.FavoriteResponse, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) UnfavoriteMessage(int, int, int) error { return nil }
func (s *chatDirectoryRepoStub) ListPins(int, int, int, int) ([]*models.PinResponse, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) ListFavorites(int, int, int, int) ([]*models.FavoriteResponse, error) {
	return nil, nil
}
func (s *chatDirectoryRepoStub) GetChatVisibleProfiles(userIDs []int) (map[int]*models.ChatVisibleProfile, error) {
	res := make(map[int]*models.ChatVisibleProfile, len(userIDs))
	for _, id := range userIDs {
		if p := s.profiles[id]; p != nil {
			cp := *p
			res[id] = &cp
		}
	}
	return res, nil
}
func (s *chatDirectoryRepoStub) FindPersonalChat(user1, user2 int) (*models.Chat, error) {
	for _, ch := range s.chats {
		if ch.IsGroup || len(ch.Members) != 2 {
			continue
		}
		has1 := false
		has2 := false
		for _, m := range ch.Members {
			if m == user1 {
				has1 = true
			}
			if m == user2 {
				has2 = true
			}
		}
		if has1 && has2 {
			cp := *ch
			cp.Members = append([]int(nil), ch.Members...)
			return &cp, nil
		}
	}
	return nil, nil
}

var _ repositories.ChatRepository = (*chatDirectoryRepoStub)(nil)

func setupChatDirectoryRouter(roleID int, repo repositories.ChatRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	userRepo := &chatTestUserRepo{
		users: map[int]*models.User{
			1: {ID: 1, IsVerified: true, Email: "me@kub.local"},
			2: {ID: 2, IsVerified: true, Email: "u2@kub.local"},
			7: {ID: 7, IsVerified: true, Email: "u7@kub.local"},
			9: {ID: 9, IsVerified: false, Email: "u9@kub.local"},
		},
	}
	svc := services.NewChatService(repo, "", userRepo)
	h := NewChatHandler(svc, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 1)
		c.Set("role_id", roleID)
		c.Next()
	})
	r.GET("/chats/users", h.ListChatDirectoryUsers)
	r.GET("/chats", h.ListChats)
	r.GET("/chats/search", h.SearchChats)
	r.GET("/chats/:id/info", h.GetChatInfo)
	r.GET("/chats/:id/messages", h.ListMessages)
	r.POST("/chats/:id/add-members", h.AddMembers)
	r.POST("/chats/personal", h.CreatePersonalChat)
	return r
}

type chatTestUserRepo struct {
	users map[int]*models.User
}

func (r *chatTestUserRepo) Create(*models.User) error { return nil }
func (r *chatTestUserRepo) GetByID(id int) (*models.User, error) {
	if u, ok := r.users[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, nil
}
func (r *chatTestUserRepo) Update(*models.User) error                  { return nil }
func (r *chatTestUserRepo) Delete(int) error                           { return nil }
func (r *chatTestUserRepo) List(int, int) ([]*models.User, error)      { return nil, nil }
func (r *chatTestUserRepo) GetByEmail(string) (*models.User, error)    { return nil, nil }
func (r *chatTestUserRepo) GetCount() (int, error)                     { return 0, nil }
func (r *chatTestUserRepo) GetCountByRole(int) (int, error)            { return 0, nil }
func (r *chatTestUserRepo) UpdatePassword(int, string) error           { return nil }
func (r *chatTestUserRepo) UpdateRefresh(int, string, time.Time) error { return nil }
func (r *chatTestUserRepo) RotateRefresh(string, string, time.Time) (*models.User, error) {
	return nil, nil
}
func (r *chatTestUserRepo) ClearRefresh(int) error                         { return nil }
func (r *chatTestUserRepo) GetByRefreshToken(string) (*models.User, error) { return nil, nil }
func (r *chatTestUserRepo) VerifyUser(int) error                           { return nil }
func (r *chatTestUserRepo) UpdateTelegramLink(int, int64, bool) error      { return nil }
func (r *chatTestUserRepo) GetByIDSimple(int) (*models.User, error)        { return nil, nil }
func (r *chatTestUserRepo) GetTelegramSettings(context.Context, int64) (int64, bool, error) {
	return 0, false, nil
}
func (r *chatTestUserRepo) GetByChatID(context.Context, int64) (*models.User, error) { return nil, nil }

func TestChatDirectory_AccessibleForSalesOperationsControl(t *testing.T) {
	repo := &chatDirectoryRepoStub{items: []*models.ChatUserDirectoryItem{{UserID: 2, DisplayName: "Ops", RoleCode: "operations", RoleName: "operations", Email: "ops@kub.local"}}}
	roles := []int{authz.RoleSales, authz.RoleOperations, authz.RoleControl}
	for _, role := range roles {
		r := setupChatDirectoryRouter(role, repo)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/chats/users", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("role=%d unexpected status: got=%d body=%s", role, w.Code, w.Body.String())
		}
	}
}

func TestChatDirectory_ExcludesCurrentUserAndNoSensitiveFields(t *testing.T) {
	repo := &chatDirectoryRepoStub{items: []*models.ChatUserDirectoryItem{
		{UserID: 1, DisplayName: "Self", RoleCode: "sales", RoleName: "sales", Email: "self@kub.local"},
		{UserID: 2, DisplayName: "Other", RoleCode: "operations", RoleName: "operations", Email: "ops@kub.local"},
	}}
	r := setupChatDirectoryRouter(authz.RoleSales, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chats/users", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), "phone") {
		t.Fatalf("unexpected sensitive fields in response: %s", w.Body.String())
	}

	var resp struct {
		Value []models.ChatUserDirectoryItem `json:"value"`
		Count int                            `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Value) != 1 || resp.Value[0].UserID != 2 {
		t.Fatalf("expected only non-self user, got %+v", resp.Value)
	}
}

func TestChatDirectory_QueryAndExistingPersonalChatID(t *testing.T) {
	chatID := 12
	repo := &chatDirectoryRepoStub{items: []*models.ChatUserDirectoryItem{
		{UserID: 2, DisplayName: "Aigerim Tulegenova", RoleCode: "operations", RoleName: "operations", Email: "aigerim@kub.local", ExistingPersonalChatID: &chatID},
		{UserID: 3, DisplayName: "Someone Else", RoleCode: "sales", RoleName: "sales", Email: "other@kub.local"},
	}}
	r := setupChatDirectoryRouter(authz.RoleOperations, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chats/users?q=aigerim", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Value []models.ChatUserDirectoryItem `json:"value"`
		Count int                            `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Value) != 1 || resp.Value[0].UserID != 2 {
		t.Fatalf("expected filtered user in response, got %+v", resp.Value)
	}
	if resp.Value[0].ExistingPersonalChatID == nil || *resp.Value[0].ExistingPersonalChatID != 12 {
		t.Fatalf("expected existing_personal_chat_id=12, got %+v", resp.Value[0].ExistingPersonalChatID)
	}
}

func TestListChats_PersonalContainsCounterparty_AndKeepsLegacyFields(t *testing.T) {
	now := time.Now().UTC()
	repo := &chatDirectoryRepoStub{
		chats: []*models.Chat{
			{ID: 12, IsGroup: false, Name: "", Members: []int{1, 7}, LastMessageText: "hi"},
		},
		profiles: map[int]*models.ChatVisibleProfile{
			1: {UserID: 1, DisplayName: "Me", RoleCode: "sales", RoleName: "sales", Email: "me@kub.local"},
			7: {UserID: 7, DisplayName: "Aigerim Tulegenova", RoleCode: "operations", RoleName: "operations", Email: "aigerim@kub.local"},
		},
		statusByID: map[int]struct {
			online   bool
			lastSeen time.Time
		}{
			1: {online: false, lastSeen: now.Add(-2 * time.Minute)},
			7: {online: true, lastSeen: now},
		},
	}
	r := setupChatDirectoryRouter(authz.RoleSales, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chats", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d body=%s", w.Code, w.Body.String())
	}
	var chats []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &chats); err != nil {
		t.Fatalf("unmarshal chats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("expected 1 chat, got %d", len(chats))
	}
	if _, ok := chats[0]["members"]; !ok {
		t.Fatalf("expected legacy field members in response: %v", chats[0])
	}
	cp, ok := chats[0]["counterparty"].(map[string]interface{})
	if !ok || cp["user_id"].(float64) != 7 {
		t.Fatalf("expected counterparty user_id=7, got %v", chats[0]["counterparty"])
	}
}

func TestSearchChats_ByCounterpartyDisplayName_Works(t *testing.T) {
	repo := &chatDirectoryRepoStub{
		chats: []*models.Chat{
			{ID: 12, IsGroup: false, Name: "", Members: []int{1, 7}, LastMessageText: "hello"},
		},
		profiles: map[int]*models.ChatVisibleProfile{
			7: {UserID: 7, DisplayName: "Aigerim Tulegenova", RoleCode: "operations", RoleName: "operations", Email: "aigerim@kub.local"},
		},
	}
	r := setupChatDirectoryRouter(authz.RoleOperations, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chats/search?q=Aigerim", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "\"id\":12") {
		t.Fatalf("expected chat id 12 in search response, got %s", w.Body.String())
	}
}

func TestListChats_GroupContainsParticipantsPreview(t *testing.T) {
	repo := &chatDirectoryRepoStub{
		chats: []*models.Chat{
			{ID: 20, IsGroup: true, Name: "ops group", Members: []int{1, 2, 3, 4}},
		},
		profiles: map[int]*models.ChatVisibleProfile{
			1: {UserID: 1, DisplayName: "Me", RoleCode: "sales", RoleName: "sales", Email: "me@kub.local"},
			2: {UserID: 2, DisplayName: "User2", RoleCode: "operations", RoleName: "operations", Email: "u2@kub.local"},
			3: {UserID: 3, DisplayName: "User3", RoleCode: "operations", RoleName: "operations", Email: "u3@kub.local"},
			4: {UserID: 4, DisplayName: "User4", RoleCode: "control", RoleName: "control", Email: "u4@kub.local"},
		},
		statusByID: map[int]struct {
			online   bool
			lastSeen time.Time
		}{},
	}
	r := setupChatDirectoryRouter(authz.RoleControl, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chats", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "participants_preview") {
		t.Fatalf("expected participants_preview in group chat response: %s", w.Body.String())
	}
}

func TestGetChatInfo_ReturnsParticipantsWithSafeMeaningfulFields(t *testing.T) {
	repo := &chatDirectoryRepoStub{
		info: &models.ChatInfoResponse{
			Chat: models.ChatInfoMeta{ID: 12, IsGroup: false, Name: ""},
			Participants: []models.ChatInfoParticipant{
				{UserID: 1, DisplayName: "Me", RoleCode: "sales", RoleName: "sales", Email: "me@kub.local"},
				{UserID: 7, DisplayName: "Aigerim Tulegenova", RoleCode: "operations", RoleName: "operations", Email: "aigerim@kub.local"},
			},
		},
	}
	r := setupChatDirectoryRouter(authz.RoleSales, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chats/12/info", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "\"email\":\"\"") {
		t.Fatalf("expected no fake empty email fields: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "\"display_name\":\"Aigerim Tulegenova\"") || !strings.Contains(w.Body.String(), "\"role_code\":\"operations\"") {
		t.Fatalf("expected participant safe fields in response: %s", w.Body.String())
	}
}

func TestCreatePersonalChat_SalesAndOperationsAllowed(t *testing.T) {
	for _, role := range []int{authz.RoleSales, authz.RoleOperations} {
		repo := &chatDirectoryRepoStub{}
		r := setupChatDirectoryRouter(role, repo)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/chats/personal", strings.NewReader(`{"user_id":2}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("role=%d expected 201, got %d body=%s", role, w.Code, w.Body.String())
		}
	}
}

func TestCreatePersonalChat_ControlAllowed(t *testing.T) {
	repo := &chatDirectoryRepoStub{}
	r := setupChatDirectoryRouter(authz.RoleControl, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chats/personal", strings.NewReader(`{"user_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCreatePersonalChat_IsIdempotentReturnsExisting(t *testing.T) {
	repo := &chatDirectoryRepoStub{
		chats: []*models.Chat{{ID: 12, IsGroup: false, Members: []int{1, 2}}},
	}
	r := setupChatDirectoryRouter(authz.RoleSales, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chats/personal", strings.NewReader(`{"user_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	if strings.Count(w.Body.String(), "\"id\":12") != 1 {
		t.Fatalf("expected existing chat id=12, got %s", w.Body.String())
	}
	if len(repo.chats) != 1 {
		t.Fatalf("expected no duplicate chat create, got chats=%d", len(repo.chats))
	}
}

func TestCreatePersonalChat_SelfAndUnverifiedTargetControlledErrors(t *testing.T) {
	repo := &chatDirectoryRepoStub{}
	r := setupChatDirectoryRouter(authz.RoleSales, repo)

	wSelf := httptest.NewRecorder()
	reqSelf := httptest.NewRequest(http.MethodPost, "/chats/personal", strings.NewReader(`{"user_id":1}`))
	reqSelf.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wSelf, reqSelf)
	if wSelf.Code != http.StatusBadRequest {
		t.Fatalf("self-chat expected 400, got %d body=%s", wSelf.Code, wSelf.Body.String())
	}

	wUnverified := httptest.NewRecorder()
	reqUnverified := httptest.NewRequest(http.MethodPost, "/chats/personal", strings.NewReader(`{"user_id":9}`))
	reqUnverified.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wUnverified, reqUnverified)
	if wUnverified.Code != http.StatusBadRequest {
		t.Fatalf("unverified target expected 400, got %d body=%s", wUnverified.Code, wUnverified.Body.String())
	}
}

func TestCreatePersonalChat_NonexistentTargetReturns404(t *testing.T) {
	repo := &chatDirectoryRepoStub{}
	r := setupChatDirectoryRouter(authz.RoleSales, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chats/personal", strings.NewReader(`{"user_id":999}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), ChatUserNotFoundCode) {
		t.Fatalf("expected error_code %s, got %s", ChatUserNotFoundCode, w.Body.String())
	}
}

func TestListMessages_NonMemberReturns403(t *testing.T) {
	repo := &chatDirectoryRepoStub{
		chats: []*models.Chat{{ID: 1, IsGroup: false, Members: []int{2, 3}}},
	}
	r := setupChatDirectoryRouter(authz.RoleSales, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chats/1/messages", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetChatInfo_NonexistentReturns404(t *testing.T) {
	repo := &chatDirectoryRepoStub{infoErr: sql.ErrNoRows}
	r := setupChatDirectoryRouter(authz.RoleSales, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chats/777/info", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAddMembers_InvalidTargetReturnsClearError(t *testing.T) {
	repo := &chatDirectoryRepoStub{
		chats: []*models.Chat{{ID: 10, IsGroup: true, Members: []int{1, 2}}},
	}
	r := setupChatDirectoryRouter(authz.RoleSales, repo)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chats/10/add-members", strings.NewReader(`{"members":[999]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), ChatUserNotFoundCode) {
		t.Fatalf("expected %s in response, got %s", ChatUserNotFoundCode, w.Body.String())
	}
}

func TestValidateSendMessagePayload_HTTPAndWSConsistency(t *testing.T) {
	req := &sendMessageRequest{Text: "   ", Attachments: nil}
	if err := validateSendMessagePayload(req); !errors.Is(err, services.ErrInvalidChatPayload) {
		t.Fatalf("expected ErrInvalidChatPayload, got %v", err)
	}
}
