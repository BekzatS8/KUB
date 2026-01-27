package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"turcompany/internal/utils"
	"turcompany/internal/whatsapp/cloud"
)

var ErrSignDeliveryDisabled = errors.New("sign delivery disabled")

type SignDelivery interface {
	SendSignCode(ctx context.Context, phoneE164, code string) error
	SendSignLink(ctx context.Context, phoneE164, url string) error
}

type SignDeliveryConfig struct {
	Enabled          bool
	DryRun           bool
	GraphBaseURL     string
	PhoneNumberID    string
	AccessToken      string
	TemplateCodeName string
	TemplateLinkName string
	TemplateLang     string
}

type WhatsAppSignDelivery struct {
	cfg    SignDeliveryConfig
	client *cloud.Client
}

func NewWhatsAppSignDelivery(cfg SignDeliveryConfig) *WhatsAppSignDelivery {
	baseURL := strings.TrimSpace(cfg.GraphBaseURL)
	if baseURL == "" {
		baseURL = "https://graph.facebook.com/v20.0"
	}
	langCode := strings.TrimSpace(cfg.TemplateLang)
	if langCode == "" {
		langCode = "ru"
	}
	cfg.GraphBaseURL = baseURL
	cfg.TemplateLang = langCode
	return &WhatsAppSignDelivery{
		cfg:    cfg,
		client: cloud.NewClient(baseURL, cfg.PhoneNumberID, cfg.AccessToken),
	}
}

func (d *WhatsAppSignDelivery) SendSignCode(ctx context.Context, phoneE164, code string) error {
	if err := d.ensureEnabled("code"); err != nil {
		return err
	}
	return d.sendTemplate(ctx, phoneE164, d.cfg.TemplateCodeName, []any{
		map[string]any{
			"type": "body",
			"parameters": []any{
				map[string]any{"type": "text", "text": code},
			},
		},
	})
}

func (d *WhatsAppSignDelivery) SendSignLink(ctx context.Context, phoneE164, url string) error {
	if err := d.ensureEnabled("link"); err != nil {
		return err
	}
	return d.sendTemplate(ctx, phoneE164, d.cfg.TemplateLinkName, []any{
		map[string]any{
			"type":     "button",
			"sub_type": "url",
			"index":    "0",
			"parameters": []any{
				map[string]any{"type": "text", "text": url},
			},
		},
	})
}

func (d *WhatsAppSignDelivery) ensureEnabled(kind string) error {
	if !d.cfg.Enabled {
		return ErrSignDeliveryDisabled
	}
	if d.cfg.DryRun {
		return nil
	}
	if strings.TrimSpace(d.cfg.AccessToken) == "" || strings.TrimSpace(d.cfg.PhoneNumberID) == "" {
		return fmt.Errorf("whatsapp sign %s configuration is incomplete", kind)
	}
	return nil
}

func (d *WhatsAppSignDelivery) sendTemplate(ctx context.Context, phoneE164, templateName string, components []any) error {
	if strings.TrimSpace(templateName) == "" {
		return errors.New("whatsapp sign template is required")
	}
	toDigits, err := utils.SanitizeE164Digits(phoneE164)
	if err != nil {
		return err
	}

	if d.cfg.DryRun {
		log.Printf("[sign][whatsapp][dry-run] to=%s template=%q", toDigits, templateName)
		return nil
	}

	if d.client == nil {
		return errors.New("whatsapp sign client is nil")
	}
	return d.client.SendTemplate(ctx, toDigits, templateName, d.cfg.TemplateLang, components)
}
