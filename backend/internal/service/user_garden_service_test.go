package service

import (
	"database/sql"
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

func newGardenService(t *testing.T) (*UserGardenService, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	gdb, mock := newServiceDB(t)
	svc := NewUserGardenService(
		gdb,
		repository.NewUserGardenRepository(gdb),
		repository.NewPlantSpeciesRepository(gdb),
		repository.NewCareReminderRepository(gdb),
		newTestLogger(),
	)
	sqlDB, _ := gdb.DB()
	return svc, mock, sqlDB
}

func plantRows(waterFreq string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "water_frequency"}).
		AddRow(7, "月季", waterFreq)
}

// 入圃成功：建花园记录 + 建首次浇水提醒 + 回写关联，三步在同一事务内。
func TestUserGardenAddSuccessCreatesReminderInTx(t *testing.T) {
	svc, mock, sqlDB := newGardenService(t)
	defer sqlDB.Close()

	mock.MatchExpectationsInOrder(true)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id` = ? ORDER BY `plant_species`.`id` LIMIT ?")).
		WithArgs(uint(7), 1).
		WillReturnRows(plantRows("每周3次"))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_gardens`")).
		WillReturnResult(sqlmock.NewResult(100, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `care_reminders`")).
		WillReturnResult(sqlmock.NewResult(200, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_gardens` SET `care_reminder_id`=? WHERE id = ?")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// buildViews: plants + reminders hydrate
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE id IN (?)")).
		WillReturnRows(plantRows("每周3次"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `care_reminders` WHERE id IN (?)")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "plant_species_id", "task_title", "remind_date", "frequency", "status"}).
			AddRow(200, 1, 7, "给月季浇水", time.Date(2026, 9, 20, 0, 0, 0, 0, time.Local), "water:interval_days:2", "pending"))

	res, err := svc.Add(1, &model.UserGarden{PlantSpeciesID: 7})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.Duplicated {
		t.Fatal("first enrollment must not be duplicated")
	}
	if res.Item.CareReminderID != 200 {
		t.Errorf("care_reminder_id = %d, want 200", res.Item.CareReminderID)
	}
	if got := res.Item.FirstWateringDate; got == nil || got.Format("2006-01-02") != "2026-09-20" {
		t.Errorf("first_watering_date = %v, want 2026-09-20", got)
	}
	if res.Item.WateringPlanText == "" || res.Item.WateringPlanText == "待设置" {
		t.Errorf("watering plan text = %q, want resolved plan", res.Item.WateringPlanText)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// 无法识别浇水频率：只建花园记录，不建提醒，不回滚，页面显示待设置。
func TestUserGardenAddUnrecognizedFrequencyNoReminder(t *testing.T) {
	svc, mock, sqlDB := newGardenService(t)
	defer sqlDB.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id` = ? ORDER BY `plant_species`.`id` LIMIT ?")).
		WithArgs(uint(3), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "water_frequency"}).AddRow(3, "碗莲", "保持水位"))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_gardens`")).
		WillReturnResult(sqlmock.NewResult(101, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE id IN (?)")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "water_frequency"}).AddRow(3, "碗莲", "保持水位"))
	// 无关联提醒 -> 不应查询 care_reminders（ID 列表为空时直接返回）

	res, err := svc.Add(1, &model.UserGarden{PlantSpeciesID: 3})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.Duplicated {
		t.Fatal("first enrollment must not be duplicated")
	}
	if res.Item.FirstWateringDate != nil {
		t.Errorf("first_watering_date = %v, want nil", res.Item.FirstWateringDate)
	}
	if res.Item.WateringPlanText != "待设置" {
		t.Errorf("plan text = %q, want 待设置", res.Item.WateringPlanText)
	}
	if res.Item.CareReminderID != 0 {
		t.Errorf("care_reminder_id = %d, want 0", res.Item.CareReminderID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// 重复入圃（刷新/并发）：插入撞唯一键 -> 回滚 -> 返回已有记录，不产生第二条提醒。
func TestUserGardenAddDuplicateIsIdempotent(t *testing.T) {
	svc, mock, sqlDB := newGardenService(t)
	defer sqlDB.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id` = ? ORDER BY `plant_species`.`id` LIMIT ?")).
		WithArgs(uint(7), 1).
		WillReturnRows(plantRows("每周1次"))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_gardens`")).
		WillReturnError(errors.New("Error 1062: Duplicate entry '1-7' for key 'uk_garden_user_plant'"))
	mock.ExpectRollback()
	// 事务外回查已有记录
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_gardens` WHERE user_id = ? AND plant_species_id = ? ORDER BY `user_gardens`.`id` LIMIT ?")).
		WithArgs(uint(1), uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "plant_species_id", "nickname", "owned_since", "care_reminder_id"}).
			AddRow(100, 1, 7, "月季", time.Date(2026, 9, 18, 0, 0, 0, 0, time.Local), 200))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE id IN (?)")).
		WillReturnRows(plantRows("每周1次"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `care_reminders` WHERE id IN (?)")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "plant_species_id", "task_title", "remind_date", "frequency", "status"}).
			AddRow(200, 1, 7, "给月季浇水", time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local), "water:interval_days:7", "pending"))

	res, err := svc.Add(1, &model.UserGarden{PlantSpeciesID: 7})
	if err != nil {
		t.Fatalf("Add duplicate: %v", err)
	}
	if !res.Duplicated {
		t.Fatal("repeated enrollment must be reported as duplicated")
	}
	if res.Item.ID != 100 || res.Item.CareReminderID != 200 {
		t.Errorf("expected existing entry id=100 reminder=200, got id=%d reminder=%d", res.Item.ID, res.Item.CareReminderID)
	}
	if got := res.Item.FirstWateringDate; got == nil || got.Format("2006-01-02") != "2026-09-25" {
		t.Errorf("first_watering_date = %v, want 2026-09-25", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// 提醒创建失败时花园记录必须一起回滚。
func TestUserGardenAddReminderFailureRollsBackGarden(t *testing.T) {
	svc, mock, sqlDB := newGardenService(t)
	defer sqlDB.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id` = ? ORDER BY `plant_species`.`id` LIMIT ?")).
		WithArgs(uint(7), 1).
		WillReturnRows(plantRows("每周1次"))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_gardens`")).
		WillReturnResult(sqlmock.NewResult(100, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `care_reminders`")).
		WillReturnError(errors.New("care_reminders insert failed"))
	mock.ExpectRollback()

	if _, err := svc.Add(1, &model.UserGarden{PlantSpeciesID: 7}); err == nil {
		t.Fatal("expected error when reminder insert fails")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// 品种不存在时 404，且不开启任何写事务。
func TestUserGardenAddUnknownPlant(t *testing.T) {
	svc, mock, sqlDB := newGardenService(t)
	defer sqlDB.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id` = ? ORDER BY `plant_species`.`id` LIMIT ?")).
		WithArgs(uint(999), 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.Add(1, &model.UserGarden{PlantSpeciesID: 999})
	if err == nil {
		t.Fatal("expected not-found error")
	}
	appErr, ok := err.(*util.AppError)
	if !ok || appErr.HTTPStatus != 404 {
		t.Errorf("expected 404 app error, got %T %v", err, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}
