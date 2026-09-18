package service

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"github.com/gbplantwiki/gbplantwiki/internal/model"
	"github.com/gbplantwiki/gbplantwiki/internal/repository"
	"github.com/gbplantwiki/gbplantwiki/internal/util"
)

func newGardenService(db *gorm.DB) *UserGardenService {
	return NewUserGardenService(db,
		repository.NewUserGardenRepository(db),
		repository.NewPlantSpeciesRepository(db),
		repository.NewCareReminderRepository(db),
		newTestLogger())
}

func expectPlantFound(mock sqlmock.Sqlmock, id uint64, name, waterFreq string) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id` = ? ORDER BY `plant_species`.`id` LIMIT ?")).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "water_frequency"}).
			AddRow(id, name, waterFreq))
}

func TestAddWithPlanCreatesWateringReminder(t *testing.T) {
	db, mock := newServiceDB(t)
	svc := newGardenService(db)
	ownedSince := time.Date(2026, 9, 18, 0, 0, 0, 0, time.Local)

	expectPlantFound(mock, 1, "月季", "每周1次")
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_gardens`")).
		WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `care_reminders`")).
		WillReturnResult(sqlmock.NewResult(20, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_gardens`")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	g, reminder, created, err := svc.AddWithPlan(2, &model.UserGarden{PlantSpeciesID: 1, Nickname: "月季", OwnedSince: ownedSince})
	if err != nil {
		t.Fatalf("AddWithPlan: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if reminder == nil {
		t.Fatal("expected a watering reminder")
	}
	if want := time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local); !reminder.RemindDate.Equal(want) {
		t.Errorf("remind_date = %v, want %v", reminder.RemindDate, want)
	}
	if reminder.TaskTitle != "给月季浇水" {
		t.Errorf("task_title = %q", reminder.TaskTitle)
	}
	if g.CareReminderID != reminder.ID {
		t.Errorf("garden care_reminder_id = %d, want %d", g.CareReminderID, reminder.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestAddWithPlanUnrecognizedFrequencyPending(t *testing.T) {
	db, mock := newServiceDB(t)
	svc := newGardenService(db)

	expectPlantFound(mock, 3, "碗莲", "保持水位")
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_gardens`")).
		WillReturnResult(sqlmock.NewResult(11, 1))
	mock.ExpectCommit()

	g, reminder, created, err := svc.AddWithPlan(2, &model.UserGarden{PlantSpeciesID: 3, Nickname: "碗莲"})
	if err != nil {
		t.Fatalf("AddWithPlan: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if reminder != nil {
		t.Errorf("expected no reminder for unrecognized frequency, got %+v", reminder)
	}
	if g.CareReminderID != 0 {
		t.Errorf("expected care_reminder_id=0, got %d", g.CareReminderID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestAddWithPlanDuplicateReturnsExisting(t *testing.T) {
	db, mock := newServiceDB(t)
	svc := newGardenService(db)
	remindDate := time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local)

	expectPlantFound(mock, 1, "月季", "每周1次")
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_gardens`")).
		WillReturnError(errors.New("Duplicate entry '2-1' for key 'user_gardens.uk_garden_user_plant'"))
	mock.ExpectRollback()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_gardens` WHERE user_id = ? AND plant_species_id = ? ORDER BY `user_gardens`.`id` LIMIT ?")).
		WithArgs(2, 1, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "plant_species_id", "nickname", "care_reminder_id"}).
			AddRow(10, 2, 1, "月季", 20))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `care_reminders` WHERE `care_reminders`.`id` = ? ORDER BY `care_reminders`.`id` LIMIT ?")).
		WithArgs(20, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "plant_species_id", "task_title", "remind_date", "status"}).
			AddRow(20, 2, 1, "给月季浇水", remindDate, "pending"))

	g, reminder, created, err := svc.AddWithPlan(2, &model.UserGarden{PlantSpeciesID: 1, Nickname: "月季"})
	if err != nil {
		t.Fatalf("AddWithPlan: %v", err)
	}
	if created {
		t.Error("expected created=false for duplicate add")
	}
	if g.ID != 10 {
		t.Errorf("garden id = %d, want 10", g.ID)
	}
	if reminder == nil || reminder.ID != 20 || !reminder.RemindDate.Equal(remindDate) {
		t.Errorf("unexpected reminder: %+v", reminder)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestAddWithPlanPlantNotFound(t *testing.T) {
	db, mock := newServiceDB(t)
	svc := newGardenService(db)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id` = ? ORDER BY `plant_species`.`id` LIMIT ?")).
		WithArgs(99, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, _, _, err := svc.AddWithPlan(2, &model.UserGarden{PlantSpeciesID: 99})
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.HTTPStatus != 404 {
		t.Errorf("expected 404 AppError, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
