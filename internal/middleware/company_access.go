package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type CompanyAccessResolver interface {
	HasUserAccess(userID, companyID int) (bool, error)
	GetUserActiveCompanyID(userID int) (*int, error)
}

func GetActiveCompanyID(c *gin.Context) (int, bool) {
	v, ok := c.Get("active_company_id")
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
		id, err := strconv.Atoi(t)
		if err == nil {
			return id, true
		}
	}
	return 0, false
}

func RequireCompanyAccess(resolver CompanyAccessResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if resolver == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Company resolver is not configured"})
			return
		}

		rawUserID, ok := c.Get("user_id")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		userID, ok := rawUserID.(int)
		if !ok || userID <= 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		var requestedCompanyID *int
		if activeFromClaims, ok := GetActiveCompanyID(c); ok && activeFromClaims > 0 {
			requestedCompanyID = &activeFromClaims
		}

		overrideRaw := strings.TrimSpace(c.GetHeader("X-Company-ID"))
		overrideEnabled := strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Company-Override")), "true") ||
			strings.TrimSpace(c.GetHeader("X-Company-Override")) == "1"
		if overrideRaw != "" {
			id, err := strconv.Atoi(overrideRaw)
			if err != nil || id <= 0 {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid X-Company-ID header"})
				return
			}
			if requestedCompanyID != nil && *requestedCompanyID != id && !overrideEnabled {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "X-Company-ID override requires X-Company-Override: true"})
				return
			}
			requestedCompanyID = &id
		}

		if requestedCompanyID == nil {
			active, err := resolver.GetUserActiveCompanyID(userID)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve active company"})
				return
			}
			if active != nil {
				requestedCompanyID = active
			}
		}

		if requestedCompanyID == nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Active company is required. Select a company first."})
			return
		}

		ok, err := resolver.HasUserAccess(userID, *requestedCompanyID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate company access"})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "No access to selected company"})
			return
		}

		c.Set("active_company_id", *requestedCompanyID)
		c.Next()
	}
}
