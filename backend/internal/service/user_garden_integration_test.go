//go:build sqlite

// Integration tests against an in-memory SQLite database. Run with:
//
//	go test -tags sqlite ./internal/service/ -run TestEnrollment
//
// These prove end-to-end (real INSERT/UNIQUE/transaction semantics) what the
// sqlmock tests assert at the statement level:
//   - entering the garden creates one garden row + one linked watering reminder
//   - repeating it (refresh/concurrent) keeps a single row and single reminder
//   - an unrecognized water frequency creates the garden row with 待设置
package service

import (
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/gbplantwiki/gbplantwiki/internal/model"
	"github.com/gbplantwiki/gbplantwiki/internal/repository"
)

func newSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_pragma=foreign_keys(1)"),
		&gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.PlantSpecies{}, &model.CareReminder{}, &model.UserGarden{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Unique composite index mirroring uk_garden_user_plant.
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS uk_garden_user_plant ON user_gardens(user_id, plant_species_id)").Error; err != nil {
		t.Fatalf("unique index: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM care_reminders")
		db.Exec("DELETE FROM user_gardens")
		db.Exec("DELETE FROM plant_species")
	})
	return db
}

func newGardenEnrollmentService(db *gorm.DB) *UserGardenService {
	return NewUserGardenService(
		db,
		repository.NewUserGardenRepository(db),
		repository.NewPlantSpeciesRepository(db),
		repository.NewCareReminderRepository(db),
		newTestLogger(),
	)
}

func createPlant(t *testing.T, db *gorm.DB, id uint, name, water string) {
	t.Helper()
	if err := db.Create(&model.PlantSpecies{ID: id, Name: name, Type: "flower", WaterFrequency: water}).Error; err != nil {
		t.Fatalf("create plant: %v", err)
	}
}

func countRows(db *gorm.DB, table string) int64 {
	var n int64
	db.Table(table).Count(&n)
	return n
}

func TestEnrollmentCreatesGardenAndReminder(t *testing.T) {
	db := newSQLiteDB(t)
	svc := newGardenEnrollmentService(db)
	createPlant(t, db, 7, "月季", "每周3次")

	owned := time.Date(2026, 9, 18, 0, 0, 0, 0, time.Local)
	res, err := svc.Add(1, &model.UserGarden{PlantSpeciesID: 7, OwnedSince: owned})
	if err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if res.Duplicated {
		t.Fatal("first Add must not be duplicated")
	}
	if got := res.Item.FirstWateringDate; got == nil || got.Format("2006-01-02") != "2026-09-20" {
		t.Fatalf("first watering date = %v, want 2026-09-20", got)
	}
	if countRows(db, "user_gardens") != 1 || countRows(db, "care_reminders") != 1 {
		t.Fatalf("after add: gardens=%d reminders=%d, want 1/1",
			countRows(db, "user_gardens"), countRows(db, "care_reminders"))
	}
}

func TestEnrollmentRepeatedIsIdempotent(t *testing.T) {
	db := newSQLiteDB(t)
	svc := newGardenEnrollmentService(db)
	createPlant(t, db, 7, "月季", "每周1次")

	first, err := svc.Add(1, &model.UserGarden{PlantSpeciesID: 7})
	if err != nil {
		t.Fatalf("first Add: %v", err)
	}
	// Simulate refresh / concurrent duplicate submission.
	second, err := svc.Add(1, &model.UserGarden{PlantSpeciesID: 7})
	if err != nil {
		t.Fatalf("second Add: %v", err)
	}
	if !second.Duplicated {
		t.Fatal("second Add must be flagged duplicated")
	}
	if second.Item.ID != first.Item.ID || second.Item.CareReminderID != first.Item.CareReminderID {
		t.Fatal("repeated Add must return the same garden entry and reminder")
	}
	if countRows(db, "user_gardens") != 1 || countRows(db, "care_reminders") != 1 {
		t.Fatalf("after repeat: gardens=%d reminders=%d, want 1/1",
			countRows(db, "user_gardens"), countRows(db, "care_reminders"))
	}

	// Two different users may each keep their own single entry.
	if _, err := svc.Add(2, &model.UserGarden{PlantSpeciesID: 7}); err != nil {
		t.Fatalf("other user Add: %v", err)
	}
	if countRows(db, "user_gardens") != 2 || countRows(db, "care_reminders") != 2 {
		t.Fatalf("after second user: gardens=%d reminders=%d, want 2/2",
			countRows(db, "user_gardens"), countRows(db, "care_reminders"))
	}
}

func TestEnrollmentConcurrentSubmissionsTakeEffectOnce(t *testing.T) {
	db := newSQLiteDB(t)
	// Serialize at the connection pool; the application-level guarantee comes
	// from the UNIQUE(user_id, plant_species_id) index plus the idempotent
	// rollback-and-refetch path, which is exactly what MySQL relies on.
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	svc := newGardenEnrollmentService(db)
	createPlant(t, db, 7, "月季", "每周1次")

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.Add(1, &model.UserGarden{PlantSpeciesID: 7})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Add #%d: %v", i, err)
		}
	}
	if countRows(db, "user_gardens") != 1 || countRows(db, "care_reminders") != 1 {
		t.Fatalf("after %d concurrent submissions: gardens=%d reminders=%d, want 1/1",
			n, countRows(db, "user_gardens"), countRows(db, "care_reminders"))
	}
}

func TestEnrollmentMonthlyAlignsToSameDay(t *testing.T) {
	db := newSQLiteDB(t)
	svc := newGardenEnrollmentService(db)
	createPlant(t, db, 2, "多肉吉娃娃", "每月1次")

	res, err := svc.Add(1, &model.UserGarden{PlantSpeciesID: 2,
		OwnedSince: time.Date(2026, 9, 18, 0, 0, 0, 0, time.Local)})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := res.Item.FirstWateringDate; got == nil || got.Format("2006-01-02") != "2026-10-18" {
		t.Fatalf("monthly first watering = %v, want 2026-10-18 (same day-of-month)", got)
	}
	if res.Item.WateringPlanText != "每月（按当日对齐）" {
		t.Fatalf("plan text = %q", res.Item.WateringPlanText)
	}
}

func TestEnrollmentUnrecognizedFrequencyNotBlocked(t *testing.T) {
	db := newSQLiteDB(t)
	svc := newGardenEnrollmentService(db)
	createPlant(t, db, 3, "碗莲", "保持水位")

	res, err := svc.Add(1, &model.UserGarden{PlantSpeciesID: 3})
	if err != nil {
		t.Fatalf("Add must not be blocked by unrecognized frequency: %v", err)
	}
	if res.Item.FirstWateringDate != nil {
		t.Fatalf("first watering date = %v, want nil", res.Item.FirstWateringDate)
	}
	if res.Item.WateringPlanText != "待设置" {
		t.Fatalf("plan text = %q, want 待设置", res.Item.WateringPlanText)
	}
	if countRows(db, "user_gardens") != 1 {
		t.Fatal("garden row must exist despite unrecognized frequency")
	}
	if countRows(db, "care_reminders") != 0 {
		t.Fatal("no reminder must be created for unrecognized frequency")
	}
}

func TestEnrollmentListShowsPlanAndReminderDate(t *testing.T) {
	db := newSQLiteDB(t)
	svc := newGardenEnrollmentService(db)
	createPlant(t, db, 7, "月季", "每两周1次")

	if _, err := svc.Add(1, &model.UserGarden{PlantSpeciesID: 7,
		OwnedSince: time.Date(2026, 9, 18, 0, 0, 0, 0, time.Local)}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	items, err := svc.List(1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List returned %d items, want 1", len(items))
	}
	it := items[0]
	if it.PlantName != "月季" || it.WateringFrequencyText != "每两周1次" {
		t.Fatalf("hydrated plant fields wrong: %+v", it)
	}
	if it.FirstWateringDate == nil || it.FirstWateringDate.Format("2006-01-02") != "2026-10-02" {
		t.Fatalf("first watering date = %v, want 2026-10-02", it.FirstWateringDate)
	}
}
