package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gofarm/internal/network"
	"gofarm/internal/utils"
	"gofarm/proto/corepb"
	"gofarm/proto/gamepb/itempb"
)

// 游戏内金币的物品 ID
const GoldItemID = 1001

// WarehouseManager 仓库管理器
type WarehouseManager struct {
	isChecking    bool
	checkTimer    *time.Timer
	loopRunning   bool
	networkEvents *network.EventEmitter
	fruitIDSet    map[int64]bool // 果实ID集合
	mu            sync.RWMutex
}

var Warehouse *WarehouseManager

// 配置: 出售检查间隔 (默认1分钟)
const SellCheckInterval = 60 * time.Second

func init() {
	Warehouse = &WarehouseManager{
		networkEvents: network.Net.GetEvents(),
		fruitIDSet:    make(map[int64]bool),
	}

	// 加载果实ID数据
	Warehouse.loadFruitIDs()
}

// loadFruitIDs 从种子商店数据加载果实ID
func (wm *WarehouseManager) loadFruitIDs() {
	// 尝试加载种子商店数据
	seedShopPath := filepath.Join("data", "seed-shop-merged-export.json")

	data, err := os.ReadFile(seedShopPath)
	if err != nil {
		utils.LogWarn("仓库系统", fmt.Sprintf("加载种子商店数据失败: %v", err))
		return
	}

	var seedShopData struct {
		Rows []struct {
			FruitID int64 `json:"fruitId"`
		} `json:"rows"`
	}

	if err := json.Unmarshal(data, &seedShopData); err != nil {
		utils.LogWarn("仓库系统", fmt.Sprintf("解析种子商店数据失败: %v", err))
		return
	}

	wm.mu.Lock()
	validCount := 0
	for _, row := range seedShopData.Rows {
		if row.FruitID > 0 {
			wm.fruitIDSet[row.FruitID] = true
			validCount++
		}
	}
	wm.mu.Unlock()

	fmt.Printf("[配置] 已加载果实配置 (%d 种)\n", validCount)
}

// isFruitID 检查物品ID是否为果实
func (wm *WarehouseManager) isFruitID(id int64) bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.fruitIDSet[id]
}

// GetBag 获取背包信息
func (wm *WarehouseManager) GetBag() (*itempb.BagReply, error) {
	req := &itempb.BagRequest{}
	resp := &itempb.BagReply{}

	err := network.Net.SendProtoMessage("gamepb.itempb.ItemService", "Bag", req, resp, 10*time.Second)
	return resp, err
}

// SellItems 出售物品
func (wm *WarehouseManager) SellItems(items []*corepb.Item) (*itempb.SellReply, error) {
	req := &itempb.SellRequest{
		Items: items,
	}
	resp := &itempb.SellReply{}

	err := network.Net.SendProtoMessage("gamepb.itempb.ItemService", "Sell", req, resp, 10*time.Second)
	return resp, err
}

// extractGold 从出售回复中提取获得的金币数量
func (wm *WarehouseManager) extractGold(reply *itempb.SellReply) int64 {
	if reply.GetItems != nil {
		for _, item := range reply.GetItems {
			if item != nil && item.Id == GoldItemID {
				return item.Count
			}
		}
	}
	return 0
}

// getBagItems 从背包回复中获取物品列表
func (wm *WarehouseManager) getBagItems(reply *itempb.BagReply) []*corepb.Item {
	if reply == nil {
		return nil
	}

	if reply.ItemBag != nil && len(reply.ItemBag.Items) > 0 {
		return reply.ItemBag.Items
	}

	return nil
}

// FruitItem 果实物品信息
type FruitItem struct {
	ID    int64
	Count int64
	UID   int64
	Name  string
}

// FruitInfo 果实信息（包含原始物品引用）
type FruitInfo struct {
	Item  *corepb.Item // 原始物品对象
	ID    int64
	Count int64
	UID   int64
	Name  string
}

// AnalyzeFruits 分析背包中的果实
func (wm *WarehouseManager) AnalyzeFruits(items []*corepb.Item) []*FruitInfo {
	var fruits []*FruitInfo

	for _, item := range items {
		if item == nil {
			continue
		}

		id := item.Id
		count := item.Count
		uid := item.Uid
		name := Config.GetItemName(int(id))

		// 调试日志
		isFruit := wm.isFruitID(id)
		utils.Log("仓库系统", fmt.Sprintf("  %s x%d (ID=%d, UID=%d, 是果实=%v)", name, count, id, uid, isFruit))

		// 检查是否为果实 (注意：有些服务器返回的 uid 为 0，但仍可正常出售)
		if isFruit && count > 0 {
			fruits = append(fruits, &FruitInfo{
				Item:  item, // 保留原始物品对象
				ID:    id,
				Count: count,
				UID:   uid,
				Name:  Config.GetFruitName(int(id)),
			})
		}
	}

	return fruits
}

// SellAllFruits 出售所有果实
func (wm *WarehouseManager) SellAllFruits() {
	if wm.isChecking {
		return
	}
	wm.isChecking = true
	defer func() { wm.isChecking = false }()

	// 获取背包
	bagReply, err := wm.GetBag()
	if err != nil {
		utils.LogWarn("仓库系统", fmt.Sprintf("获取背包失败: %v", err))
		return
	}

	items := wm.getBagItems(bagReply)
	utils.Log("仓库系统", fmt.Sprintf("背包共有 %d 个物品", len(items)))

	if len(items) == 0 {
		return
	}

	// 分析果实
	fruits := wm.AnalyzeFruits(items)
	utils.Log("仓库系统", fmt.Sprintf("分析到 %d 个果实", len(fruits)))

	if len(fruits) == 0 {
		return
	}

	// 准备出售的物品（使用原始物品对象，保留所有字段）
	var toSell []*corepb.Item
	var fruitNames []string

	for _, fruit := range fruits {
		// 直接使用从背包获取的原始物品对象
		toSell = append(toSell, fruit.Item)
		fruitNames = append(fruitNames, fmt.Sprintf("%s x%d", fruit.Name, fruit.Count))
	}

	utils.Log("仓库系统", fmt.Sprintf("准备出售 %d 个物品: %v", len(toSell), fruitNames))

	// 出售
	reply, err := wm.SellItems(toSell)
	if err != nil {
		utils.LogWarn("仓库系统", fmt.Sprintf("出售失败: %v", err))
		return
	}

	// 提取获得的金币
	gold := wm.extractGold(reply)

	utils.Log("仓库系统", fmt.Sprintf("出售 %s，获得 %d 金币", fruitNames, gold))

	// 触发运行时提示更新
	utils.EmitRuntimeHint(false)
}

// GetWarehouseStats 获取仓库统计
func (wm *WarehouseManager) GetWarehouseStats() map[string]interface{} {
	bagReply, err := wm.GetBag()
	if err != nil {
		return map[string]interface{}{
			"total_items": 0,
			"fruit_count": 0,
			"fruit_types": 0,
		}
	}

	items := wm.getBagItems(bagReply)
	fruits := wm.AnalyzeFruits(items)

	var fruitCount int64
	for _, fruit := range fruits {
		fruitCount += fruit.Count
	}

	return map[string]interface{}{
		"total_items": len(items),
		"fruit_count": fruitCount,
		"fruit_types": len(fruits),
	}
}

// PrintBagStatus 打印背包状态（用于调试）
func (wm *WarehouseManager) PrintBagStatus() {
	bagReply, err := wm.GetBag()
	if err != nil {
		utils.LogWarn("仓库系统", fmt.Sprintf("获取背包失败: %v", err))
		return
	}

	items := wm.getBagItems(bagReply)
	if len(items) == 0 {
		utils.Log("仓库系统", "背包为空")
		return
	}

	utils.Log("仓库系统", fmt.Sprintf("背包共 %d 种物品:", len(items)))

	for _, item := range items {
		if item == nil {
			continue
		}

		id := item.Id
		count := item.Count
		isFruit := wm.isFruitID(id)

		if isFruit {
			name := Config.GetFruitName(int(id))
			utils.Log("仓库系统", fmt.Sprintf("  [果实] %s (ID:%d) x%d", name, id, count))
		} else {
			name := Config.GetItemName(int(id))
			utils.Log("仓库系统", fmt.Sprintf("  [物品] %s (ID:%d) x%d", name, id, count))
		}
	}

	// 统计果实
	fruits := wm.AnalyzeFruits(items)
	if len(fruits) > 0 {
		var totalCount int64
		for _, fruit := range fruits {
			totalCount += fruit.Count
		}
		utils.Log("仓库系统", fmt.Sprintf("共有 %d 种果实，总计 %d 个", len(fruits), totalCount))
	}
}

// StartSellLoop 启动自动出售循环
func (wm *WarehouseManager) StartSellLoop() {
	if wm.loopRunning {
		return
	}

	wm.loopRunning = true
	utils.Log("仓库系统", "自动出售循环已启动")

	// 立即执行一次
	go wm.SellAllFruits()

	// 定时器循环
	go func() {
		for wm.loopRunning {
			// 等待间隔时间
			time.Sleep(SellCheckInterval)

			if !wm.loopRunning {
				break
			}

			// 出售果实
			wm.SellAllFruits()
		}
	}()
}

// StopSellLoop 停止自动出售循环
func (wm *WarehouseManager) StopSellLoop() {
	wm.loopRunning = false
	if wm.checkTimer != nil {
		wm.checkTimer.Stop()
	}
	utils.Log("仓库系统", "自动出售循环已停止")
}

// IsLoopRunning 检查循环是否正在运行
func (wm *WarehouseManager) IsLoopRunning() bool {
	return wm.loopRunning
}

// ForceSellNow 立即强制出售（用于手动触发）
func (wm *WarehouseManager) ForceSellNow() {
	go wm.SellAllFruits()
}
