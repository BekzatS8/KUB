package services

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

// CaptchaService — лёгкая капча на вход (замечания заказчика 10.11.2026:
// «защита от ботов для взлома»). Своя, без внешних сервисов и ключей:
// картинка SVG с искажёнными цифрами и шумом, ответ хранится в памяти
// сервера, одноразовый, живёт 5 минут. После перезапуска API старые капчи
// недействительны — страница входа просто покажет новую.
type CaptchaService struct {
	mu    sync.Mutex
	items map[string]captchaItem
	ttl   time.Duration
	now   func() time.Time
}

type captchaItem struct {
	answer  string
	expires time.Time
}

const (
	captchaLength   = 4
	captchaMaxItems = 20000
	captchaDigits   = "23456789" // без 0/1 — их легко спутать с О/l
)

func NewCaptchaService() *CaptchaService {
	return &CaptchaService{items: map[string]captchaItem{}, ttl: 5 * time.Minute, now: time.Now}
}

func randInt(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

// New выдаёт новую капчу: id и картинку (data:image/svg+xml;base64,...).
func (s *CaptchaService) New() (string, string) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	var answer strings.Builder
	for i := 0; i < captchaLength; i++ {
		answer.WriteByte(captchaDigits[randInt(len(captchaDigits))])
	}
	now := s.now()
	s.mu.Lock()
	if len(s.items) >= captchaMaxItems {
		for k, v := range s.items {
			if now.After(v.expires) {
				delete(s.items, k)
			}
		}
		// Всё ещё переполнено (поток запросов) — выбрасываем произвольные.
		for k := range s.items {
			if len(s.items) < captchaMaxItems {
				break
			}
			delete(s.items, k)
		}
	}
	s.items[id] = captchaItem{answer: answer.String(), expires: now.Add(s.ttl)}
	s.mu.Unlock()
	return id, "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(captchaSVG(answer.String())))
}

// Verify проверяет ответ; капча одноразовая — после проверки удаляется.
func (s *CaptchaService) Verify(id, answer string) bool {
	id = strings.TrimSpace(id)
	answer = strings.TrimSpace(answer)
	if id == "" || answer == "" {
		return false
	}
	s.mu.Lock()
	item, ok := s.items[id]
	delete(s.items, id)
	s.mu.Unlock()
	if !ok || s.now().After(item.expires) {
		return false
	}
	return item.answer == answer
}

// captchaSVG рисует цифры с поворотом, смещением и шумом.
func captchaSVG(text string) string {
	const w, h = 160, 56
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, w, h, w, h)
	b.WriteString(`<rect width="100%" height="100%" fill="#f1f5f9"/>`)
	colors := []string{"#1e3a8a", "#7c2d12", "#14532d", "#581c87", "#334155"}
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&b, `<path d="M%d %d Q%d %d %d %d" stroke="%s" stroke-width="%d" fill="none" opacity="0.5"/>`,
			randInt(30), randInt(h), randInt(w), randInt(h), w-randInt(30), randInt(h), colors[randInt(len(colors))], 1+randInt(2))
	}
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%d" fill="%s" opacity="0.4"/>`, randInt(w), randInt(h), 1+randInt(2), colors[randInt(len(colors))])
	}
	step := (w - 20) / len(text)
	for i, ch := range text {
		x := 18 + i*step + randInt(10)
		y := 36 + randInt(12) - 6
		rot := randInt(50) - 25
		size := 26 + randInt(8)
		fmt.Fprintf(&b, `<text x="%d" y="%d" font-family="Verdana,Arial,sans-serif" font-size="%d" font-weight="bold" fill="%s" transform="rotate(%d %d %d)">%c</text>`,
			x, y, size, colors[randInt(len(colors))], rot, x, y, ch)
	}
	for i := 0; i < 2; i++ {
		fmt.Fprintf(&b, `<line x1="0" y1="%d" x2="%d" y2="%d" stroke="#64748b" stroke-width="1.5" opacity="0.6"/>`, randInt(h), w, randInt(h))
	}
	b.WriteString(`</svg>`)
	return b.String()
}
