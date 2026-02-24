package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const logDir = "logs"

var (
	initialized    bool
	currentDateKey string
	file           *os.File
	disabled       bool
	mu             sync.Mutex
)

func pad2(n int) string {
	if n < 10 {
		return fmt.Sprintf("0%d", n)
	}
	return fmt.Sprintf("%d", n)
}

func getDateKey(d time.Time) string {
	return fmt.Sprintf("%d-%s-%s", d.Year(), pad2(int(d.Month())), pad2(d.Day()))
}

func getDateTime(d time.Time) string {
	return fmt.Sprintf("%s %s:%s:%s", getDateKey(d), pad2(d.Hour()), pad2(d.Minute()), pad2(d.Second()))
}

func ensureStream() {
	if disabled {
		return
	}

	now := time.Now()
	dateKey := getDateKey(now)

	mu.Lock()
	defer mu.Unlock()

	if file != nil && dateKey == currentDateKey {
		return
	}

	if file != nil {
		file.Close()
		file = nil
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		disabled = true
		fmt.Fprintf(os.Stderr, "[logger] 初始化日志文件失败: %v\n", err)
		return
	}

	logFile := filepath.Join(logDir, fmt.Sprintf("%s.log", dateKey))
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		disabled = true
		fmt.Fprintf(os.Stderr, "[logger] 初始化日志文件失败: %v\n", err)
		return
	}

	file = f
	currentDateKey = dateKey
}

func appendLine(level string, msg string) {
	ensureStream()
	if file == nil || disabled {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	line := fmt.Sprintf("[%s] [%s] %s\n", getDateTime(now), level, msg)
	file.WriteString(line)
}

// InitFileLogger 初始化文件日志
func InitFileLogger() {
	mu.Lock()
	if initialized {
		mu.Unlock()
		return
	}
	initialized = true
	mu.Unlock()

	// 保存原始的输出函数
	rawLog := fmt.Println
	rawWarn := fmt.Println
	rawError := fmt.Println

	// 重定向标准输出
	// 注意：Go中无法像Node.js那样直接重定向console.log
	// 这里使用一个简化的方式
	_ = rawLog
	_ = rawWarn
	_ = rawError
}

// Log 记录信息日志
func Log(tag, msg string) {
	line := fmt.Sprintf("[%s] [%s] %s", time.Now().Format("15:04:05"), tag, msg)
	fmt.Println(line)
	appendLine("INFO", line)
}

// LogWarn 记录警告日志
func LogWarn(tag, msg string) {
	line := fmt.Sprintf("[%s] [%s] ⚠ %s", time.Now().Format("15:04:05"), tag, msg)
	fmt.Println(line)
	appendLine("WARN", line)
}

// LogError 记录错误日志
func LogError(tag, msg string) {
	line := fmt.Sprintf("[%s] [%s] ✗ %s", time.Now().Format("15:04:05"), tag, msg)
	fmt.Println(line)
	appendLine("ERROR", line)
}

func init() {
	// 程序退出时关闭日志文件
	go func() {
		for {
			time.Sleep(1 * time.Second)
			mu.Lock()
			if file != nil {
				file.Sync()
			}
			mu.Unlock()
		}
	}()
}
