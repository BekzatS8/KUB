package handlers

import (
	"testing"
	"time"

	"turcompany/internal/models"
)

func TestListWithCountNilSliceReturnsEmptyValueAndZeroCount(t *testing.T) {
	resp := listWithCount[*models.Chat](nil)

	value, ok := resp["value"].([]*models.Chat)
	if !ok {
		t.Fatalf("value has unexpected type: %T", resp["value"])
	}
	if value == nil {
		t.Fatalf("value must be non-nil slice")
	}
	if len(value) != 0 {
		t.Fatalf("expected empty value, got len=%d", len(value))
	}

	count, ok := resp["Count"].(int)
	if !ok {
		t.Fatalf("Count has unexpected type: %T", resp["Count"])
	}
	if count != 0 {
		t.Fatalf("expected Count=0, got %d", count)
	}
}

func TestBuildMessagesWithAttachmentsResponseUsesEmptyAttachmentsForMissingValues(t *testing.T) {
	messages := []*models.ChatMessage{{
		ID:        1,
		ChatID:    10,
		SenderID:  22,
		Text:      "hello",
		CreatedAt: time.Now(),
	}}

	resp := buildMessagesWithAttachmentsResponse(messages, map[int][]models.AttachmentResponse{})
	if len(resp) != 1 {
		t.Fatalf("expected 1 message in response, got %d", len(resp))
	}

	attachments, ok := resp[0]["attachments"].([]models.AttachmentResponse)
	if !ok {
		t.Fatalf("attachments has unexpected type: %T", resp[0]["attachments"])
	}
	if attachments == nil {
		t.Fatalf("attachments must be non-nil slice")
	}
	if len(attachments) != 0 {
		t.Fatalf("expected empty attachments, got len=%d", len(attachments))
	}
}

func TestBuildMessagesWithAttachmentsResponseDeletedMessageAlwaysEmptyAttachments(t *testing.T) {
	messages := []*models.ChatMessage{{
		ID:        1,
		ChatID:    10,
		SenderID:  22,
		Text:      "[deleted]",
		CreatedAt: time.Now(),
		IsDeleted: true,
	}}

	attached := map[int][]models.AttachmentResponse{
		1: {
			{ID: "att-1", URL: "/attachments/att-1/download", FileName: "a.pdf", MimeType: "application/pdf", SizeBytes: 123},
		},
	}

	resp := buildMessagesWithAttachmentsResponse(messages, attached)
	attachments, ok := resp[0]["attachments"].([]models.AttachmentResponse)
	if !ok {
		t.Fatalf("attachments has unexpected type: %T", resp[0]["attachments"])
	}
	if attachments == nil {
		t.Fatalf("attachments must be non-nil slice")
	}
	if len(attachments) != 0 {
		t.Fatalf("expected empty attachments for deleted message, got len=%d", len(attachments))
	}
}
