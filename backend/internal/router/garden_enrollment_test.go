//go:build sqlite

// End-to-end HTTP tests exercising the real Gin router against in-memory
// SQLite: login -> enter garden -> repeat submit -> inspect garden & reminders.
//
//	go test -tags sqlite ./internal/router/ -run TestGardenEnrollmentHTTP -v
package router_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/gbplantwiki/gbplantwiki/internal/config"
	"github.com/gbplantwiki/gbplantwiki/internal/constants"
	"github.com/gbplantwiki/gbplantwiki/internal/model"
	"github.com/gbplantwiki/gbplantwiki/internal/router"
)

type apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func setupEnrollmentRouter(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"),
		&gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.PlantSpecies{}, &model.CareArticle{}, &model.DiseasePest{},
		&model.CareReminder{}, &model.Favorite{}, &model.UserGarden{},
		&model.Question{}, &model.Answer{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS uk_garden_user_plant ON user_gardens(user_id, plant_species_id)").Error; err != nil {
		t.Fatalf("unique index: %v", err)
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte("user123"), bcrypt.MinCost)
	user := &model.User{
		Username: "smoke", Email: "smoke@gbplantwiki.local",
		PasswordHash: string(hash), Nickname: "冒烟用户", Role: "user",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	plants := []model.PlantSpecies{
		{Name: "月季", Type: constants.PlantTypeFlower, WaterFrequency: "每周3次"},
		{Name: "碗莲", Type: constants.PlantTypeAquatic, WaterFrequency: "保持水位"},
	}
	if err := db.Create(&plants).Error; err != nil {
		t.Fatalf("create plants: %v", err)
	}

	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError}))
	return router.Setup(cfg, db, logger), db
}

func doJSON(t *testing.T, h http.Handler, method, path, token string, body any) (int, apiEnvelope) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var env apiEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return rec.Code, env
}

func loginToken(t *testing.T, h http.Handler) string {
	t.Helper()
	status, env := doJSON(t, h, http.MethodPost, "/api/v1/users/login", "",
		map[string]string{"username": "smoke", "password": "user123"})
	if status != http.StatusOK {
		t.Fatalf("login status=%d body=%s", status, env.Message)
	}
	var data struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil || data.Token == "" {
		t.Fatalf("login token missing: %v", err)
	}
	return data.Token
}

func TestGardenEnrollmentHTTP(t *testing.T) {
	h, db := setupEnrollmentRouter(t)
	token := loginToken(t, h)

	// 首次入圃：品种 月季（每周3次，入圃日 2026-09-18 -> 首次浇水 2026-09-20）。
	status, env := doJSON(t, h, http.MethodPost, "/api/v1/gardens", token,
		map[string]any{"plant_species_id": 1, "owned_since": "2026-09-18T00:00:00Z"})
	if status != http.StatusCreated || env.Code != 0 {
		t.Fatalf("first enroll status=%d code=%d msg=%s", status, env.Code, env.Message)
	}
	var first struct {
		ID                uint   `json:"id"`
		PlantName         string `json:"plant_name"`
		WateringPlanText  string `json:"watering_plan_text"`
		FirstWateringDate string `json:"first_watering_date"`
		Duplicated        bool   `json:"duplicated"`
	}
	if err := json.Unmarshal(env.Data, &first); err != nil {
		t.Fatalf("decode first result: %v", err)
	}
	if first.Duplicated || first.PlantName != "月季" {
		t.Fatalf("unexpected first result: %+v", first)
	}
	if first.FirstWateringDate != "2026-09-20T00:00:00Z" && !bytes.HasPrefix([]byte(first.FirstWateringDate), []byte("2026-09-20")) {
		t.Fatalf("first watering date = %q, want 2026-09-20", first.FirstWateringDate)
	}

	// 刷新/重复提交：200 + duplicated=true，且仍是同一个条目。
	status, env = doJSON(t, h, http.MethodPost, "/api/v1/gardens", token,
		map[string]any{"plant_species_id": 1, "owned_since": "2026-09-18T00:00:00Z"})
	if status != http.StatusOK {
		t.Fatalf("repeat enroll status=%d, want 200", status)
	}
	var second struct {
		ID         uint `json:"id"`
		Duplicated bool `json:"duplicated"`
	}
	if err := json.Unmarshal(env.Data, &second); err != nil {
		t.Fatalf("decode repeat result: %v", err)
	}
	if !second.Duplicated || second.ID != first.ID {
		t.Fatalf("repeat must return same entry duplicated: %+v (first id=%d)", second, first.ID)
	}

	// 花园列表只保留一条，且带计划与提醒日期。
	status, env = doJSON(t, h, http.MethodGet, "/api/v1/gardens", token, nil)
	if status != http.StatusOK {
		t.Fatalf("list gardens status=%d", status)
	}
	var items []map[string]any
	if err := json.Unmarshal(env.Data, &items); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("garden count = %d, want 1", len(items))
	}

	// 无法识别频率（碗莲-保持水位）：入圃成功，提醒日期为 nil，计划显示待设置。
	status, env = doJSON(t, h, http.MethodPost, "/api/v1/gardens", token,
		map[string]any{"plant_species_id": 2})
	if status != http.StatusCreated {
		t.Fatalf("unrecognized enroll status=%d msg=%s, must not be blocked", status, env.Message)
	}
	var unrec struct {
		WateringPlanText  *string `json:"watering_plan_text"`
		FirstWateringDate *string `json:"first_watering_date"`
	}
	if err := json.Unmarshal(env.Data, &unrec); err != nil {
		t.Fatalf("decode unrecognized result: %v", err)
	}
	if unrec.WateringPlanText == nil || *unrec.WateringPlanText != "待设置" {
		t.Fatalf("plan text = %v, want 待设置", unrec.WateringPlanText)
	}
	if unrec.FirstWateringDate != nil {
		t.Fatalf("first watering date = %v, want nil", *unrec.FirstWateringDate)
	}

	// 提醒侧：仅月季生成 1 条首次浇水提醒。
	var gardenCount, reminderCount int64
	db.Table("user_gardens").Count(&gardenCount)
	db.Table("care_reminders").Count(&reminderCount)
	if gardenCount != 2 || reminderCount != 1 {
		t.Fatalf("rows: gardens=%d reminders=%d, want 2/1", gardenCount, reminderCount)
	}

	status, env = doJSON(t, h, http.MethodGet, "/api/v1/reminders", token, nil)
	if status != http.StatusOK {
		t.Fatalf("list reminders status=%d", status)
	}
	var reminders []struct {
		TaskTitle     string `json:"task_title"`
		FrequencyText string `json:"frequency_text"`
	}
	if err := json.Unmarshal(env.Data, &reminders); err != nil {
		t.Fatalf("decode reminders: %v", err)
	}
	if len(reminders) != 1 || reminders[0].TaskTitle != "给月季浇水" {
		t.Fatalf("unexpected reminders: %+v", reminders)
	}
	if reminders[0].FrequencyText == "" || reminders[0].FrequencyText == "待设置" {
		t.Fatalf("watering reminder frequency_text = %q", reminders[0].FrequencyText)
	}

	// 未认证必须被拦截。
	if status, _ := doJSON(t, h, http.MethodGet, "/api/v1/gardens", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated gardens status=%d, want 401", status)
	}
}
