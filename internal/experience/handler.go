// Package experience — HTTP handler for experience draft API.
// The DB operations are in experience.go (model layer).
package experience

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler serves the experience draft API.
type Handler struct {
	db     *sql.DB
	logger *zap.SugaredLogger
	// applier persists an approved draft to a skill file or the knowledge
	// base. Wired by app.go; nil keeps approve as status-only (backward compat).
	applier func(d *Draft, appliedTo string) (string, error)
}

// NewHandler creates an experience handler. Call EnsureSchema first.
func NewHandler(db *sql.DB, logger *zap.SugaredLogger) *Handler {
	return &Handler{db: db, logger: logger}
}

// SetApplier wires the approve-time persistence backend.
func (h *Handler) SetApplier(f func(d *Draft, appliedTo string) (string, error)) {
	h.applier = f
}

// ListDrafts: GET /api/experience/drafts?status=draft|approved|rejected|all
func (h *Handler) ListDrafts(c *gin.Context) {
	status := c.Query("status")
	if status == "all" || status == "" {
		drafts, err := ListAll(h.db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"drafts": drafts})
		return
	}
	drafts, err := ListByStatus(h.db, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"drafts": drafts})
}

// CreateDraft: POST /api/experience/drafts
// Body: {"title": "...", "content": "...", "category": "methodology|knowledge", "project_id": "...", "fact_keys": "[...]"}
func (h *Handler) CreateDraft(c *gin.Context) {
	var req struct {
		Title     string `json:"title" binding:"required"`
		Content   string `json:"content" binding:"required"`
		Category  string `json:"category"`
		ProjectID string `json:"project_id"`
		Source    string `json:"source"`
		FactKeys  string `json:"fact_keys"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Category == "" {
		req.Category = "methodology"
	}
	if req.Source == "" {
		req.Source = "manual"
	}
	d := &Draft{
		Title:     req.Title,
		Content:   req.Content,
		Category:  req.Category,
		ProjectID: req.ProjectID,
		Source:    req.Source,
		FactKeys:  req.FactKeys,
	}
	id, err := CreateDraft(h.db, d)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "status": "draft"})
}

// GetDraft: GET /api/experience/drafts/:id
func (h *Handler) GetDraft(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	d, err := GetDraft(h.db, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "draft not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

// ApproveDraft: POST /api/experience/drafts/:id/approve
// Body: {"applied_to": "skill:<name>" | "knowledge"} — empty defaults to
// "knowledge". The applier persists the content BEFORE the status flips, so
// a failed write leaves the draft in "draft" for retry.
func (h *Handler) ApproveDraft(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		AppliedTo string `json:"applied_to"`
	}
	_ = c.ShouldBindJSON(&req) // body is optional

	d, err := GetDraft(h.db, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "draft not found"})
		return
	}
	if d.Status != "draft" {
		c.JSON(http.StatusConflict, gin.H{"error": "draft already " + d.Status})
		return
	}

	reviewer := c.GetString("username")
	if reviewer == "" {
		reviewer = "operator"
	}

	appliedTo := strings.TrimSpace(req.AppliedTo)
	if appliedTo == "" {
		appliedTo = "knowledge"
	}
	if h.applier != nil {
		result, aerr := h.applier(d, appliedTo)
		if aerr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "apply failed: " + aerr.Error()})
			return
		}
		appliedTo = result
	}

	if err := ApproveDraft(h.db, id, reviewer, appliedTo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "status": "approved", "applied_to": appliedTo})
}

// RejectDraft: POST /api/experience/drafts/:id/reject
func (h *Handler) RejectDraft(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	reviewer := c.GetString("username")
	if reviewer == "" {
		reviewer = "operator"
	}
	if err := RejectDraft(h.db, id, reviewer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "status": "rejected"})
}

// Stats: GET /api/experience/stats
func (h *Handler) Stats(c *gin.Context) {
	counts, err := CountByStatus(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"counts": counts})
}
