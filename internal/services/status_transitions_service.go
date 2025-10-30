package services

// Допустимые переходы статусов.
// NB: у тебя ConvertLeadToDeal требует lead.status == "confirmed".
var LeadTransitions = map[string]map[string]bool{
	"new":       {"in_review": true, "rejected": true, "confirmed": true},
	"in_review": {"confirmed": true, "rejected": true},
	"confirmed": {"rejected": true}, // перевод в "converted" делает отдельный /convert
	"recycled":  {"in_review": true},
	"rejected":  {},
	"converted": {}, // финалка; выставляется ConvertLeadToDeal
}

var DealTransitions = map[string]map[string]bool{
	"new":         {"in_progress": true, "cancelled": true},
	"in_progress": {"negotiation": true, "cancelled": true},
	"negotiation": {"won": true, "lost": true, "cancelled": true},
	"won":         {},
	"lost":        {},
	"cancelled":   {},
}

func canTransition(current, to string, table map[string]map[string]bool) bool {
	if current == "" {
		// если в БД пусто — разрешим перейти в любую стартовую (new/*)
		return true
	}
	nexts, ok := table[current]
	if !ok {
		return false
	}
	return nexts[to]
}
