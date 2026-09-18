package dto

import (
	"time"
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

// GardenItem is the enriched view of a garden entry: plant info plus the first
// watering plan derived on entering the garden.
type GardenItem struct {
	ID             uint      `json:"id"`
	UserID         uint      `json:"user_id"`
	PlantSpeciesID uint      `json:"plant_species_id"`
	PlantName      string    `json:"plant_name"`
	Nickname       string    `json:"nickname"`
	OwnedSince     time.Time `json:"owned_since"`
	Location       string    `json:"location"`
	CareReminderID uint      `json:"care_reminder_id"`
	CreatedAt      time.Time `json:"created_at"`

	// WateringFrequencyText is the plant's original water_frequency text.
	WateringFrequencyText string `json:"watering_frequency_text"`
	// WateringPlanText is the resolved plan in Chinese, or 待设置 when the
	// frequency text cannot be converted.
	WateringPlanText string `json:"watering_plan_text"`
	// FirstWateringDate is the auto-computed first watering date; nil when the
	// plan is 待设置 (no reminder was created).
	FirstWateringDate *time.Time `json:"first_watering_date"`
}

// GardenAddResult is returned by POST /gardens. Duplicated=true means the plant
// was already in the garden: no new entry or reminder was created and the
// existing one is returned (idempotent under refresh/concurrent resubmits).
type GardenAddResult struct {
	GardenItem
	Duplicated bool `json:"duplicated"`
}
