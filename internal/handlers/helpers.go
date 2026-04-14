package handlers

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"turcompany/internal/repositories"
)

// более устойчиво к типам (int / int64 / float64 / string)
func getIntFromCtx(c *gin.Context, key string) (int, bool) {
	v, ok := c.Get(key)
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case string:
		if n, err := strconv.Atoi(t); err == nil {
			return n, true
		}
	}
	return 0, false
}

func getUserAndRole(c *gin.Context) (userID, roleID int) {
	if id, ok := getIntFromCtx(c, "user_id"); ok {
		userID = id
	}
	if id, ok := getIntFromCtx(c, "role_id"); ok {
		roleID = id
	}
	return
}

func archiveScopeFromQuery(c *gin.Context) (repositories.ArchiveScope, bool) {
	raw := strings.ToLower(strings.TrimSpace(c.Query("archive")))
	switch raw {
	case "", "active":
		return repositories.ArchiveScopeActiveOnly, true
	case "archived":
		return repositories.ArchiveScopeArchivedOnly, true
	case "all":
		return repositories.ArchiveScopeAll, true
	default:
		return repositories.ArchiveScopeActiveOnly, false
	}
}
