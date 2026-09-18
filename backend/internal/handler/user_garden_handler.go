package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/gbplantwiki/gbplantwiki/internal/constants"
	"github.com/gbplantwiki/gbplantwiki/internal/dto"
	"github.com/gbplantwiki/gbplantwiki/internal/middleware"
	"github.com/gbplantwiki/gbplantwiki/internal/model"
	"github.com/gbplantwiki/gbplantwiki/internal/service"
	"github.com/gbplantwiki/gbplantwiki/internal/util"
)

// UserGardenHandler exposes "my garden" endpoints.
type UserGardenHandler struct {
	svc    *service.UserGardenService
	logger *slog.Logger
}

// NewUserGardenHandler creates a UserGardenHandler.
func NewUserGardenHandler(svc *service.UserGardenService, logger *slog.Logger) *UserGardenHandler {
	return &UserGardenHandler{svc: svc, logger: logger}
}

// List handles GET /gardens.
func (h *UserGardenHandler) List(c *gin.Context) {
	items, err := h.svc.List(middleware.GetUserID(c))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.OK(items))
}

// Add handles POST /gardens. The response carries the enrollment result
// (garden entry + first watering reminder date) and a duplicated flag so the
// page can show whether this submission created the entry or was deduplicated.
func (h *UserGardenHandler) Add(c *gin.Context) {
	var req dto.GardenAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(util.NewAppError(http.StatusBadRequest, constants.CodeBadRequest, constants.MsgInvalidParam))
		return
	}
	g := &model.UserGarden{
		PlantSpeciesID: req.PlantSpeciesID, Nickname: req.Nickname,
		OwnedSince: req.OwnedSince, Location: req.Location,
	}
	result, err := h.svc.Add(middleware.GetUserID(c), g)
	if err != nil {
		c.Error(err)
		return
	}
	// A deduped submission (refresh / concurrent double submit) returns 200;
	// a genuinely created entry returns 201. Either way only one row exists.
	status := http.StatusCreated
	if result.Duplicated {
		status = http.StatusOK
	}
	c.JSON(status, dto.OK(dto.GardenAddResult{
		GardenItem: result.Item,
		Duplicated: result.Duplicated,
	}))
}

// BindReminder handles PUT /gardens/:id/reminder.
func (h *UserGardenHandler) BindReminder(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(util.NewAppError(http.StatusBadRequest, constants.CodeBadRequest, "invalid garden id"))
		return
	}
	var req dto.GardenBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(util.NewAppError(http.StatusBadRequest, constants.CodeBadRequest, constants.MsgInvalidParam))
		return
	}
	g, err := h.svc.BindReminder(middleware.GetUserID(c), uint(id), req.ReminderID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.OK(g))
}

// Remove handles DELETE /gardens/:id.
func (h *UserGardenHandler) Remove(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(util.NewAppError(http.StatusBadRequest, constants.CodeBadRequest, "invalid garden id"))
		return
	}
	if err := h.svc.Remove(middleware.GetUserID(c), uint(id)); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.OK(gin.H{"removed": true}))
}
