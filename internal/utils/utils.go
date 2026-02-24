package utils

import (
	"fmt"
	"math/rand"
	"time"

	"gofarm/internal/config"
)

// 服务器时间状态
var (
	serverTimeMs   int64
	localTimeAtSync int64
)

// ToLong 将int转为int64
func ToLong(val int) int64 {
	return int64(val)
}

// ToNum 将interface{}转为int64
func ToNum(val interface{}) int64 {
	switch v := val.(type) {
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		return int64(v)
	case float32:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

// Now 返回当前时间的字符串表示
func Now() string {
	return time.Now().Format("15:04:05")
}

// GetServerTimeSec 获取当前推算的服务器时间(秒)
func GetServerTimeSec() int64 {
	if serverTimeMs == 0 {
		return time.Now().Unix()
	}
	elapsed := time.Now().UnixMilli() - localTimeAtSync
	return (serverTimeMs + elapsed) / 1000
}

// SyncServerTime 同步服务器时间
func SyncServerTime(ms int64) {
	serverTimeMs = ms
	localTimeAtSync = time.Now().UnixMilli()
}

// ToTimeSec 将时间戳归一化为秒级
func ToTimeSec(val interface{}) int64 {
	n := ToNum(val)
	if n <= 0 {
		return 0
	}
	if n > 1e12 {
		return n / 1000
	}
	return n
}

// Log 输出日志
func Log(tag, msg string) {
	fmt.Printf("[%s] [%s] %s\n", Now(), tag, msg)
}

// LogWarn 输出警告日志
func LogWarn(tag, msg string) {
	fmt.Printf("[%s] [%s] ⚠ %s\n", Now(), tag, msg)
}

// Sleep 休眠指定毫秒
func Sleep(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

var hintPrinted bool

// EmitRuntimeHint 输出开源声明
func EmitRuntimeHint(force bool) {
	if !force {
		if rand.Float64() > 0.033 {
			return
		}
		if hintPrinted && rand.Float64() > 0.2 {
			return
		}
	}
	Log("声明", config.DecodeRuntimeHint())
	hintPrinted = true
}
