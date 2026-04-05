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
}

type ExtraKeySpec struct {
	Key            string `json:"key"`
	Optional       bool   `json:"optional"`
	FallbackToDeal bool   `json:"fallback_to_deal"`
}

var documentTypeRegistry = map[string]DocumentTypeSpec{
	"termination_transfer":      mkDocx("termination_transfer", "Соглашение о расторжении с передачей", "termination_transfer.docx", baseRequired(), nil, nil),
	"termination_waiver":        mkDocx("termination_waiver", "Соглашение о расторжении с отказом от претензий", "termination_waiver.docx", baseRequired(), nil, nil),
	"cancel_appointment":        mkDocx("cancel_appointment", "Заявление на отмену записи", "cancel_appointment.docx", baseRequired(), extraKeys(mandatory("reason_code"), optional("reason_codes"), optional("CANCEL_REASON_TEXT"), optional("DESTINATION_PLACE"), optional("APPOINTMENT_DATE_TEXT"), optional("CANCEL_OTHER_TEXT")), map[string]string{"reason_code": "R1", "reason_codes": "[\"R1\",\"R4\"]", "CANCEL_REASON_TEXT": "Личные обстоятельства"}),
	"receipt_refund_partial":    mkDocx("receipt_refund_partial", "Расписка о частичном возврате", "receipt_refund_partial.docx", baseRequired(), extraKeys(optionalDealFallback("REFUND_AMOUNT_NUM"), optionalDealFallback("REFUND_AMOUNT_TEXT")), map[string]string{"REFUND_AMOUNT_NUM": "50000", "REFUND_AMOUNT_TEXT": "Пятьдесят тысяч тенге"}),
	"receipt_refund_full":       mkDocx("receipt_refund_full", "Расписка о полном возврате", "receipt_refund_full.docx", baseRequired(), extraKeys(optionalDealFallback("REFUND_AMOUNT_NUM"), optionalDealFallback("REFUND_AMOUNT_TEXT")), map[string]string{"REFUND_AMOUNT_NUM": "100000", "REFUND_AMOUNT_TEXT": "Сто тысяч тенге"}),
	"documents_handover_act":    mkDocx("documents_handover_act", "Акт приёма-передачи документов", "documents_handover_act.docx", baseRequired(), extraKeys(optional("DOCS_ITEMS_JSON"), optional("DOCS_PRESENT"), optional("ACT_NUMBER"), optional("DOCS_MARK_1"), optional("DOCS_MARK_2"), optional("DOCS_MARK_3")), map[string]string{"DOCS_PRESENT": "[1,2,5,9]", "DOCS_ITEMS_JSON": "[{\"name\":\"Паспорт\",\"mark\":\"☑\"}]"}),
	"visa_questionnaire":        mkDocx("visa_questionnaire", "Визовый опросник", "visa_questionnaire.docx", consentRequired(), extraKeys(optional("QUESTIONNAIRE_PURPOSE"), optional("TRIP_COUNTRY"), optional("TRIP_PURPOSE")), map[string]string{"QUESTIONNAIRE_PURPOSE": "Туризм"}),
	"contract_paid_50_50_ru":    mkDocxWithLegal("contract_paid_50_50_ru", "Договор 50/50 (рус)", "contract_paid_50_50_ru.docx", "contract_paid_50_50_ru.docx", baseRequired(), extraKeys(optionalDealFallback("PREPAY_AMOUNT_NUM"), optionalDealFallback("PREPAY_AMOUNT_TEXT"), optionalDealFallback("CONTRACT_NUMBER"), optionalDealFallback("CONTRACT_DATE_TEXT")), map[string]string{"PREPAY_AMOUNT_NUM": "100000", "PREPAY_AMOUNT_TEXT": "Сто тысяч тенге"}),
	"contract_free_ru":          mkDocxWithLegal("contract_free_ru", "Договор (бесплатный, рус)", "contract_free_ru.docx", "contract_free_ru.docx", baseRequired(), nil, nil),
	"contract_paid_full_ru":     mkDocxWithLegal("contract_paid_full_ru", "Договор полной оплаты (рус)", "contract_paid_full_ru.docx", "contract_paid_full_ru.docx", baseRequired(), nil, nil),
	"contract_language_courses": mkDocxWithLegal("contract_language_courses", "Договор на языковые курсы", "contract_language_courses.docx", "contract_language_courses.docx", baseRequired(), extraKeys(optional("COURSE_NAME"), optionalDealFallback("CONTRACT_NUMBER"), optionalDealFallback("CONTRACT_DATE_TEXT")), map[string]string{"COURSE_NAME": "General English"}),
	"addendum_korea":            mkDocx("addendum_korea", "Дополнительное соглашение (Корея)", "addendum_korea.docx", baseRequired(), nil, nil),
	"refund_application":        mkDocx("refund_application", "Заявление на возврат", "refund_application.docx", baseRequired(), extraKeys(mandatory("reason_code"), optional("reason_codes"), optional("REFUND_REASON_TEXT"), optionalDealFallback("REFUND_AMOUNT_NUM"), optionalDealFallback("REFUND_AMOUNT_TEXT")), map[string]string{"reason_code": "R1", "reason_codes": "[\"R1\",\"R4\"]", "REFUND_REASON_TEXT": "Отказ в визе", "REFUND_AMOUNT_NUM": "100000", "REFUND_AMOUNT_TEXT": "Сто тысяч тенге"}),
	"pause_application":         mkDocx("pause_application", "Заявление на приостановку", "pause_application.docx", append(baseRequired(), DocumentFieldRequirement{Key: "reason_code", Scope: FieldScopeExtra, Required: true}), extraKeys(mandatory("reason_code"), optional("PAUSE_REASON_TEXT"), optional("PAUSE_FROM_DATE"), optional("PAUSE_TO_DATE"), optional("PAUSE_DAYS"), optional("PAUSE_START_DATE_TEXT"), optional("PAUSE_END_DATE_TEXT")), map[string]string{"reason_code": "R1", "PAUSE_REASON_TEXT": "По семейным обстоятельствам", "PAUSE_FROM_DATE": "01.06.2026", "PAUSE_TO_DATE": "30.06.2026"}),
	"avr_kub_group":             mkXlsx("avr_kub_group", "АВР KUB Group", "avr_kub_group.xlsx", baseRequired(), nil, nil),
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
