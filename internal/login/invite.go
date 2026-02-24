package login

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gofarm/internal/config"
	"gofarm/internal/network"
	"gofarm/proto/gamepb/userpb"
	"gofarm/internal/utils"
)

// InviteInfo 邀请信息
type InviteInfo struct {
	UID         string
	OpenID      string
	ShareSource string
	DocID       string
}

// 请求间隔时间（毫秒）
const InviteRequestDelay = 2 * time.Second

// ParseShareLink 解析分享链接，提取 uid 和 openid
// 格式: ?uid=xxx&openid=xxx&share_source=xxx&doc_id=xxx
func ParseShareLink(link string) *InviteInfo {
	result := &InviteInfo{}

	// 移除开头的 ? 如果有
	queryStr := link
	if strings.HasPrefix(link, "?") {
		queryStr = link[1:]
	}

	// 解析参数
	values, err := url.ParseQuery(queryStr)
	if err != nil {
		return result
	}

	result.UID = values.Get("uid")
	result.OpenID = values.Get("openid")
	result.ShareSource = values.Get("share_source")
	result.DocID = values.Get("doc_id")

	return result
}

// ReadShareFile 读取 share.txt 文件并去重
func ReadShareFile() []*InviteInfo {
	// 获取项目根目录（假设 share.txt 在 gofarm 目录下）
	shareFilePath := filepath.Join("share.txt")

	// 检查文件是否存在
	if _, err := os.Stat(shareFilePath); os.IsNotExist(err) {
		return nil
	}

	file, err := os.Open(shareFilePath)
	if err != nil {
		utils.LogWarn("邀请", fmt.Sprintf("打开 share.txt 失败: %v", err))
		return nil
	}
	defer file.Close()

	var invites []*InviteInfo
	seenUIDs := make(map[string]bool) // 用于去重

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, "openid=") {
			continue
		}

		parsed := ParseShareLink(line)
		if parsed.OpenID != "" && parsed.UID != "" {
			// 按 uid 去重，同一个用户只处理一次
			if !seenUIDs[parsed.UID] {
				seenUIDs[parsed.UID] = true
				invites = append(invites, parsed)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		utils.LogWarn("邀请", fmt.Sprintf("读取 share.txt 失败: %v", err))
	}

	return invites
}

// SendReportArkClick 发送 ReportArkClick 请求
// 模拟已登录状态下点击分享链接，触发服务器向分享者发送好友申请
func SendReportArkClick(sharerID int64, sharerOpenID string, shareSource string) (*userpb.ReportArkClickReply, error) {
	shareCfgID := int64(0)
	if shareSource != "" {
		// 尝试解析 share_source 为数字
		fmt.Sscanf(shareSource, "%d", &shareCfgID)
	}

	req := &userpb.ReportArkClickRequest{
		SharerId:       sharerID,
		SharerOpenId:   sharerOpenID,
		ShareCfgId:     shareCfgID,
		SceneId:        "1256", // 模拟微信场景
	}
	resp := &userpb.ReportArkClickReply{}

	err := network.Net.SendProtoMessage("gamepb.userpb.UserService", "ReportArkClick", req, resp, 10*time.Second)
	return resp, err
}

// ProcessInviteCodes 处理邀请码列表
// 仅在微信环境下执行
func ProcessInviteCodes() {
	// 检查是否为微信环境
	if config.Current.Platform != "wx" {
		utils.Log("邀请", "当前为 QQ 环境，跳过邀请码处理（仅微信支持）")
		return
	}

	invites := ReadShareFile()
	if len(invites) == 0 {
		return
	}

	utils.Log("邀请", fmt.Sprintf("读取到 %d 个邀请码（已去重），开始逐个处理...", len(invites)))

	successCount := 0
	failCount := 0

	for i, invite := range invites {
		// 解析 uid 为 int64
		var uid int64
		fmt.Sscanf(invite.UID, "%d", &uid)

		if uid == 0 {
			utils.LogWarn("邀请", fmt.Sprintf("[%d/%d] 无效的 uid: %s", i+1, len(invites), invite.UID))
			failCount++
			continue
		}

		try := func() error {
			// 发送 ReportArkClick 请求，模拟点击分享链接
			_, err := SendReportArkClick(uid, invite.OpenID, invite.ShareSource)
			return err
		}

		if err := try(); err != nil {
			failCount++
			utils.LogWarn("邀请", fmt.Sprintf("[%d/%d] 向 uid=%s 发送申请失败: %v", i+1, len(invites), invite.UID, err))
		} else {
			successCount++
			utils.Log("邀请", fmt.Sprintf("[%d/%d] 已向 uid=%s 发送好友申请", i+1, len(invites), invite.UID))
		}

		// 每个请求之间延迟，避免请求过快被限流
		if i < len(invites)-1 {
			time.Sleep(InviteRequestDelay)
		}
	}

	utils.Log("邀请", fmt.Sprintf("处理完成: 成功 %d, 失败 %d", successCount, failCount))

	// 处理完成后清空文件
	ClearShareFile()
}

// ClearShareFile 清空已处理的邀请码文件
func ClearShareFile() {
	shareFilePath := filepath.Join("share.txt")
	if err := os.WriteFile(shareFilePath, []byte(""), 0644); err != nil {
		// 静默失败
		return
	}
	utils.Log("邀请", "已清空 share.txt")
}
