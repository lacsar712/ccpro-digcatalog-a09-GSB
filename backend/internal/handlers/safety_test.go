package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"digcatalog/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupSafetyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Site{}, &models.SafetyRound{}, &models.SafetyItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&models.Site{Name: "测试工地", Period: "商周"}).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	return db
}

func serve(h *Handler, method, path string, body any) (*httptest.ResponseRecorder, func(http.ResponseWriter, *http.Request)) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/safety-rounds", h.ListSafetyRounds)
	r.POST("/safety-rounds", h.CreateSafetyRound)
	r.PUT("/safety-rounds/:id", h.UpdateSafetyRound)
	r.DELETE("/safety-rounds/:id", h.DeleteSafetyRound)

	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w, nil
}

func TestSafetyRoundValidation(t *testing.T) {
	db := setupSafetyDB(t)
	h := New(db, "secret")

	items := func(results ...string) []map[string]any {
		out := make([]map[string]any, 0, len(results))
		for i, r := range results {
			out = append(out, map[string]any{"itemCode": string(rune('A'+i)), "result": r})
		}
		return out
	}

	// 1. ok 轮无 fail：允许
	w, _ := serve(h, "POST", "/safety-rounds", map[string]any{
		"siteId": 1, "inspector": "张三", "conclusion": "ok", "items": items("pass", "na"),
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("ok round expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// 2. risk 轮含 fail：允许
	w, _ = serve(h, "POST", "/safety-rounds", map[string]any{
		"siteId": 1, "inspector": "张三", "conclusion": "risk", "items": items("pass", "fail"),
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("risk round expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// 3. ok 轮含 fail：必须 400
	w, _ = serve(h, "POST", "/safety-rounds", map[string]any{
		"siteId": 1, "inspector": "张三", "conclusion": "ok", "items": items("pass", "fail"),
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ok+fail expected 400, got %d: %s", w.Code, w.Body.String())
	}

	// 4. 条目为空：400（禁止单轮无条目）
	w, _ = serve(h, "POST", "/safety-rounds", map[string]any{
		"siteId": 1, "inspector": "张三", "conclusion": "ok", "items": []any{},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty items expected 400, got %d", w.Code)
	}

	// 5. 同轮 itemCode 重复：400
	w, _ = serve(h, "POST", "/safety-rounds", map[string]any{
		"siteId": 1, "inspector": "张三", "conclusion": "risk",
		"items": []map[string]any{
			{"itemCode": "S1", "result": "fail"},
			{"itemCode": "S1", "result": "pass"},
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("dup itemCode expected 400, got %d: %s", w.Code, w.Body.String())
	}

	// 6. 非法枚举：400
	w, _ = serve(h, "POST", "/safety-rounds", map[string]any{
		"siteId": 1, "inspector": "张三", "conclusion": "maybe", "items": items("pass"),
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad conclusion expected 400, got %d", w.Code)
	}
}

func TestSafetyRoundUpdateCascadeAndRules(t *testing.T) {
	db := setupSafetyDB(t)
	h := New(db, "secret")

	// 创建 risk 轮（含 fail）
	w, _ := serve(h, "POST", "/safety-rounds", map[string]any{
		"siteId": 1, "inspector": "张三", "weather": "晴", "conclusion": "risk", "summary": "有隐患",
		"items": []map[string]any{
			{"itemCode": "S1", "result": "pass", "comment": "ok"},
			{"itemCode": "S2", "result": "fail", "comment": "裂缝"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID    uint `json:"id"`
		Items []struct {
			ID uint `json:"id"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	if len(created.Items) != 2 {
		t.Fatalf("expected 2 items preloaded, got %d", len(created.Items))
	}

	// 改为 ok 但仍带 fail：后端必须拦截
	w, _ = serve(h, "PUT", "/safety-rounds/1", map[string]any{
		"siteId": 1, "inspector": "张三", "conclusion": "ok",
		"items": []map[string]any{
			{"id": created.Items[0].ID, "itemCode": "S1", "result": "pass"},
			{"id": created.Items[1].ID, "itemCode": "S2", "result": "fail"},
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("update ok+fail expected 400, got %d: %s", w.Code, w.Body.String())
	}

	// 整改完成：fail 改 pass，结论改 ok，并删除 S2（用只提交 S1 的方式），再加 S3
	w, _ = serve(h, "PUT", "/safety-rounds/1", map[string]any{
		"siteId": 1, "inspector": "李四", "conclusion": "ok",
		"items": []map[string]any{
			{"id": created.Items[0].ID, "itemCode": "S1", "result": "pass", "comment": "复查通过"},
			{"itemCode": "S3", "result": "na"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("rectify expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var rectified struct {
		Items []struct {
			ID       uint   `json:"id"`
			ItemCode string `json:"itemCode"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &rectified)
	if len(rectified.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(rectified.Items))
	}
	var updated models.SafetyRound
	db.Preload("Items").First(&updated, 1)
	if updated.Conclusion != "ok" || updated.Inspector != "李四" {
		t.Fatalf("round not updated: %+v", updated)
	}
	for _, it := range updated.Items {
		if it.Result == "fail" {
			t.Fatalf("fail item still present: %+v", it)
		}
	}
	// 被删除的条目必须物理消失
	var n int64
	db.Unscoped().Model(&models.SafetyItem{}).Where("round_id = 1").Count(&n)
	if n != 2 {
		t.Fatalf("expected 2 physical items after delete, got %d", n)
	}

	// 两个既有条目交换编号：应用层查重通过（编号各自唯一），唯一索引必须兜底 -> 非 200
	idByCode := map[string]uint{}
	for _, it := range rectified.Items {
		idByCode[it.ItemCode] = it.ID
	}
	w, _ = serve(h, "PUT", "/safety-rounds/1", map[string]any{
		"siteId": 1, "inspector": "李四", "conclusion": "ok",
		"items": []map[string]any{
			{"id": idByCode["S1"], "itemCode": "S3", "result": "pass"},
			{"id": idByCode["S3"], "itemCode": "S1", "result": "na"},
		},
	})
	if w.Code == http.StatusOK {
		t.Fatalf("swapping itemCodes into unique-index collision should not return 200")
	}
	// 撞车后整轮回滚，原编号不变
	db.Preload("Items").First(&updated, 1)
	codes := map[string]bool{}
	for _, it := range updated.Items {
		codes[it.ItemCode] = true
	}
	if !codes["S1"] || !codes["S3"] {
		t.Fatalf("rollback failed, items after collision: %v", codes)
	}

	// 删除轮次：条目级联物理删除
	w, _ = serve(h, "DELETE", "/safety-rounds/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d", w.Code)
	}
	var rounds, itemCount int64
	db.Model(&models.SafetyRound{}).Count(&rounds)
	db.Unscoped().Model(&models.SafetyItem{}).Count(&itemCount)
	if rounds != 0 || itemCount != 0 {
		t.Fatalf("cascade delete failed, rounds=%d items=%d", rounds, itemCount)
	}
}
