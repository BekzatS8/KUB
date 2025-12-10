package realtime

import (
	"sync"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type ChatHub struct {
	mu    sync.RWMutex
	chats map[int]map[int]map[*Conn]struct{}
	repo  repositories.ChatRepository
}

func NewChatHub(repo repositories.ChatRepository) *ChatHub {
	return &ChatHub{
		repo:  repo,
		chats: make(map[int]map[int]map[*Conn]struct{}),
	}
}

func (h *ChatHub) Register(chatID int, userID int, conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.chats[chatID] == nil {
		h.chats[chatID] = make(map[int]map[*Conn]struct{})
	}
	if h.chats[chatID][userID] == nil {
		h.chats[chatID][userID] = make(map[*Conn]struct{})
	}
	h.chats[chatID][userID][conn] = struct{}{}
	_ = h.repo.SetOnline(userID, true)
}

func (h *ChatHub) Unregister(chatID int, userID int, conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.chats[chatID]; ok {
		if userConns, ok := conns[userID]; ok {
			delete(userConns, conn)
			if len(userConns) == 0 {
				delete(conns, userID)
			}
		}
		if len(conns) == 0 {
			delete(h.chats, chatID)
		}
	}
	_ = conn.Close()
	_ = h.repo.SetOnline(userID, false)
}

func (h *ChatHub) Broadcast(msg *models.ChatMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns := h.chats[msg.ChatID]
	for _, userConns := range conns {
		for conn := range userConns {
			_ = conn.WriteJSON(msg)
		}
	}
}

func (h *ChatHub) NotifyUnread(chatID int, userID int, unreadCount int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns := h.chats[chatID][userID]
	if len(conns) == 0 {
		return
	}
	payload := struct {
		Type        string `json:"type"`
		ChatID      int    `json:"chat_id"`
		UnreadCount int    `json:"unread_count"`
	}{Type: "unread", ChatID: chatID, UnreadCount: unreadCount}
	for conn := range conns {
		_ = conn.WriteJSON(payload)
	}
}
