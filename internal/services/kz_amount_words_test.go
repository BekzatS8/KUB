package services

import "testing"

// Казахская часть двуязычных договоров должна содержать сумму прописью
// по-казахски (обратная связь 16.07.2026).
func TestBuildKZTextPlaceholdersForContractAmounts(t *testing.T) {
	ph := map[string]string{
		"TOTAL_AMOUNT_NUM": "450000",
		"PREPAYMENT_NUM":   "225000",
	}
	buildKZTextPlaceholders(ph)

	if got := ph["TOTAL_AMOUNT_WORDS_KZ"]; got == "" {
		t.Fatal("TOTAL_AMOUNT_WORDS_KZ пуст")
	} else {
		t.Logf("450000 → %q", got)
	}
	if got := ph["PREPAYMENT_WORDS_KZ"]; got == "" {
		t.Fatal("PREPAYMENT_WORDS_KZ пуст")
	} else {
		t.Logf("225000 → %q", got)
	}
}

func TestAmountToKzWords(t *testing.T) {
	cases := []int64{0, 1, 15, 100, 1000, 225000, 450000, 1600000}
	for _, n := range cases {
		t.Logf("%9d → %s", n, amountToKzWords(n, 0))
	}
}

// Регрессия: PREPAYMENT_NUM появляется только в applyUKABYPlaceholders, а
// buildKZTextPlaceholders отрабатывает раньше — на пустом значении. Если
// пересчёт в конце applyUKABYPlaceholders потерять, казахская часть договора
// уйдёт с пустой предоплатой и упадёт на strict-проверке.
func TestUKABYPlaceholdersFillKZPrepaymentWords(t *testing.T) {
	ph := map[string]string{
		"DEAL_PREPAY_KZT":      "225000",
		"DEAL_PREPAY_KZT_TEXT": "Двести двадцать пять тысяч",
		"DEAL_TOTAL_KZT":       "450000",
	}
	applyUKABYPlaceholders("contract_paid_50_50_ru", ph)

	if ph["PREPAYMENT_NUM"] == "" {
		t.Fatal("PREPAYMENT_NUM не заполнен")
	}
	if got := ph["PREPAYMENT_WORDS_KZ"]; got == "" {
		t.Fatal("PREPAYMENT_WORDS_KZ пуст — казахская предоплата не посчиталась")
	} else {
		t.Logf("предоплата 225000 → каз. %q", got)
	}
}

// Уже заполненный _KZ ключ не должен затираться расчётным.
func TestBuildKZTextPlaceholdersKeepsExplicitValue(t *testing.T) {
	ph := map[string]string{
		"TOTAL_AMOUNT_NUM":      "450000",
		"TOTAL_AMOUNT_WORDS_KZ": "қолмен енгізілген",
	}
	buildKZTextPlaceholders(ph)
	if ph["TOTAL_AMOUNT_WORDS_KZ"] != "қолмен енгізілген" {
		t.Fatalf("явное значение затёрто: %q", ph["TOTAL_AMOUNT_WORDS_KZ"])
	}
}
