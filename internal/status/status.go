package status

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gofarm/internal/game"
)

// 状态数据
type StatusData struct {
	Platform string
	Name     string
	Level    int
	Gold     int64
	Exp      int64
	mu       sync.RWMutex
}

var (
	statusData    StatusData
	statusEnabled bool
	termRows      = 24
	mu            sync.Mutex
)

const (
	statusLines    = 3
	freeProjectTip = "本程序在GitHub免费开源。"

	// ANSI 转义码
	esc           = "\x1b"
	saveCursor    = esc + "7"
	restoreCursor = esc + "8"
	clearLine     = esc + "[2K"
	resetScroll   = esc + "[r"
	bold          = esc + "[1m"
	reset         = esc + "[0m"
	dim           = esc + "[2m"
	cyan          = esc + "[36m"
	yellow        = esc + "[33m"
	green         = esc + "[32m"
	magenta       = esc + "[35m"
)

func moveTo(row, col int) string {
	return fmt.Sprintf("%s[%d;%dH", esc, row, col)
}

func scrollRegion(top, bottom int) string {
	return fmt.Sprintf("%s[%d;%dr", esc, top, bottom)
}

// InitStatusBar 初始化状态栏
func InitStatusBar() bool {
	mu.Lock()
	defer mu.Unlock()

	// 检测终端是否支持
	if os.Getenv("TERM") == "" && os.Getenv("ConEmuANSI") == "" && os.Getenv("WT_SESSION") == "" {
		// 可能不支持ANSI转义码
		// 但在Windows 10+中通常支持
	}

	statusEnabled = true

	// 初始渲染
	renderStatusBar()
	return true
}

// CleanupStatusBar 清理状态栏
func CleanupStatusBar() {
	mu.Lock()
	defer mu.Unlock()

	if !statusEnabled {
		return
	}
	statusEnabled = false

	// 重置滚动区域
	fmt.Print(resetScroll)
	// 清除状态栏
	fmt.Print(moveTo(1, 1) + clearLine)
	fmt.Print(moveTo(2, 1) + clearLine)
	fmt.Print(moveTo(3, 1) + clearLine)
}

// renderStatusBar 渲染状态栏
func renderStatusBar() {
	if !statusEnabled {
		return
	}

	statusData.mu.RLock()
	platform := statusData.Platform
	name := statusData.Name
	level := statusData.Level
	gold := statusData.Gold
	exp := statusData.Exp
	statusData.mu.RUnlock()

	// 构建状态行
	platformStr := cyan + "QQ" + reset
	if platform == "wx" {
		platformStr = magenta + "微信" + reset
	}

	nameStr := name
	if nameStr == "" {
		nameStr = "未登录"
	} else {
		nameStr = bold + nameStr + reset
	}

	levelStr := fmt.Sprintf("%sLv%d%s", green, level, reset)
	goldStr := fmt.Sprintf("%s金币:%d%s", yellow, gold, reset)

	// 显示经验值
	var expStr string
	if level > 0 && exp >= 0 {
		levelExpTable := game.Config.GetLevelExpTable()
		if len(levelExpTable) > 0 {
			current, needed := game.Config.GetLevelExpProgress(level, exp)
			expStr = fmt.Sprintf("%s经验:%d/%d%s", dim, current, needed, reset)
		} else {
			expStr = fmt.Sprintf("%s经验:%d%s", dim, exp, reset)
		}
	}

	// 第一行：平台 | 昵称 | 等级 | 金币 | 经验
	line1 := fmt.Sprintf("%s | %s | %s | %s", platformStr, nameStr, levelStr, goldStr)
	if expStr != "" {
		line1 += " | " + expStr
	}

	// 第二行：固定提醒
	line2 := dim + freeProjectTip + reset

	// 第三行：分隔线
	width := 80
	line3 := dim + strings.Repeat("─", width) + reset

	// 保存光标位置并渲染
	fmt.Print(saveCursor)
	fmt.Print(moveTo(1, 1) + clearLine + line1)
	fmt.Print(moveTo(2, 1) + clearLine + line2)
	fmt.Print(moveTo(3, 1) + clearLine + line3)
	fmt.Print(restoreCursor)
}

// updateStatus 更新状态数据并刷新显示
func updateStatus(data map[string]interface{}) {
	changed := false

	statusData.mu.Lock()
	if platform, ok := data["platform"].(string); ok && statusData.Platform != platform {
		statusData.Platform = platform
		changed = true
	}
	if name, ok := data["name"].(string); ok && statusData.Name != name {
		statusData.Name = name
		changed = true
	}
	if level, ok := data["level"].(int); ok && statusData.Level != level {
		statusData.Level = level
		changed = true
	}
	if gold, ok := data["gold"].(int64); ok && statusData.Gold != gold {
		statusData.Gold = gold
		changed = true
	}
	if exp, ok := data["exp"].(int64); ok && statusData.Exp != exp {
		statusData.Exp = exp
		changed = true
	}
	statusData.mu.Unlock()

	if changed && statusEnabled {
		renderStatusBar()
	}
}

// SetStatusPlatform 设置平台
func SetStatusPlatform(platform string) {
	updateStatus(map[string]interface{}{"platform": platform})
}

// UpdateStatusFromLogin 从登录数据更新状态
func UpdateStatusFromLogin(name string, level int, gold, exp int64) {
	updateStatus(map[string]interface{}{
		"name":  name,
		"level": level,
		"gold":  gold,
		"exp":   exp,
	})
}

// UpdateStatusGold 更新金币
func UpdateStatusGold(gold int64) {
	updateStatus(map[string]interface{}{"gold": gold})
}

// UpdateStatusLevel 更新等级和经验
func UpdateStatusLevel(level int, exp int64) {
	updateStatus(map[string]interface{}{
		"level": level,
		"exp":   exp,
	})
}

// GetStatusData 获取状态数据
func GetStatusData() (string, int, int64, int64) {
	statusData.mu.RLock()
	defer statusData.mu.RUnlock()
	return statusData.Name, statusData.Level, statusData.Gold, statusData.Exp
}
