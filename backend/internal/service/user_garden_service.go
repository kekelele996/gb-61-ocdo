package service

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/gbplantwiki/gbplantwiki/internal/constants"
	"github.com/gbplantwiki/gbplantwiki/internal/dto"
	"github.com/gbplantwiki/gbplantwiki/internal/model"
	"github.com/gbplantwiki/gbplantwiki/internal/repository"
	"github.com/gbplantwiki/gbplantwiki/internal/util"
)

// Frequency encoding for auto-created watering reminders:
// "water:<recurrence>:<interval_days>", e.g. "water:interval_days:3" or
// "water:monthly:0". EncodeWateringFrequency / DecodeWateringFrequency keep the
// encoding in one place so reminders created elsewhere stay compatible.
const wateringFrequencyPrefix = "water:"

// EncodeWateringFrequency packs a parsed watering plan into reminder.frequency.
func EncodeWateringFrequency(plan WaterSchedule) string {
	if !plan.Recognized() {
		return ""
	}
	return fmt.Sprintf("%s%s:%d", wateringFrequencyPrefix, plan.Recurrence, plan.IntervalDays)
}

// DecodeWateringFrequency unpacks a reminder.frequency value. ok is false for
// manually created reminders and the zero value (待设置).
func DecodeWateringFrequency(frequency string) (recurrence string, intervalDays int, ok bool) {
	if !strings.HasPrefix(frequency, wateringFrequencyPrefix) {
		return "", 0, false
	}
	parts := strings.Split(strings.TrimPrefix(frequency, wateringFrequencyPrefix), ":")
	if len(parts) != 2 {
		return "", 0, false
	}
	days, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}
	return parts[0], days, true
}

// UserGardenService implements "my garden" list logic. Entering the garden also
// arms the first-watering care plan derived from the plant's water_frequency.
type UserGardenService struct {
	db           *gorm.DB
	repo         *repository.UserGardenRepository
	plantRepo    *repository.PlantSpeciesRepository
	reminderRepo *repository.CareReminderRepository
	logger       *slog.Logger
}

// NewUserGardenService creates a UserGardenService.
func NewUserGardenService(db *gorm.DB, repo *repository.UserGardenRepository,
	plantRepo *repository.PlantSpeciesRepository, reminderRepo *repository.CareReminderRepository,
	logger *slog.Logger) *UserGardenService {
	return &UserGardenService{db: db, repo: repo, plantRepo: plantRepo, reminderRepo: reminderRepo, logger: logger}
}

// GardenEnrollment is the result of one "enter the garden" attempt.
type GardenEnrollment struct {
	Item       dto.GardenItem
	Duplicated bool
}

// Add puts a plant into the user's garden and, in the same transaction, creates
// the first-watering reminder computed from the plant's water_frequency. The
// operation is idempotent: repeating it for the same user+plant (page refresh,
// double click, concurrent requests) returns the single existing entry without
// creating a duplicate reminder. An unrecognized water_frequency is surfaced as
// 待设置 and does not block enrollment.
func (s *UserGardenService) Add(userID uint, g *model.UserGarden) (*GardenEnrollment, error) {
	g.UserID = userID
	if g.OwnedSince.IsZero() {
		g.OwnedSince = time.Now()
	}
	plant, err := s.plantRepo.FindByID(g.PlantSpeciesID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(404, constants.CodeNotFound,
				fmt.Sprintf("PlantSpecies[id=%d] not found", g.PlantSpeciesID))
		}
		return nil, fmt.Errorf("garden add plant find: %w", err)
	}
	if g.Nickname == "" {
		g.Nickname = plant.Name
	}
	plan := ParseWaterSchedule(plant.WaterFrequency)
	firstDate := plan.FirstWateringDate(g.OwnedSince)

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.repo.CreateTx(tx, g); err != nil {
			return err
		}
		// Unrecognized frequency: garden entry stays, reminder is 待设置.
		if !plan.Recognized() {
			return nil
		}
		reminder := &model.CareReminder{
			UserID:         userID,
			PlantSpeciesID: g.PlantSpeciesID,
			TaskTitle:      fmt.Sprintf("给%s浇水", plant.Name),
			RemindDate:     firstDate,
			Frequency:      EncodeWateringFrequency(plan),
			Status:         model.ReminderPending,
		}
		if err := s.reminderRepo.CreateTx(tx, reminder); err != nil {
			return fmt.Errorf("garden add watering reminder: %w", err)
		}
		// Link the reminder to the garden entry; both rows commit or roll back together.
		if err := s.repo.UpdateReminderIDTx(tx, g.ID, reminder.ID); err != nil {
			return fmt.Errorf("garden add reminder link: %w", err)
		}
		g.CareReminderID = reminder.ID
		return nil
	})

	if err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			// Idempotent path: the insert lost a race (or simply repeated the
			// request). Return the already-present entry without duplicating it.
			existing, findErr := s.repo.Find(userID, g.PlantSpeciesID)
			if findErr != nil {
				return nil, fmt.Errorf("garden add duplicate find: %w", findErr)
			}
			items, viewErr := s.buildViews([]model.UserGarden{*existing})
			if viewErr != nil {
				return nil, viewErr
			}
			s.logger.Info("garden item already present, enrollment deduplicated",
				"plant_id", g.PlantSpeciesID, "user_id", userID, "id", existing.ID)
			return &GardenEnrollment{Item: items[0], Duplicated: true}, nil
		}
		s.logger.Error(fmt.Sprintf(constants.LogGardenAddFailed, g.PlantSpeciesID, userID), "error", err)
		return nil, fmt.Errorf("user garden add: %w", err)
	}

	views, err := s.buildViews([]model.UserGarden{*g})
	if err != nil {
		return nil, err
	}
	s.logger.Info(fmt.Sprintf(constants.LogGardenAddSuccess, g.PlantSpeciesID, userID),
		"id", g.ID, "watering_recognized", plan.Recognized(),
		"first_watering_date", util.FormatDate(firstDate))
	return &GardenEnrollment{Item: views[0]}, nil
}

// List returns a user's garden items enriched with plant and first-watering info.
func (s *UserGardenService) List(userID uint) ([]dto.GardenItem, error) {
	items, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("user garden list: %w", err)
	}
	return s.buildViews(items)
}

// buildViews hydrates garden entries with plant names and the linked watering
// reminders. Missing plant/reminder rows render as 待设置 rather than failing.
func (s *UserGardenService) buildViews(items []model.UserGarden) ([]dto.GardenItem, error) {
	plantIDs := make([]uint, 0, len(items))
	reminderIDs := make([]uint, 0, len(items))
	seenPlant := map[uint]struct{}{}
	seenReminder := map[uint]struct{}{}
	for _, g := range items {
		if g.PlantSpeciesID != 0 {
			if _, ok := seenPlant[g.PlantSpeciesID]; !ok {
				seenPlant[g.PlantSpeciesID] = struct{}{}
				plantIDs = append(plantIDs, g.PlantSpeciesID)
			}
		}
		if g.CareReminderID != 0 {
			if _, ok := seenReminder[g.CareReminderID]; !ok {
				seenReminder[g.CareReminderID] = struct{}{}
				reminderIDs = append(reminderIDs, g.CareReminderID)
			}
		}
	}

	plants, err := s.plantRepo.FindByIDs(plantIDs)
	if err != nil {
		return nil, fmt.Errorf("garden view plants: %w", err)
	}
	plantByID := make(map[uint]model.PlantSpecies, len(plants))
	for _, p := range plants {
		plantByID[p.ID] = p
	}
	reminders, err := s.reminderRepo.FindByIDs(reminderIDs)
	if err != nil {
		return nil, fmt.Errorf("garden view reminders: %w", err)
	}
	reminderByID := make(map[uint]model.CareReminder, len(reminders))
	for _, r := range reminders {
		reminderByID[r.ID] = r
	}

	views := make([]dto.GardenItem, 0, len(items))
	for _, g := range items {
		v := dto.GardenItem{
			ID:             g.ID,
			UserID:         g.UserID,
			PlantSpeciesID: g.PlantSpeciesID,
			Nickname:       g.Nickname,
			OwnedSince:     g.OwnedSince,
			Location:       g.Location,
			CareReminderID: g.CareReminderID,
			CreatedAt:      g.CreatedAt,
		}
		if plant, ok := plantByID[g.PlantSpeciesID]; ok {
			v.PlantName = plant.Name
			v.WateringFrequencyText = plant.WaterFrequency
		}
		if reminder, ok := reminderByID[g.CareReminderID]; ok {
			recurrence, intervalDays, decoded := DecodeWateringFrequency(reminder.Frequency)
			if decoded {
				v.WateringPlanText = util.WateringRecurrenceText(recurrence, intervalDays)
				d := reminder.RemindDate
				v.FirstWateringDate = &d
			}
		}
		if v.WateringPlanText == "" {
			v.WateringPlanText = util.WateringRecurrenceText("", 0) // 待设置
		}
		views = append(views, v)
	}
	return views, nil
}

// Remove deletes a garden item owned by the user.
func (s *UserGardenService) Remove(userID, id uint) error {
	g, err := s.repo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return util.NewAppError(404, constants.CodeNotFound, fmt.Sprintf("UserGarden[id=%d] not found", id))
		}
		return fmt.Errorf("user garden remove find: %w", err)
	}
	if g.UserID != userID {
		return util.NewAppError(403, constants.CodeForbidden, fmt.Sprintf("UserGarden[id=%d] remove failed: not owner", id))
	}
	if err := s.repo.Delete(g.ID); err != nil {
		return fmt.Errorf("user garden remove: %w", err)
	}
	s.logger.Info("user garden item removed", "user_id", userID, "id", id)
	return nil
}

// BindReminder associates a care reminder with a garden item.
func (s *UserGardenService) BindReminder(userID, gardenID, reminderID uint) (*model.UserGarden, error) {
	item, err := s.repo.FindByID(gardenID)
	if err != nil {
		return nil, fmt.Errorf("user garden bind find: %w", err)
	}
	if item.UserID != userID {
		return nil, util.NewAppError(403, constants.CodeForbidden, fmt.Sprintf("UserGarden[id=%d] bind failed: not owner", gardenID))
	}
	item.CareReminderID = reminderID
	if err := s.repo.Update(item); err != nil {
		return nil, fmt.Errorf("user garden bind update: %w", err)
	}
	return item, nil
}
