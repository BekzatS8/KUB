package realtime

import "log"

// BoardHub рассылает подписчикам канбан-доски сигнал «в воронке что-то
// изменилось». По сокету НЕ передаются данные карточек — только funnel_id;
// клиент по сигналу перечитывает доску через REST (getFunnelBoard сам режет
// выдачу по филиалу/отделу зрителя). Так real-time есть, а утечки чужих
// карточек между филиалами нет. Комната = funnelID (по образцу ChatHub).
type BoardHub struct {
	rooms      map[int]map[*Conn]struct{}
	register   chan boardSub
	unregister chan boardSub
	broadcast  chan int
	stop       chan struct{}
}

type boardSub struct {
	funnelID int
	conn     *Conn
}

func NewBoardHub() *BoardHub {
	return &BoardHub{
		rooms:      make(map[int]map[*Conn]struct{}),
		register:   make(chan boardSub, 64),
		unregister: make(chan boardSub, 64),
		broadcast:  make(chan int, 256),
		stop:       make(chan struct{}),
	}
}

// Run запускает цикл хаба; вызывать в отдельной горутине.
func (h *BoardHub) Run() {
	for {
		select {
		case s := <-h.register:
			if h.rooms[s.funnelID] == nil {
				h.rooms[s.funnelID] = make(map[*Conn]struct{})
			}
			h.rooms[s.funnelID][s.conn] = struct{}{}
		case s := <-h.unregister:
			if conns, ok := h.rooms[s.funnelID]; ok {
				delete(conns, s.conn)
				if len(conns) == 0 {
					delete(h.rooms, s.funnelID)
				}
			}
			_ = s.conn.Close()
		case funnelID := <-h.broadcast:
			h.handleBroadcast(funnelID)
		case <-h.stop:
			h.shutdown()
			return
		}
	}
}

func (h *BoardHub) Stop() { close(h.stop) }

func (h *BoardHub) Register(funnelID int, conn *Conn)   { h.register <- boardSub{funnelID, conn} }
func (h *BoardHub) Unregister(funnelID int, conn *Conn) { h.unregister <- boardSub{funnelID, conn} }

// NotifyBoardChanged сообщает подписчикам воронки, что доску нужно перечитать.
// Неблокирующая отправка: если очередь переполнена — пропускаем (поллинг на
// клиенте подстрахует), чтобы никогда не тормозить бизнес-логику переноса.
func (h *BoardHub) NotifyBoardChanged(funnelID int) {
	if funnelID <= 0 {
		return
	}
	select {
	case h.broadcast <- funnelID:
	default:
	}
}

func (h *BoardHub) handleBroadcast(funnelID int) {
	conns := h.rooms[funnelID]
	if len(conns) == 0 {
		return
	}
	payload := struct {
		Type     string `json:"type"`
		FunnelID int    `json:"funnel_id"`
	}{Type: "board_changed", FunnelID: funnelID}
	for conn := range conns {
		if err := conn.WriteJSON(payload); err != nil {
			log.Printf("[board_hub] write failed for funnel %d: %v", funnelID, err)
			h.unregister <- boardSub{funnelID: funnelID, conn: conn}
		}
	}
}

func (h *BoardHub) shutdown() {
	for funnelID, conns := range h.rooms {
		for conn := range conns {
			_ = conn.Close()
		}
		delete(h.rooms, funnelID)
	}
}
