package services

import (
	"testing"
	"time"

	"turcompany/internal/models"
)

// TestBuildConvertedDeal_InheritsLeadBranch verifies that converting a lead with a
// branch produces a deal whose branch_id equals the lead's branch_id (NOT NULL).
// This is the fix for the asymmetry where convert dropped branch to NULL while
// DealService.Create inherited it. Applies regardless of the converting role
// (sales/management/admin) — branch is taken from the lead, not the actor.
func TestBuildConvertedDeal_InheritsLeadBranch(t *testing.T) {
	branch := 7
	lead := &models.Leads{ID: 1, OwnerID: 10, BranchID: &branch}

	deal := buildConvertedDeal(1, 2, models.ClientTypeIndividual, 10, 1000, "KZT", lead, time.Now())

	if deal.BranchID == nil {
		t.Fatal("converted deal must inherit lead branch, got nil")
	}
	if *deal.BranchID != branch {
		t.Fatalf("converted deal branch = %d, want %d (lead branch)", *deal.BranchID, branch)
	}
	// sanity: other core fields are carried through
	if deal.OwnerID != 10 || deal.ClientID != 2 || deal.Status != "new" {
		t.Fatalf("unexpected deal fields: %+v", deal)
	}
}

// TestBuildConvertedDeal_NilLeadBranchStaysNil verifies that a lead with no branch
// yields a nil-branch deal (we don't invent a branch; we only remove the artificial loss).
func TestBuildConvertedDeal_NilLeadBranchStaysNil(t *testing.T) {
	lead := &models.Leads{ID: 1, OwnerID: 10, BranchID: nil}

	deal := buildConvertedDeal(1, 2, models.ClientTypeIndividual, 10, 1000, "KZT", lead, time.Now())

	if deal.BranchID != nil {
		t.Fatalf("converted deal branch must stay nil when lead has no branch, got %v", *deal.BranchID)
	}
}
