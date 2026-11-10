package services

import (
	"strings"
	"testing"
	"time"
)

func TestCaptchaVerify(t *testing.T) {
	s := NewCaptchaService()
	id, img := s.New()
	if !strings.HasPrefix(img, "data:image/svg+xml;base64,") || id == "" {
		t.Fatalf("bad captcha: %q %q", id, img[:30])
	}
	answer := s.items[id].answer
	if len(answer) != captchaLength {
		t.Fatalf("answer length %d", len(answer))
	}
	if s.Verify(id, "0000") {
		t.Fatal("wrong answer accepted")
	}
	// после неверной попытки капча сгорает — перебор невозможен
	if s.Verify(id, answer) {
		t.Fatal("captcha must be one-time")
	}
	id2, _ := s.New()
	if !s.Verify(id2, " "+s.items[id2].answer+" ") {
		t.Fatal("correct answer rejected")
	}
	id3, _ := s.New()
	ans3 := s.items[id3].answer
	s.now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	if s.Verify(id3, ans3) {
		t.Fatal("expired captcha accepted")
	}
}
