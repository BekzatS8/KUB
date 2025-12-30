package services

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"turcompany/internal/models"
)

func buildClientPlaceholders(
	client *models.Client,
	deal *models.Deals,
	extra map[string]string,
	now time.Time,
) map[string]string {

	ph := make(map[string]string)

	// ================== КЛИЕНТ ==================
	if client != nil {
		ph["CLIENT_ID"] = strconv.Itoa(client.ID)
	}

	// --- ФИО ---
	var (
		lastName   string
		firstName  string
		middleName string
		fullName   string
	)

	if client != nil {
		// если уже есть раздельные поля — используем их
		lastName = strings.TrimSpace(client.LastName)
		firstName = strings.TrimSpace(client.FirstName)
		middleName = strings.TrimSpace(client.MiddleName)

		if lastName != "" || firstName != "" {
			fullName = strings.TrimSpace(strings.Join(
				[]string{lastName, firstName, middleName},
				" ",
			))
		} else {
			// fallback: старое поле Name
			fullName = strings.TrimSpace(client.Name)
			// на случай, если Name "Фамилия Имя Отчество" — попробуем разложить
			l2, f2, m2 := splitFIO(fullName)
			if lastName == "" {
				lastName = l2
			}
			if firstName == "" {
				firstName = f2
			}
			if middleName == "" {
				middleName = m2
			}
		}
	}

	ph["CLIENT_FULL_NAME"] = fullName
	if lastName != "" {
		ph["CLIENT_LAST_NAME"] = lastName
	}
	if firstName != "" {
		ph["CLIENT_FIRST_NAME"] = firstName
	}
	if middleName != "" {
		ph["CLIENT_MIDDLE_NAME"] = middleName
	}

	// Фамилия И.О. (для подписи)
	if lastName != "" && firstName != "" {
		firstRune := []rune(firstName)
		middleRune := []rune(middleName)

		initials := string(firstRune[0]) + "."
		if len(middleRune) > 0 {
			initials += string(middleRune[0]) + "."
		}
		ph["CLIENT_LAST_NAME_INITIALS"] = fmt.Sprintf("%s %s", lastName, initials) // Иванов И.И.
	}

	// --- ИИН / БИН ---
	if client != nil {
		iin := strings.TrimSpace(client.IIN)
		if iin == "" {
			iin = strings.TrimSpace(client.BinIin)
		}
		ph["CLIENT_IIN"] = iin
		ph["CLIENT_BIN_IIN"] = strings.TrimSpace(client.BinIin) // если где-то нужен именно BIN_IIN
	}

	// --- ПАСПОРТ / УДОСТОВЕРЕНИЕ ---
	if client != nil {
		ph["CLIENT_ID_NUMBER"] = strings.TrimSpace(client.IDNumber) // номер удост-ния
		ph["CLIENT_PASSPORT_SERIES"] = strings.TrimSpace(client.PassportSeries)
		ph["CLIENT_PASSPORT_NUMBER"] = strings.TrimSpace(client.PassportNumber)
	}

	// --- ТЕЛЕФОН / EMAIL ---
	if client != nil {
		// приоритет: новые поля
		if client.Phone != "" {
			ph["CLIENT_PHONE"] = strings.TrimSpace(client.Phone)
		}
		if client.Email != "" {
			ph["CLIENT_EMAIL"] = strings.TrimSpace(client.Email)
		}

		// fallback из contact_info
		ph["CLIENT_CONTACT_INFO"] = strings.TrimSpace(client.ContactInfo)
		if ph["CLIENT_PHONE"] == "" || ph["CLIENT_EMAIL"] == "" {
			phone, email := splitContactInfo(client.ContactInfo)
			if ph["CLIENT_PHONE"] == "" && phone != "" {
				ph["CLIENT_PHONE"] = phone
			}
			if ph["CLIENT_EMAIL"] == "" && email != "" {
				ph["CLIENT_EMAIL"] = email
			}
		}
	}

	// --- АДРЕСА ---
	if client != nil {
		// Основной адрес (для простых шаблонов)
		addr := strings.TrimSpace(client.Address)
		if addr == "" {
			// если старое поле пустое — используем фактический или рег.
			if client.ActualAddress != "" {
				addr = strings.TrimSpace(client.ActualAddress)
			} else if client.RegistrationAddress != "" {
				addr = strings.TrimSpace(client.RegistrationAddress)
			}
		}
		ph["CLIENT_ADDRESS"] = addr

		// Более точные поля
		ph["CLIENT_REG_ADDRESS"] = strings.TrimSpace(client.RegistrationAddress)
		ph["CLIENT_FACT_ADDRESS"] = strings.TrimSpace(client.ActualAddress)
	}

	// ================== ДАТЫ ДОКУМЕНТА ==================
	// базовая дата документа — "сейчас"
	ph["DOC_DATE"] = now.Format("02.01.2006")                // 07.12.2025
	ph["DOC_DATE_DAY"] = now.Format("02")                    // "07"
	ph["DOC_DATE_MONTH_NUM"] = now.Format("01")              // "12"
	ph["DOC_DATE_YEAR"] = now.Format("2006")                 // "2025"
	ph["DOC_DATE_MONTH_TEXT"] = ruMonthGenitive(now.Month()) // "декабря"

	ph["DOC_DATE_TEXT"] = fmt.Sprintf("%d %s %d г.",
		now.Day(),
		ruMonthGenitive(now.Month()),
		now.Year(),
	) // "7 декабря 2025 г."

	// Для контрактов — по умолчанию использовать DOC_DATE
	if _, ok := ph["CONTRACT_DATE"]; !ok {
		ph["CONTRACT_DATE"] = ph["DOC_DATE_TEXT"] // "7 декабря 2025 г."
	}
	if _, ok := ph["CONTRACT_DATE_RAW"]; !ok {
		ph["CONTRACT_DATE_RAW"] = ph["DOC_DATE"] // "07.12.2025"
	}
	if _, ok := ph["CONTRACT_YEAR"]; !ok {
		ph["CONTRACT_YEAR"] = ph["DOC_DATE_YEAR"] // "2025"
	}

	// ================== СДЕЛКА ==================
	if deal != nil {
		amountStr := strconv.FormatFloat(deal.Amount, 'f', 2, 64)
		ph["DEAL_ID"] = strconv.Itoa(deal.ID)
		ph["DEAL_AMOUNT_NUM"] = strings.TrimSpace(amountStr)
		ph["DEAL_CURRENCY"] = strings.TrimSpace(deal.Currency)
		ph["DEAL_STATUS"] = strings.TrimSpace(deal.Status)
		ph["DEAL_DATE"] = deal.CreatedAt.Format("02.01.2006")

		// Номер договора по умолчанию
		if _, ok := ph["CONTRACT_NUMBER"]; !ok {
			ph["CONTRACT_NUMBER"] = fmt.Sprintf("KUB-%06d", deal.ID)
		}

		// Общая сумма договора по умолчанию = сумма сделки
		if _, ok := ph["TOTAL_AMOUNT_NUM"]; !ok {
			ph["TOTAL_AMOUNT_NUM"] = strings.TrimSpace(amountStr)
		}
	}

	// ================== ОБЩИЕ СУММЫ ==================
	if numStr, ok := ph["TOTAL_AMOUNT_NUM"]; ok && numStr != "" {
		if n, err := parseAmountToInt(numStr); err == nil {
			// красиво отформатированное число с пробелами
			ph["TOTAL_AMOUNT_NUM_SPACED"] = formatAmountWithSpaces(n)

			// если текст ещё не задан извне — генерируем
			if _, ok2 := ph["TOTAL_AMOUNT_TEXT"]; !ok2 {
				ph["TOTAL_AMOUNT_TEXT"] = numToRuWordsInt(n)
			}
		} else {
			// если не смогли распарсить — хотя бы дубль числа
			if _, ok2 := ph["TOTAL_AMOUNT_TEXT"]; !ok2 {
				ph["TOTAL_AMOUNT_TEXT"] = numStr
			}
		}
	} else {
		// нет TOTAL_AMOUNT_NUM — ничего не трогаем
		if _, ok := ph["TOTAL_AMOUNT_TEXT"]; !ok {
			ph["TOTAL_AMOUNT_TEXT"] = ""
		}
	}

	// PREPAY_AMOUNT_TEXT — всё так же
	if _, ok := ph["PREPAY_AMOUNT_TEXT"]; !ok {
		if v, ok2 := ph["PREPAY_AMOUNT_NUM"]; ok2 {
			ph["PREPAY_AMOUNT_TEXT"] = v
		}
	}

	// ================== EXTRA ПЕРЕКРЫВАЕТ ВСЁ ==================
	// всё, что передал сервис/handler, имеет приоритет
	for k, v := range extra {
		ph[k] = v
	}

	return ph
}

// splitFIO пытается разложить "Фамилия Имя Отчество" на части
func splitFIO(fullName string) (last, first, middle string) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return "", "", ""
	}
	parts := strings.Fields(fullName)
	switch len(parts) {
	case 1:
		return parts[0], "", ""
	case 2:
		return parts[0], parts[1], ""
	default:
		last = parts[0]
		first = parts[1]
		middle = strings.Join(parts[2:], " ")
		return
	}
}

// splitContactInfo — достаёт телефон и email из строки вида
// "+7700..., client@example.com" или "+7700...; client@example.com"
func splitContactInfo(s string) (phone, email string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}

	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '/'
	})
	if len(parts) > 0 {
		phone = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		email = strings.TrimSpace(parts[1])
	}
	return
}

// ruMonthGenitive — название месяца в родительном падеже ("декабря")
func ruMonthGenitive(m time.Month) string {
	switch m {
	case time.January:
		return "января"
	case time.February:
		return "февраля"
	case time.March:
		return "марта"
	case time.April:
		return "апреля"
	case time.May:
		return "мая"
	case time.June:
		return "июня"
	case time.July:
		return "июля"
	case time.August:
		return "августа"
	case time.September:
		return "сентября"
	case time.October:
		return "октября"
	case time.November:
		return "ноября"
	case time.December:
		return "декабря"
	default:
		return ""
	}
}

// parseAmountToInt убирает пробелы и нецифры и парсит в int64
func parseAmountToInt(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty amount")
	}

	var digits strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
		}
	}

	if digits.Len() == 0 {
		return 0, fmt.Errorf("no digits in amount: %q", s)
	}

	return strconv.ParseInt(digits.String(), 10, 64)
}

// formatAmountWithSpaces: "1600000" -> "1 600 000"
func formatAmountWithSpaces(n int64) string {
	s := strconv.FormatInt(n, 10)
	l := len(s)
	if l <= 3 {
		return s
	}
	var parts []string
	for l > 3 {
		parts = append([]string{s[l-3:]}, parts...) // prepend
		l -= 3
	}
	parts = append([]string{s[:l]}, parts...)
	return strings.Join(parts, " ")
}

// numToRuWordsInt: 1600000 -> "один миллион шестьсот тысяч"
func numToRuWordsInt(n int64) string {
	if n == 0 {
		return "ноль"
	}
	if n < 0 {
		return "минус " + numToRuWordsInt(-n)
	}

	type triadForm struct {
		one, two, five string
		female         bool
	}

	unitsMale := []string{"", "один", "два", "три", "четыре", "пять", "шесть", "семь", "восемь", "девять"}
	unitsFemale := []string{"", "одна", "две", "три", "четыре", "пять", "шесть", "семь", "восемь", "девять"}
	teens := []string{"десять", "одиннадцать", "двенадцать", "тринадцать", "четырнадцать", "пятнадцать", "шестнадцать", "семнадцать", "восемнадцать", "девятнадцать"}
	tens := []string{"", "десять", "двадцать", "тридцать", "сорок", "пятьдесят", "шестьдесят", "семьдесят", "восемьдесят", "девяносто"}
	hundreds := []string{"", "сто", "двести", "триста", "четыреста", "пятьсот", "шестьсот", "семьсот", "восемьсот", "девятьсот"}

	forms := []triadForm{
		{"", "", "", false}, // единицы (тенге)
		{"тысяча", "тысячи", "тысяч", true},            // тысячи
		{"миллион", "миллиона", "миллионов", false},    // миллионы
		{"миллиард", "миллиарда", "миллиардов", false}, // миллиарды (с запасом)
	}

	var parts []string
	triadIndex := 0

	for n > 0 && triadIndex < len(forms) {
		triad := int(n % 1000)
		n /= 1000

		if triad == 0 {
			triadIndex++
			continue
		}

		h := triad / 100
		t := (triad / 10) % 10
		u := triad % 10

		var triadWords []string

		if h > 0 {
			triadWords = append(triadWords, hundreds[h])
		}
		if t == 1 { // 10–19
			triadWords = append(triadWords, teens[u])
		} else {
			if t > 0 {
				triadWords = append(triadWords, tens[t])
			}
			if u > 0 {
				if forms[triadIndex].female {
					triadWords = append(triadWords, unitsFemale[u])
				} else {
					triadWords = append(triadWords, unitsMale[u])
				}
			}
		}

		// форма слова "тысяча/тысячи/тысяч", "миллион/..." и т.д.
		if triadIndex > 0 {
			var formWord string
			if t == 1 { // 10-19 -> всегда "тысяч", "миллионов"
				formWord = forms[triadIndex].five
			} else {
				switch u {
				case 1:
					formWord = forms[triadIndex].one
				case 2, 3, 4:
					formWord = forms[triadIndex].two
				default:
					formWord = forms[triadIndex].five
				}
			}
			if formWord != "" {
				triadWords = append(triadWords, formWord)
			}
		}

		parts = append([]string{strings.Join(triadWords, " ")}, parts...)
		triadIndex++
	}

	return strings.TrimSpace(strings.Join(parts, " "))
}
