package game

import (
	"fmt"
	"sync"
	"time"

	"gofarm/internal/network"
	"gofarm/proto/corepb"
	"gofarm/proto/gamepb/taskpb"
	"gofarm/internal/utils"
)

// TaskManager 任务管理器
type TaskManager struct {
	isChecking      bool
	checkTimer      *time.Timer
	loopRunning     bool
	networkEvents   *network.EventEmitter
	taskInfo        *taskpb.TaskInfo
	mu              sync.RWMutex
}

var Task *TaskManager

// 任务类型常量
const (
	TaskTypeGrowth = 1 // 成长任务
	TaskTypeDaily  = 2 // 每日任务
)

// 配置: 任务检查间隔
const TaskCheckInterval = 5 * time.Minute // 每5分钟检查一次任务

func init() {
	Task = &TaskManager{
		networkEvents: network.Net.GetEvents(),
	}
}

// GetTaskInfo 获取任务信息
func (tm *TaskManager) GetTaskInfo() (*taskpb.TaskInfoReply, error) {
	req := &taskpb.TaskInfoRequest{}
	resp := &taskpb.TaskInfoReply{}
	
	err := network.Net.SendProtoMessage("gamepb.taskpb.TaskService", "TaskInfo", req, resp, 10*time.Second)
	if err != nil {
		return nil, err
	}
	
	// 更新本地任务信息
	tm.mu.Lock()
	tm.taskInfo = resp.TaskInfo
	tm.mu.Unlock()
	
	return resp, nil
}

// ClaimTaskReward 领取单个任务奖励
func (tm *TaskManager) ClaimTaskReward(taskID int64, doShared bool) (*taskpb.ClaimTaskRewardReply, error) {
	req := &taskpb.ClaimTaskRewardRequest{
		Id:        taskID,
		DoShared:  doShared,
	}
	resp := &taskpb.ClaimTaskRewardReply{}
	
	err := network.Net.SendProtoMessage("gamepb.taskpb.TaskService", "ClaimTaskReward", req, resp, 10*time.Second)
	if err != nil {
		return nil, err
	}
	
	// 更新本地任务信息
	if resp.TaskInfo != nil {
		tm.mu.Lock()
		tm.taskInfo = resp.TaskInfo
		tm.mu.Unlock()
	}
	
	return resp, nil
}

// BatchClaimTaskReward 批量领取任务奖励
func (tm *TaskManager) BatchClaimTaskReward(taskIDs []int64, doShared bool) (*taskpb.BatchClaimTaskRewardReply, error) {
	req := &taskpb.BatchClaimTaskRewardRequest{
		Ids:       taskIDs,
		DoShared:  doShared,
	}
	resp := &taskpb.BatchClaimTaskRewardReply{}
	
	err := network.Net.SendProtoMessage("gamepb.taskpb.TaskService", "BatchClaimTaskReward", req, resp, 10*time.Second)
	if err != nil {
		return nil, err
	}
	
	// 更新本地任务信息
	if resp.TaskInfo != nil {
		tm.mu.Lock()
		tm.taskInfo = resp.TaskInfo
		tm.mu.Unlock()
	}
	
	return resp, nil
}

// ClaimableTask 可领取的任务信息
type ClaimableTask struct {
	ID            int64
	Desc          string
	ShareMultiple int64
	Rewards       []*corepb.Item
	TaskType      int32
}

// AnalyzeTasks 分析任务列表，找出可领取的任务
func (tm *TaskManager) AnalyzeTasks(taskInfo *taskpb.TaskInfo) []*ClaimableTask {
	if taskInfo == nil {
		return nil
	}
	
	var allTasks []*taskpb.Task
	allTasks = append(allTasks, taskInfo.GrowthTasks...)
	allTasks = append(allTasks, taskInfo.DailyTasks...)
	allTasks = append(allTasks, taskInfo.Tasks...)
	
	var claimable []*ClaimableTask
	seenIDs := make(map[int64]bool) // 用于去重
	
	for _, task := range allTasks {
		if task == nil {
			continue
		}
		
		// 跳过重复的任务ID
		if seenIDs[task.Id] {
			continue
		}
		seenIDs[task.Id] = true
		
		// 可领取条件: 已解锁 + 未领取 + 进度完成
		if task.IsUnlocked && !task.IsClaimed && task.Progress >= task.TotalProgress && task.TotalProgress > 0 {
			desc := task.Desc
			if desc == "" {
				desc = fmt.Sprintf("任务#%d", task.Id)
			}
			
			claimable = append(claimable, &ClaimableTask{
				ID:            task.Id,
				Desc:          desc,
				ShareMultiple: task.ShareMultiple,
				TaskType:      task.TaskType,
			})
		}
	}
	
	return claimable
}

// GetRewardSummary 计算奖励摘要
func (tm *TaskManager) GetRewardSummary(items []*corepb.Item) string {
	if len(items) == 0 {
		return "无"
	}
	
	var summaries []string
	for _, item := range items {
		if item == nil {
			continue
		}
		
		itemID := item.Id
		count := item.Count
		
		// 常见物品ID: 1=金币, 2=经验
		var name string
		switch itemID {
		case 1:
			name = "金币"
		case 2:
			name = "经验"
		default:
			name = Config.GetItemName(int(itemID))
		}
		
		summaries = append(summaries, fmt.Sprintf("%s x%d", name, count))
	}
	
	if len(summaries) == 0 {
		return "无"
	}
	return fmt.Sprintf("%v", summaries)
}

// CheckAndClaimTasks 检查并领取所有可领取的任务奖励
func (tm *TaskManager) CheckAndClaimTasks() {
	if tm.isChecking {
		return
	}
	tm.isChecking = true
	defer func() { tm.isChecking = false }()

	// 获取任务信息
	reply, err := tm.GetTaskInfo()
	if err != nil {
		utils.LogWarn("任务系统", fmt.Sprintf("获取任务信息失败: %v", err))
		return
	}

	if reply.TaskInfo == nil {
		return
	}

	// 直接在这里执行领取逻辑，而不是调用 checkAndClaimFromTaskInfo
	// 避免重复检查 isChecking 标志
	tm.doClaimTasks(reply.TaskInfo)
}

// checkAndClaimFromTaskInfo 从任务信息中检查并领取奖励（供推送处理使用）
func (tm *TaskManager) checkAndClaimFromTaskInfo(taskInfo *taskpb.TaskInfo) {
	if tm.isChecking {
		return
	}
	tm.isChecking = true
	defer func() { tm.isChecking = false }()

	tm.doClaimTasks(taskInfo)
}

// doClaimTasks 执行领取任务的核心逻辑
func (tm *TaskManager) doClaimTasks(taskInfo *taskpb.TaskInfo) {
	if taskInfo == nil {
		return
	}

	// 分析可领取的任务
	claimable := tm.AnalyzeTasks(taskInfo)
	if len(claimable) == 0 {
		return
	}

	utils.Log("任务系统", fmt.Sprintf("发现 %d 个可领取任务", len(claimable)))

	// 逐个领取任务，根据每个任务的 ShareMultiple 决定是否使用分享翻倍
	for _, task := range claimable {
		// 如果任务有分享翻倍（ShareMultiple > 1），则使用翻倍领取
		useShare := task.ShareMultiple > 1
		multipleStr := ""
		if useShare {
			multipleStr = fmt.Sprintf(" (%dx)", task.ShareMultiple)
		}

		reply, err := tm.ClaimTaskReward(task.ID, useShare)
		if err != nil {
			utils.LogWarn("任务系统", fmt.Sprintf("领取任务 #%d %s%s 失败: %v", task.ID, task.Desc, multipleStr, err))
			continue
		}

		// 记录获得的奖励
		rewardSummary := tm.formatRewardItems(reply.Items)
		utils.Log("任务系统", fmt.Sprintf("领取 #%d: %s%s → %s", task.ID, task.Desc, multipleStr, rewardSummary))

		// 间隔，避免请求过快
		time.Sleep(300 * time.Millisecond)
	}
}

// formatRewardItems 格式化奖励物品
func (tm *TaskManager) formatRewardItems(items []*corepb.Item) string {
	if len(items) == 0 {
		return "无"
	}
	
	var summaries []string
	for _, item := range items {
		if item == nil {
			continue
		}
		
		itemID := item.Id
		count := item.Count
		
		// 常见物品ID: 1=金币, 2=经验
		var name string
		switch itemID {
		case 1:
			name = "金币"
		case 2:
			name = "经验"
		default:
			name = Config.GetItemName(int(itemID))
		}
		
		summaries = append(summaries, fmt.Sprintf("%s x%d", name, count))
	}
	
	if len(summaries) == 0 {
		return "无"
	}
	
	return fmt.Sprintf("%v", summaries)
}

// GetTaskStats 获取任务统计信息
func (tm *TaskManager) GetTaskStats() map[string]interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	if tm.taskInfo == nil {
		return map[string]interface{}{
			"growth_total":   0,
			"growth_claimed": 0,
			"daily_total":    0,
			"daily_claimed":  0,
		}
	}
	
	growthTotal := len(tm.taskInfo.GrowthTasks)
	growthClaimed := 0
	for _, task := range tm.taskInfo.GrowthTasks {
		if task != nil && task.IsClaimed {
			growthClaimed++
		}
	}
	
	dailyTotal := len(tm.taskInfo.DailyTasks)
	dailyClaimed := 0
	for _, task := range tm.taskInfo.DailyTasks {
		if task != nil && task.IsClaimed {
			dailyClaimed++
		}
	}
	
	return map[string]interface{}{
		"growth_total":   growthTotal,
		"growth_claimed": growthClaimed,
		"daily_total":    dailyTotal,
		"daily_claimed":  dailyClaimed,
	}
}

// StartTaskCheckLoop 启动任务检查循环
func (tm *TaskManager) StartTaskCheckLoop() {
	if tm.loopRunning {
		return
	}
	
	tm.loopRunning = true
	utils.Log("任务系统", "任务检查循环已启动")
	
	// 立即执行一次
	go tm.CheckAndClaimTasks()
	
	// 定时器循环
	go func() {
		for tm.loopRunning {
			// 等待间隔时间
			time.Sleep(TaskCheckInterval)
			
			if !tm.loopRunning {
				break
			}
			
			// 检查并领取任务
			tm.CheckAndClaimTasks()
		}
	}()
	
	// 监听任务推送通知
	tm.networkEvents.On("task_info_notify", func(data interface{}) {
		// 收到任务状态变化通知，延迟后使用推送中的任务信息检查
		time.Sleep(1 * time.Second)
		
		if taskInfo, ok := data.(*taskpb.TaskInfo); ok && taskInfo != nil {
			// 更新本地任务信息
			tm.mu.Lock()
			tm.taskInfo = taskInfo
			tm.mu.Unlock()
			
			// 使用推送中的任务信息检查
			go tm.checkAndClaimFromTaskInfo(taskInfo)
		} else {
			// 如果推送数据无效，回退到请求 TaskInfo
			go tm.CheckAndClaimTasks()
		}
	})
}

// StopTaskCheckLoop 停止任务检查循环
func (tm *TaskManager) StopTaskCheckLoop() {
	tm.loopRunning = false
	if tm.checkTimer != nil {
		tm.checkTimer.Stop()
	}
	utils.Log("任务系统", "任务检查循环已停止")
}

// IsLoopRunning 检查循环是否正在运行
func (tm *TaskManager) IsLoopRunning() bool {
	return tm.loopRunning
}

// PrintTaskStatus 打印任务状态（用于调试）
func (tm *TaskManager) PrintTaskStatus() {
	tm.mu.RLock()
	taskInfo := tm.taskInfo
	tm.mu.RUnlock()
	
	if taskInfo == nil {
		utils.Log("任务系统", "暂无任务信息")
		return
	}
	
	// 统计成长任务
	growthTotal := len(taskInfo.GrowthTasks)
	growthCompleted := 0
	growthClaimed := 0
	for _, task := range taskInfo.GrowthTasks {
		if task == nil {
			continue
		}
		if task.IsClaimed {
			growthClaimed++
		} else if task.Progress >= task.TotalProgress && task.TotalProgress > 0 {
			growthCompleted++
		}
	}
	
	// 统计每日任务
	dailyTotal := len(taskInfo.DailyTasks)
	dailyCompleted := 0
	dailyClaimed := 0
	for _, task := range taskInfo.DailyTasks {
		if task == nil {
			continue
		}
		if task.IsClaimed {
			dailyClaimed++
		} else if task.Progress >= task.TotalProgress && task.TotalProgress > 0 {
			dailyCompleted++
		}
	}
	
	utils.Log("任务系统", fmt.Sprintf("成长任务: %d/%d 已领取, %d 可领取", 
		growthClaimed, growthTotal, growthCompleted))
	utils.Log("任务系统", fmt.Sprintf("每日任务: %d/%d 已领取, %d 可领取", 
		dailyClaimed, dailyTotal, dailyCompleted))
}
