package service

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/gbplantwiki/gbplantwiki/internal/constants"
	"github.com/gbplantwiki/gbplantwiki/internal/model"
	"github.com/gbplantwiki/gbplantwiki/internal/repository"
	"github.com/gbplantwiki/gbplantwiki/internal/util"
)

// UserGardenService implements "my garden" list logic.
type UserGardenService struct {
	db           *gorm.DB
	repo         *repository.UserGardenRepository
	plantRepo    *repository.PlantSpeciesRepository
	reminderRepo *repository.CareReminderRepository
	logger       *slog.Logger
}

// NewUserGardenService creates a UserGardenService.
func NewUserGardenService(db *gorm.DB, repo *repository.UserGardenRepository, plantRepo *repository.PlantSpeciesRepository, reminderRepo *repository.CareReminderRepository, logger *slog.Logger) *UserGardenService {
	return &UserGardenService{db: db, repo: repo, plantRepo: plantRepo, reminderRepo: reminderRepo, logger: logger}
}

// AddWithPlan adds a plant to the user's garden and, when the species water
// frequency is recognizable, creates the first watering reminder derived from
// it. Garden record and reminder commit in one transaction so they succeed or
// fail together. Repeating the same add (refresh / concurrent submit) returns
// the existing record with created=false and causes no side effects.
func (s *UserGardenService) AddWithPlan(userID uint, g *model.UserGarden) (*model.UserGarden, *model.CareReminder, bool, error) {
	g.UserID = userID
	if g.OwnedSince.IsZero() {
		g.OwnedSince = time.Now()
	}
	plant, err := s.plantRepo.FindByID(g.PlantSpeciesID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, false, util.NewAppError(404, constants.CodeNotFound,
				fmt.Sprintf("PlantSpecies[id=%d] not found", g.PlantSpeciesID))
		}
		return nil, nil, false, fmt.Errorf("user garden add find plant: %w", err)
	}
	firstWater, ok := util.FirstWaterDate(g.OwnedSince, plant.WaterFrequency)
	if !ok {
		s.logger.Info(fmt.Sprintf(constants.LogGardenWaterPlanPending, g.PlantSpeciesID, userID, plant.WaterFrequency))
	}
	name := g.Nickname
	if name == "" {
		name = plant.Name
	}
	var reminder *model.CareReminder
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.repo.CreateTx(tx, g); err != nil {
			return err
		}
		if !ok {
			return nil
		}
		reminder = &model.CareReminder{
			UserID:         userID,
			PlantSpeciesID: g.PlantSpeciesID,
			TaskTitle:      fmt.Sprintf("给%s浇水", name),
			RemindDate:     firstWater,
			Frequency:      util.WaterFrequencyTag(plant.WaterFrequency),
			Status:         model.ReminderPending,
		}
		if err := s.reminderRepo.CreateTx(tx, reminder); err != nil {
			return err
		}
		g.CareReminderID = reminder.ID
		return s.repo.UpdateTx(tx, g)
	})
	if err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return s.existingPlan(userID, g.PlantSpeciesID)
		}
		s.logger.Error(fmt.Sprintf(constants.LogGardenAddFailed, g.PlantSpeciesID, userID), "error", err)
		return nil, nil, false, fmt.Errorf("user garden add: %w", err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogGardenAddSuccess, g.PlantSpeciesID, userID), "id", g.ID)
	return g, reminder, true, nil
}

// existingPlan loads the previously created garden item and its watering
// reminder for an idempotent repeat add.
func (s *UserGardenService) existingPlan(userID, plantID uint) (*model.UserGarden, *model.CareReminder, bool, error) {
	g, err := s.repo.Find(userID, plantID)
	if err != nil {
		return nil, nil, false, fmt.Errorf("user garden add find existing: %w", err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogGardenAddDuplicate, plantID, userID), "id", g.ID)
	if g.CareReminderID == 0 {
		return g, nil, false, nil
	}
	reminder, err := s.reminderRepo.FindByID(g.CareReminderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return g, nil, false, nil
		}
		return nil, nil, false, fmt.Errorf("user garden add find reminder: %w", err)
	}
	return g, reminder, false, nil
}

// List returns a user's garden items.
func (s *UserGardenService) List(userID uint) ([]model.UserGarden, error) {
	items, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("user garden list: %w", err)
	}
	return items, nil
}

// Remove deletes a garden item owned by the user.
func (s *UserGardenService) Remove(userID, id uint) error {
	g, err := s.repo.Find(userID, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return util.NewAppError(404, constants.CodeNotFound, fmt.Sprintf("UserGarden[id=%d] not found", id))
		}
		return fmt.Errorf("user garden remove find: %w", err)
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
