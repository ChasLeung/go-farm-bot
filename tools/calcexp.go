package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// 种植速度常量
const (
	NoFertPlantsPer2Sec       = 18
	NormalFertPlantsPer2Sec   = 12
	NoFertPlantSpeedPerSec    = NoFertPlantsPer2Sec / 2.0  // 9 块/秒
	NormalFertPlantSpeedPerSec = NormalFertPlantsPer2Sec / 2.0 // 6 块/秒
)

// SeedExpInfo 种子经验信息
type SeedExpInfo struct {
	SeedID                int64   `json:"seedId"`
	GoodsID               int64   `json:"goodsId"`
	PlantID               int64   `json:"plantId"`
	Name                  string  `json:"name"`
	RequiredLevel         int     `json:"requiredLevel"`
	Unlocked              bool    `json:"unlocked"`
	Price                 int64   `json:"price"`
	ExpHarvest            int64   `json:"expHarvest"`
	ExpPerCycle           int64   `json:"expPerCycle"`
	GrowTimeSec           int64   `json:"growTimeSec"`
	GrowTimeStr           string  `json:"growTimeStr"`
	NormalFertReduceSec   int64   `json:"normalFertReduceSec"`
	GrowTimeNormalFert    int64   `json:"growTimeNormalFert"`
	GrowTimeNormalFertStr string  `json:"growTimeNormalFertStr"`
	CycleSecNoFert        float64 `json:"cycleSecNoFert"`
	CycleSecNormalFert    float64 `json:"cycleSecNormalFert"`
	FarmExpPerHourNoFert  float64 `json:"farmExpPerHourNoFert"`
	FarmExpPerHourNormalFert float64 `json:"farmExpPerHourNormalFert"`
	FarmExpPerDayNoFert   float64 `json:"farmExpPerDayNoFert"`
	FarmExpPerDayNormalFert float64 `json:"farmExpPerDayNormalFert"`
	GainPercent           float64 `json:"gainPercent"`
	ExpPerGoldSeed        float64 `json:"expPerGoldSeed"`
	FruitID               int64   `json:"fruitId"`
	FruitCount            int64   `json:"fruitCount"`
}

// PlantingRecommendation 种植推荐
type PlantingRecommendation struct {
	Level                 int                `json:"level"`
	Lands                 int                `json:"lands"`
	BestNoFert            *SeedExpInfo       `json:"bestNoFert"`
	BestNormalFert        *SeedExpInfo       `json:"bestNormalFert"`
	CandidatesNoFert      []*SeedExpInfo     `json:"candidatesNoFert"`
	CandidatesNormalFert  []*SeedExpInfo     `json:"candidatesNormalFert"`
}

// toNum 字符串转数字
func toNum(s string, fallback int64) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

// parseGrowPhases 解析生长阶段
func parseGrowPhases(growPhases string) []int64 {
	if growPhases == "" {
		return nil
	}
	
	var phases []int64
	segments := strings.Split(growPhases, ";")
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		parts := strings.Split(seg, ":")
		if len(parts) >= 2 {
			sec := toNum(parts[1], 0)
			if sec > 0 {
				phases = append(phases, sec)
			}
		}
	}
	return phases
}

// loadSeedPhaseReduceMap 加载种子阶段减少时间
func loadSeedPhaseReduceMap() map[int64]int64 {
	plantConfigPath := filepath.Join("gameConfig", "Plant.json")
	
	data, err := os.ReadFile(plantConfigPath)
	if err != nil {
		return make(map[int64]int64)
	}
	
	var rows []map[string]interface{}
	if err := json.Unmarshal(data, &rows); err != nil {
		return make(map[int64]int64)
	}
	
	result := make(map[int64]int64)
	for _, p := range rows {
		seedID := int64(0)
		if v, ok := p["seed_id"].(float64); ok {
			seedID = int64(v)
		}
		if seedID <= 0 {
			continue
		}
		
		growPhases := ""
		if v, ok := p["grow_phases"].(string); ok {
			growPhases = v
		}
		
		phases := parseGrowPhases(growPhases)
		if len(phases) > 0 {
			result[seedID] = phases[0] // 普通肥减少一个阶段：以首个阶段时长为准
		}
	}
	
	return result
}

// formatSec 格式化秒数
func formatSec(sec int64) string {
	s := sec
	if s < 0 {
		s = 0
	}
	
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	
	m := s / 60
	r := s % 60
	if m < 60 {
		if r > 0 {
			return fmt.Sprintf("%dm%ds", m, r)
		}
		return fmt.Sprintf("%dm", m)
	}
	
	h := m / 60
	mm := m % 60
	if r > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, mm, r)
	}
	return fmt.Sprintf("%dh%dm", h, mm)
}

// calcEffectiveGrowTime 计算有效生长时间（考虑普通肥）
func calcEffectiveGrowTime(growSec int64, seedID int64, seedPhaseReduceMap map[int64]int64) int64 {
	reduce := seedPhaseReduceMap[seedID]
	if reduce <= 0 {
		return growSec
	}
	result := growSec - reduce
	if result < 1 {
		result = 1
	}
	return result
}

// loadSeeds 加载种子数据
func loadSeeds() []map[string]interface{} {
	seedShopPath := filepath.Join("tools", "seed-shop-merged-export.json")
	
	data, err := os.ReadFile(seedShopPath)
	if err != nil {
		return nil
	}
	
	var result struct {
		Rows []map[string]interface{} `json:"rows"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		// 尝试直接解析为数组
		var rows []map[string]interface{}
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil
		}
		return rows
	}
	
	return result.Rows
}

// CalculateSeedExp 计算所有种子的经验效率
func CalculateSeedExp(lands int) []*SeedExpInfo {
	if lands <= 0 {
		lands = 18
	}
	
	seedPhaseReduceMap := loadSeedPhaseReduceMap()
	rawSeeds := loadSeeds()
	
	if rawSeeds == nil {
		return nil
	}
	
	plantSecondsNoFert := float64(lands) / NoFertPlantSpeedPerSec
	plantSecondsNormalFert := float64(lands) / NormalFertPlantSpeedPerSec
	
	var results []*SeedExpInfo
	
	for _, s := range rawSeeds {
		seedID := int64(0)
		if v, ok := s["seedId"].(float64); ok {
			seedID = int64(v)
		} else if v, ok := s["seed_id"].(float64); ok {
			seedID = int64(v)
		}
		
		name := ""
		if v, ok := s["name"].(string); ok {
			name = v
		}
		if name == "" {
			name = fmt.Sprintf("seed_%d", seedID)
		}
		
		requiredLevel := 1
		if v, ok := s["requiredLevel"].(float64); ok {
			requiredLevel = int(v)
		} else if v, ok := s["required_level"].(float64); ok {
			requiredLevel = int(v)
		}
		
		price := int64(0)
		if v, ok := s["price"].(float64); ok {
			price = int64(v)
		}
		
		expHarvest := int64(0)
		if v, ok := s["exp"].(float64); ok {
			expHarvest = int64(v)
		}
		
		growTimeSec := int64(0)
		if v, ok := s["growTimeSec"].(float64); ok {
			growTimeSec = int64(v)
		} else if v, ok := s["growTime"].(float64); ok {
			growTimeSec = int64(v)
		} else if v, ok := s["grow_time"].(float64); ok {
			growTimeSec = int64(v)
		}
		
		if seedID <= 0 || growTimeSec <= 0 {
			continue
		}
		
		goodsID := int64(0)
		if v, ok := s["goodsId"].(float64); ok {
			goodsID = int64(v)
		} else if v, ok := s["goods_id"].(float64); ok {
			goodsID = int64(v)
		}
		
		plantID := int64(0)
		if v, ok := s["plantId"].(float64); ok {
			plantID = int64(v)
		} else if v, ok := s["plant_id"].(float64); ok {
			plantID = int64(v)
		}
		
		unlocked := false
		if v, ok := s["unlocked"].(bool); ok {
			unlocked = v
		}
		
		growTimeStr := ""
		if v, ok := s["growTimeStr"].(string); ok {
			growTimeStr = v
		}
		if growTimeStr == "" {
			growTimeStr = formatSec(growTimeSec)
		}
		
		expPerCycle := expHarvest
		reduceSec := seedPhaseReduceMap[seedID]
		growTimeNormalFert := calcEffectiveGrowTime(growTimeSec, seedID, seedPhaseReduceMap)
		
		// 整个农场一轮 = 生长时间 + 本轮全部地块种植耗时
		cycleSecNoFert := float64(growTimeSec) + plantSecondsNoFert
		cycleSecNormalFert := float64(growTimeNormalFert) + plantSecondsNormalFert
		
		farmExpPerHourNoFert := (float64(lands) * float64(expPerCycle) / cycleSecNoFert) * 3600
		farmExpPerHourNormalFert := (float64(lands) * float64(expPerCycle) / cycleSecNormalFert) * 3600
		
		gainPercent := 0.0
		if farmExpPerHourNoFert > 0 {
			gainPercent = ((farmExpPerHourNormalFert - farmExpPerHourNoFert) / farmExpPerHourNoFert) * 100
		}
		
		expPerGoldSeed := 0.0
		if price > 0 {
			expPerGoldSeed = float64(expPerCycle) / float64(price)
		}
		
		fruitID := int64(0)
		fruitCount := int64(0)
		if fruit, ok := s["fruit"].(map[string]interface{}); ok {
			if v, ok := fruit["id"].(float64); ok {
				fruitID = int64(v)
			}
			if v, ok := fruit["count"].(float64); ok {
				fruitCount = int64(v)
			}
		}
		
		info := &SeedExpInfo{
			SeedID:                seedID,
			GoodsID:               goodsID,
			PlantID:               plantID,
			Name:                  name,
			RequiredLevel:         requiredLevel,
			Unlocked:              unlocked,
			Price:                 price,
			ExpHarvest:            expHarvest,
			ExpPerCycle:           expPerCycle,
			GrowTimeSec:           growTimeSec,
			GrowTimeStr:           growTimeStr,
			NormalFertReduceSec:   reduceSec,
			GrowTimeNormalFert:    growTimeNormalFert,
			GrowTimeNormalFertStr: formatSec(growTimeNormalFert),
			CycleSecNoFert:        cycleSecNoFert,
			CycleSecNormalFert:    cycleSecNormalFert,
			FarmExpPerHourNoFert:  farmExpPerHourNoFert,
			FarmExpPerHourNormalFert: farmExpPerHourNormalFert,
			FarmExpPerDayNoFert:   farmExpPerHourNoFert * 24,
			FarmExpPerDayNormalFert: farmExpPerHourNormalFert * 24,
			GainPercent:           gainPercent,
			ExpPerGoldSeed:        expPerGoldSeed,
			FruitID:               fruitID,
			FruitCount:            fruitCount,
		}
		
		results = append(results, info)
	}
	
	return results
}

// GetPlantingRecommendation 获取种植推荐
func GetPlantingRecommendation(level, lands int) *PlantingRecommendation {
	if level <= 0 {
		level = 1
	}
	if lands <= 0 {
		lands = 18
	}
	
	allSeeds := CalculateSeedExp(lands)
	if allSeeds == nil {
		return nil
	}
	
	// 筛选可用种子
	var available []*SeedExpInfo
	for _, seed := range allSeeds {
		if seed.RequiredLevel <= level {
			available = append(available, seed)
		}
	}
	
	if len(available) == 0 {
		return &PlantingRecommendation{
			Level: level,
			Lands: lands,
		}
	}
	
	// 按经验效率排序
	sort.Slice(available, func(i, j int) bool {
		return available[i].FarmExpPerHourNoFert > available[j].FarmExpPerHourNoFert
	})
	bestNoFert := available[0]
	
	sort.Slice(available, func(i, j int) bool {
		return available[i].FarmExpPerHourNormalFert > available[j].FarmExpPerHourNormalFert
	})
	bestNormalFert := available[0]
	
	// 获取前20候选
	topCount := 20
	if len(available) < topCount {
		topCount = len(available)
	}
	
	candidatesNoFert := make([]*SeedExpInfo, topCount)
	copy(candidatesNoFert, available)
	
	// 重新排序获取普通肥候选
	sort.Slice(available, func(i, j int) bool {
		return available[i].FarmExpPerHourNormalFert > available[j].FarmExpPerHourNormalFert
	})
	candidatesNormalFert := make([]*SeedExpInfo, topCount)
	copy(candidatesNormalFert, available)
	
	return &PlantingRecommendation{
		Level:                level,
		Lands:                lands,
		BestNoFert:           bestNoFert,
		BestNormalFert:       bestNormalFert,
		CandidatesNoFert:     candidatesNoFert,
		CandidatesNormalFert: candidatesNormalFert,
	}
}

// PrintRecommendation 打印推荐信息
func PrintRecommendation(rec *PlantingRecommendation) {
	if rec == nil {
		fmt.Println("无法获取种植推荐")
		return
	}
	
	fmt.Printf("\n========== 等级 Lv%d 种植推荐 (地块数: %d) ==========\n", rec.Level, rec.Lands)
	
	if rec.BestNoFert != nil {
		fmt.Printf("\n不施肥最优:\n")
		fmt.Printf("  种子: %s (ID: %d)\n", rec.BestNoFert.Name, rec.BestNoFert.SeedID)
		fmt.Printf("  等级要求: Lv%d\n", rec.BestNoFert.RequiredLevel)
		fmt.Printf("  生长时间: %s\n", rec.BestNoFert.GrowTimeStr)
		fmt.Printf("  每小时经验: %.2f\n", rec.BestNoFert.FarmExpPerHourNoFert)
		fmt.Printf("  每天经验: %.2f\n", rec.BestNoFert.FarmExpPerDayNoFert)
	}
	
	if rec.BestNormalFert != nil {
		fmt.Printf("\n普通肥最优:\n")
		fmt.Printf("  种子: %s (ID: %d)\n", rec.BestNormalFert.Name, rec.BestNormalFert.SeedID)
		fmt.Printf("  等级要求: Lv%d\n", rec.BestNormalFert.RequiredLevel)
		fmt.Printf("  生长时间: %s (施肥后: %s)\n", rec.BestNormalFert.GrowTimeStr, rec.BestNormalFert.GrowTimeNormalFertStr)
		fmt.Printf("  每小时经验: %.2f (+%.2f%%)\n", rec.BestNormalFert.FarmExpPerHourNormalFert, rec.BestNormalFert.GainPercent)
		fmt.Printf("  每天经验: %.2f\n", rec.BestNormalFert.FarmExpPerDayNormalFert)
	}
	
	fmt.Printf("\n不施肥 Top 10:\n")
	fmt.Printf("%-4s %-20s %-6s %-10s %-12s\n", "排名", "名称", "等级", "每小时经验", "生长时间")
	for i, seed := range rec.CandidatesNoFert {
		if i >= 10 {
			break
		}
		fmt.Printf("%-4d %-20s Lv%-4d %-12.2f %-12s\n", i+1, seed.Name, seed.RequiredLevel, seed.FarmExpPerHourNoFert, seed.GrowTimeStr)
	}
	
	fmt.Printf("\n普通肥 Top 10:\n")
	fmt.Printf("%-4s %-20s %-6s %-10s %-12s\n", "排名", "名称", "等级", "每小时经验", "施肥后生长")
	for i, seed := range rec.CandidatesNormalFert {
		if i >= 10 {
			break
		}
		fmt.Printf("%-4d %-20s Lv%-4d %-12.2f %-12s\n", i+1, seed.Name, seed.RequiredLevel, seed.FarmExpPerHourNormalFert, seed.GrowTimeNormalFertStr)
	}
	
	fmt.Println("\n=================================================")
}

// GetBestSeedForLevel 获取指定等级的最佳种子
func GetBestSeedForLevel(level, lands int, useNormalFert bool) *SeedExpInfo {
	rec := GetPlantingRecommendation(level, lands)
	if rec == nil {
		return nil
	}

	if useNormalFert {
		return rec.BestNormalFert
	}
	return rec.BestNoFert
}

// ExportToJSON 导出种子经验数据到JSON文件
func ExportToJSON(seeds []*SeedExpInfo, filename string) error {
	data, err := json.MarshalIndent(seeds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

// ExportToCSV 导出种子经验数据到CSV文件
func ExportToCSV(seeds []*SeedExpInfo, filename string) error {
	var lines []string

	// 表头
	headers := []string{
		"seedId", "name", "requiredLevel", "price", "expHarvest",
		"growTimeSec", "growTimeNormalFert", "cycleSecNoFert", "cycleSecNormalFert",
		"farmExpPerHourNoFert", "farmExpPerHourNormalFert",
		"farmExpPerDayNoFert", "farmExpPerDayNormalFert",
		"gainPercent", "expPerGoldSeed",
	}
	lines = append(lines, strings.Join(headers, ","))

	// 数据行
	for _, s := range seeds {
		values := []string{
			fmt.Sprintf("%d", s.SeedID),
			fmt.Sprintf("\"%s\"", s.Name),
			fmt.Sprintf("%d", s.RequiredLevel),
			fmt.Sprintf("%d", s.Price),
			fmt.Sprintf("%d", s.ExpHarvest),
			fmt.Sprintf("%d", s.GrowTimeSec),
			fmt.Sprintf("%d", s.GrowTimeNormalFert),
			fmt.Sprintf("%.2f", s.CycleSecNoFert),
			fmt.Sprintf("%.2f", s.CycleSecNormalFert),
			fmt.Sprintf("%.4f", s.FarmExpPerHourNoFert),
			fmt.Sprintf("%.4f", s.FarmExpPerHourNormalFert),
			fmt.Sprintf("%.2f", s.FarmExpPerDayNoFert),
			fmt.Sprintf("%.2f", s.FarmExpPerDayNormalFert),
			fmt.Sprintf("%.2f", s.GainPercent),
			fmt.Sprintf("%.4f", s.ExpPerGoldSeed),
		}
		lines = append(lines, strings.Join(values, ","))
	}

	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filename, []byte(content), 0644)
}

// ExportSummary 导出摘要文本
func ExportSummary(rec *PlantingRecommendation, filename string) error {
	if rec == nil {
		return fmt.Errorf("推荐数据为空")
	}

	var lines []string
	lines = append(lines, "经验收益率分析结果")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("等级: Lv%d", rec.Level))
	lines = append(lines, fmt.Sprintf("地块数: %d", rec.Lands))
	lines = append(lines, "")

	if rec.BestNoFert != nil {
		lines = append(lines, "不施肥最优:")
		lines = append(lines, fmt.Sprintf("  种子: %s (ID: %d)", rec.BestNoFert.Name, rec.BestNoFert.SeedID))
		lines = append(lines, fmt.Sprintf("  等级要求: Lv%d", rec.BestNoFert.RequiredLevel))
		lines = append(lines, fmt.Sprintf("  生长时间: %s", rec.BestNoFert.GrowTimeStr))
		lines = append(lines, fmt.Sprintf("  每小时经验: %.2f", rec.BestNoFert.FarmExpPerHourNoFert))
		lines = append(lines, fmt.Sprintf("  每天经验: %.2f", rec.BestNoFert.FarmExpPerDayNoFert))
		lines = append(lines, "")
	}

	if rec.BestNormalFert != nil {
		lines = append(lines, "普通肥最优:")
		lines = append(lines, fmt.Sprintf("  种子: %s (ID: %d)", rec.BestNormalFert.Name, rec.BestNormalFert.SeedID))
		lines = append(lines, fmt.Sprintf("  等级要求: Lv%d", rec.BestNormalFert.RequiredLevel))
		lines = append(lines, fmt.Sprintf("  生长时间: %s (施肥后: %s)", rec.BestNormalFert.GrowTimeStr, rec.BestNormalFert.GrowTimeNormalFertStr))
		lines = append(lines, fmt.Sprintf("  每小时经验: %.2f (+%.2f%%)", rec.BestNormalFert.FarmExpPerHourNormalFert, rec.BestNormalFert.GainPercent))
		lines = append(lines, fmt.Sprintf("  每天经验: %.2f", rec.BestNormalFert.FarmExpPerDayNormalFert))
		lines = append(lines, "")
	}

	lines = append(lines, "不施肥 Top 10:")
	lines = append(lines, "排名 | 名称 | 等级 | 每小时经验 | 生长时间")
	for i, seed := range rec.CandidatesNoFert {
		if i >= 10 {
			break
		}
		lines = append(lines, fmt.Sprintf("%2d | %s | Lv%d | %.2f | %s",
			i+1, seed.Name, seed.RequiredLevel, seed.FarmExpPerHourNoFert, seed.GrowTimeStr))
	}
	lines = append(lines, "")

	lines = append(lines, "普通肥 Top 10:")
	lines = append(lines, "排名 | 名称 | 等级 | 每小时经验 | 施肥后生长")
	for i, seed := range rec.CandidatesNormalFert {
		if i >= 10 {
			break
		}
		lines = append(lines, fmt.Sprintf("%2d | %s | Lv%d | %.2f | %s",
			i+1, seed.Name, seed.RequiredLevel, seed.FarmExpPerHourNormalFert, seed.GrowTimeNormalFertStr))
	}

	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filename, []byte(content), 0644)
}

// RunExpAnalysis 运行经验分析并导出结果
func RunExpAnalysis(level, lands int, outDir string) error {
	if level <= 0 {
		level = 1
	}
	if lands <= 0 {
		lands = 18
	}
	if outDir == "" {
		outDir = "."
	}

	fmt.Printf("正在计算等级 Lv%d、%d 块地的经验效率...\n", level, lands)

	// 计算所有种子的经验效率
	allSeeds := CalculateSeedExp(lands)
	if allSeeds == nil {
		return fmt.Errorf("无法计算种子经验数据")
	}

	// 获取推荐
	rec := GetPlantingRecommendation(level, lands)

	// 导出JSON
	jsonFile := filepath.Join(outDir, "exp-yield-result.json")
	if err := ExportToJSON(allSeeds, jsonFile); err != nil {
		return fmt.Errorf("导出JSON失败: %v", err)
	}
	fmt.Printf("[导出] JSON: %s\n", jsonFile)

	// 导出CSV
	csvFile := filepath.Join(outDir, "exp-yield-result.csv")
	if err := ExportToCSV(allSeeds, csvFile); err != nil {
		return fmt.Errorf("导出CSV失败: %v", err)
	}
	fmt.Printf("[导出] CSV: %s\n", csvFile)

	// 导出摘要
	txtFile := filepath.Join(outDir, "exp-yield-summary.txt")
	if err := ExportSummary(rec, txtFile); err != nil {
		return fmt.Errorf("导出摘要失败: %v", err)
	}
	fmt.Printf("[导出] 摘要: %s\n", txtFile)

	// 打印推荐
	PrintRecommendation(rec)

	fmt.Printf("\n分析完成！共 %d 个种子\n", len(allSeeds))
	return nil
}
