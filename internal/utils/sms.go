package utils

type SendSMSResponse struct {
	Code int `json:"code"`
	Data struct {
		MessageID string `json:"messageId"`
	} `json:"data"`
}
