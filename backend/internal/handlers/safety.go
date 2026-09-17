package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"digcatalog/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ---------- 工地安全巡检 ----------

type safetyItemReq struct {
	ID       uint   `json:"id"`
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

var validItemResults = map[string]bool{"pass": true, "fail": true, "na": true}

// validateSafetyRound 后端强校验：字段枚举、条目非空、同轮 itemCode 唯一、
// conclusion=ok 不允许 fail 项；存在 fail 项 conclusion 必须为 risk。
func validateSafetyRound(req *safetyRoundReq) string {
	if req.SiteID == 0 {
		return "所属工地必填"
	}
	if strings.TrimSpace(req.Inspector) == "" {
		return "巡检人必填"
	}
	if req.Conclusion != "ok" && req.Conclusion != "risk" {
		return "结论只能为 ok 或 risk"
	}
	if len(req.Items) == 0 {
		return "每轮巡检至少包含一个巡检条目"
	}
	seen := make(map[string]bool, len(req.Items))
	hasFail := false
	for _, it := range req.Items {
		code := strings.TrimSpace(it.ItemCode)
		if code == "" {
			return "条目编号不能为空"
		}
		if !validItemResults[it.Result] {
			return "条目「" + code + "」结果只能为 pass / fail / na"
		}
		if seen[code] {
			return "同轮巡检中条目编号重复：" + code
		}
		seen[code] = true
		if it.Result == "fail" {
			hasFail = true
		}
	}
	if req.Conclusion == "ok" && hasFail {
		return "结论为 ok 时不允许存在 fail 条目，请改为 risk 或修正条目结果"
	}
	if hasFail && req.Conclusion != "risk" {
		return "存在 fail 条目时结论必须为 risk"
	}
	return ""
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
	if msg := validateSafetyRound(&req); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
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
		Inspector:  strings.TrimSpace(req.Inspector),
		Weather:    req.Weather,
		Conclusion: req.Conclusion,
		Summary:    req.Summary,
	}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&round).Error; err != nil {
			return err
		}
		items := buildSafetyItems(round.ID, req.Items)
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
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
	// 更新时允许改挂工地，但目标工地必须存在
	if msg := validateSafetyRound(&req); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	var site models.Site
	if err := h.DB.First(&site, req.SiteID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "所属工地不存在"})
		return
	}

	round.SiteID = req.SiteID
	round.RoundDate = parseDate(req.RoundDate)
	round.Inspector = strings.TrimSpace(req.Inspector)
	round.Weather = req.Weather
	round.Conclusion = req.Conclusion
	round.Summary = req.Summary

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&round).Error; err != nil {
			return err
		}
		// 全量同步条目：删除缺失的（物理删除以避开唯一索引软删残留），新增/更新其余
		var existing []models.SafetyItem
		if err := tx.Where("round_id = ?", round.ID).Find(&existing).Error; err != nil {
			return err
		}
		keep := make(map[uint]bool)
		var toCreate []models.SafetyItem
		for _, it := range req.Items {
			if it.ID != 0 {
				keep[it.ID] = true
			} else {
				toCreate = append(toCreate, models.SafetyItem{
					RoundID:  round.ID,
					ItemCode: strings.TrimSpace(it.ItemCode),
					Result:   it.Result,
					Comment:  it.Comment,
				})
			}
		}
		for _, ex := range existing {
			if !keep[ex.ID] {
				if err := tx.Unscoped().Delete(&models.SafetyItem{}, ex.ID).Error; err != nil {
					return err
				}
			}
		}
		for _, it := range req.Items {
			if it.ID == 0 {
				continue
			}
			res := tx.Model(&models.SafetyItem{}).
				Where("id = ? AND round_id = ?", it.ID, round.ID).
				Updates(map[string]interface{}{
					"item_code": strings.TrimSpace(it.ItemCode),
					"result":    it.Result,
					"comment":   it.Comment,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				// 前端携带了不属于本轮的条目 id，按非法请求处理
				return gorm.ErrRecordNotFound
			}
		}
		if len(toCreate) > 0 {
			if err := tx.Create(&toCreate).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusBadRequest, gin.H{"error": "条目不属于该轮巡检，保存失败"})
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
	var round models.SafetyRound
	if err := h.DB.First(&round, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "巡检轮次不存在"})
		return
	}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		// 物理删除条目，避免唯一索引残留
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

func buildSafetyItems(roundID uint, reqs []safetyItemReq) []models.SafetyItem {
	items := make([]models.SafetyItem, 0, len(reqs))
	for _, it := range reqs {
		items = append(items, models.SafetyItem{
			RoundID:  roundID,
			ItemCode: strings.TrimSpace(it.ItemCode),
			Result:   it.Result,
			Comment:  it.Comment,
		})
	}
	return items
}
