package wazzup

import (
	"context"
	"errors"
	"testing"

	"turcompany/internal/models"
)

// pruneRepo фиксирует, с каким «белым списком» вызвали уборку каналов.
type pruneRepo struct {
	stubRepo
	stored    []models.WazzupChannel
	pruneKeep []string
	pruned    bool
}

func (r *pruneRepo) UpsertChannels(_ context.Context, _ int, channels []models.WazzupChannel) error {
	r.stored = channels
	return nil
}

func (r *pruneRepo) DeleteChannelsNotIn(_ context.Context, _ int, keep []string) (int64, error) {
	r.pruned = true
	r.pruneKeep = keep
	return int64(len(keep)), nil
}

type channelsClient struct {
	noopClient
	channels []Channel
	err      error
}

func (c channelsClient) ListChannels(context.Context, string) ([]Channel, error) {
	return c.channels, c.err
}

func newPruneService(repo *pruneRepo, client Client) *Service {
	return NewService(repo, client, "", "", "", "")
}

// Каналы, отключённые на стороне Wazzup, должны исчезать из справочника CRM:
// раньше синхронизация только добавляла, и мёртвые номера висели вечно —
// попадали в «Написать первым» и в привязку «канал → филиал».
func TestSyncChannelsPrunesChannelsMissingAtProvider(t *testing.T) {
	repo := &pruneRepo{stubRepo: stubRepo{
		integration: &models.WazzupIntegration{ID: 1, OwnerUserID: 1, APIKeyEnc: "key", Enabled: true},
	}}
	client := channelsClient{channels: []Channel{
		{ID: "ch-alive-1", Transport: "whatsapp", Name: "Алматы"},
		{ID: "ch-alive-2", Transport: "tgapi", Name: "Шымкент"},
	}}

	if _, err := newPruneService(repo, client).SyncChannels(context.Background(), 1); err != nil {
		t.Fatalf("SyncChannels failed: %v", err)
	}
	if !repo.pruned {
		t.Fatal("expected stale channels to be pruned after a successful sync")
	}
	if len(repo.pruneKeep) != 2 || repo.pruneKeep[0] != "ch-alive-1" || repo.pruneKeep[1] != "ch-alive-2" {
		t.Fatalf("prune keep-list must be the provider's channel ids, got %v", repo.pruneKeep)
	}
}

// Если провайдер недоступен, чистить нельзя — отдаём кэш и не трогаем справочник.
func TestSyncChannelsDoesNotPruneWhenProviderFails(t *testing.T) {
	repo := &pruneRepo{stubRepo: stubRepo{
		integration: &models.WazzupIntegration{ID: 1, OwnerUserID: 1, APIKeyEnc: "key", Enabled: true},
	}}
	client := channelsClient{err: errors.New("upstream is down")}

	_, _ = newPruneService(repo, client).SyncChannels(context.Background(), 1)
	if repo.pruned {
		t.Fatal("must not prune the channel directory when the provider call failed")
	}
}

// Пустой ответ провайдера тоже не должен вычищать справочник — защита стоит в
// репозитории, но белый список при этом обязан быть пустым.
func TestSyncChannelsEmptyProviderListKeepsDirectory(t *testing.T) {
	repo := &pruneRepo{stubRepo: stubRepo{
		integration: &models.WazzupIntegration{ID: 1, OwnerUserID: 1, APIKeyEnc: "key", Enabled: true},
	}}
	client := channelsClient{channels: nil}

	if _, err := newPruneService(repo, client).SyncChannels(context.Background(), 1); err != nil {
		t.Fatalf("SyncChannels failed: %v", err)
	}
	if len(repo.pruneKeep) != 0 {
		t.Fatalf("expected empty keep-list for an empty provider response, got %v", repo.pruneKeep)
	}
}

func (c channelsClient) DeleteChannel(context.Context, string, string, bool) error { return nil }
