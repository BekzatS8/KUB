package services

import (
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

// Корзина задач (ТЗ «доработки CRM КУБ», п.11): админ и руководство убирают в
// корзину любую задачу, остальные — только поставленную ими самими; ОКК —
// наблюдатель, ему нельзя.
func TestCanTrashTask(t *testing.T) {
	task := &models.Task{ID: 1, CreatorID: 10, AssigneeID: 11, AssigneeIDs: []int64{11}}
	cases := []struct {
		name   string
		roleID int
		userID int64
		want   bool
	}{
		{"админ — любую", authz.RoleSystemAdmin, 99, true},
		{"руководство — любую", authz.RoleManagement, 99, true},
		{"МОП-постановщик — свою", authz.RoleSales, 10, true},
		{"МОП-исполнитель — нет", authz.RoleSales, 11, false},
		{"кадры-постановщик — свою", authz.RoleHR, 10, true},
		{"чужой визовый — нет", authz.RoleVisa, 12, false},
		{"ОКК даже свою — нет", authz.RoleControl, 10, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanTrashTask(tc.roleID, tc.userID, task); got != tc.want {
				t.Fatalf("CanTrashTask = %v, want %v", got, tc.want)
			}
		})
	}
	if CanTrashTask(authz.RoleSystemAdmin, 1, nil) {
		t.Fatal("nil task must not be trashable")
	}
}
