package config

import (
	"time"
)

// 平台类型
type Platform string

const (
	PlatformQQ Platform = "qq"
	PlatformWX Platform = "wx"
)

// 生长阶段枚举
type PlantPhase int

const (
	PlantPhaseUnknown      PlantPhase = 0
	PlantPhaseSeed         PlantPhase = 1
	PlantPhaseGermination  PlantPhase = 2
	PlantPhaseSmallLeaves  PlantPhase = 3
	PlantPhaseLargeLeaves  PlantPhase = 4
	PlantPhaseBlooming     PlantPhase = 5
	PlantPhaseMature       PlantPhase = 6
	PlantPhaseDead         PlantPhase = 7
)

var PhaseNames = []string{"未知", "种子", "发芽", "小叶", "大叶", "开花", "成熟", "枯死"}

// 设备信息
type DeviceInfo struct {
	ClientVersion string `json:"client_version"`
	SysSoftware   string `json:"sys_software"`
	Network       string `json:"network"`
	Memory        string `json:"memory"`
	DeviceID      string `json:"device_id"`
}

// 全局配置
type Config struct {
	ServerUrl            string
	ClientVersion        string
	Platform             Platform
	OS                   string
	HeartbeatInterval    time.Duration
	FarmCheckInterval    time.Duration
	FriendCheckInterval  time.Duration
	ForceLowestLevelCrop bool
	HarvestDelay         time.Duration // 延时收获时间
	DeviceInfo           DeviceInfo
}

// 默认配置
var DefaultConfig = Config{
	ServerUrl:            "wss://gate-obt.nqf.qq.com/prod/ws",
	ClientVersion:        "1.6.0.14_20251224",
	Platform:             PlatformQQ,
	OS:                   "iOS",
	HeartbeatInterval:    25 * time.Second,
	FarmCheckInterval:    1 * time.Second,
	FriendCheckInterval:  10 * time.Second,
	ForceLowestLevelCrop: false,
	HarvestDelay:         0, // 默认不延时
	DeviceInfo: DeviceInfo{
		ClientVersion: "1.6.0.14_20251224",
		SysSoftware:   "iOS 26.2.1",
		Network:       "wifi",
		Memory:        "7672",
		DeviceID:      "iPhone X<iPhone18,3>",
	},
}

// 当前配置（可在运行时被修改）
var Current = DefaultConfig

// 运行时提示文案（做了简单编码，避免明文散落）
const RuntimeHintMask = 23

var RuntimeHintData = []int{
	12295, 22759, 26137, 12294, 26427, 39022, 30457, 24343, 28295, 20826,
	36142, 65307, 20018, 31126, 20485, 21313, 12309, 35808, 20185, 20859,
	24343, 20164, 24196, 20826, 36142, 33696, 21441, 12309,
}

// 解码运行时提示
func DecodeRuntimeHint() string {
	runes := make([]rune, len(RuntimeHintData))
	for i, n := range RuntimeHintData {
		runes[i] = rune(n ^ RuntimeHintMask)
	}
	return string(runes)
}
