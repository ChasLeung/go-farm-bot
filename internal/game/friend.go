package game

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"gofarm/internal/config"
	"gofarm/internal/network"
	"gofarm/proto/gamepb/friendpb"
	"gofarm/proto/gamepb/plantpb"
	"gofarm/proto/gamepb/visitpb"
	"gofarm/internal/utils"
)

// 操作类型ID常量
const (
	OpHarvest     = 10001 // 收获
	OpRemovePlant = 10002 // 铲除
	OpPutWeeds    = 10003 // 放草
	OpPutInsects  = 10004 // 放虫
	OpWeedOut     = 10005 // 帮好友除草
	OpInsecticide = 10006 // 帮好友除虫
	OpWaterLand   = 10007 // 帮好友浇水
	OpSteal       = 10008 // 偷菜
)

// 操作类型名称映射
var OpNames = map[int32]string{
	OpHarvest:     "收获",
	OpRemovePlant: "铲除",
	OpPutWeeds:    "放草",
	OpPutInsects:  "放虫",
	OpWeedOut:     "除草",
	OpInsecticide: "除虫",
	OpWaterLand:   "浇水",
	OpSteal:       "偷菜",
}

// FriendManager 好友管理器
type FriendManager struct {
	isCheckingFriends bool
	isFirstFriendCheck bool
	friendCheckTimer  *time.Timer
	friendLoopRunning bool
	lastResetDate     string
	networkEvents     *network.EventEmitter
	operationLimits   map[int32]*plantpb.OperationLimit
	expTracker        map[int32]int64 // opId -> 帮助前的 dayExpTimes
	expExhausted      map[int32]bool  // 经验已耗尽的操作类型
	mu                sync.RWMutex
}

var Friend *FriendManager

// 配置: 是否只在有经验时才帮助好友
const HelpOnlyWithExp = true

// 配置: 是否启用放虫放草功能 (默认关闭，避免被拉黑)
const EnablePutBadThings = false

func init() {
	Friend = &FriendManager{
		isFirstFriendCheck: true,
		lastResetDate:      getLocalDateKey(),
		networkEvents:      network.Net.GetEvents(),
		operationLimits:    make(map[int32]*plantpb.OperationLimit),
		expTracker:         make(map[int32]int64),
		expExhausted:       make(map[int32]bool),
	}
}

// getLocalDateKey 获取本地日期键 (YYYY-MM-DD)
func getLocalDateKey() string {
	now := time.Now()
	return fmt.Sprintf("%04d-%02d-%02d", now.Year(), now.Month(), now.Day())
}

// checkDailyReset 检查是否需要重置每日限制 (0点刷新)
func (fm *FriendManager) checkDailyReset() {
	today := getLocalDateKey()
	if today != fm.lastResetDate {
		fm.mu.Lock()
		fm.lastResetDate = today
		fm.operationLimits = make(map[int32]*plantpb.OperationLimit)
		fm.expTracker = make(map[int32]int64)
		fm.expExhausted = make(map[int32]bool)
		fm.mu.Unlock()
		utils.Log("好友系统", "每日限制已重置")
	}
}

// GetAllFriends 获取所有好友
func (fm *FriendManager) GetAllFriends() (*friendpb.GetAllReply, error) {
	req := &friendpb.GetAllRequest{}
	resp := &friendpb.GetAllReply{}
	
	err := network.Net.SendProtoMessage("gamepb.friendpb.FriendService", "GetAll", req, resp, 10*time.Second)
	return resp, err
}

// GetApplications 获取好友申请列表
func (fm *FriendManager) GetApplications() (*friendpb.GetApplicationsReply, error) {
	req := &friendpb.GetApplicationsRequest{}
	resp := &friendpb.GetApplicationsReply{}
	
	err := network.Net.SendProtoMessage("gamepb.friendpb.FriendService", "GetApplications", req, resp, 10*time.Second)
	return resp, err
}

// AcceptFriends 同意好友申请
func (fm *FriendManager) AcceptFriends(gids []int64) (*friendpb.AcceptFriendsReply, error) {
	req := &friendpb.AcceptFriendsRequest{
		FriendGids: gids,
	}
	resp := &friendpb.AcceptFriendsReply{}
	
	err := network.Net.SendProtoMessage("gamepb.friendpb.FriendService", "AcceptFriends", req, resp, 10*time.Second)
	return resp, err
}

// EnterFriendFarm 进入好友农场
func (fm *FriendManager) EnterFriendFarm(friendGid int64) (*visitpb.EnterReply, error) {
	req := &visitpb.EnterRequest{
		HostGid: friendGid,
		Reason:  int32(visitpb.EnterReason_ENTER_REASON_FRIEND),
	}
	resp := &visitpb.EnterReply{}
	
	err := network.Net.SendProtoMessage("gamepb.visitpb.VisitService", "Enter", req, resp, 10*time.Second)
	return resp, err
}

// LeaveFriendFarm 离开好友农场
func (fm *FriendManager) LeaveFriendFarm(friendGid int64) {
	req := &visitpb.LeaveRequest{
		HostGid: friendGid,
	}
	resp := &visitpb.LeaveReply{}
	
	// 离开失败不影响主流程
	_ = network.Net.SendProtoMessage("gamepb.visitpb.VisitService", "Leave", req, resp, 5*time.Second)
}

// StealFromFriend 从好友农场偷菜
func (fm *FriendManager) StealFromFriend(landIds []int64, hostGID int64) (*plantpb.HarvestReply, error) {
	req := &plantpb.HarvestRequest{
		LandIds: landIds,
		HostGid: hostGID,
		IsAll:   false,
	}
	resp := &plantpb.HarvestReply{}
	
	err := network.Net.SendProtoMessage("gamepb.plantpb.PlantService", "Harvest", req, resp, 10*time.Second)
	
	// 更新操作限制
	if err == nil && resp.OperationLimits != nil {
		fm.updateOperationLimits(resp.OperationLimits)
	}
	
	return resp, err
}

// HelpWaterLand 帮好友浇水
func (fm *FriendManager) HelpWaterLand(landIds []int64, hostGID int64) (*plantpb.WaterLandReply, error) {
	return Farm.WaterLand(landIds, hostGID)
}

// HelpWeedOut 帮好友除草
func (fm *FriendManager) HelpWeedOut(landIds []int64, hostGID int64) (*plantpb.WeedOutReply, error) {
	return Farm.WeedOut(landIds, hostGID)
}

// HelpInsecticide 帮好友除虫
func (fm *FriendManager) HelpInsecticide(landIds []int64, hostGID int64) (*plantpb.InsecticideReply, error) {
	return Farm.Insecticide(landIds, hostGID)
}

// PutWeeds 放草
func (fm *FriendManager) PutWeeds(landIds []int64, hostGID int64) (*plantpb.PutWeedsReply, error) {
	req := &plantpb.PutWeedsRequest{
		LandIds: landIds,
		HostGid: hostGID,
	}
	resp := &plantpb.PutWeedsReply{}
	
	err := network.Net.SendProtoMessage("gamepb.plantpb.PlantService", "PutWeeds", req, resp, 10*time.Second)
	
	// 更新操作限制
	if err == nil && resp.OperationLimits != nil {
		fm.updateOperationLimits(resp.OperationLimits)
	}
	
	return resp, err
}

// PutInsects 放虫
func (fm *FriendManager) PutInsects(landIds []int64, hostGID int64) (*plantpb.PutInsectsReply, error) {
	req := &plantpb.PutInsectsRequest{
		LandIds: landIds,
		HostGid: hostGID,
	}
	resp := &plantpb.PutInsectsReply{}
	
	err := network.Net.SendProtoMessage("gamepb.plantpb.PlantService", "PutInsects", req, resp, 10*time.Second)
	
	// 更新操作限制
	if err == nil && resp.OperationLimits != nil {
		fm.updateOperationLimits(resp.OperationLimits)
	}
	
	return resp, err
}

// updateOperationLimits 更新操作限制
func (fm *FriendManager) updateOperationLimits(limits []*plantpb.OperationLimit) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	
	for _, limit := range limits {
		if limit != nil {
			opId := int32(limit.Id)
			fm.operationLimits[opId] = limit
			
			// 检查经验是否耗尽
			if HelpOnlyWithExp {
				if beforeExp, ok := fm.expTracker[opId]; ok {
					if limit.DayExpTimes <= beforeExp {
						// 经验没有增长，标记为已耗尽
						if !fm.expExhausted[opId] {
							fm.expExhausted[opId] = true
							utils.Log("好友系统", fmt.Sprintf("操作 %s 今日经验已耗尽", OpNames[opId]))
						}
					}
				}
			}
		}
	}
}

// canGetExp 检查操作是否还能获得经验
func (fm *FriendManager) canGetExp(opId int32) bool {
	if !HelpOnlyWithExp {
		return true
	}
	
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	
	return !fm.expExhausted[opId]
}

// trackExpBefore 记录操作前的经验值
func (fm *FriendManager) trackExpBefore(opId int32) {
	if !HelpOnlyWithExp {
		return
	}
	
	fm.mu.Lock()
	defer fm.mu.Unlock()
	
	if limit, ok := fm.operationLimits[opId]; ok && limit != nil {
		fm.expTracker[opId] = limit.DayExpTimes
	}
}

// isLimitReached 检查操作是否达到限制
func (fm *FriendManager) isLimitReached(opId int32) bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	
	limit, ok := fm.operationLimits[opId]
	if !ok || limit == nil {
		return false
	}
	
	// 检查是否达到每日次数上限
	if limit.DayTimesLt > 0 && limit.DayTimes >= limit.DayTimesLt {
		return true
	}
	
	return false
}

// getRemainingTimes 获取剩余操作次数
func (fm *FriendManager) getRemainingTimes(opId int32) int64 {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	
	limit, ok := fm.operationLimits[opId]
	if !ok || limit == nil {
		return -1 // 无限制信息
	}
	
	if limit.DayTimesLt <= 0 {
		return -1 // 无上限
	}
	
	remaining := limit.DayTimesLt - limit.DayTimes
	if remaining < 0 {
		return 0
	}
	return remaining
}

// FriendLandStatus 好友农场土地状态
type FriendLandStatus struct {
	CanSteal      []int64           // 可偷的土地
	NeedWater     []int64           // 需要浇水的土地
	NeedWeed      []int64           // 需要除草的土地
	NeedBug       []int64           // 需要除虫的土地
	CanPutWeeds   []int64           // 可以放草的土地
	CanPutInsects []int64           // 可以放虫的土地
	StealInfo     []StealablePlant  // 可偷作物信息
}

// StealablePlant 可偷作物信息
type StealablePlant struct {
	LandID    int64
	PlantID   int64
	PlantName string
	FruitNum  int64
}

// AnalyzeFriendLands 分析好友农场土地状态
func (fm *FriendManager) AnalyzeFriendLands(lands []*plantpb.LandInfo) *FriendLandStatus {
	result := &FriendLandStatus{
		CanSteal:      []int64{},
		NeedWater:     []int64{},
		NeedWeed:      []int64{},
		NeedBug:       []int64{},
		CanPutWeeds:   []int64{},
		CanPutInsects: []int64{},
		StealInfo:     []StealablePlant{},
	}
	
	nowSec := utils.GetServerTimeSec()
	
	for _, land := range lands {
		if land == nil || !land.Unlocked {
			continue
		}
		
		landID := land.Id
		plant := land.Plant
		
		// 如果土地没有作物，直接处理空地逻辑
		if plant == nil {
			// 空地可以放虫放草
			if EnablePutBadThings {
				result.CanPutWeeds = append(result.CanPutWeeds, landID)
				result.CanPutInsects = append(result.CanPutInsects, landID)
			}
			continue
		}
		
		// 获取当前生长阶段（先获取，用于判断各种状态）
		currentPhase := fm.getCurrentPhase(plant.Phases, nowSec)
		if currentPhase == nil {
			continue
		}
		
		phaseVal := config.PlantPhase(currentPhase.Phase)
		
		// 检查是否可以偷菜：必须同时满足：1. 成熟阶段 2. Stealable=true 3. 有剩余果实
		if phaseVal == config.PlantPhaseMature && plant.Stealable && plant.LeftFruitNum > 0 {
			result.CanSteal = append(result.CanSteal, landID)
			plantName := Config.GetPlantName(int(plant.Id))
			result.StealInfo = append(result.StealInfo, StealablePlant{
				LandID:    landID,
				PlantID:   plant.Id,
				PlantName: plantName,
				FruitNum:  plant.LeftFruitNum,
			})
		}
		
		// 检查是否需要浇水
		if plant.DryNum > 0 {
			result.NeedWater = append(result.NeedWater, landID)
		}
		
		dryTime := utils.ToTimeSec(currentPhase.DryTime)
		if dryTime > 0 && dryTime <= nowSec {
			if !containsInt64(result.NeedWater, landID) {
				result.NeedWater = append(result.NeedWater, landID)
			}
		}
		
		// 检查是否需要除草
		if len(plant.WeedOwners) > 0 {
			result.NeedWeed = append(result.NeedWeed, landID)
		}
		
		weedsTime := utils.ToTimeSec(currentPhase.WeedsTime)
		if weedsTime > 0 && weedsTime <= nowSec {
			if !containsInt64(result.NeedWeed, landID) {
				result.NeedWeed = append(result.NeedWeed, landID)
			}
		}
		
		// 检查是否需要除虫
		if len(plant.InsectOwners) > 0 {
			result.NeedBug = append(result.NeedBug, landID)
		}
		
		insectTime := utils.ToTimeSec(currentPhase.InsectTime)
		if insectTime > 0 && insectTime <= nowSec {
			if !containsInt64(result.NeedBug, landID) {
				result.NeedBug = append(result.NeedBug, landID)
			}
		}
		
		// 检查是否可以放虫放草 (作物在生长中，不是枯死也不是成熟)
		if EnablePutBadThings && phaseVal != config.PlantPhaseDead && phaseVal != config.PlantPhaseMature {
			result.CanPutWeeds = append(result.CanPutWeeds, landID)
			result.CanPutInsects = append(result.CanPutInsects, landID)
		}
	}
	
	return result
}

// getCurrentPhase 获取当前生长阶段 (从farm.go复用)
func (fm *FriendManager) getCurrentPhase(phases []*plantpb.PlantPhaseInfo, nowSec int64) *plantpb.PlantPhaseInfo {
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

// CheckFriendFarm 检查单个好友农场
func (fm *FriendManager) CheckFriendFarm(friend *friendpb.GameFriend) {
	if friend == nil {
		return
	}
	
	friendGid := friend.Gid
	friendName := friend.Name
	
	// 进入好友农场
	utils.Log("好友巡查", fmt.Sprintf("进入 %s 的农场 (GID: %d)", friendName, friendGid))
	
	enterReply, err := fm.EnterFriendFarm(friendGid)
	if err != nil {
		utils.LogWarn("好友巡查", fmt.Sprintf("进入 %s 的农场失败: %v", friendName, err))
		return
	}
	
	// 确保离开农场
	defer fm.LeaveFriendFarm(friendGid)
	
	lands := enterReply.Lands
	if len(lands) == 0 {
		return
	}
	
	// 分析土地状态
	status := fm.AnalyzeFriendLands(lands)
	
	// 执行操作
	fm.performFriendOperations(friendGid, friendName, status)
}

// performFriendOperations 执行好友农场操作
func (fm *FriendManager) performFriendOperations(friendGid int64, friendName string, status *FriendLandStatus) {
	// 1. 偷菜 (优先级最高)
	if len(status.CanSteal) > 0 && !fm.isLimitReached(OpSteal) {
		stealCount := 0
		plantNameSet := make(map[string]bool)
		
		for _, info := range status.StealInfo {
			if fm.isLimitReached(OpSteal) {
				break
			}
			
			_, err := fm.StealFromFriend([]int64{info.LandID}, friendGid)
			if err != nil {
				utils.LogWarn("偷菜", fmt.Sprintf("从 %s 的土地#%d 偷菜失败: %v", friendName, info.LandID, err))
				continue
			}
			
			stealCount++
			plantNameSet[info.PlantName] = true
			
			// 偷菜间隔
			time.Sleep(100 * time.Millisecond)
		}
		
		if stealCount > 0 {
			// 构建植物名称列表（去重）
			plantNames := make([]string, 0, len(plantNameSet))
			for name := range plantNameSet {
				plantNames = append(plantNames, name)
			}
			utils.Log("偷菜", fmt.Sprintf("从 %s 偷了 %d 块地的(%s)",
				friendName, stealCount, strings.Join(plantNames, "/")))
		}
	}
	
	// 2. 帮好友浇水
	if len(status.NeedWater) > 0 && fm.canGetExp(OpWaterLand) && !fm.isLimitReached(OpWaterLand) {
		fm.trackExpBefore(OpWaterLand)
		
		watered := int64(0)
		for _, landID := range status.NeedWater {
			if fm.isLimitReached(OpWaterLand) {
				break
			}
			
			_, err := fm.HelpWaterLand([]int64{landID}, friendGid)
			if err != nil {
				continue
			}
			watered++
			time.Sleep(50 * time.Millisecond)
		}
		
		if watered > 0 {
			utils.Log("帮浇水", fmt.Sprintf("帮 %s 浇了 %d 块地", friendName, watered))
		}
	}
	
	// 3. 帮好友除草
	if len(status.NeedWeed) > 0 && fm.canGetExp(OpWeedOut) && !fm.isLimitReached(OpWeedOut) {
		fm.trackExpBefore(OpWeedOut)
		
		weeded := int64(0)
		for _, landID := range status.NeedWeed {
			if fm.isLimitReached(OpWeedOut) {
				break
			}
			
			_, err := fm.HelpWeedOut([]int64{landID}, friendGid)
			if err != nil {
				continue
			}
			weeded++
			time.Sleep(50 * time.Millisecond)
		}
		
		if weeded > 0 {
			utils.Log("帮除草", fmt.Sprintf("帮 %s 除了 %d 块地的草", friendName, weeded))
		}
	}
	
	// 4. 帮好友除虫
	if len(status.NeedBug) > 0 && fm.canGetExp(OpInsecticide) && !fm.isLimitReached(OpInsecticide) {
		fm.trackExpBefore(OpInsecticide)
		
		bugged := int64(0)
		for _, landID := range status.NeedBug {
			if fm.isLimitReached(OpInsecticide) {
				break
			}
			
			_, err := fm.HelpInsecticide([]int64{landID}, friendGid)
			if err != nil {
				continue
			}
			bugged++
			time.Sleep(50 * time.Millisecond)
		}
		
		if bugged > 0 {
			utils.Log("帮除虫", fmt.Sprintf("帮 %s 除了 %d 块地的虫", friendName, bugged))
		}
	}
	
	// 5. 放虫放草 (默认关闭)
	if EnablePutBadThings {
		// 放草
		if len(status.CanPutWeeds) > 0 && !fm.isLimitReached(OpPutWeeds) {
			// 随机选择一块地放草
			landID := status.CanPutWeeds[0]
			_, err := fm.PutWeeds([]int64{landID}, friendGid)
			if err == nil {
				utils.Log("放草", fmt.Sprintf("在 %s 的土地#%d 放了草", friendName, landID))
			}
		}
		
		// 放虫
		if len(status.CanPutInsects) > 0 && !fm.isLimitReached(OpPutInsects) {
			// 随机选择一块地放虫
			landID := status.CanPutInsects[0]
			_, err := fm.PutInsects([]int64{landID}, friendGid)
			if err == nil {
				utils.Log("放虫", fmt.Sprintf("在 %s 的土地#%d 放了虫", friendName, landID))
			}
		}
	}
}

// CheckAllFriends 检查所有好友农场
func (fm *FriendManager) CheckAllFriends() {
	if fm.isCheckingFriends {
		return
	}
	fm.isCheckingFriends = true
	defer func() { fm.isCheckingFriends = false }()
	
	// 检查每日重置
	fm.checkDailyReset()
	
	// 获取好友列表
	friendsReply, err := fm.GetAllFriends()
	if err != nil {
		utils.LogWarn("好友系统", fmt.Sprintf("获取好友列表失败: %v", err))
		return
	}
	
	friends := friendsReply.GameFriends
	if len(friends) == 0 {
		utils.Log("好友系统", "没有好友")
		return
	}
	
	utils.Log("好友系统", fmt.Sprintf("开始巡查 %d 位好友的农场", len(friends)))
	
	// 遍历好友
	for i, friend := range friends {
		if friend == nil {
			continue
		}
		
		// 检查好友农场摘要信息
		plant := friend.Plant
		if plant == nil {
			continue
		}
		
		// 快速筛选：有可偷作物、需要帮助的好友
		hasAction := false
		actionHints := []string{}
		
		if plant.StealPlantNum > 0 && !fm.isLimitReached(OpSteal) {
			hasAction = true
			actionHints = append(actionHints, fmt.Sprintf("可偷%d个", plant.StealPlantNum))
		}
		
		if plant.DryNum > 0 && fm.canGetExp(OpWaterLand) && !fm.isLimitReached(OpWaterLand) {
			hasAction = true
			actionHints = append(actionHints, fmt.Sprintf("需浇水%d块", plant.DryNum))
		}
		
		if plant.WeedNum > 0 && fm.canGetExp(OpWeedOut) && !fm.isLimitReached(OpWeedOut) {
			hasAction = true
			actionHints = append(actionHints, fmt.Sprintf("需除草%d块", plant.WeedNum))
		}
		
		if plant.InsectNum > 0 && fm.canGetExp(OpInsecticide) && !fm.isLimitReached(OpInsecticide) {
			hasAction = true
			actionHints = append(actionHints, fmt.Sprintf("需除虫%d块", plant.InsectNum))
		}
		
		if !hasAction {
			continue
		}
		
		utils.Log("好友巡查", fmt.Sprintf("[%d/%d] %s: %s", i+1, len(friends), friend.Name, actionHints))
		
		// 检查该好友农场
		fm.CheckFriendFarm(friend)
		
		// 好友间巡查间隔
		time.Sleep(config.Current.FriendCheckInterval)
	}
	
	utils.Log("好友系统", "好友农场巡查完成")
	fm.isFirstFriendCheck = false
}

// AcceptAllApplications 同意所有好友申请
func (fm *FriendManager) AcceptAllApplications() {
	reply, err := fm.GetApplications()
	if err != nil {
		utils.LogWarn("好友系统", fmt.Sprintf("获取好友申请失败: %v", err))
		return
	}
	
	applications := reply.Applications
	if len(applications) == 0 {
		return
	}
	
	gids := []int64{}
	for _, app := range applications {
		if app != nil {
			gids = append(gids, app.Gid)
		}
	}
	
	if len(gids) == 0 {
		return
	}
	
	_, err = fm.AcceptFriends(gids)
	if err != nil {
		utils.LogWarn("好友系统", fmt.Sprintf("同意好友申请失败: %v", err))
		return
	}
	
	utils.Log("好友系统", fmt.Sprintf("已同意 %d 个好友申请", len(gids)))
}

// StartFriendCheckLoop 启动好友巡查循环
func (fm *FriendManager) StartFriendCheckLoop() {
	if fm.friendLoopRunning {
		return
	}
	
	fm.friendLoopRunning = true
	utils.Log("好友系统", "好友巡查循环已启动")
	
	// 立即执行一次
	go fm.CheckAllFriends()
	
	// 定时器循环
	go func() {
		for fm.friendLoopRunning {
			// 等待间隔时间
			time.Sleep(config.Current.FriendCheckInterval)
			
			if !fm.friendLoopRunning {
				break
			}
			
			// 执行好友巡查
			fm.CheckAllFriends()
		}
	}()
	
	// 监听土地变化推送 (可能是有好友来偷菜或帮忙)
	fm.networkEvents.On("lands_notify", func(data interface{}) {
		// 收到土地变化通知，可以触发一次好友巡查
		// 但为了避免过于频繁，这里可以添加节流逻辑
		// TODO: 实现节流逻辑
	})
}

// StopFriendCheckLoop 停止好友巡查循环
func (fm *FriendManager) StopFriendCheckLoop() {
	fm.friendLoopRunning = false
	if fm.friendCheckTimer != nil {
		fm.friendCheckTimer.Stop()
	}
	utils.Log("好友系统", "好友巡查循环已停止")
}

// IsLoopRunning 检查循环是否正在运行
func (fm *FriendManager) IsLoopRunning() bool {
	return fm.friendLoopRunning
}
