package realtime

import (
	"log"
	"sync"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type presence struct {
	Online   bool
	LastSeen time.Time
}

type subscription struct {
	chatID int
	userID int
	conn   *Conn
}

type unreadNotification struct {
	chatID int
	userID int
	unread int
}

type readNotification struct {
	event models.ChatReadEvent
}

type chatEventNotification struct {
	chatID  int
	payload interface{}
}

type ChatHub struct {
	chats        map[int]map[int]map[*Conn]struct{}
	repo         repositories.ChatRepository
	register     chan subscription
	unregister   chan subscription
	broadcast    chan *models.ChatMessage
	notifyUnread chan unreadNotification
	notifyRead   chan readNotification
	notifyEvent  chan chatEventNotification
	stop         chan struct{}

	presence   map[int]presence
	presenceMu sync.RWMutex
}

func NewChatHub(repo repositories.ChatRepository) *ChatHub {
	return &ChatHub{
		repo:         repo,
		chats:        make(map[int]map[int]map[*Conn]struct{}),
		register:     make(chan subscription, 64),
		unregister:   make(chan subscription, 64),
		broadcast:    make(chan *models.ChatMessage, 128),
		notifyUnread: make(chan unreadNotification, 128),
		notifyRead:   make(chan readNotification, 128),
		notifyEvent:  make(chan chatEventNotification, 128),
		stop:         make(chan struct{}),
		presence:     make(map[int]presence),
	}
}

// Run starts the hub event loop. Should be launched in a dedicated goroutine.
func (h *ChatHub) Run() {
	for {
		select {
		case sub := <-h.register:
			h.handleRegister(sub)
		case sub := <-h.unregister:
			h.handleUnregister(sub)
		case msg := <-h.broadcast:
			h.handleBroadcast(msg)
		case unread := <-h.notifyUnread:
			h.handleNotifyUnread(unread)
		case read := <-h.notifyRead:
			h.handleNotifyRead(read)
		case evt := <-h.notifyEvent:
			h.handleNotifyEvent(evt)
		case <-h.stop:
			h.shutdown()
			return
		}
	}
}

func (h *ChatHub) Stop() {
	close(h.stop)
}

func (h *ChatHub) Register(chatID int, userID int, conn *Conn) {
	h.register <- subscription{chatID: chatID, userID: userID, conn: conn}
}

func (h *ChatHub) Unregister(chatID int, userID int, conn *Conn) {
	h.unregister <- subscription{chatID: chatID, userID: userID, conn: conn}
}

func (h *ChatHub) Broadcast(msg *models.ChatMessage) {
	if msg == nil {
		return
	}
	h.broadcast <- msg
}

func (h *ChatHub) NotifyUnread(chatID int, userID int, unreadCount int) {
	h.notifyUnread <- unreadNotification{chatID: chatID, userID: userID, unread: unreadCount}
}

func (h *ChatHub) NotifyRead(event models.ChatReadEvent) {
	h.notifyRead <- readNotification{event: event}
}

func (h *ChatHub) NotifyMessageUpdated(chatID int, msg *models.ChatMessage) {
	if msg == nil {
		return
	}
	h.notifyEvent <- chatEventNotification{chatID: chatID, payload: struct {
		Type    string              `json:"type"`
		ChatID  int                 `json:"chat_id"`
		Message *models.ChatMessage `json:"message"`
	}{Type: "message:updated", ChatID: chatID, Message: msg}}
}

func (h *ChatHub) NotifyMessageDeleted(chatID, messageID, deletedBy int, deletedAt time.Time) {
	h.notifyEvent <- chatEventNotification{chatID: chatID, payload: struct {
		Type      string    `json:"type"`
		ChatID    int       `json:"chat_id"`
		MessageID int       `json:"message_id"`
		DeletedAt time.Time `json:"deleted_at"`
		DeletedBy int       `json:"deleted_by"`
	}{Type: "message:deleted", ChatID: chatID, MessageID: messageID, DeletedAt: deletedAt, DeletedBy: deletedBy}}
}

func (h *ChatHub) NotifyMessagePinned(chatID int, p *models.PinResponse) {
	if p == nil {
		return
	}
	h.notifyEvent <- chatEventNotification{chatID: chatID, payload: struct {
		Type      string    `json:"type"`
		ChatID    int       `json:"chat_id"`
		MessageID int       `json:"message_id"`
		PinnedAt  time.Time `json:"pinned_at"`
		PinnedBy  int       `json:"pinned_by"`
	}{Type: "message:pinned", ChatID: chatID, MessageID: p.MessageID, PinnedAt: p.PinnedAt, PinnedBy: p.PinnedBy}}
}

func (h *ChatHub) NotifyMessageUnpinned(chatID, messageID int) {
	h.notifyEvent <- chatEventNotification{chatID: chatID, payload: struct {
		Type      string `json:"type"`
		ChatID    int    `json:"chat_id"`
		MessageID int    `json:"message_id"`
	}{Type: "message:unpinned", ChatID: chatID, MessageID: messageID}}
}

func (h *ChatHub) handleRegister(sub subscription) {
	if h.chats[sub.chatID] == nil {
		h.chats[sub.chatID] = make(map[int]map[*Conn]struct{})
	}
	if h.chats[sub.chatID][sub.userID] == nil {
		h.chats[sub.chatID][sub.userID] = make(map[*Conn]struct{})
	}
	h.chats[sub.chatID][sub.userID][sub.conn] = struct{}{}
	h.setPresence(sub.userID, true)
	if err := h.repo.SetOnline(sub.userID, true); err != nil {
		log.Printf("[chat_hub] failed to set user %d online: %v", sub.userID, err)
	}
}

func (h *ChatHub) handleUnregister(sub subscription) {
	if conns, ok := h.chats[sub.chatID]; ok {
		if userConns, ok := conns[sub.userID]; ok {
			if _, exists := userConns[sub.conn]; exists {
				delete(userConns, sub.conn)
			}
			stillOnline := len(userConns) > 0
			if len(userConns) == 0 {
				delete(conns, sub.userID)
			}
			h.setPresence(sub.userID, stillOnline)
			if !stillOnline {
				if err := h.repo.SetOnline(sub.userID, false); err != nil {
					log.Printf("[chat_hub] failed to set user %d offline: %v", sub.userID, err)
				}
			}
		}
		if len(conns) == 0 {
			delete(h.chats, sub.chatID)
		}
	}
	if err := sub.conn.Close(); err != nil {
		log.Printf("[chat_hub] error closing websocket: %v", err)
	}
}

func (h *ChatHub) handleBroadcast(msg *models.ChatMessage) {
	conns := h.chats[msg.ChatID]
	for userID, userConns := range conns {
		for conn := range userConns {
			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("[chat_hub] failed to write message to user %d: %v", userID, err)
				h.unregister <- subscription{chatID: msg.ChatID, userID: userID, conn: conn}
			}
		}
	}
}

func (h *ChatHub) handleNotifyUnread(unread unreadNotification) {
	chatConns, ok := h.chats[unread.chatID]
	if !ok {
		return
	}
	conns := chatConns[unread.userID]
	if len(conns) == 0 {
		return
	}
	payload := struct {
		Type        string `json:"type"`
		ChatID      int    `json:"chat_id"`
		UnreadCount int    `json:"unread_count"`
	}{Type: "unread", ChatID: unread.chatID, UnreadCount: unread.unread}
	for conn := range conns {
		if err := conn.WriteJSON(payload); err != nil {
			log.Printf("[chat_hub] failed to notify unread to user %d: %v", unread.userID, err)
			h.unregister <- subscription{chatID: unread.chatID, userID: unread.userID, conn: conn}
		}
	}
}

func (h *ChatHub) handleNotifyRead(read readNotification) {
	chatConns, ok := h.chats[read.event.ChatID]
	if !ok {
		return
	}
	for userID, conns := range chatConns {
		for conn := range conns {
			if err := conn.WriteJSON(read.event); err != nil {
				log.Printf("[chat_hub] failed to notify read to user %d: %v", userID, err)
				h.unregister <- subscription{chatID: read.event.ChatID, userID: userID, conn: conn}
			}
		}
	}
}

func (h *ChatHub) handleNotifyEvent(evt chatEventNotification) {
	chatConns, ok := h.chats[evt.chatID]
	if !ok {
		return
	}
	for userID, conns := range chatConns {
		for conn := range conns {
			if err := conn.WriteJSON(evt.payload); err != nil {
				log.Printf("[chat_hub] failed to notify event to user %d: %v", userID, err)
				h.unregister <- subscription{chatID: evt.chatID, userID: userID, conn: conn}
			}
		}
	}
}

func (h *ChatHub) shutdown() {
	for chatID, userConns := range h.chats {
		for userID, conns := range userConns {
			for conn := range conns {
				if err := conn.Close(); err != nil {
					log.Printf("[chat_hub] error closing connection for user %d: %v", userID, err)
				}
			}
			h.setPresence(userID, false)
			if err := h.repo.SetOnline(userID, false); err != nil {
				log.Printf("[chat_hub] failed to persist offline for user %d: %v", userID, err)
			}
		}
		delete(h.chats, chatID)
	}
}

// PresenceSnapshot returns online status for provided users.
func (h *ChatHub) PresenceSnapshot(userIDs []int) map[int]presence {
	h.presenceMu.RLock()
	defer h.presenceMu.RUnlock()
	result := make(map[int]presence, len(userIDs))
	for _, id := range userIDs {
		if p, ok := h.presence[id]; ok {
			result[id] = p
		}
	}
	return result
}

func (h *ChatHub) setPresence(userID int, online bool) {
	h.presenceMu.Lock()
	defer h.presenceMu.Unlock()
	h.presence[userID] = presence{Online: online, LastSeen: time.Now()}
}
