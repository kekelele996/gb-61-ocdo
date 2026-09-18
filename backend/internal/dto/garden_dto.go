package dto

import (
	"time"

	"github.com/gbplantwiki/gbplantwiki/internal/model"
)

// GardenAddRequest adds a plant to the user's garden.
type GardenAddRequest struct {
	PlantSpeciesID uint      `json:"plant_species_id" binding:"required"`
	Nickname       string    `json:"nickname" binding:"omitempty,max=64"`
	OwnedSince     time.Time `json:"owned_since"`
	Location       string    `json:"location" binding:"omitempty,max=128"`
}

// GardenBindRequest binds a reminder to a garden item.
type GardenBindRequest struct {
	ReminderID uint `json:"care_reminder_id" binding:"required"`
}

// Water plan statuses returned by the garden add endpoint.
const (
	WaterPlanScheduled = "scheduled"
	WaterPlanPending   = "pending"
)

// GardenAddResponse is the result of adding a plant to the garden: the garden
// record plus the auto-created first watering reminder. WaterPlanStatus is
// "pending" (待设置) when the species water frequency is unrecognized;
// Created is false when the plant was already in the garden.
type GardenAddResponse struct {
	Garden          *model.UserGarden   `json:"garden"`
	Reminder        *model.CareReminder `json:"reminder,omitempty"`
	WaterPlanStatus string              `json:"water_plan_status"`
	FirstWaterDate  string              `json:"first_water_date,omitempty"`
	Created         bool                `json:"created"`
}
