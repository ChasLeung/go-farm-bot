package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// 等级经验配置
type RoleLevel struct {
	Level int   `json:"level"`
	Exp   int64 `json:"exp"`
}

// 植物果实配置
type Fruit struct {
	ID    int    `json:"id"`
	Count int    `json:"count"`
	Name  string `json:"name"`
}

// 植物配置
type Plant struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	SeedID      int    `json:"seed_id"`
	Fruit       Fruit  `json:"fruit"`
	Exp         int    `json:"exp"`
	GrowPhases  string `json:"grow_phases"`
	UnlockLevel int    `json:"unlock_level"`
}

// 物品配置
type ItemInfo struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// 游戏配置管理器
type ConfigManager struct {
	roleLevelConfig []RoleLevel
	levelExpTable   map[int]int64
	plantConfig     []Plant
	plantMap        map[int]*Plant
	seedToPlant     map[int]*Plant
	fruitToPlant    map[int]*Plant
	itemInfoConfig  []ItemInfo
	itemInfoMap     map[int]*ItemInfo
}

var Config *ConfigManager

func init() {
	Config = &ConfigManager{
		levelExpTable: make(map[int]int64),
		plantMap:      make(map[int]*Plant),
		seedToPlant:   make(map[int]*Plant),
		fruitToPlant:  make(map[int]*Plant),
		itemInfoMap:   make(map[int]*ItemInfo),
	}
	Config.LoadConfigs()
}

// 获取项目根目录
func getProjectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename) // internal/game/
	dir = filepath.Dir(dir)       // internal/
	return filepath.Dir(dir)      // 项目根目录
}

// 加载所有配置
func (cm *ConfigManager) LoadConfigs() {
	root := getProjectRoot()
	configDir := filepath.Join(root, "data", "config")

	// 加载等级经验配置
	roleLevelPath := filepath.Join(configDir, "RoleLevel.json")
	if data, err := os.ReadFile(roleLevelPath); err == nil {
		if err := json.Unmarshal(data, &cm.roleLevelConfig); err == nil {
			for _, item := range cm.roleLevelConfig {
				cm.levelExpTable[item.Level] = item.Exp
			}
			fmt.Printf("[配置] 已加载等级经验表 (%d 级)\n", len(cm.roleLevelConfig))
		}
	}

	// 加载植物配置
	plantPath := filepath.Join(configDir, "Plant.json")
	if data, err := os.ReadFile(plantPath); err == nil {
		if err := json.Unmarshal(data, &cm.plantConfig); err == nil {
			for i := range cm.plantConfig {
				plant := &cm.plantConfig[i]
				cm.plantMap[plant.ID] = plant
				if plant.SeedID > 0 {
					cm.seedToPlant[plant.SeedID] = plant
				}
				if plant.Fruit.ID > 0 {
					cm.fruitToPlant[plant.Fruit.ID] = plant
				}
			}
			fmt.Printf("[配置] 已加载植物配置 (%d 种)\n", len(cm.plantConfig))
		}
	}

	// 加载物品配置
	itemInfoPath := filepath.Join(configDir, "ItemInfo.json")
	if data, err := os.ReadFile(itemInfoPath); err == nil {
		if err := json.Unmarshal(data, &cm.itemInfoConfig); err == nil {
			for i := range cm.itemInfoConfig {
				item := &cm.itemInfoConfig[i]
				cm.itemInfoMap[item.ID] = item
			}
			fmt.Printf("[配置] 已加载物品配置 (%d 条)\n", len(cm.itemInfoConfig))
		}
	}
}

// 获取等级经验表
func (cm *ConfigManager) GetLevelExpTable() map[int]int64 {
	return cm.levelExpTable
}

// 计算当前等级的经验进度
func (cm *ConfigManager) GetLevelExpProgress(level int, totalExp int64) (current, needed int64) {
	if level <= 0 {
		return 0, 0
	}
	currentLevelStart := cm.levelExpTable[level]
	nextLevelStart := cm.levelExpTable[level+1]
	if nextLevelStart == 0 {
		nextLevelStart = currentLevelStart + 100000
	}
	current = totalExp - currentLevelStart
	if current < 0 {
		current = 0
	}
	needed = nextLevelStart - currentLevelStart
	return
}

// 根据植物ID获取植物信息
func (cm *ConfigManager) GetPlantByID(plantID int) *Plant {
	return cm.plantMap[plantID]
}

// 根据种子ID获取植物信息
func (cm *ConfigManager) GetPlantBySeedID(seedID int) *Plant {
	return cm.seedToPlant[seedID]
}

// 获取植物名称
func (cm *ConfigManager) GetPlantName(plantID int) string {
	if plant := cm.plantMap[plantID]; plant != nil {
		return plant.Name
	}
	return fmt.Sprintf("植物%d", plantID)
}

// 根据种子ID获取植物名称
func (cm *ConfigManager) GetPlantNameBySeedID(seedID int) string {
	if plant := cm.seedToPlant[seedID]; plant != nil {
		return plant.Name
	}
	return fmt.Sprintf("种子%d", seedID)
}

// 获取植物的收获经验
func (cm *ConfigManager) GetPlantExp(plantID int) int {
	if plant := cm.plantMap[plantID]; plant != nil {
		return plant.Exp
	}
	return 0
}

// 获取植物的生长时间（秒）
func (cm *ConfigManager) GetPlantGrowTime(plantID int) int {
	plant := cm.plantMap[plantID]
	if plant == nil || plant.GrowPhases == "" {
		return 0
	}

	// 解析 "种子:30;发芽:30;成熟:0;" 格式
	phases := strings.Split(plant.GrowPhases, ";")
	totalSeconds := 0
	for _, phase := range phases {
		if phase == "" {
			continue
		}
		parts := strings.Split(phase, ":")
		if len(parts) == 2 {
			if sec, err := strconv.Atoi(parts[1]); err == nil {
				totalSeconds += sec
			}
		}
	}
	return totalSeconds
}

// 格式化时间
func FormatGrowTime(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%d秒", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%d分钟", seconds/60)
	}
	hours := seconds / 3600
	mins := (seconds % 3600) / 60
	if mins > 0 {
		return fmt.Sprintf("%d小时%d分", hours, mins)
	}
	return fmt.Sprintf("%d小时", hours)
}

// 根据果实ID获取植物名称
func (cm *ConfigManager) GetFruitName(fruitID int) string {
	if plant := cm.fruitToPlant[fruitID]; plant != nil {
		return plant.Name
	}
	return fmt.Sprintf("果实%d", fruitID)
}

// 根据果实ID获取植物信息
func (cm *ConfigManager) GetPlantByFruitID(fruitID int) *Plant {
	return cm.fruitToPlant[fruitID]
}

// 根据物品ID获取物品配置
func (cm *ConfigManager) GetItemInfoByID(itemID int) *ItemInfo {
	return cm.itemInfoMap[itemID]
}

// 根据物品ID获取名称
func (cm *ConfigManager) GetItemName(itemID int) string {
	if item := cm.itemInfoMap[itemID]; item != nil && item.Name != "" {
		return item.Name
	}
	if plant := cm.seedToPlant[itemID]; plant != nil {
		return plant.Name + "种子"
	}
	if plant := cm.fruitToPlant[itemID]; plant != nil {
		return plant.Name + "果实"
	}
	return fmt.Sprintf("未知物品%d", itemID)
}
