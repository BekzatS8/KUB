package services

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"
	"time"

	"turcompany/internal/models"
)

type chatRepoMock struct {
	lastCreateCreator int
	chats             map[int]*models.Chat
	infos             map[int]*models.ChatInfoResponse
	attachedIDs       []string
	attachedMessageID int
	attachmentByID    map[string]*models.Attachment
	membership        map[int]map[int]bool
	lastSearchMode    string
	lastSearchLimit   int
	lastSearchOffset  int
	markedReadMsgID   int
	markedReadAt      time.Time
	lastPinRole       string
	favoritesCount    int
	messageStore      map[int]*models.ChatMessage
}

func newChatRepoMock() *chatRepoMock {
	return &chatRepoMock{
		chats:          make(map[int]*models.Chat),
		infos:          make(map[int]*models.ChatInfoResponse),
		attachmentByID: make(map[string]*models.Attachment),
		membership:     make(map[int]map[int]bool),
		messageStore:   make(map[int]*models.ChatMessage),
	}
}

func (m *chatRepoMock) ListUserChats(userID int) ([]*models.Chat, error) { return nil, nil }
func (m *chatRepoMock) ListMessages(chatID int, limit, offset int) ([]*models.ChatMessage, error) {
	return nil, nil
}
func (m *chatRepoMock) CreateMessage(chatID, senderID int, text string, attachments []string) (*models.ChatMessage, error) {
	return &models.ChatMessage{ID: 101, ChatID: chatID, SenderID: senderID, Text: text, Attachments: attachments, CreatedAt: time.Now()}, nil
}
func (m *chatRepoMock) IsMember(chatID, userID int) (bool, error) {
	if mm, ok := m.membership[chatID]; ok {
		return mm[userID], nil
	}
	return false, nil
}
func (m *chatRepoMock) CreateChat(name string, isGroup bool, creatorID int, memberIDs []int) (*models.Chat, error) {
	m.lastCreateCreator = creatorID
	chat := &models.Chat{ID: len(m.chats) + 1, Name: name, IsGroup: isGroup, Members: memberIDs, CreatorID: creatorID, CreatedAt: time.Now()}
	m.chats[chat.ID] = chat
	if _, ok := m.membership[chat.ID]; !ok {
		m.membership[chat.ID] = map[int]bool{}
	}
	participants := make([]models.ChatInfoParticipant, 0, len(memberIDs))
	for _, id := range memberIDs {
		role := models.ChatMemberRoleMember
		if id == creatorID {
			role = models.ChatMemberRoleOwner
		}
		participants = append(participants, models.ChatInfoParticipant{UserID: id, Role: role, JoinedAt: time.Now(), Email: "u@test"})
		m.membership[chat.ID][id] = true
	}
	m.infos[chat.ID] = &models.ChatInfoResponse{Chat: models.ChatInfoMeta{ID: chat.ID, CreatorID: creatorID}, Participants: participants}
	return chat, nil
}
func (m *chatRepoMock) AddMembers(chatID int, memberIDs []int) error { return nil }
func (m *chatRepoMock) RemoveMember(chatID int, userID int) error    { return nil }
func (m *chatRepoMock) DeleteChat(chatID int) error                  { return nil }
func (m *chatRepoMock) GetMemberRole(chatID, userID int) (string, error) {
	return models.ChatMemberRoleOwner, nil
}
func (m *chatRepoMock) GetChatInfo(chatID, userID int) (*models.ChatInfoResponse, error) {
	info, ok := m.infos[chatID]
	if !ok {
		return nil, errors.New("not found")
	}
	return info, nil
}
func (m *chatRepoMock) GetChatByID(chatID int) (*models.Chat, error) { return m.chats[chatID], nil }
func (m *chatRepoMock) LastMessage(chatID int) (*models.ChatMessage, error) {
	return nil, nil
}
func (m *chatRepoMock) SetOnline(userID int, online bool) error { return nil }
func (m *chatRepoMock) GetOnlineStatus(userID int) (bool, time.Time, error) {
	return false, time.Time{}, nil
}
func (m *chatRepoMock) UpdateLastRead(chatID, userID, messageID int) error { return nil }
func (m *chatRepoMock) MarkChatRead(chatID, userID int, messageID *int) (int, time.Time, error) {
	msgID := 0
	if messageID != nil {
		msgID = *messageID
	}
	m.markedReadMsgID = msgID
	m.markedReadAt = time.Now()
	for i := range m.infos[chatID].Participants {
		p := &m.infos[chatID].Participants[i]
		if p.UserID == userID {
			v := msgID
			p.LastReadMessageID = &v
			t := m.markedReadAt
			p.ReadAt = &t
		}
	}
	return msgID, m.markedReadAt, nil
}
func (m *chatRepoMock) CountUnread(chatID, userID int) (int, error)                  { return 0, nil }
func (m *chatRepoMock) SearchChats(userID int, query string) ([]*models.Chat, error) { return nil, nil }
func (m *chatRepoMock) SearchMessagesFTS(chatID, userID int, query string, limit, offset int) ([]*models.ChatMessage, error) {
	m.lastSearchMode = "fts"
	m.lastSearchLimit = limit
	m.lastSearchOffset = offset
	res := make([]*models.ChatMessage, 0, limit)
	for i := 0; i < limit; i++ {
		res = append(res, &models.ChatMessage{ID: offset + i + 1, ChatID: chatID, SenderID: userID, Text: query})
	}
	return res, nil
}
func (m *chatRepoMock) SearchMessagesILIKE(chatID, userID int, query string, limit, offset int) ([]*models.ChatMessage, error) {
	m.lastSearchMode = "ilike"
	m.lastSearchLimit = limit
	m.lastSearchOffset = offset
	res := make([]*models.ChatMessage, 0, limit)
	for i := 0; i < limit; i++ {
		res = append(res, &models.ChatMessage{ID: offset + i + 1, ChatID: chatID, SenderID: userID, Text: query})
	}
	return res, nil
}
func (m *chatRepoMock) CreateAttachment(chatID, uploaderID int, fileName, mime string, size int64, storageKey string) (*models.Attachment, error) {
	a := &models.Attachment{ID: "a1", ChatID: chatID, UploaderID: uploaderID, FileName: fileName, MimeType: mime, SizeBytes: size, StorageKey: storageKey}
	m.attachmentByID[a.ID] = a
	return a, nil
}
func (m *chatRepoMock) AttachToMessage(attachmentIDs []string, messageID, chatID, uploaderID int) error {
	m.attachedIDs = attachmentIDs
	m.attachedMessageID = messageID
	for _, id := range attachmentIDs {
		if a, ok := m.attachmentByID[id]; ok {
			a.MessageID = &messageID
		}
	}
	return nil
}
func (m *chatRepoMock) GetAttachmentsByMessageIDs(messageIDs []int) (map[int][]models.AttachmentResponse, error) {
	return map[int][]models.AttachmentResponse{}, nil
}
func (m *chatRepoMock) GetAttachmentForDownload(id string) (*models.Attachment, error) {
	if a, ok := m.attachmentByID[id]; ok {
		return a, nil
	}
	return nil, errors.New("not found")
}

func (m *chatRepoMock) EditMessage(chatID, messageID, editorUserID int, newText string) (*models.ChatMessage, error) {
	msg := &models.ChatMessage{ID: messageID, ChatID: chatID, SenderID: editorUserID, Text: newText, CreatedAt: time.Now()}
	now := time.Now()
	msg.EditedAt = &now
	m.messageStore[messageID] = msg
	return msg, nil
}
func (m *chatRepoMock) DeleteMessage(chatID, messageID, userID int) (*models.ChatMessage, error) {
	msg := &models.ChatMessage{ID: messageID, ChatID: chatID, SenderID: userID, Text: "", CreatedAt: time.Now(), IsDeleted: true}
	now := time.Now()
	msg.DeletedAt = &now
	msg.DeletedBy = &userID
	m.messageStore[messageID] = msg
	return msg, nil
}
func (m *chatRepoMock) PinMessage(chatID, messageID, userID int) (*models.PinResponse, error) {
	if m.lastPinRole == "member" {
		return nil, sql.ErrNoRows
	}
	return &models.PinResponse{MessageID: messageID, PinnedBy: userID, PinnedAt: time.Now()}, nil
}
func (m *chatRepoMock) UnpinMessage(chatID, messageID, userID int) error {
	if m.lastPinRole == "member" {
		return sql.ErrNoRows
	}
	return nil
}
func (m *chatRepoMock) FavoriteMessage(chatID, messageID, userID int) (*models.FavoriteResponse, error) {
	m.favoritesCount++
	return &models.FavoriteResponse{MessageID: messageID, CreatedAt: time.Now()}, nil
}
func (m *chatRepoMock) UnfavoriteMessage(chatID, messageID, userID int) error { return nil }
func (m *chatRepoMock) ListPins(chatID, userID, limit, offset int) ([]*models.PinResponse, error) {
	return []*models.PinResponse{}, nil
}
func (m *chatRepoMock) ListFavorites(chatID, userID, limit, offset int) ([]*models.FavoriteResponse, error) {
	return []*models.FavoriteResponse{}, nil
}

func TestCreateGroupChatCreatorIsOwner(t *testing.T) {
	repo := newChatRepoMock()
	svc := NewChatService(repo, t.TempDir())

	chat, err := svc.CreateGroupChat("team", 10, []int{11, 12})
	if err != nil {
		t.Fatalf("CreateGroupChat error: %v", err)
	}
	if repo.lastCreateCreator != 10 {
		t.Fatalf("creator passed incorrectly: got %d", repo.lastCreateCreator)
	}

	info, err := svc.GetChatInfo(chat.ID, 10)
	if err != nil {
		t.Fatalf("GetChatInfo error: %v", err)
	}
	foundOwner := false
	for _, p := range info.Participants {
		if p.UserID == 10 && p.Role == models.ChatMemberRoleOwner {
			foundOwner = true
		}
	}
	if !foundOwner {
		t.Fatalf("creator must be owner in participants")
	}
}

func TestGetChatInfoAsParticipant(t *testing.T) {
	repo := newChatRepoMock()
	svc := NewChatService(repo, t.TempDir())

	chat, err := svc.CreateGroupChat("team", 20, []int{21})
	if err != nil {
		t.Fatalf("CreateGroupChat error: %v", err)
	}

	info, err := svc.GetChatInfo(chat.ID, 21)
	if err != nil {
		t.Fatalf("GetChatInfo error: %v", err)
	}
	if len(info.Participants) == 0 {
		t.Fatalf("participants should not be empty")
	}
}

type readSeekNopCloser struct {
	*bytes.Reader
}

func (r *readSeekNopCloser) Close() error { return nil }

type fakeStorage struct {
	content map[string][]byte
}

func (f *fakeStorage) Save(_ context.Context, reader io.Reader, key string) error {
	b, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.content[key] = b
	return nil
}
func (f *fakeStorage) Open(_ context.Context, key string) (io.ReadSeekCloser, int64, error) {
	b, ok := f.content[key]
	if !ok {
		return nil, 0, errors.New("not found")
	}
	r := &readSeekNopCloser{Reader: bytes.NewReader(b)}
	return r, int64(len(b)), nil
}
func (f *fakeStorage) Delete(_ context.Context, key string) error { delete(f.content, key); return nil }

func TestSendMessageWithAttachmentIDsAssignsMessage(t *testing.T) {
	repo := newChatRepoMock()
	svc := NewChatService(repo, t.TempDir())
	_, err := svc.CreateGroupChat("team", 10, []int{11})
	if err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	repo.attachmentByID["att-1"] = &models.Attachment{ID: "att-1", ChatID: 1, UploaderID: 10}

	msg, _, err := svc.SendMessage(1, 10, "hello", nil, []string{"att-1"})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if repo.attachedMessageID != msg.ID {
		t.Fatalf("expected attached message id %d got %d", msg.ID, repo.attachedMessageID)
	}
}

func TestDownloadAttachmentMembership(t *testing.T) {
	repo := newChatRepoMock()
	svc := NewChatService(repo, t.TempDir())
	svc.storage = &fakeStorage{content: map[string][]byte{"chat/1/file.pdf": []byte("data")}}
	_, _ = svc.CreateGroupChat("team", 10, []int{11})
	repo.attachmentByID["att-1"] = &models.Attachment{ID: "att-1", ChatID: 1, FileName: "file.pdf", MimeType: "application/pdf", StorageKey: "chat/1/file.pdf"}

	_, r, _, err := svc.DownloadAttachment("att-1", 11)
	if err != nil {
		t.Fatalf("expected member download success: %v", err)
	}
	_ = r.Close()

	_, _, _, err = svc.DownloadAttachment("att-1", 99)
	if !errors.Is(err, ErrNotChatMember) {
		t.Fatalf("expected ErrNotChatMember, got %v", err)
	}
}

func TestSearchMessagesPaginationFTS(t *testing.T) {
	repo := newChatRepoMock()
	svc := NewChatService(repo, t.TempDir())
	_, _ = svc.CreateGroupChat("team", 10, []int{11})

	messages, err := svc.SearchMessages(1, 10, "hello", "fts", 10, 0)
	if err != nil {
		t.Fatalf("SearchMessages error: %v", err)
	}
	if len(messages) != 10 {
		t.Fatalf("expected 10 messages, got %d", len(messages))
	}
	if repo.lastSearchMode != "fts" || repo.lastSearchLimit != 10 || repo.lastSearchOffset != 0 {
		t.Fatalf("unexpected repo call: mode=%s limit=%d offset=%d", repo.lastSearchMode, repo.lastSearchLimit, repo.lastSearchOffset)
	}

	messages2, err := svc.SearchMessages(1, 10, "hello", "fts", 10, 10)
	if err != nil {
		t.Fatalf("SearchMessages page2 error: %v", err)
	}
	if len(messages2) != 10 {
		t.Fatalf("expected 10 messages on page2, got %d", len(messages2))
	}
	if messages[0].ID == messages2[0].ID {
		t.Fatalf("expected different offsets, got same first id %d", messages[0].ID)
	}
}

func TestSearchMessagesModeILIKE(t *testing.T) {
	repo := newChatRepoMock()
	svc := NewChatService(repo, t.TempDir())
	_, _ = svc.CreateGroupChat("team", 10, []int{11})

	_, err := svc.SearchMessages(1, 10, "hello", "ilike", 20, 0)
	if err != nil {
		t.Fatalf("SearchMessages error: %v", err)
	}
	if repo.lastSearchMode != "ilike" {
		t.Fatalf("expected ilike mode, got %s", repo.lastSearchMode)
	}
}

func TestMarkChatReadWithMessageIDReturnsEvent(t *testing.T) {
	repo := newChatRepoMock()
	svc := NewChatService(repo, t.TempDir())
	_, _ = svc.CreateGroupChat("team", 10, []int{11})
	msgID := 123

	unread, evt, err := svc.MarkChatRead(1, 10, &msgID)
	if err != nil {
		t.Fatalf("MarkChatRead error: %v", err)
	}
	if unread != 0 {
		t.Fatalf("expected unread=0 got %d", unread)
	}
	if evt == nil {
		t.Fatalf("expected read event")
	}
	if evt.LastReadMessageID != 123 {
		t.Fatalf("expected last_read_message_id=123 got %d", evt.LastReadMessageID)
	}
	if evt.Type != "chat:read" {
		t.Fatalf("unexpected event type: %s", evt.Type)
	}
}

func TestGetChatInfoReturnsReadState(t *testing.T) {
	repo := newChatRepoMock()
	svc := NewChatService(repo, t.TempDir())
	_, _ = svc.CreateGroupChat("team", 10, []int{11})
	msgID := 55
	_, _, _ = svc.MarkChatRead(1, 10, &msgID)

	info, err := svc.GetChatInfo(1, 10)
	if err != nil {
		t.Fatalf("GetChatInfo error: %v", err)
	}
	found := false
	for _, p := range info.Participants {
		if p.UserID == 10 {
			if p.LastReadMessageID == nil || *p.LastReadMessageID != 55 {
				t.Fatalf("expected participant read message id 55")
			}
			if p.ReadAt == nil {
				t.Fatalf("expected participant read_at")
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("participant not found")
	}
}

func TestEditMessageSetsEditedAt(t *testing.T) {
	repo := newChatRepoMock()
	svc := NewChatService(repo, t.TempDir())
	msg, err := svc.EditMessage(1, 100, 10, "updated")
	if err != nil {
		t.Fatalf("EditMessage: %v", err)
	}
	if msg.EditedAt == nil {
		t.Fatalf("expected edited_at")
	}
}

func TestMemberCannotPin(t *testing.T) {
	repo := newChatRepoMock()
	repo.lastPinRole = "member"
	svc := NewChatService(repo, t.TempDir())
	if _, err := svc.PinMessage(1, 100, 10); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestOwnerCanPinUnpin(t *testing.T) {
	repo := newChatRepoMock()
	repo.lastPinRole = "owner"
	svc := NewChatService(repo, t.TempDir())
	if _, err := svc.PinMessage(1, 100, 10); err != nil {
		t.Fatalf("pin: %v", err)
	}
	if err := svc.UnpinMessage(1, 100, 10); err != nil {
		t.Fatalf("unpin: %v", err)
	}
}

func TestSenderDeleteMessageMarksDeleted(t *testing.T) {
	repo := newChatRepoMock()
	svc := NewChatService(repo, t.TempDir())
	msg, err := svc.DeleteMessage(1, 100, 10)
	if err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if !msg.IsDeleted || msg.DeletedAt == nil {
		t.Fatalf("expected deleted message")
	}
}

func TestFavoritesUpsertLikeBehavior(t *testing.T) {
	repo := newChatRepoMock()
	svc := NewChatService(repo, t.TempDir())
	if _, err := svc.FavoriteMessage(1, 100, 10); err != nil {
		t.Fatalf("favorite1: %v", err)
	}
	if _, err := svc.FavoriteMessage(1, 100, 10); err != nil {
		t.Fatalf("favorite2: %v", err)
	}
	if repo.favoritesCount != 2 {
		t.Fatalf("expected 2 upsert attempts")
	}
}
