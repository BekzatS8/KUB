package services

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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

	if client != nil {
		ph["CLIENT_COUNTRY"] = strings.TrimSpace(client.Country)
		ph["CLIENT_TRIP_PURPOSE"] = strings.TrimSpace(client.TripPurpose)
		if client.BirthDate != nil {
			ph["CLIENT_BIRTH_DATE"] = client.BirthDate.Format("2006-01-02")
		} else {
			ph["CLIENT_BIRTH_DATE"] = ""
		}
		ph["CLIENT_BIRTH_PLACE"] = strings.TrimSpace(client.BirthPlace)
		ph["CLIENT_CITIZENSHIP"] = strings.TrimSpace(client.Citizenship)
		ph["CLIENT_SEX"] = strings.TrimSpace(client.Sex)
		ph["CLIENT_MARITAL_STATUS"] = strings.TrimSpace(client.MaritalStatus)
		if client.PassportIssueDate != nil {
			ph["CLIENT_PASSPORT_ISSUE_DATE"] = client.PassportIssueDate.Format("2006-01-02")
		} else {
			ph["CLIENT_PASSPORT_ISSUE_DATE"] = ""
		}
		if client.PassportExpireDate != nil {
			ph["CLIENT_PASSPORT_EXPIRE_DATE"] = client.PassportExpireDate.Format("2006-01-02")
		} else {
			ph["CLIENT_PASSPORT_EXPIRE_DATE"] = ""
		}
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
	if _, ok := ph["CONTRACT_DATE_TEXT"]; !ok {
		ph["CONTRACT_DATE_TEXT"] = ph["DOC_DATE_TEXT"]
	}
	if _, ok := ph["CONTRACT_DATE_RAW"]; !ok {
		ph["CONTRACT_DATE_RAW"] = ph["DOC_DATE"] // "07.12.2025"
	}
	if _, ok := ph["CONTRACT_YEAR"]; !ok {
		ph["CONTRACT_YEAR"] = ph["DOC_DATE_YEAR"] // "2025"
	}

	// ================== СДЕЛКА ==================
	if deal != nil {
		ph["DEAL_ID"] = strconv.Itoa(deal.ID)
		ph["DEAL_CURRENCY"] = strings.TrimSpace(deal.Currency)
		ph["DEAL_STATUS"] = strings.TrimSpace(deal.Status)
		ph["DEAL_DATE"] = deal.CreatedAt.Format("02.01.2006")

		if tenge, tiyn, formatted, err := NormalizeMoney(strconv.FormatFloat(deal.Amount, 'f', 2, 64)); err == nil {
			ph["DEAL_AMOUNT_NUM"] = formatted
			ph["DEAL_AMOUNT_NUM_SPACED"] = formatAmountWithSpaces(tenge)
			ph["DEAL_AMOUNT_TEXT"] = amountToRuWords(tenge, tiyn)
			if _, ok := ph["TOTAL_AMOUNT_NUM"]; !ok {
				ph["TOTAL_AMOUNT_NUM"] = formatted
			}
		} else {
			amountStr := strconv.FormatFloat(deal.Amount, 'f', 2, 64)
			ph["DEAL_AMOUNT_NUM"] = strings.TrimSpace(amountStr)
			if _, ok := ph["TOTAL_AMOUNT_NUM"]; !ok {
				ph["TOTAL_AMOUNT_NUM"] = strings.TrimSpace(amountStr)
			}
		}

		// Номер договора по умолчанию
		if _, ok := ph["CONTRACT_NUMBER"]; !ok {
			ph["CONTRACT_NUMBER"] = fmt.Sprintf("KUB-%06d", deal.ID)
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

	// ================== ОБЩИЕ СУММЫ ==================
	if rawAmount, ok := ph["TOTAL_AMOUNT_NUM"]; ok && strings.TrimSpace(rawAmount) != "" {
		tenge, tiyn, formatted, err := NormalizeMoney(rawAmount)
		if err == nil {
			ph["TOTAL_AMOUNT_NUM"] = formatted
			ph["TOTAL_AMOUNT_NUM_SPACED"] = formatAmountWithSpaces(tenge)
			if _, ok2 := ph["TOTAL_AMOUNT_TEXT"]; !ok2 {
				ph["TOTAL_AMOUNT_TEXT"] = amountToRuWords(tenge, tiyn)
			}
		} else if _, ok2 := ph["TOTAL_AMOUNT_TEXT"]; !ok2 {
			ph["TOTAL_AMOUNT_TEXT"] = rawAmount
		}
	} else if _, ok := ph["TOTAL_AMOUNT_TEXT"]; !ok {
		ph["TOTAL_AMOUNT_TEXT"] = ""
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

func NormalizeMoney(raw any) (tenge int64, tiyn int64, formatted string, err error) {
	canonical, err := canonicalMoneyString(raw)
	if err != nil {
		return 0, 0, "", err
	}

	parts := strings.SplitN(canonical, ".", 2)
	tenge, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, "", fmt.Errorf("parse tenge from %q: %w", canonical, err)
	}
	if len(parts) == 2 {
		v, convErr := strconv.ParseInt(parts[1], 10, 64)
		if convErr != nil {
			return 0, 0, "", fmt.Errorf("parse tiyn from %q: %w", canonical, convErr)
		}
		tiyn = v
	}

	formatted = fmt.Sprintf("%s.%02d", formatAmountWithSpaces(tenge), tiyn)
	return tenge, tiyn, formatted, nil
}

func canonicalMoneyString(raw any) (string, error) {
	switch v := raw.(type) {
	case nil:
		return "", fmt.Errorf("empty amount")
	case string:
		return canonicalizeMoneyString(v)
	case []byte:
		return canonicalizeMoneyString(string(v))
	case int:
		return fmt.Sprintf("%d.00", v), nil
	case int64:
		return fmt.Sprintf("%d.00", v), nil
	case int32:
		return fmt.Sprintf("%d.00", v), nil
	case uint:
		return fmt.Sprintf("%d.00", v), nil
	case uint64:
		return fmt.Sprintf("%d.00", v), nil
	case uint32:
		return fmt.Sprintf("%d.00", v), nil
	case fmt.Stringer:
		return canonicalizeMoneyString(v.String())
	default:
		return canonicalizeMoneyString(fmt.Sprint(v))
	}
}

func canonicalizeMoneyString(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", fmt.Errorf("empty amount")
	}
	s = strings.ReplaceAll(s, " ", "")

	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' && r != ',' {
			return "", fmt.Errorf("invalid amount %q", input)
		}
	}

	sepPos := -1
	decimalSep := rune(0)
	countDot := strings.Count(s, ".")
	countComma := strings.Count(s, ",")

	if countDot > 0 && countComma > 0 {
		lastDot := strings.LastIndex(s, ".")
		lastComma := strings.LastIndex(s, ",")
		if lastDot > lastComma {
			sepPos = lastDot
			decimalSep = '.'
		} else {
			sepPos = lastComma
			decimalSep = ','
		}
	} else if countDot > 0 || countComma > 0 {
		sep := "."
		if countComma > 0 {
			sep = ","
		}
		idx := strings.LastIndex(s, sep)
		if len(s)-idx-1 == 2 {
			sepPos = idx
			decimalSep = rune(sep[0])
		}
	}

	wholeRaw := s
	fractionRaw := ""
	if sepPos >= 0 {
		wholeRaw = s[:sepPos]
		fractionRaw = s[sepPos+1:]
		if fractionRaw == "" || strings.ContainsAny(fractionRaw, ".,") {
			return "", fmt.Errorf("invalid amount %q", input)
		}
	}

	if decimalSep != 0 {
		if err := validateThousandGroups(wholeRaw, decimalSep); err != nil {
			return "", fmt.Errorf("invalid amount %q: %w", input, err)
		}
		if strings.ContainsRune(wholeRaw, decimalSep) {
			if err := validateGroupedWithSeparator(wholeRaw, decimalSep); err != nil {
				return "", fmt.Errorf("invalid amount %q: %w", input, err)
			}
		}
	}
	if sepPos == -1 {
		if err := validateAsThousandsOnly(wholeRaw); err != nil {
			return "", fmt.Errorf("invalid amount %q: %w", input, err)
		}
	}

	wholeDigits := strings.NewReplacer(",", "", ".", "").Replace(wholeRaw)
	if wholeDigits == "" {
		wholeDigits = "0"
	}
	if !isAllDigits(wholeDigits) {
		return "", fmt.Errorf("invalid amount %q", input)
	}

	tiyin := "00"
	if sepPos >= 0 {
		tiyin = fractionRaw
	}

	return wholeDigits + "." + tiyin, nil
}

func validateAsThousandsOnly(s string) error {
	if strings.Contains(s, "..") || strings.Contains(s, ",,") {
		return fmt.Errorf("invalid thousands separators")
	}
	return nil
}

func validateGroupedWithSeparator(s string, sep rune) error {
	chunks := strings.Split(s, string(sep))
	if len(chunks) < 2 {
		return nil
	}
	for i, part := range chunks {
		if part == "" || !isAllDigits(part) {
			return fmt.Errorf("invalid thousands separators")
		}
		if i == 0 {
			if len(part) < 1 || len(part) > 3 {
				return fmt.Errorf("invalid thousands separators")
			}
		} else if len(part) != 3 {
			return fmt.Errorf("invalid thousands separators")
		}
	}
	return nil
}

func validateThousandGroups(whole string, decimalSep rune) error {
	var thousandSep rune
	if decimalSep == '.' {
		thousandSep = ','
	} else {
		thousandSep = '.'
	}
	if !strings.ContainsRune(whole, thousandSep) {
		return nil
	}
	chunks := strings.Split(whole, string(thousandSep))
	if len(chunks) < 2 {
		return nil
	}
	for i, part := range chunks {
		if part == "" || !isAllDigits(part) {
			return fmt.Errorf("invalid thousands separators")
		}
		if i == 0 {
			if len(part) < 1 || len(part) > 3 {
				return fmt.Errorf("invalid thousands separators")
			}
		} else if len(part) != 3 {
			return fmt.Errorf("invalid thousands separators")
		}
	}
	return nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func amountToRuWords(tenge int64, tiyn int64) string {
	return fmt.Sprintf("%s тенге %02d тиын", numToRuWordsInt(tenge), tiyn)
}

// formatAmountWithSpaces: "1600000" -> "1 600 000"
func formatAmountWithSpaces(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for i := len(s); i > 0; i -= 3 {
		start := i - 3
		if start < 0 {
			start = 0
		}
		parts = append([]string{s[start:i]}, parts...)
	}
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
