package game

import (
	"fmt"
	"sync"
	"time"

	"gofarm/internal/config"
	"gofarm/proto/gamepb/plantpb"
	"gofarm/proto/gamepb/shoppb"
	"gofarm/internal/network"
	"gofarm/tools"
	"gofarm/internal/utils"
)

// 普通肥料ID
const NormalFertilizerID = 1011

// 种子商店ID
const SeedShopID = 2

// FarmManager 农场管理器
type FarmManager struct {
	isChecking     bool
	isFirstCheck   bool
	checkTimer     *time.Timer
	loopRunning    bool
	networkEvents  *network.EventEmitter
	operationLimits map[int32]*plantpb.OperationLimit
	mu             sync.RWMutex
}

var Farm *FarmManager

func init() {
	Farm = &FarmManager{
		isFirstCheck:    true,
		networkEvents:   network.Net.GetEvents(),
		operationLimits: make(map[int32]*plantpb.OperationLimit),
	}
}

// GetAllLands 获取所有土地信息
func (fm *FarmManager) GetAllLands() (*plantpb.AllLandsReply, error) {
	req := &plantpb.AllLandsRequest{}
	resp := &plantpb.AllLandsReply{}
	
	err := network.Net.SendProtoMessage("gamepb.plantpb.PlantService", "AllLands", req, resp, 10*time.Second)
	if err != nil {
		return nil, err
	}
	
	// 更新操作限制
	if resp.OperationLimits != nil {
		fm.mu.Lock()
		for _, limit := range resp.OperationLimits {
			if limit != nil {
				fm.operationLimits[int32(limit.Id)] = limit
			}
		}
		fm.mu.Unlock()
	}
	
	return resp, nil
}

// Harvest 收获作物
func (fm *FarmManager) Harvest(landIds []int64) (*plantpb.HarvestReply, error) {
	state := network.Net.GetUserState()
	req := &plantpb.HarvestRequest{
		LandIds:  landIds,
		HostGid:  state.GID,
		IsAll:    true,
	}
	resp := &plantpb.HarvestReply{}
	
	err := network.Net.SendProtoMessage("gamepb.plantpb.PlantService", "Harvest", req, resp, 10*time.Second)
	return resp, err
}

// WaterLand 浇水
func (fm *FarmManager) WaterLand(landIds []int64, hostGID int64) (*plantpb.WaterLandReply, error) {
	req := &plantpb.WaterLandRequest{
		LandIds: landIds,
		HostGid: hostGID,
	}
	resp := &plantpb.WaterLandReply{}
	
	err := network.Net.SendProtoMessage("gamepb.plantpb.PlantService", "WaterLand", req, resp, 10*time.Second)
	return resp, err
}

// WeedOut 除草
func (fm *FarmManager) WeedOut(landIds []int64, hostGID int64) (*plantpb.WeedOutReply, error) {
	req := &plantpb.WeedOutRequest{
		LandIds: landIds,
		HostGid: hostGID,
	}
	resp := &plantpb.WeedOutReply{}
	
	err := network.Net.SendProtoMessage("gamepb.plantpb.PlantService", "WeedOut", req, resp, 10*time.Second)
	return resp, err
}

// Insecticide 除虫
func (fm *FarmManager) Insecticide(landIds []int64, hostGID int64) (*plantpb.InsecticideReply, error) {
	req := &plantpb.InsecticideRequest{
		LandIds: landIds,
		HostGid: hostGID,
	}
	resp := &plantpb.InsecticideReply{}
	
	err := network.Net.SendProtoMessage("gamepb.plantpb.PlantService", "Insecticide", req, resp, 10*time.Second)
	return resp, err
}

// Fertilize 施肥
func (fm *FarmManager) Fertilize(landIds []int64, fertilizerID int64) (int, error) {
	successCount := 0
	for _, landId := range landIds {
		req := &plantpb.FertilizeRequest{
			LandIds:      []int64{landId},
			FertilizerId: fertilizerID,
		}
		resp := &plantpb.FertilizeReply{}
		
		err := network.Net.SendProtoMessage("gamepb.plantpb.PlantService", "Fertilize", req, resp, 5*time.Second)
		if err != nil {
			// 施肥失败（可能肥料不足），停止继续
			break
		}
		successCount++
		
		if len(landIds) > 1 {
			time.Sleep(50 * time.Millisecond) // 50ms间隔
		}
	}
	return successCount, nil
}

// RemovePlant 铲除作物
func (fm *FarmManager) RemovePlant(landIds []int64) (*plantpb.RemovePlantReply, error) {
	req := &plantpb.RemovePlantRequest{
		LandIds: landIds,
	}
	resp := &plantpb.RemovePlantReply{}
	
	err := network.Net.SendProtoMessage("gamepb.plantpb.PlantService", "RemovePlant", req, resp, 10*time.Second)
	return resp, err
}

// PlantSeeds 种植
func (fm *FarmManager) PlantSeeds(seedID int64, landIds []int64) (int, error) {
	successCount := 0
	for _, landId := range landIds {
		req := &plantpb.PlantRequest{
			Items: []*plantpb.PlantItem{
				{
					SeedId:  seedID,
					LandIds: []int64{landId},
				},
			},
		}
		resp := &plantpb.PlantReply{}
		
		err := network.Net.SendProtoMessage("gamepb.plantpb.PlantService", "Plant", req, resp, 5*time.Second)
		if err != nil {
			utils.LogWarn("种植", fmt.Sprintf("土地#%d 失败: %v", landId, err))
			continue
		}
		successCount++
		
		if len(landIds) > 1 {
			time.Sleep(50 * time.Millisecond) // 50ms间隔
		}
	}
	return successCount, nil
}

// GetShopInfo 获取商店信息
func (fm *FarmManager) GetShopInfo(shopID int64) (*shoppb.ShopInfoReply, error) {
	req := &shoppb.ShopInfoRequest{
		ShopId: shopID,
	}
	resp := &shoppb.ShopInfoReply{}
	
	err := network.Net.SendProtoMessage("gamepb.shoppb.ShopService", "ShopInfo", req, resp, 10*time.Second)
	return resp, err
}

// BuyGoods 购买商品
func (fm *FarmManager) BuyGoods(goodsID int64, num int64, price int64) (*shoppb.BuyGoodsReply, error) {
	req := &shoppb.BuyGoodsRequest{
		GoodsId: goodsID,
		Num:     num,
		Price:   price,
	}
	resp := &shoppb.BuyGoodsReply{}
	
	err := network.Net.SendProtoMessage("gamepb.shoppb.ShopService", "BuyGoods", req, resp, 10*time.Second)
	return resp, err
}

// LandStatus 土地状态分析结果
type LandStatus struct {
	Harvestable     []int64
	NeedWater       []int64
	NeedWeed        []int64
	NeedBug         []int64
	Growing         []int64
	Empty           []int64
	Dead            []int64
	HarvestableInfo []HarvestablePlant
}

// HarvestablePlant 可收获植物信息
type HarvestablePlant struct {
	LandID   int64
	PlantID  int64
	Name     string
	Exp      int
}

// AnalyzeLands 分析土地状态
func (fm *FarmManager) AnalyzeLands(lands []*plantpb.LandInfo) *LandStatus {
	result := &LandStatus{
		Harvestable:     []int64{},
		NeedWater:       []int64{},
		NeedWeed:        []int64{},
		NeedBug:         []int64{},
		Growing:         []int64{},
		Empty:           []int64{},
		Dead:            []int64{},
		HarvestableInfo: []HarvestablePlant{},
	}
	
	nowSec := utils.GetServerTimeSec()
	
	for _, land := range lands {
		if land == nil || !land.Unlocked {
			continue
		}
		
		landID := land.Id
		plant := land.Plant
		
		if plant == nil || len(plant.Phases) == 0 {
			result.Empty = append(result.Empty, landID)
			continue
		}
		
		currentPhase := fm.getCurrentPhase(plant.Phases, nowSec)
		if currentPhase == nil {
			result.Empty = append(result.Empty, landID)
			continue
		}
		
		phaseVal := config.PlantPhase(currentPhase.Phase)
		
		switch phaseVal {
		case config.PlantPhaseDead:
			result.Dead = append(result.Dead, landID)
			
		case config.PlantPhaseMature:
			result.Harvestable = append(result.Harvestable, landID)
			plantID := plant.Id
			plantName := Config.GetPlantName(int(plantID))
			plantExp := Config.GetPlantExp(int(plantID))
			result.HarvestableInfo = append(result.HarvestableInfo, HarvestablePlant{
				LandID:  landID,
				PlantID: plantID,
				Name:    plantName,
				Exp:     plantExp,
			})
			
		default:
			// 生长中，检查需求
			needs := []string{}
			
			if plant.DryNum > 0 {
				result.NeedWater = append(result.NeedWater, landID)
				needs = append(needs, "缺水")
			}
			
			dryTime := utils.ToTimeSec(currentPhase.DryTime)
			if dryTime > 0 && dryTime <= nowSec {
				if !containsInt64(result.NeedWater, landID) {
					result.NeedWater = append(result.NeedWater, landID)
					needs = append(needs, "缺水")
				}
			}
			
			if len(plant.WeedOwners) > 0 {
				result.NeedWeed = append(result.NeedWeed, landID)
				needs = append(needs, "有草")
			}
			
			weedsTime := utils.ToTimeSec(currentPhase.WeedsTime)
			if weedsTime > 0 && weedsTime <= nowSec {
				if !containsInt64(result.NeedWeed, landID) {
					result.NeedWeed = append(result.NeedWeed, landID)
					needs = append(needs, "有草")
				}
			}
			
			if len(plant.InsectOwners) > 0 {
				result.NeedBug = append(result.NeedBug, landID)
				needs = append(needs, "有虫")
			}
			
			insectTime := utils.ToTimeSec(currentPhase.InsectTime)
			if insectTime > 0 && insectTime <= nowSec {
				if !containsInt64(result.NeedBug, landID) {
					result.NeedBug = append(result.NeedBug, landID)
					needs = append(needs, "有虫")
				}
			}
			
			result.Growing = append(result.Growing, landID)
		}
	}
	
	return result
}

// getCurrentPhase 获取当前生长阶段
func (fm *FarmManager) getCurrentPhase(phases []*plantpb.PlantPhaseInfo, nowSec int64) *plantpb.PlantPhaseInfo {
	if len(phases) == 0 {
		return nil
	}
	
	// 从后往前找，找到已开始的最晚阶段
	for i := len(phases) - 1; i >= 0; i-- {
		beginTime := utils.ToTimeSec(phases[i].BeginTime)
		if beginTime > 0 && beginTime <= nowSec {
			return phases[i]
		}
	}
	
	// 所有阶段都在未来，返回第一个
	return phases[0]
}

// CheckFarm 检查农场并执行操作
func (fm *FarmManager) CheckFarm() {
	if fm.isChecking {
		return
	}
	fm.isChecking = true
	defer func() { fm.isChecking = false }()
	
	state := network.Net.GetUserState()
	if state.GID == 0 {
		return
	}
	
	landsReply, err := fm.GetAllLands()
	if err != nil {
		utils.LogWarn("农场", fmt.Sprintf("获取土地失败: %v", err))
		return
	}
	
	if landsReply == nil || len(landsReply.Lands) == 0 {
		utils.Log("农场", "没有土地数据")
		return
	}
	
	status := fm.AnalyzeLands(landsReply.Lands)
	unlockedCount := 0
	for _, land := range landsReply.Lands {
		if land != nil && land.Unlocked {
			unlockedCount++
		}
	}
	
	fm.isFirstCheck = false
	
	// 构建状态摘要
	statusParts := []string{}
	if len(status.Harvestable) > 0 {
		statusParts = append(statusParts, fmt.Sprintf("收:%d", len(status.Harvestable)))
	}
	if len(status.NeedWeed) > 0 {
		statusParts = append(statusParts, fmt.Sprintf("草:%d", len(status.NeedWeed)))
	}
	if len(status.NeedBug) > 0 {
		statusParts = append(statusParts, fmt.Sprintf("虫:%d", len(status.NeedBug)))
	}
	if len(status.NeedWater) > 0 {
		statusParts = append(statusParts, fmt.Sprintf("水:%d", len(status.NeedWater)))
	}
	if len(status.Dead) > 0 {
		statusParts = append(statusParts, fmt.Sprintf("枯:%d", len(status.Dead)))
	}
	if len(status.Empty) > 0 {
		statusParts = append(statusParts, fmt.Sprintf("空:%d", len(status.Empty)))
	}
	statusParts = append(statusParts, fmt.Sprintf("长:%d", len(status.Growing)))
	
	hasWork := len(status.Harvestable) > 0 || len(status.NeedWeed) > 0 || 
	           len(status.NeedBug) > 0 || len(status.NeedWater) > 0 || 
	           len(status.Dead) > 0 || len(status.Empty) > 0
	
	actions := []string{}
	
	// 并行执行除草、除虫、浇水
	var wg sync.WaitGroup
	
	if len(status.NeedWeed) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := fm.WeedOut(status.NeedWeed, state.GID); err != nil {
				utils.LogWarn("除草", err.Error())
			} else {
				actions = append(actions, fmt.Sprintf("除草%d", len(status.NeedWeed)))
			}
		}()
	}
	
	if len(status.NeedBug) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := fm.Insecticide(status.NeedBug, state.GID); err != nil {
				utils.LogWarn("除虫", err.Error())
			} else {
				actions = append(actions, fmt.Sprintf("除虫%d", len(status.NeedBug)))
			}
		}()
	}
	
	if len(status.NeedWater) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := fm.WaterLand(status.NeedWater, state.GID); err != nil {
				utils.LogWarn("浇水", err.Error())
			} else {
				actions = append(actions, fmt.Sprintf("浇水%d", len(status.NeedWater)))
			}
		}()
	}
	
	wg.Wait()
	
	// 收获（支持延时）
	harvestedLandIds := []int64{}
	if len(status.Harvestable) > 0 {
		// 检查是否需要延时收获
		if config.Current.HarvestDelay > 0 {
			utils.Log("收获", fmt.Sprintf("等待 %v 后收获...", config.Current.HarvestDelay))
			time.Sleep(config.Current.HarvestDelay)
		}

		if _, err := fm.Harvest(status.Harvestable); err != nil {
			utils.LogWarn("收获", err.Error())
		} else {
			actions = append(actions, fmt.Sprintf("收获%d", len(status.Harvestable)))
			harvestedLandIds = append(harvestedLandIds, status.Harvestable...)
		}
	}
	
	// 铲除和种植
	allDeadLands := append(status.Dead, harvestedLandIds...)
	allEmptyLands := status.Empty
	
	if len(allDeadLands) > 0 || len(allEmptyLands) > 0 {
		if err := fm.AutoPlantEmptyLands(allDeadLands, allEmptyLands, unlockedCount); err != nil {
			utils.LogWarn("种植", err.Error())
		} else {
			actions = append(actions, fmt.Sprintf("种植%d", len(allDeadLands)+len(allEmptyLands)))
		}
	}
	
	// 输出日志
	if hasWork {
		actionStr := ""
		if len(actions) > 0 {
			actionStr = " → " + joinStrings(actions, "/")
		}
		utils.Log("农场", fmt.Sprintf("[%s]%s", joinStrings(statusParts, " "), actionStr))
	}
}

// AutoPlantEmptyLands 自动种植空地
func (fm *FarmManager) AutoPlantEmptyLands(deadLandIds, emptyLandIds []int64, unlockedCount int) error {
	state := network.Net.GetUserState()
	
	// 1. 铲除枯死作物
	landsToPlant := make([]int64, len(emptyLandIds))
	copy(landsToPlant, emptyLandIds)
	
	if len(deadLandIds) > 0 {
		if _, err := fm.RemovePlant(deadLandIds); err != nil {
			utils.LogWarn("铲除", fmt.Sprintf("批量铲除失败: %v", err))
		} else {
			utils.Log("铲除", fmt.Sprintf("已铲除 %d 块地", len(deadLandIds)))
			landsToPlant = append(landsToPlant, deadLandIds...)
		}
	}
	
	if len(landsToPlant) == 0 {
		return nil
	}
	
	// 2. 查询最佳种子
	bestSeed, err := fm.FindBestSeed(unlockedCount)
	if err != nil {
		return fmt.Errorf("查询种子失败: %w", err)
	}
	if bestSeed == nil {
		return fmt.Errorf("没有可购买的种子")
	}
	
	seedName := Config.GetPlantNameBySeedID(int(bestSeed.SeedId))
	growTime := Config.GetPlantGrowTime(int(1020000 + (bestSeed.SeedId - 20000)))
	growTimeStr := ""
	if growTime > 0 {
		growTimeStr = fmt.Sprintf(" 生长%s", FormatGrowTime(growTime))
	}
	
	utils.Log("商店", fmt.Sprintf("最佳种子: %s (%d) 价格=%d金币%s",
		seedName, bestSeed.SeedId, bestSeed.Price, growTimeStr))
	
	// 3. 购买种子
	needCount := len(landsToPlant)
	totalCost := bestSeed.Price * int64(needCount)
	
	if totalCost > state.Gold {
		utils.LogWarn("商店", fmt.Sprintf("金币不足! 需要 %d 金币, 当前 %d 金币", totalCost, state.Gold))
		canBuy := int(state.Gold / bestSeed.Price)
		if canBuy <= 0 {
			return fmt.Errorf("金币不足")
		}
		landsToPlant = landsToPlant[:canBuy]
		utils.Log("商店", fmt.Sprintf("金币有限，只种 %d 块地", canBuy))
	}
	
	actualSeedId := bestSeed.SeedId
	buyReply, err := fm.BuyGoods(bestSeed.GoodsId, int64(len(landsToPlant)), bestSeed.Price)
	if err != nil {
		return fmt.Errorf("购买失败: %w", err)
	}
	
	if len(buyReply.GetItems) > 0 {
		gotItem := buyReply.GetItems[0]
		if gotItem.Id > 0 {
			actualSeedId = gotItem.Id
		}
	}
	
	boughtName := Config.GetPlantNameBySeedID(int(actualSeedId))
	utils.Log("购买", fmt.Sprintf("已购买 %s种子 x%d, 花费 %d 金币",
		boughtName, len(landsToPlant), bestSeed.Price*int64(len(landsToPlant))))
	
	// 4. 种植
	planted, err := fm.PlantSeeds(actualSeedId, landsToPlant)
	if err != nil {
		return fmt.Errorf("种植失败: %w", err)
	}
	utils.Log("种植", fmt.Sprintf("已在 %d 块地种植", planted))
	
	// 5. 施肥
	if planted > 0 {
		plantedLands := landsToPlant[:planted]
		fertilized, _ := fm.Fertilize(plantedLands, NormalFertilizerID)
		if fertilized > 0 {
			utils.Log("施肥", fmt.Sprintf("已为 %d/%d 块地施肥", fertilized, len(plantedLands)))
		}
	}
	
	return nil
}

// SeedInfo 种子信息
type SeedInfo struct {
	Goods       *shoppb.GoodsInfo
	GoodsId     int64
	SeedId      int64
	Price       int64
	RequiredLevel int
}

// FindBestSeed 查找最佳种子
func (fm *FarmManager) FindBestSeed(landsCount int) (*SeedInfo, error) {
	shopReply, err := fm.GetShopInfo(SeedShopID)
	if err != nil {
		return nil, err
	}
	
	if len(shopReply.GoodsList) == 0 {
		return nil, fmt.Errorf("种子商店无商品")
	}
	
	state := network.Net.GetUserState()
	available := []*SeedInfo{}
	
	for _, goods := range shopReply.GoodsList {
		if goods == nil || !goods.Unlocked {
			continue
		}
		
		meetsConditions := true
		requiredLevel := 0
		
		for _, cond := range goods.Conds {
			if cond.Type == 1 { // 等级限制
				requiredLevel = int(cond.Param)
				if int64(state.Level) < cond.Param {
					meetsConditions = false
					break
				}
			}
		}
		
		if !meetsConditions {
			continue
		}
		
		// 检查限购
		if goods.LimitCount > 0 && goods.BoughtNum >= goods.LimitCount {
			continue
		}
		
		available = append(available, &SeedInfo{
			Goods:         goods,
			GoodsId:       goods.Id,
			SeedId:        goods.ItemId,
			Price:         goods.Price,
			RequiredLevel: requiredLevel,
		})
	}
	
	if len(available) == 0 {
		return nil, fmt.Errorf("没有可购买的种子")
	}
	
	// 如果强制种最低等级作物
	if config.Current.ForceLowestLevelCrop {
		// 按等级排序，选最低
		best := available[0]
		for _, s := range available {
			if s.RequiredLevel < best.RequiredLevel || 
			   (s.RequiredLevel == best.RequiredLevel && s.Price < best.Price) {
				best = s
			}
		}
		return best, nil
	}
	
	// 使用经验效率算法选择最佳种子
	// 获取当前等级和土地数量的最佳种子推荐
	rec := tools.GetPlantingRecommendation(state.Level, landsCount)
	if rec != nil && rec.BestNoFert != nil {
		bestSeed := rec.BestNoFert
		
		// 在可用种子中查找匹配的种子
		for _, s := range available {
			if s.SeedId == bestSeed.SeedID {
				utils.Log("种植", fmt.Sprintf("使用经验效率推荐: %s (每小时 %.2f 经验)", 
					bestSeed.Name, bestSeed.FarmExpPerHourNoFert))
				return s, nil
			}
		}
		
		// 如果没找到精确匹配，尝试按等级匹配
		for _, s := range available {
			if s.RequiredLevel == bestSeed.RequiredLevel {
				utils.Log("种植", fmt.Sprintf("使用经验效率推荐(同级): %s (每小时 %.2f 经验)", 
					bestSeed.Name, bestSeed.FarmExpPerHourNoFert))
				return s, nil
			}
		}
	}
	
	// 兜底策略：28级以前种白萝卜，28级以上种最高等级
	if state.Level <= 28 {
		// 选等级最低的
		best := available[0]
		for _, s := range available {
			if s.RequiredLevel < best.RequiredLevel {
				best = s
			}
		}
		return best, nil
	} else {
		// 选等级最高的
		best := available[0]
		for _, s := range available {
			if s.RequiredLevel > best.RequiredLevel {
				best = s
			}
		}
		return best, nil
	}
}

// StartFarmCheckLoop 启动农场巡查循环
func (fm *FarmManager) StartFarmCheckLoop() {
	if fm.loopRunning {
		return
	}
	fm.loopRunning = true
	
	// 监听土地变化推送
	fm.networkEvents.On("landsChanged", func(data interface{}) {
		if fm.isChecking {
			return
		}
		utils.Log("农场", "收到推送: 土地变化，检查中...")
		time.Sleep(100 * time.Millisecond)
		fm.CheckFarm()
	})
	
	// 延迟2秒后启动循环
	time.Sleep(2 * time.Second)
	
	go fm.farmCheckLoop()
}

// farmCheckLoop 巡查循环
func (fm *FarmManager) farmCheckLoop() {
	for fm.loopRunning {
		fm.CheckFarm()
		if !fm.loopRunning {
			break
		}
		time.Sleep(config.Current.FarmCheckInterval)
	}
}

// StopFarmCheckLoop 停止农场巡查循环
func (fm *FarmManager) StopFarmCheckLoop() {
	fm.loopRunning = false
	if fm.checkTimer != nil {
		fm.checkTimer.Stop()
	}
	fm.networkEvents.Off("landsChanged", nil)
}

// 辅助函数
func containsInt64(slice []int64, val int64) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
