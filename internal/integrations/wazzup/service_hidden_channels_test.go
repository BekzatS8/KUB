package wazzup

import (
	"context"
	"testing"

	"turcompany/internal/models"
)

// hiddenRepo — номера, удалённые вручную, и учёт скрытия/возврата.
type hiddenRepo struct {
	pruneRepo
	hidden   map[string]bool
	unhidden []string
	rows     []models.WazzupChannel
	deleted  []int64
}

func (r *hiddenRepo) ListHiddenChannelIDs(context.Context, int) ([]string, error) {
	out := []string{}
	for id, ok := range r.hidden {
		if ok {
			out = append(out, id)
		}
	}
	return out, nil
}
func (r *hiddenRepo) HideChannel(_ context.Context, _ int, id string) error {
	r.hidden[id] = true
	return nil
}
func (r *hiddenRepo) UnhideChannel(_ context.Context, _ int, id string) error {
	r.hidden[id] = false
	r.unhidden = append(r.unhidden, id)
	return nil
}
func (r *hiddenRepo) ListChannels(context.Context, int) ([]models.WazzupChannel, error) {
	return r.rows, nil
}
func (r *hiddenRepo) DeleteChannel(_ context.Context, id int64) error {
	r.deleted = append(r.deleted, id)
	return nil
}

func newHiddenRepo(hidden ...string) *hiddenRepo {
	r := &hiddenRepo{hidden: map[string]bool{}}
	r.integration = &models.WazzupIntegration{ID: 1, OwnerUserID: 1, APIKeyEnc: "key", Enabled: true}
	for _, id := range hidden {
		r.hidden[id] = true
	}
	return r
}

// Заблокированный Meta номер Wazzup продолжает отдавать после удаления со
// статусом disabled — синхронизация не должна возвращать его в CRM.
func TestSyncChannelsSkipsDeletedDeadChannel(t *testing.T) {
	repo := newHiddenRepo("ch-banned")
	client := channelsClient{channels: []Channel{
		{ID: "ch-banned", Transport: "whatsapp", Status: "disabled"},
		{ID: "ch-alive", Transport: "whatsapp", Status: "active"},
	}}

	if _, err := NewService(repo, client, "", "", "", "").SyncChannels(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if len(repo.stored) != 1 || repo.stored[0].ExternalChannelID != "ch-alive" {
		t.Fatalf("deleted dead channel must not be stored again, got %+v", repo.stored)
	}
	if len(repo.pruneKeep) != 1 || repo.pruneKeep[0] != "ch-alive" {
		t.Fatalf("prune keep-list must exclude the hidden channel, got %v", repo.pruneKeep)
	}
}

// Удалённый номер переподключили в кабинете Wazzup и он заработал — он
// возвращается в CRM и перестаёт считаться удалённым.
func TestSyncChannelsReturnsDeletedChannelOnceItWorks(t *testing.T) {
	repo := newHiddenRepo("ch-back")
	client := channelsClient{channels: []Channel{{ID: "ch-back", Transport: "whatsapp", Status: "active"}}}

	if _, err := NewService(repo, client, "", "", "", "").SyncChannels(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if len(repo.stored) != 1 || repo.stored[0].ExternalChannelID != "ch-back" {
		t.Fatalf("working channel must come back, got %+v", repo.stored)
	}
	if len(repo.unhidden) != 1 || repo.unhidden[0] != "ch-back" {
		t.Fatalf("working channel must be unhidden, got %v", repo.unhidden)
	}
}

// Удаление номера запоминается, чтобы следующая синхронизация его не вернула.
func TestDeleteChannelRemembersDeletion(t *testing.T) {
	repo := newHiddenRepo()
	repo.rows = []models.WazzupChannel{{ID: 7, IntegrationID: 1, ExternalChannelID: "ch-banned"}}

	deleted, err := NewService(repo, channelsClient{}, "key", "", "", "").DeleteChannel(context.Background(), 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted || !repo.hidden["ch-banned"] || len(repo.deleted) != 1 {
		t.Fatalf("deletion must be remembered: provider=%v hidden=%v deleted=%v", deleted, repo.hidden, repo.deleted)
	}
}
