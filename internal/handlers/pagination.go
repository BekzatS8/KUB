package handlers

import (
	"math"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
)

const (
	paginationDefaultPage = 1
	paginationDefaultSize = 15
	paginationMinSize     = 1
	paginationMaxSize     = 100
)

func isPaginatedMode(c *gin.Context) bool {
	return strings.EqualFold(strings.TrimSpace(c.Query("paginate")), "true")
}

func normalizedPageAndSize(c *gin.Context) (int, int) {
	page, err := strconv.Atoi(strings.TrimSpace(c.Query("page")))
	if err != nil || page < 1 {
		page = paginationDefaultPage
	}
	size, err := strconv.Atoi(strings.TrimSpace(c.Query("size")))
	if err != nil || size < paginationMinSize {
		size = paginationDefaultSize
	}
	if size > paginationMaxSize {
		size = paginationMaxSize
	}
	return page, size
}

func offsetFromPage(page, size int) int {
	return (page - 1) * size
}

func buildPaginationMeta(page, size, total int) models.PaginationMeta {
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(size)))
	}
	return models.PaginationMeta{
		Page:       page,
		Size:       size,
		Total:      total,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
	}
}
