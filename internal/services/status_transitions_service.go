package services

// Допустимые переходы статусов ЛИДА
// ключ: текущий статус → карта допустимых целевых статусов
var LeadTransitions = map[string]map[string]bool{
	"new": {
		"in_progress": true,
		"confirmed":   true,
		"cancelled":   true,
	},
	"in_progress": {
		"confirmed": true,
		"cancelled": true,
	},
	"confirmed": {
		// "converted" делаем только через /leads/:id/convert
		"cancelled": true,
	},
	"converted": {},
	"cancelled": {},
}

// Допустимые переходы статусов СДЕЛКИ
// (пример, если у тебя уже был свой — можешь расширить)
var DealTransitions = map[string]map[string]bool{
	"new": {
		"in_progress": true,
		"negotiation": true,
		"won":         true,
		"lost":        true,
		"cancelled":   true,
	},
	"in_progress": {
		"negotiation": true,
		"won":         true,
		"lost":        true,
		"cancelled":   true,
	},
	"negotiation": {
		"won":       true,
		"lost":      true,
		"cancelled": true,
	},
	"won":       {},
	"lost":      {},
	"cancelled": {},
}

// Общая функция проверки перехода статуса
// current — текущий статус, to — целевой, transitions — карта допустимых переходов
func canTransition(current, to string, transitions map[string]map[string]bool) bool {
	if current == to {
		return true
	}
	next, ok := transitions[current]
	if !ok {
		return false
	}
	return next[to]
}
