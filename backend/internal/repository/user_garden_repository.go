package repository

import (
	"errors"

	"gorm.io/gorm"

	"github.com/gbplantwiki/gbplantwiki/internal/model"
)

// UserGardenRepository handles persistence of user garden items.
type UserGardenRepository struct {
	db *gorm.DB
}

// NewUserGardenRepository creates a UserGardenRepository.
func NewUserGardenRepository(db *gorm.DB) *UserGardenRepository {
	return &UserGardenRepository{db: db}
}

// Create inserts a garden item.
func (r *UserGardenRepository) Create(g *model.UserGarden) error {
	return r.CreateTx(r.db, g)
}

// CreateTx inserts a garden item inside an existing transaction.
func (r *UserGardenRepository) CreateTx(tx *gorm.DB, g *model.UserGarden) error {
	if err := tx.Create(g).Error; err != nil {
		if isDuplicate(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

// Find locates a garden item by user and plant.
func (r *UserGardenRepository) Find(userID, plantID uint) (*model.UserGarden, error) {
	return r.FindTx(r.db, userID, plantID)
}

// FindTx locates a garden item by user and plant inside an existing transaction.
func (r *UserGardenRepository) FindTx(tx *gorm.DB, userID, plantID uint) (*model.UserGarden, error) {
	var g model.UserGarden
	if err := tx.Where("user_id = ? AND plant_species_id = ?", userID, plantID).First(&g).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &g, nil
}

// FindByID locates a garden item by primary key.
func (r *UserGardenRepository) FindByID(id uint) (*model.UserGarden, error) {
	var g model.UserGarden
	if err := r.db.First(&g, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &g, nil
}

// Update persists a garden item.
func (r *UserGardenRepository) Update(g *model.UserGarden) error {
	return r.db.Save(g).Error
}

// UpdateReminderIDTx links a care reminder to a garden item in a transaction.
func (r *UserGardenRepository) UpdateReminderIDTx(tx *gorm.DB, id, reminderID uint) error {
	return tx.Model(&model.UserGarden{}).Where("id = ?", id).
		Update("care_reminder_id", reminderID).Error
}

// Delete removes a garden item by id.
func (r *UserGardenRepository) Delete(id uint) error {
	return r.db.Delete(&model.UserGarden{}, id).Error
}

// ListByUser returns all garden items of a user.
func (r *UserGardenRepository) ListByUser(userID uint) ([]model.UserGarden, error) {
	var items []model.UserGarden
	if err := r.db.Where("user_id = ?", userID).Order("id DESC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
