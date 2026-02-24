# QQ经典农场挂机脚本 (Go版本)

一个基于 Go 语言编写的 QQ经典农场/微信农场自动挂机脚本，支持自动种植、收获、偷菜、完成任务等功能。

## 功能特性

- 自动收获成熟作物 → 购买种子 → 种植 → 施肥
- 自动除草、除虫、浇水
- 自动铲除枯死作物
- 自动巡查好友农场: 帮忙浇水/除草/除虫 + 偷菜
- 自动领取任务奖励 (支持分享翻倍)
- 每分钟自动出售仓库果实
- 支持 QQ扫码登录 和 微信登录
- 心跳保活机制
- 经验效率分析: 计算最优种植策略并导出 JSON/CSV

## 环境要求

- Go 1.23 或更高版本
- QQ 或 微信账号

## 安装

### 从源码编译

```bash
# 克隆仓库
git clone https://github.com/ChasLeung/go-farm-bot.git
cd gofarm

# 编译
go build -o gofarm.exe ./cmd/gofarm
```

### 依赖

- github.com/gorilla/websocket v1.5.1
- google.golang.org/protobuf v1.36.11

## 使用方法

### 1. QQ扫码登录（推荐）

```bash
gofarm --qr
```

启动后会显示二维码，使用 QQ 扫码即可登录。

### 2. 使用登录 Code

```bash
# QQ平台
gofarm --code <你的登录code>

# 微信平台
gofarm --code <你的登录code> --wx
```

### 3. 常用参数

```bash
gofarm --code <code> [选项]

选项:
  --code              小程序 login() 返回的临时凭证
  --qr                使用QQ扫码登录
  --wx                使用微信登录 (默认为QQ小程序)
  --interval          自己农场巡查间隔(秒), 默认10秒
  --friend-interval   好友巡查间隔(秒), 默认1秒
  --harvest-delay     成熟后延时收获秒数, 默认0秒(立即收获)
```

### 4. 经验效率分析

```bash
# 分析等级30、18块地的最优种植策略
gofarm --exp-analysis --exp-level 30 --exp-lands 18

# 指定输出目录
gofarm --exp-analysis --exp-level 50 --exp-lands 24 --exp-out ./output
```

### 5. 数据解码工具

```bash
# 解码PB数据
gofarm --decode <数据> [--hex] [--gate] [--type <消息类型>]

# 查看详细帮助
gofarm --decode
```

## 邀请码

在项目根目录创建 `share.txt` 文件，每行一个邀请链接：

```
?uid=xxx&openid=xxx&share_source=xxx&doc_id=xxx
?uid=yyy&openid=yyy&share_source=yyy&doc_id=yyy
```

启动时会自动尝试通过 SyncAll API 同步这些好友（仅微信）。

## 项目结构

```
gofarm/
├── cmd/gofarm/          # 主程序入口
├── internal/
│   ├── config/          # 配置管理
│   ├── game/            # 游戏逻辑（农场、好友、任务、仓库）
│   ├── logger/          # 日志系统
│   ├── login/           # 登录相关
│   ├── network/         # 网络连接
│   ├── status/          # 状态栏
│   └── utils/           # 工具函数
├── proto/               # Protobuf 定义和生成的代码
├── tools/               # 工具（解码器、经验分析）
├── data/                # 数据文件（配置、种子商店等）
├── go.mod               # Go模块定义
└── go.sum               # 依赖校验
```

## 注意事项

1. **风险提示**: 使用本脚本可能导致账号被封禁，请自行承担风险
2. **频率控制**: 建议合理设置巡查间隔，避免过于频繁的请求
3. **登录凭证**: 登录 code 具有时效性，过期后需要重新获取
4. **微信限制**: 部分功能仅支持 QQ 平台

## 免责声明

本项目仅供学习交流使用，请勿用于商业用途。使用本项目造成的任何后果由使用者自行承担。

## 开源协议

本项目采用 [MIT](LICENSE) 协议开源。

## 贡献

欢迎提交 Issue 和 Pull Request！

## 更新日志

详见 [CHANGELOG.md](CHANGELOG.md)
