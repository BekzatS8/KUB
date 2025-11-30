package realtime

import (
	"sync"

	"turcompany/internal/models"
)

type ChatHub struct {
	mu    sync.RWMutex
	chats map[int]map[*Conn]struct{}
}

func NewChatHub() *ChatHub {
	return &ChatHub{
		chats: make(map[int]map[*Conn]struct{}),
	}
}

func (h *ChatHub) Register(chatID int, conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.chats[chatID] == nil {
		h.chats[chatID] = make(map[*Conn]struct{})
	}
	h.chats[chatID][conn] = struct{}{}
}

func (h *ChatHub) Unregister(chatID int, conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.chats[chatID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.chats, chatID)
		}
	}
	_ = conn.Close()
}

func (h *ChatHub) Broadcast(msg *models.ChatMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns := h.chats[msg.ChatID]
	for conn := range conns {
		_ = conn.WriteJSON(msg)
	}
}
