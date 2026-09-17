package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"digcatalog/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ---------- Safety rounds ----------

type safetyItemReq struct {
	ItemCode string `json:"itemCode"`
	Result   string `json:"result"`
	Comment  string `json:"comment"`
}

type safetyRoundReq struct {
	SiteID     uint            `json:"siteId"`
	RoundDate  *string         `json:"roundDate"`
	Inspector  string          `json:"inspector"`
	Weather    string          `json:"weather"`
	Conclusion string          `json:"conclusion"`
	Summary    string          `json:"summary"`
	Items      []safetyItemReq `json:"items"`
}

var (
	errRoundOKWithFail  = errors.New("结论为合格(ok)时不允许存在不合格(fail)条目")
	errFailRequiresRisk = errors.New("存在不合格(fail)条目时，结论必须为风险(risk)")
)

func validResult(r string) bool {
	return r == "pass" || r == "fail" || r == "na"
}

// validateSafetyRound 在保存前强校验轮次与条目的业务约束：
//  1. conclusion 只能是 ok / risk；
//  2. 条目 result 只能是 pass / fail / na，itemCode 非空且同轮唯一；
//  3. conclusion=ok 时不允许存在 fail 项；
//  4. 存在 fail 项时 conclusion 必须为 risk。
func validateSafetyRound(req *safetyRoundReq) ([]models.SafetyItem, error) {
	if req.SiteID == 0 {
		return nil, errors.New("所属工地必填")
	}
	if req.Inspector == "" {
		return nil, errors.New("巡检人必填")
	}
	if req.Conclusion != "ok" && req.Conclusion != "risk" {
		return nil, errors.New("结论只能为合格(ok)或风险(risk)")
	}
	if len(req.Items) == 0 {
		return nil, errors.New("至少填写一条巡检条目")
	}

	items := make([]models.SafetyItem, 0, len(req.Items))
	seen := make(map[string]struct{}, len(req.Items))
	hasFail := false
	for _, it := range req.Items {
		if it.ItemCode == "" {
			return nil, errors.New("条目编号不能为空")
		}
		if !validResult(it.Result) {
			return nil, errors.New("条目「" + it.ItemCode + "」结果无效，只能为 pass/fail/na")
		}
		if _, dup := seen[it.ItemCode]; dup {
			return nil, errors.New("条目编号「" + it.ItemCode + "」在本轮次中重复")
		}
		seen[it.ItemCode] = struct{}{}
		if it.Result == "fail" {
			hasFail = true
		}
		items = append(items, models.SafetyItem{
			ItemCode: it.ItemCode,
			Result:   it.Result,
			Comment:  it.Comment,
		})
	}

	if req.Conclusion == "ok" && hasFail {
		return nil, errRoundOKWithFail
	}
	if hasFail && req.Conclusion != "risk" {
		return nil, errFailRequiresRisk
	}
	return items, nil
}

func (h *Handler) ListSafetyRounds(c *gin.Context) {
	var rounds []models.SafetyRound
	q := h.DB.Preload("Site").Preload("Items").Order("round_date desc, id desc")
	if siteID := c.Query("siteId"); siteID != "" {
		q = q.Where("site_id = ?", siteID)
	}
	if err := q.Find(&rounds).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rounds)
}

func (h *Handler) GetSafetyRound(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var round models.SafetyRound
	if err := h.DB.Preload("Site").Preload("Items").First(&round, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "巡检轮次不存在"})
		return
	}
	c.JSON(http.StatusOK, round)
}

func (h *Handler) CreateSafetyRound(c *gin.Context) {
	var req safetyRoundReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数无效"})
		return
	}
	items, err := validateSafetyRound(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var site models.Site
	if err := h.DB.First(&site, req.SiteID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "所属工地不存在"})
		return
	}

	round := models.SafetyRound{
		SiteID:     req.SiteID,
		RoundDate:  parseDate(req.RoundDate),
		Inspector:  req.Inspector,
		Weather:    req.Weather,
		Conclusion: req.Conclusion,
		Summary:    req.Summary,
		Items:      items,
	}

	// 事务内写库，并再次在数据库层面复查 fail/结论约束，防止并发绕过后端校验。
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&round).Error; err != nil {
			return err
		}
		return recheckSafetyInvariant(tx, round.ID, round.Conclusion)
	})
	if err != nil {
		if errors.Is(err, errRoundOKWithFail) || errors.Is(err, errFailRequiresRisk) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.DB.Preload("Site").Preload("Items").First(&round, round.ID)
	c.JSON(http.StatusCreated, round)
}

func (h *Handler) UpdateSafetyRound(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var round models.SafetyRound
	if err := h.DB.First(&round, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "巡检轮次不存在"})
		return
	}
	var req safetyRoundReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数无效"})
		return
	}
	items, err := validateSafetyRound(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var site models.Site
	if err := h.DB.First(&site, req.SiteID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "所属工地不存在"})
		return
	}

	round.SiteID = req.SiteID
	round.RoundDate = parseDate(req.RoundDate)
	round.Inspector = req.Inspector
	round.Weather = req.Weather
	round.Conclusion = req.Conclusion
	round.Summary = req.Summary

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		// 轮次与条目整体替换保存，物理清除旧条目，避免软删除残留与复合唯一索引冲突。
		if err := tx.Unscoped().Where("round_id = ?", round.ID).Delete(&models.SafetyItem{}).Error; err != nil {
			return err
		}
		if err := tx.Save(&round).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].RoundID = round.ID
			if err := tx.Create(&items[i]).Error; err != nil {
				return err
			}
		}
		return recheckSafetyInvariant(tx, round.ID, round.Conclusion)
	})
	if err != nil {
		if errors.Is(err, errRoundOKWithFail) || errors.Is(err, errFailRequiresRisk) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.DB.Preload("Site").Preload("Items").First(&round, round.ID)
	c.JSON(http.StatusOK, round)
}

func (h *Handler) DeleteSafetyRound(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("round_id = ?", id).Delete(&models.SafetyItem{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.SafetyRound{}, id).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已删除"})
}

// recheckSafetyInvariant 在数据库事务内统计 fail 条目并复查结论一致性。
// 入参 conclusion 已由 validateSafetyRound 限定为 ok/risk。
func recheckSafetyInvariant(tx *gorm.DB, roundID uint, conclusion string) error {
	var failCount int64
	if err := tx.Model(&models.SafetyItem{}).
		Where("round_id = ? AND result = ?", roundID, "fail").
		Count(&failCount).Error; err != nil {
		return err
	}
	if conclusion == "ok" && failCount > 0 {
		return errRoundOKWithFail
	}
	// risk 允许有 fail（典型风险轮），也允许无 fail（观察性风险），不强制。
	return nil
}
