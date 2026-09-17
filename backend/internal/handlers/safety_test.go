package handlers

import "testing"

func TestValidateSafetyRound(t *testing.T) {
	base := func() safetyRoundReq {
		return safetyRoundReq{
			SiteID:     1,
			Inspector:  "周慎行",
			Conclusion: "ok",
			Items: []safetyItemReq{
				{ItemCode: "S-01", Result: "pass"},
				{ItemCode: "S-02", Result: "na"},
			},
		}
	}

	t.Run("ok 轮无 fail 通过", func(t *testing.T) {
		if _, err := validateSafetyRound(ptr(base())); err != nil {
			t.Fatalf("expected pass, got %v", err)
		}
	})

	t.Run("ok 轮存在 fail 必须拒绝", func(t *testing.T) {
		req := base()
		req.Items = append(req.Items, safetyItemReq{ItemCode: "S-03", Result: "fail"})
		if _, err := validateSafetyRound(ptr(req)); err != errRoundOKWithFail {
			t.Fatalf("expected errRoundOKWithFail, got %v", err)
		}
	})

	t.Run("risk 轮含 fail 通过", func(t *testing.T) {
		req := base()
		req.Conclusion = "risk"
		req.Items = append(req.Items, safetyItemReq{ItemCode: "S-03", Result: "fail"})
		if _, err := validateSafetyRound(ptr(req)); err != nil {
			t.Fatalf("expected pass, got %v", err)
		}
	})

	t.Run("risk 轮无 fail 也通过", func(t *testing.T) {
		req := base()
		req.Conclusion = "risk"
		if _, err := validateSafetyRound(ptr(req)); err != nil {
			t.Fatalf("expected pass, got %v", err)
		}
	})

	t.Run("同轮 itemCode 重复拒绝", func(t *testing.T) {
		req := base()
		req.Items = append(req.Items, safetyItemReq{ItemCode: "S-01", Result: "pass"})
		if _, err := validateSafetyRound(ptr(req)); err == nil {
			t.Fatal("expected duplicate itemCode error, got nil")
		}
	})

	t.Run("非法 result 拒绝", func(t *testing.T) {
		req := base()
		req.Items[0].Result = "maybe"
		if _, err := validateSafetyRound(ptr(req)); err == nil {
			t.Fatal("expected invalid result error, got nil")
		}
	})

	t.Run("条目为空拒绝（禁止单表无条目）", func(t *testing.T) {
		req := base()
		req.Items = nil
		if _, err := validateSafetyRound(ptr(req)); err == nil {
			t.Fatal("expected at-least-one-item error, got nil")
		}
	})

	t.Run("非法 conclusion 拒绝", func(t *testing.T) {
		req := base()
		req.Conclusion = "unknown"
		if _, err := validateSafetyRound(ptr(req)); err == nil {
			t.Fatal("expected invalid conclusion error, got nil")
		}
	})

	t.Run("缺工地拒绝", func(t *testing.T) {
		req := base()
		req.SiteID = 0
		if _, err := validateSafetyRound(ptr(req)); err == nil {
			t.Fatal("expected site required error, got nil")
		}
	})

	t.Run("缺巡检人拒绝", func(t *testing.T) {
		req := base()
		req.Inspector = ""
		if _, err := validateSafetyRound(ptr(req)); err == nil {
			t.Fatal("expected inspector required error, got nil")
		}
	})
}

func ptr(r safetyRoundReq) *safetyRoundReq { return &r }
