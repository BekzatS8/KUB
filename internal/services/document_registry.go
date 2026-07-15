package services

import "sort"

type DocumentFormat string

const (
	DocumentFormatDOCX DocumentFormat = "docx"
	DocumentFormatXLSX DocumentFormat = "xlsx"
)

type FieldScope string

const (
	FieldScopeClient FieldScope = "client"
	FieldScopeDeal   FieldScope = "deal"
	FieldScopeExtra  FieldScope = "extra"
)

type DocumentFieldRequirement struct {
	Key      string     `json:"key"`
	Scope    FieldScope `json:"scope"`
	Required bool       `json:"required"`
}

type DocumentTypeSpec struct {
	DocType        string                     `json:"doc_type"`
	TitleRU        string                     `json:"title_ru"`
	Format         DocumentFormat             `json:"format"`
	TemplateFile   string                     `json:"template_file"`
	LegalTemplate  string                     `json:"legal_template_file,omitempty"`
	RequiredFields []DocumentFieldRequirement `json:"required_fields"`
	ExtraKeys      []ExtraKeySpec             `json:"extra_keys"`
	ExampleExtra   map[string]string          `json:"example_extra"`
	Placeholders   []string                   `json:"placeholders"`
	// Departments — отделы, которым доступен шаблон. Приходит не из реестра, а
	// из таблицы document_template_departments: раскладку настраивает админ.
	Departments []string `json:"departments"`
}

type ExtraKeySpec struct {
	Key            string `json:"key"`
	Optional       bool   `json:"optional"`
	FallbackToDeal bool   `json:"fallback_to_deal"`
}

var documentTypeRegistry = map[string]DocumentTypeSpec{
	// Шаблоны обновлены 16.07.2026: старые бланки заменены новой редакцией
	// (ТОО «UKABY», подписант — директор по Уставу). Удалены типы, которых нет
	// в новой редакции: contract_ukaby_visa, contract_free_ru, visa_questionnaire,
	// addendum_korea. Добавлен power_of_attorney_application. АВР переведён с
	// XLSX на DOCX.
	//
	// Поля, которые нельзя вывести из карточки клиента и сделки (срок паузы,
	// дата записи в консульство, дата основного договора), объявлены в ExtraKeys —
	// менеджер вводит их в диалоге генерации.

	// ── Договоры (двуязычные, рус/каз) ──
	"contract_paid_50_50_ru":    mkDocxWithLegal("contract_paid_50_50_ru", "Договор 50% + 50%", "contract_paid_50_50_ru.docx", "contract_paid_50_50_ru.docx", baseRequired(), extraKeys(optionalDealFallback("PREPAYMENT_NUM"), optionalDealFallback("PREPAYMENT_WORDS"), optionalDealFallback("CONTRACT_NUMBER"), optionalDealFallback("CONTRACT_DATE_TEXT")), map[string]string{"PREPAYMENT_NUM": "225000", "PREPAYMENT_WORDS": "Двести двадцать пять тысяч"}),
	"contract_paid_full_ru":     mkDocxWithLegal("contract_paid_full_ru", "Договор 100%", "contract_paid_full_ru.docx", "contract_paid_full_ru.docx", baseRequired(), extraKeys(optionalDealFallback("CONTRACT_NUMBER"), optionalDealFallback("CONTRACT_DATE_TEXT")), nil),
	"contract_ukaby_30_35_35":   mkDocxWithLegal("contract_ukaby_30_35_35", "Договор 30% + 35% + 35%", "contract_ukaby_30_35_35.docx", "contract_ukaby_30_35_35.docx", baseRequired(), extraKeys(optionalDealFallback("PREPAYMENT_NUM"), optionalDealFallback("PREPAYMENT_WORDS"), optionalDealFallback("CONTRACT_NUMBER"), optionalDealFallback("CONTRACT_DATE_TEXT"), optional("PAYMENT_35_DAY15"), optional("PAYMENT_35_DAY30")), map[string]string{"CONTRACT_NUMBER": "KUB-000001"}),
	"contract_language_courses": mkDocxWithLegal("contract_language_courses", "Договор на обучение", "contract_language_courses.docx", "contract_language_courses.docx", baseRequired(), extraKeys(optional("COURSE_NAME"), optionalDealFallback("CONTRACT_NUMBER"), optionalDealFallback("CONTRACT_DATE_TEXT")), map[string]string{"COURSE_NAME": "General English"}),

	// ── Допсоглашения ──
	// MAIN_CONTRACT_* — реквизиты основного договора; при пустом значении
	// подтягиваются из сделки (см. applyPlaceholderAliases).
	"addendum_c01_extension": mkDocx("addendum_c01_extension", "Доп. соглашение С-01 (продление сроков)", "addendum_c01_extension.docx", baseRequired(), extraKeys(optionalDealFallback("MAIN_CONTRACT_NUMBER"), optionalDealFallback("MAIN_CONTRACT_DATE_TEXT"), optional("ADDENDUM_NUMBER"), optional("ADDENDUM_DATE_TEXT"), optional("EXTENSION_MONTHS"), optional("TOTAL_MONTHS")), map[string]string{"ADDENDUM_NUMBER": "С-01", "EXTENSION_MONTHS": "12", "TOTAL_MONTHS": "30"}),
	"addendum_k01_korea":     mkDocx("addendum_k01_korea", "Доп. соглашение К-01 (Корея)", "addendum_k01_korea.docx", baseRequired(), extraKeys(optionalDealFallback("MAIN_CONTRACT_NUMBER"), optionalDealFallback("MAIN_CONTRACT_DATE_TEXT"), optional("ADDENDUM_NUMBER"), optional("ADDENDUM_DATE_TEXT"), optional("KOREA_ADDITIONAL_PAYMENT"), optional("KOREA_ADDITIONAL_PAYMENT_TEXT"), optional("KOREA_BASE_PAYMENT"), optional("KOREA_BASE_PAYMENT_TEXT")), map[string]string{"ADDENDUM_NUMBER": "К-01", "KOREA_ADDITIONAL_PAYMENT": "1 600 000.00", "KOREA_BASE_PAYMENT": "500 000.00"}),

	// ── Расписки ──
	"receipt_refund_full":    mkDocx("receipt_refund_full", "Расписка о получении полного возврата", "receipt_refund_full.docx", baseRequired(), extraKeys(optionalDealFallback("REFUND_AMOUNT_NUM"), optionalDealFallback("REFUND_AMOUNT_TEXT")), map[string]string{"REFUND_AMOUNT_NUM": "100000", "REFUND_AMOUNT_TEXT": "Сто тысяч тенге"}),
	"receipt_refund_partial": mkDocx("receipt_refund_partial", "Расписка на частичный возврат", "receipt_refund_partial.docx", baseRequired(), extraKeys(optionalDealFallback("REFUND_AMOUNT_NUM"), optionalDealFallback("REFUND_AMOUNT_TEXT")), map[string]string{"REFUND_AMOUNT_NUM": "50000", "REFUND_AMOUNT_TEXT": "Пятьдесят тысяч тенге"}),

	// ── Заявления ──
	"refund_application": mkDocx("refund_application", "Заявление на возврат", "refund_application.docx", baseRequired(), extraKeys(mandatory("reason_code"), optional("reason_codes"), optional("REFUND_REASON_TEXT"), optionalDealFallback("REFUND_AMOUNT_NUM"), optionalDealFallback("REFUND_AMOUNT_TEXT")), map[string]string{"reason_code": "R1", "reason_codes": "[\"R1\",\"R4\"]", "REFUND_REASON_TEXT": "Отказ в визе"}),
	// PAUSE_DAYS / PAUSE_DAYS_TEXT и даты паузы менеджер вводит руками —
	// из карточки клиента их не вывести.
	"pause_application":    mkDocx("pause_application", "Заявление на паузу", "pause_application.docx", append(baseRequired(), DocumentFieldRequirement{Key: "reason_code", Scope: FieldScopeExtra, Required: true}), extraKeys(mandatory("reason_code"), optional("PAUSE_REASON_TEXT"), optional("PAUSE_FROM_DATE"), optional("PAUSE_TO_DATE"), optional("PAUSE_DAYS"), optional("PAUSE_DAYS_TEXT"), optional("PAUSE_START_DATE_TEXT"), optional("PAUSE_END_DATE_TEXT")), map[string]string{"reason_code": "R1", "PAUSE_REASON_TEXT": "По семейным обстоятельствам", "PAUSE_DAYS": "30", "PAUSE_DAYS_TEXT": "тридцать", "PAUSE_START_DATE_TEXT": "«01» августа 2026 года", "PAUSE_END_DATE_TEXT": "«30» августа 2026 года"}),
	"termination_transfer": mkDocx("termination_transfer", "Заявление на расторжение и перенос", "termination_transfer.docx", baseRequired(), extraKeys(optionalDealFallback("CONTRACT_NUMBER"), optionalDealFallback("CONTRACT_DATE_TEXT")), nil),
	"termination_waiver":   mkDocx("termination_waiver", "Заявление на расторжение", "termination_waiver.docx", baseRequired(), extraKeys(optionalDealFallback("CONTRACT_NUMBER"), optionalDealFallback("CONTRACT_DATE_TEXT")), nil),
	// APPOINTMENT_DATE_TEXT — дата записи в консульство, вводится руками.
	"cancel_appointment":            mkDocx("cancel_appointment", "Заявление об отмене записи", "cancel_appointment.docx", baseRequired(), extraKeys(mandatory("reason_code"), optional("reason_codes"), optional("CANCEL_REASON_TEXT"), optional("DESTINATION_PLACE"), optional("APPOINTMENT_DATE_TEXT"), optional("CANCEL_OTHER_TEXT")), map[string]string{"reason_code": "R1", "reason_codes": "[\"R1\",\"R4\"]", "DESTINATION_PLACE": "Консульство Республики Корея", "APPOINTMENT_DATE_TEXT": "«20» августа 2026 года"}),
	"power_of_attorney_application": mkDocx("power_of_attorney_application", "Заявление на подачу по доверенности", "power_of_attorney_application.docx", baseRequired(), extraKeys(optionalDealFallback("CONTRACT_NUMBER"), optionalDealFallback("CONTRACT_DATE_TEXT")), nil),

	// ── Акты ──
	"documents_handover_act": mkDocx("documents_handover_act", "Акт приёма-передачи документов", "documents_handover_act.docx", baseRequired(), extraKeys(optional("ACT_NUMBER"), optionalDealFallback("CONTRACT_NUMBER"), optionalDealFallback("CONTRACT_DATE_TEXT")), map[string]string{"ACT_NUMBER": "АКТ-000001"}),
	// АВР с 16.07.2026 — DOCX, а не XLSX (новая редакция бланка).
	"avr_kub_group": mkDocx("avr_kub_group", "АВР", "avr_kub_group.docx", baseRequired(), extraKeys(optionalDealFallback("CONTRACT_NUMBER")), nil),
}

func mkDocx(t, title, tpl string, req []DocumentFieldRequirement, extra []ExtraKeySpec, ex map[string]string) DocumentTypeSpec {
	if extra == nil {
		extra = []ExtraKeySpec{}
	}
	if ex == nil {
		ex = map[string]string{}
	}
	return DocumentTypeSpec{DocType: t, TitleRU: title, Format: DocumentFormatDOCX, TemplateFile: tpl, RequiredFields: req, ExtraKeys: extra, ExampleExtra: ex, Placeholders: []string{}}
}

func mkDocxWithLegal(t, title, tpl, legalTpl string, req []DocumentFieldRequirement, extra []ExtraKeySpec, ex map[string]string) DocumentTypeSpec {
	spec := mkDocx(t, title, tpl, req, extra, ex)
	spec.LegalTemplate = legalTpl
	return spec
}

func mkXlsx(t, title, tpl string, req []DocumentFieldRequirement, extra []ExtraKeySpec, ex map[string]string) DocumentTypeSpec {
	if extra == nil {
		extra = []ExtraKeySpec{}
	}
	if ex == nil {
		ex = map[string]string{}
	}
	return DocumentTypeSpec{DocType: t, TitleRU: title, Format: DocumentFormatXLSX, TemplateFile: tpl, RequiredFields: req, ExtraKeys: extra, ExampleExtra: ex, Placeholders: []string{}}
}

func baseRequired() []DocumentFieldRequirement {
	return []DocumentFieldRequirement{{Key: "full_name", Scope: FieldScopeClient, Required: true}, {Key: "iin_or_bin", Scope: FieldScopeClient, Required: true}, {Key: "address", Scope: FieldScopeClient, Required: true}, {Key: "phone", Scope: FieldScopeClient, Required: true}, {Key: "contract_number", Scope: FieldScopeDeal, Required: true}}
}

func consentRequired() []DocumentFieldRequirement {
	r := append([]DocumentFieldRequirement{}, baseRequired()...)
	return append(r, DocumentFieldRequirement{Key: "id_number", Scope: FieldScopeClient, Required: true}, DocumentFieldRequirement{Key: "passport_number", Scope: FieldScopeClient, Required: true})
}

func extraKeys(values ...ExtraKeySpec) []ExtraKeySpec { return values }
func mandatory(key string) ExtraKeySpec {
	return ExtraKeySpec{Key: key, Optional: false, FallbackToDeal: false}
}
func optional(key string) ExtraKeySpec {
	return ExtraKeySpec{Key: key, Optional: true, FallbackToDeal: false}
}
func optionalDealFallback(key string) ExtraKeySpec {
	return ExtraKeySpec{Key: key, Optional: true, FallbackToDeal: true}
}

func ListDocumentTypeSpecs() []DocumentTypeSpec {
	items := make([]DocumentTypeSpec, 0, len(documentTypeRegistry))
	for _, spec := range documentTypeRegistry {
		items = append(items, spec)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DocType < items[j].DocType })
	return items
}

func GetDocumentTypeSpec(docType string) (DocumentTypeSpec, bool) {
	spec, ok := documentTypeRegistry[normalizeDocType(docType)]
	return spec, ok
}
