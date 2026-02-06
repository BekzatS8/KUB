package services

import (
	"context"
	"errors"
)

var ErrSignDeliveryDisabled = errors.New("sign delivery disabled")

type SignDelivery interface {
	SendSignCode(ctx context.Context, phoneE164, code string) error
	SendSignLink(ctx context.Context, phoneE164, url string) error
}

type DisabledSignDelivery struct{}

func NewDisabledSignDelivery() *DisabledSignDelivery {
	return &DisabledSignDelivery{}
}

func (d *DisabledSignDelivery) SendSignCode(ctx context.Context, phoneE164, code string) error {
	return ErrSignDeliveryDisabled
}

func (d *DisabledSignDelivery) SendSignLink(ctx context.Context, phoneE164, url string) error {
	return ErrSignDeliveryDisabled
}
