package handlers

import (
	"strconv"
	"turcompany/internal/models"
	"turcompany/internal/services"

	"github.com/gin-gonic/gin"
)

type DocumentHandler struct {
	Service *services.DocumentService
}

func NewDocumentHandler(service *services.DocumentService) *DocumentHandler {
	return &DocumentHandler{Service: service}
}

// @Summary      Создание документа
// @Description  Создает новый документ
// @Tags         Documents
// @Accept       json
// @Produce      json
// @Param        document  body      models.Document  true  "Данные документа"
// @Success      201  {object}  map[string]int64
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /documents [post]
func (h *DocumentHandler) CreateDocument(c *gin.Context) {
	var document models.Document

	if err := c.ShouldBindJSON(&document); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	id, err := h.Service.CreateDocument(&document)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(201, gin.H{"id": id})
}

// @Summary      Получить документ по ID
// @Description  Возвращает один документ по его ID
// @Tags         Documents
// @Produce      json
// @Param        id   path      int64  true  "ID документа"
// @Success      200  {object}  models.Document
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /documents/{id} [get]
func (h *DocumentHandler) GetDocument(c *gin.Context) {
	idparam := c.Param("id")

	id, err := strconv.ParseInt(idparam, 10, 64)

	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}

	doc, err := h.Service.GetDocument(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "document not found"})
		return
	}

	c.JSON(200, doc)
}

// @Summary      Получить документы сделки
// @Description  Возвращает все документы, связанные с определенной сделкой
// @Tags         Documents
// @Produce      json
// @Param        dealid  path  int64  true  "ID сделки"
// @Success      200  {array}   models.Document
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /documents/deal/{dealid} [get]
func (h *DocumentHandler) ListDocumentsByDeal(c *gin.Context) {
	dealIDParam := c.Param("dealid")
	dealID, err := strconv.ParseInt(dealIDParam, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid deal id"})
		return
	}

	docs, err := h.Service.ListDocumentsByDeal(dealID)
	if err != nil {
		c.JSON(500, gin.H{"error": "could not fetch documents"})
		return
	}

	c.JSON(200, docs)
}

// @Summary      Удалить документ
// @Description  Удаляет документ по ID
// @Tags         Documents
// @Param        id   path  int64  true  "ID документа"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /documents/{id} [delete]
func (h *DocumentHandler) DeleteDocument(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}

	if err := h.Service.DeleteDocument(id); err != nil {
		c.JSON(500, gin.H{"error": "failed to delete document"})
		return
	}

	c.Status(204)
}

// @Summary      Создание документа из лида
// @Description  Создает документ на основе лида и типа документа
// @Tags         Documents
// @Accept       json
// @Produce      json
// @Param        input  body  object{lead_id=int,doc_type=string}  true  "ID лида и тип документа"
// @Success      201  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /documents/from-lead [post]
func (h *DocumentHandler) CreateDocumentFromLead(c *gin.Context) {
	var request struct {
		LeadID  int    `json:"lead_id" binding:"required"`
		DocType string `json:"doc_type" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"error": "Неверные параметры запроса: " + err.Error()})
		return
	}

	doc, err := h.Service.CreateDocumentFromLead(request.LeadID, request.DocType)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Ошибка при создании документа: " + err.Error(),
		})
		return
	}

	c.JSON(201, gin.H{
		"message":  "Документ успешно создан",
		"document": doc,
	})
}

// @Summary      Получить список документов
// @Description  Возвращает список всех документов с пагинацией
// @Tags         Documents
// @Produce      json
// @Param        page  query int false "Номер страницы"
// @Param        size  query int false "Размер страницы"
// @Success      200  {array}   models.Document
// @Failure      500  {object}  map[string]string
// @Router       /documents [get]
func (h *DocumentHandler) ListDocuments(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	offset := (page - 1) * size

	docs, err := h.Service.ListDocuments(size, offset)
	if err != nil {
		c.JSON(500, gin.H{"error": "could not fetch documents"})
		return
	}

	c.JSON(200, docs)
}
