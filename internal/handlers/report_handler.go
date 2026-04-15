package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/services"
)

const dateLayout = "2006-01-02"

type ReportHandler struct {
	Service *services.ReportService
}

func NewReportHandler(service *services.ReportService) *ReportHandler {
	return &ReportHandler{Service: service}
}

func parseDateParam(c *gin.Context, key string) (time.Time, bool) {
	value := c.Query(key)
	if value == "" {
		badRequest(c, "missing parameter: "+key)
		return time.Time{}, false
	}

	t, err := time.Parse(dateLayout, value)
	if err != nil {
		badRequest(c, "invalid date format, expected YYYY-MM-DD")
		return time.Time{}, false
	}

	return t, true
}

func parseOptionalBranchID(c *gin.Context) (*int, bool) {
	raw := strings.TrimSpace(c.Query("branch_id"))
	if raw == "" {
		return nil, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		badRequest(c, "invalid branch_id")
		return nil, false
	}
	return &v, true
}

func (h *ReportHandler) GetFunnel(c *gin.Context) {
	from, ok := parseDateParam(c, "from")
	if !ok {
		return
	}

	to, ok := parseDateParam(c, "to")
	if !ok {
		return
	}

	userID, roleID := getUserAndRole(c)
	requestedBranchID, ok := parseOptionalBranchID(c)
	if !ok {
		return
	}
	report, err := h.Service.GetSalesFunnel(c.Request.Context(), from, to, userID, roleID, requestedBranchID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "forbidden")
			return
		}
		internalError(c, "failed to build funnel report")
		return
	}

	c.JSON(http.StatusOK, report)
}

func (h *ReportHandler) GetLeadsSummary(c *gin.Context) {
	from, ok := parseDateParam(c, "from")
	if !ok {
		return
	}

	to, ok := parseDateParam(c, "to")
	if !ok {
		return
	}

	userID, roleID := getUserAndRole(c)
	requestedBranchID, ok := parseOptionalBranchID(c)
	if !ok {
		return
	}
	report, err := h.Service.GetLeadsSummary(c.Request.Context(), from, to, userID, roleID, requestedBranchID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "forbidden")
			return
		}
		internalError(c, "failed to build leads summary report")
		return
	}

	c.JSON(http.StatusOK, report)
}

func (h *ReportHandler) GetRevenue(c *gin.Context) {
	from, ok := parseDateParam(c, "from")
	if !ok {
		return
	}

	to, ok := parseDateParam(c, "to")
	if !ok {
		return
	}

	period := c.DefaultQuery("period", "month")
	switch period {
	case "month", "quarter", "year":
	default:
		badRequest(c, "invalid period value")
		return
	}

	userID, roleID := getUserAndRole(c)
	requestedBranchID, ok := parseOptionalBranchID(c)
	if !ok {
		return
	}
	report, err := h.Service.GetRevenueStats(c.Request.Context(), from, to, userID, roleID, period, requestedBranchID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "forbidden")
			return
		}
		internalError(c, "failed to build revenue report")
		return
	}

	c.JSON(http.StatusOK, report)
}

func (h *ReportHandler) ExportRevenue(c *gin.Context) {
	from, ok := parseDateParam(c, "from")
	if !ok {
		return
	}

	to, ok := parseDateParam(c, "to")
	if !ok {
		return
	}

	period := c.DefaultQuery("period", "month")
	if period != "month" && period != "quarter" && period != "year" {
		badRequest(c, "invalid period value")
		return
	}

	_, _ = c.GetQuery("format")

	userID, roleID := getUserAndRole(c)
	requestedBranchID, ok := parseOptionalBranchID(c)
	if !ok {
		return
	}
	report, err := h.Service.GetRevenueStats(c.Request.Context(), from, to, userID, roleID, period, requestedBranchID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "forbidden")
			return
		}
		internalError(c, "failed to export revenue report")
		return
	}

	c.JSON(http.StatusOK, report)
}
