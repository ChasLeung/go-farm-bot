package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"gofarm/internal/config"
	"gofarm/internal/game"
	"gofarm/internal/logger"
	"gofarm/internal/login"
	"gofarm/internal/network"
	"gofarm/internal/status"
	"gofarm/tools"
	"gofarm/internal/utils"
)

// 显示帮助信息
func showHelp() {
	fmt.Println(`
QQ经典农场 挂机脚本 (Go版本)
====================

用法:
  gofarm --code <登录code> [--wx] [--interval <秒>] [--friend-interval <秒>] [--harvest-delay <秒>]
  gofarm --qr [--interval <秒>] [--friend-interval <秒>] [--harvest-delay <秒>]
  gofarm --verify
  gofarm --decode <数据> [--hex] [--gate] [--type <消息类型>]
  gofarm --exp-analysis [--exp-level <等级>] [--exp-lands <地块数>] [--exp-out <目录>]

参数:
  --code              小程序 login() 返回的临时凭证 (必需)
  --qr                启动后使用QQ扫码获取登录code（仅QQ平台）
  --wx                使用微信登录 (默认为QQ小程序)
  --interval          自己农场巡查完成后等待秒数, 默认10秒, 最低10秒
  --friend-interval   好友巡查完成后等待秒数, 默认1秒, 最低1秒
  --harvest-delay     成熟后延时收获秒数, 默认0秒(立即收获)
  --verify            验证proto定义
  --decode            解码PB数据 (运行 --decode 无参数查看详细帮助)
  --exp-analysis      运行经验效率分析
  --exp-level         经验分析等级, 默认1
  --exp-lands         经验分析地块数, 默认18
  --exp-out           经验分析输出目录, 默认当前目录

功能:
  - 自动收获成熟作物 → 购买种子 → 种植 → 施肥
  - 自动除草、除虫、浇水
  - 自动铲除枯死作物
  - 自动巡查好友农场: 帮忙浇水/除草/除虫 + 偷菜
  - 自动领取任务奖励 (支持分享翻倍)
  - 每分钟自动出售仓库果实
  - 启动时读取 share.txt 处理邀请码 (仅微信)
  - 心跳保活
  - 经验效率分析: 计算最优种植策略并导出JSON/CSV

邀请码文件 (share.txt):
  每行一个邀请链接，格式: ?uid=xxx&openid=xxx&share_source=xxx&doc_id=xxx
  启动时会尝试通过 SyncAll API 同步这些好友

示例:
  gofarm --exp-analysis --exp-level 30 --exp-lands 18
  gofarm --exp-analysis --exp-level 50 --exp-lands 24 --exp-out ./output
  gofarm --code xxx --harvest-delay 300  # 成熟后延时5分钟收获
`)
}

// 命令行参数
type Options struct {
	Code              string
	QrLogin           bool
	WxPlatform        bool
	Interval          int
	FriendInterval    int
	HarvestDelay      int
	Verify            bool
	Decode            bool
	DecodeData        string
	DecodeHex         bool
	DecodeGate        bool
	DecodeType        string
	ExpAnalysis       bool
	ExpLevel          int
	ExpLands          int
	ExpOutDir         string
}

func parseArgs() Options {
	var opts Options

	flag.StringVar(&opts.Code, "code", "", "登录凭证")
	flag.BoolVar(&opts.QrLogin, "qr", false, "使用QQ扫码登录")
	flag.BoolVar(&opts.WxPlatform, "wx", false, "使用微信平台")
	flag.IntVar(&opts.Interval, "interval", 10, "农场巡查间隔(秒)")
	flag.IntVar(&opts.FriendInterval, "friend-interval", 10, "好友巡查间隔(秒)")
	flag.IntVar(&opts.HarvestDelay, "harvest-delay", 0, "成熟后延时收获秒数")
	flag.BoolVar(&opts.Verify, "verify", false, "验证proto定义")
	flag.BoolVar(&opts.Decode, "decode", false, "解码PB数据")
	flag.BoolVar(&opts.DecodeHex, "hex", false, "数据为hex编码")
	flag.BoolVar(&opts.DecodeGate, "gate", false, "外层是gatepb.Message")
	flag.StringVar(&opts.DecodeType, "type", "", "指定消息类型")
	flag.BoolVar(&opts.ExpAnalysis, "exp-analysis", false, "运行经验效率分析")
	flag.IntVar(&opts.ExpLevel, "exp-level", 0, "经验分析等级(默认当前等级)")
	flag.IntVar(&opts.ExpLands, "exp-lands", 18, "经验分析地块数")
	flag.StringVar(&opts.ExpOutDir, "exp-out", ".", "经验分析输出目录")

	flag.Parse()

	// 获取解码数据（非flag参数）
	args := flag.Args()
	if len(args) > 0 && opts.Decode {
		opts.DecodeData = args[0]
	}

	return opts
}

func main() {
	// 初始化日志
	logger.InitFileLogger()

	// 解析命令行参数
	opts := parseArgs()

	// 验证模式
	if opts.Verify {
		tools.VerifyMode()
		return
	}

	// 解码模式
	if opts.Decode {
		if opts.DecodeData == "" {
			tools.PrintDecodeHelp()
			return
		}

		opts := tools.DecodeOptions{
			Data:          opts.DecodeData,
			IsHex:         opts.DecodeHex,
			IsGateWrapped: opts.DecodeGate,
			TypeName:      opts.DecodeType,
		}
		result := tools.DecodePB(opts)
		if !result.Success {
			fmt.Printf("解码失败: %s\n", result.Error)
		}
		return
	}

	// 经验效率分析模式
	if opts.ExpAnalysis {
		level := opts.ExpLevel
		if level <= 0 {
			// 如果没有指定等级，使用默认等级1
			level = 1
		}
		if err := tools.RunExpAnalysis(level, opts.ExpLands, opts.ExpOutDir); err != nil {
			fmt.Printf("经验分析失败: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 设置平台
	if opts.WxPlatform {
		config.Current.Platform = config.PlatformWX
	}

	// 设置间隔
	if opts.Interval >= 1 {
		config.Current.FarmCheckInterval = time.Duration(opts.Interval) * time.Second
	}
	if opts.FriendInterval >= 1 {
		config.Current.FriendCheckInterval = time.Duration(opts.FriendInterval) * time.Second
	}
	if opts.HarvestDelay >= 0 {
		config.Current.HarvestDelay = time.Duration(opts.HarvestDelay) * time.Second
	}

	// 处理登录code
	usedQrLogin := false
	code := opts.Code

	// QQ平台支持扫码登录: 显式 --qr，或未传 --code 时自动触发
	if code == "" && config.Current.Platform == config.PlatformQQ && (opts.QrLogin || !opts.WxPlatform) {
		fmt.Println("[扫码登录] 正在获取二维码...")
		var err error
		code, err = login.GetQQFarmCodeByScan()
		if err != nil {
			fmt.Printf("[扫码登录] 失败: %v\n", err)
			os.Exit(1)
		}
		usedQrLogin = true
		fmt.Printf("[扫码登录] 获取成功，code=%s...\n", code[:min(8, len(code))])
	}

	if code == "" {
		if config.Current.Platform == config.PlatformWX {
			fmt.Println("[参数] 微信模式仍需通过 --code 传入登录凭证")
		}
		showHelp()
		os.Exit(1)
	}

	// 扫码阶段结束后清屏
	if usedQrLogin {
		fmt.Print("\x1b[2J\x1b[H")
	}

	// 初始化状态栏
	status.InitStatusBar()
	status.SetStatusPlatform(string(config.Current.Platform))
	utils.EmitRuntimeHint(true)

	platformName := "QQ"
	if config.Current.Platform == config.PlatformWX {
		platformName = "微信"
	}
	fmt.Printf("[启动] %s code=%s... 农场%ds 好友%ds\n",
		platformName,
		code[:min(8, len(code))],
		int(config.Current.FarmCheckInterval.Seconds()),
		int(config.Current.FriendCheckInterval.Seconds()))

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 连接并登录
	err := network.Net.Connect(code, func() {
		fmt.Println("\n========== 登录成功 ==========")
		gid, name, level, gold, exp := network.Net.GetUserState().Get()
		fmt.Printf("  GID:    %d\n", gid)
		fmt.Printf("  昵称:   %s\n", name)
		fmt.Printf("  等级:   %d\n", level)
		fmt.Printf("  金币:   %d\n", gold)
		fmt.Println("===============================")
		fmt.Println()

		// 更新状态栏
		status.UpdateStatusFromLogin(name, level, gold, exp)

		// 启动心跳
		network.Net.StartHeartbeat()

		// 处理邀请码（仅微信环境）
		login.ProcessInviteCodes()

		// 启动农场巡查
		fmt.Println("[系统] 启动农场巡查模块...")
		game.Farm.StartFarmCheckLoop()
		fmt.Println("[系统] 农场巡查已启动")
		fmt.Println()

		// 启动好友巡查
		fmt.Println("[系统] 启动好友巡查模块...")
		game.Friend.StartFriendCheckLoop()
		fmt.Println("[系统] 好友巡查已启动")
		fmt.Println()

		// 启动任务系统 (延迟4秒，避免同时发送大量请求)
		fmt.Println("[系统] 任务系统将在4秒后启动...")
		go func() {
			time.Sleep(4 * time.Second)
			game.Task.StartTaskCheckLoop()
		}()

		// 启动仓库系统 (延迟5秒，避免同时发送大量请求)
		fmt.Println("[系统] 仓库系统将在5秒后启动...")
		go func() {
			time.Sleep(5 * time.Second)
			game.Warehouse.StartSellLoop()
		}()

		fmt.Println("[系统] 所有核心模块启动中...")

		// 监听断开连接事件（被踢下线或连接异常）
		network.Net.GetEvents().On("disconnected", func(data interface{}) {
			fmt.Println("\n[系统] 连接已断开，程序即将退出...")
			// 触发退出信号
			sigChan <- syscall.SIGTERM
		})
	})

	if err != nil {
		fmt.Printf("启动失败: %v\n", err)
		os.Exit(1)
	}

	// 等待退出信号
	<-sigChan

	// 清理
	fmt.Println("\n[退出] 正在停止农场巡查...")
	game.Farm.StopFarmCheckLoop()
	fmt.Println("[退出] 正在停止好友巡查...")
	game.Friend.StopFriendCheckLoop()
	fmt.Println("[退出] 正在停止任务系统...")
	game.Task.StopTaskCheckLoop()
	fmt.Println("[退出] 正在停止仓库系统...")
	game.Warehouse.StopSellLoop()
	status.CleanupStatusBar()
	fmt.Println("[退出] 正在断开...")
	network.Net.Cleanup()
	fmt.Println("[退出] 已断开连接")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 字符串转int64
func parseInt64(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}
