package network

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"gofarm/internal/config"
	"gofarm/proto/gatepb"
	"gofarm/proto/gamepb/userpb"
	"gofarm/internal/utils"
)

// 用户状态
type UserState struct {
	GID   int64
	Name  string
	Level int
	Gold  int64
	Exp   int64
	mu    sync.RWMutex
}

func (u *UserState) Set(gid int64, name string, level int, gold, exp int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.GID = gid
	u.Name = name
	u.Level = level
	u.Gold = gold
	u.Exp = exp
}

func (u *UserState) Get() (int64, string, int, int64, int64) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.GID, u.Name, u.Level, u.Gold, u.Exp
}

func (u *UserState) UpdateGold(gold int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Gold = gold
}

func (u *UserState) UpdateExp(exp int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Exp = exp
}

func (u *UserState) UpdateLevel(level int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Level = level
}

// 网络管理器
type NetworkManager struct {
	ws               *websocket.Conn
	clientSeq        int64
	serverSeq        int64
	heartbeatTimer   *time.Timer
	pendingCallbacks map[int64]chan *Response
	userState        UserState
	onLoginSuccess   func()
	events           *EventEmitter
	mu               sync.RWMutex
	writeMu          sync.Mutex // 专门用于保护 WebSocket 写入
	connected        bool
}

// 事件发射器
type EventEmitter struct {
	handlers map[string][]func(interface{})
	mu       sync.RWMutex
}

func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		handlers: make(map[string][]func(interface{})),
	}
}

func (e *EventEmitter) On(event string, handler func(interface{})) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[event] = append(e.handlers[event], handler)
}

func (e *EventEmitter) Off(event string, handler func(interface{})) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if handlers, ok := e.handlers[event]; ok {
		for i, h := range handlers {
			if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
				e.handlers[event] = append(handlers[:i], handlers[i+1:]...)
				break
			}
		}
	}
}

func (e *EventEmitter) Emit(event string, data interface{}) {
	e.mu.RLock()
	handlers := e.handlers[event]
	e.mu.RUnlock()
	for _, handler := range handlers {
		go handler(data)
	}
}

// Response 响应结构
type Response struct {
	Body []byte
	Meta *gatepb.Meta
	Err  error
}

// 全局网络实例
var Net *NetworkManager

func init() {
	Net = &NetworkManager{
		pendingCallbacks: make(map[int64]chan *Response),
		events:           NewEventEmitter(),
	}
}

// GetUserState 获取用户状态
func (nm *NetworkManager) GetUserState() *UserState {
	return &nm.userState
}

// GetEvents 获取事件发射器
func (nm *NetworkManager) GetEvents() *EventEmitter {
	return nm.events
}

// EncodeMessage 编码消息
func (nm *NetworkManager) EncodeMessage(serviceName, methodName string, body proto.Message) ([]byte, int64, error) {
	seq := atomic.AddInt64(&nm.clientSeq, 1)

	bodyBytes, err := proto.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("序列化消息失败: %w", err)
	}

	msg := &gatepb.Message{
		Meta: &gatepb.Meta{
			ServiceName:  serviceName,
			MethodName:   methodName,
			MessageType:  1, // Request
			ClientSeq:    seq,
			ServerSeq:    nm.serverSeq,
		},
		Body: bodyBytes,
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, 0, fmt.Errorf("序列化网关消息失败: %w", err)
	}

	return data, seq, nil
}

// SendProtoMessage 发送protobuf消息
func (nm *NetworkManager) SendProtoMessage(serviceName, methodName string, req proto.Message, resp proto.Message, timeout ...time.Duration) error {
	nm.mu.RLock()
	ws := nm.ws
	connected := nm.connected
	nm.mu.RUnlock()

	if !connected || ws == nil {
		return fmt.Errorf("连接未打开")
	}

	data, seq, err := nm.EncodeMessage(serviceName, methodName, req)
	if err != nil {
		return err
	}

	// 创建回调通道
	callback := make(chan *Response, 1)
	nm.mu.Lock()
	nm.pendingCallbacks[seq] = callback
	nm.mu.Unlock()

	// 发送消息（使用 writeMu 保护，防止并发写入）
	nm.writeMu.Lock()
	err = ws.WriteMessage(websocket.BinaryMessage, data)
	nm.writeMu.Unlock()
	
	if err != nil {
		nm.mu.Lock()
		delete(nm.pendingCallbacks, seq)
		nm.mu.Unlock()
		return fmt.Errorf("发送消息失败: %w", err)
	}

	// 等待响应
	to := 10 * time.Second
	if len(timeout) > 0 {
		to = timeout[0]
	}

	select {
	case response := <-callback:
		if response.Err != nil {
			return response.Err
		}
		if resp != nil && response.Body != nil {
			if err := proto.Unmarshal(response.Body, resp); err != nil {
				return fmt.Errorf("解析响应失败: %w", err)
			}
		}
		return nil
	case <-time.After(to):
		nm.mu.Lock()
		delete(nm.pendingCallbacks, seq)
		nm.mu.Unlock()
		return fmt.Errorf("请求超时")
	}
}

// Connect 建立WebSocket连接
func (nm *NetworkManager) Connect(code string, onLoginSuccess func()) error {
	nm.mu.Lock()
	nm.onLoginSuccess = onLoginSuccess
	nm.mu.Unlock()

	url := fmt.Sprintf("%s?platform=%s&os=%s&ver=%s&code=%s&openID=",
		config.Current.ServerUrl,
		config.Current.Platform,
		config.Current.OS,
		config.Current.ClientVersion,
		code)

	headers := http.Header{
		"User-Agent": []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36 MicroMessenger/7.0.20.1781(0x6700143B) NetType/WIFI MiniProgramEnv/Windows WindowsWechat/WMPF WindowsWechat(0x63090a13)"},
		"Origin":     []string{"https://gate-obt.nqf.qq.com"},
	}

	ws, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}

	nm.mu.Lock()
	nm.ws = ws
	nm.connected = true
	nm.mu.Unlock()

	// 启动消息接收循环
	go nm.receiveLoop()

	// 发送登录请求
	go nm.sendLogin()

	return nil
}

// 接收循环
func (nm *NetworkManager) receiveLoop() {
	for {
		nm.mu.RLock()
		ws := nm.ws
		nm.mu.RUnlock()

		if ws == nil {
			break
		}

		_, data, err := ws.ReadMessage()
		if err != nil {
			utils.LogWarn("WS", fmt.Sprintf("读取错误: %v", err))
			nm.Cleanup()
			break
		}

		nm.handleMessage(data)
	}
}

// 处理消息
func (nm *NetworkManager) handleMessage(data []byte) {
	var msg gatepb.Message
	if err := proto.Unmarshal(data, &msg); err != nil {
		utils.LogWarn("网络", fmt.Sprintf("解码消息失败: %v", err))
		return
	}

	if msg.Meta == nil {
		return
	}

	meta := msg.Meta

	// 更新serverSeq
	if meta.ServerSeq > 0 {
		atomic.StoreInt64(&nm.serverSeq, meta.ServerSeq)
	}

	msgType := meta.MessageType

	// Notify (推送)
	if msgType == 3 {
		nm.handleNotify(&msg)
		return
	}

	// Response (响应)
	if msgType == 2 {
		clientSeq := meta.ClientSeq
		nm.mu.Lock()
		callback, exists := nm.pendingCallbacks[clientSeq]
		if exists {
			delete(nm.pendingCallbacks, clientSeq)
		}
		nm.mu.Unlock()

		if exists {
			resp := &Response{
				Body: msg.Body,
				Meta: meta,
			}
			if meta.ErrorCode != 0 {
				resp.Err = fmt.Errorf("%s.%s 错误: code=%d %s",
					meta.ServiceName, meta.MethodName,
					meta.ErrorCode, meta.ErrorMessage)
			}
			callback <- resp
		}
		return
	}
}

// 处理推送消息
func (nm *NetworkManager) handleNotify(msg *gatepb.Message) {
	if msg.Body == nil || len(msg.Body) == 0 {
		return
	}

	var eventMsg gatepb.EventMessage
	if err := proto.Unmarshal(msg.Body, &eventMsg); err != nil {
		return
	}

	msgType := eventMsg.MessageType

	// 被踢下线
	if contains(msgType, "Kickout") {
		utils.Log("推送", "被踢下线!")
		nm.events.Emit("kickout", msgType)
		return
	}

	// 土地状态变化
	if contains(msgType, "LandsNotify") {
		nm.events.Emit("landsChanged", eventMsg.Body)
		return
	}

	// 物品变化通知
	if contains(msgType, "ItemNotify") {
		nm.events.Emit("itemNotify", eventMsg.Body)
		return
	}

	// 基本信息变化 (升级/金币变化等)
	if contains(msgType, "BasicNotify") {
		nm.handleBasicNotify(eventMsg.Body)
		nm.events.Emit("basicNotify", eventMsg.Body)
		return
	}

	// 任务状态变化
	if contains(msgType, "TaskInfoNotify") {
		nm.events.Emit("taskInfoNotify", eventMsg.Body)
		return
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// 发送登录请求
func (nm *NetworkManager) sendLogin() {
	time.Sleep(500 * time.Millisecond)

	req := &userpb.LoginRequest{
		SharerId:     0,
		SharerOpenId: "",
		DeviceInfo: &userpb.DeviceInfo{
			ClientVersion: config.Current.DeviceInfo.ClientVersion,
			SysSoftware:   config.Current.DeviceInfo.SysSoftware,
			Network:       config.Current.DeviceInfo.Network,
			Memory:        7672,
			DeviceId:      config.Current.DeviceInfo.DeviceID,
		},
		ShareCfgId: 0,
		SceneId:    "1256",
		ReportData: &userpb.ReportData{
			MinigameChannel: "other",
			MinigamePlatid:  2,
		},
	}

	resp := &userpb.LoginReply{}
	err := nm.SendProtoMessage("gamepb.userpb.UserService", "Login", req, resp, 15*time.Second)
	if err != nil {
		utils.LogWarn("登录", fmt.Sprintf("失败: %v", err))
		return
	}

	if resp.Basic != nil {
		nm.userState.Set(
			resp.Basic.Gid,
			resp.Basic.Name,
			int(resp.Basic.Level),
			resp.Basic.Gold,
			resp.Basic.Exp,
		)

		if resp.TimeNowMillis > 0 {
			utils.SyncServerTime(resp.TimeNowMillis)
		}

		nm.mu.RLock()
		onSuccess := nm.onLoginSuccess
		nm.mu.RUnlock()

		if onSuccess != nil {
			onSuccess()
		}
	}
}

// StartHeartbeat 启动心跳
func (nm *NetworkManager) StartHeartbeat() {
	nm.mu.Lock()
	if nm.heartbeatTimer != nil {
		nm.heartbeatTimer.Stop()
	}
	nm.mu.Unlock()

	lastResponseTime := time.Now()
	heartbeatMissCount := 0

	ticker := time.NewTicker(config.Current.HeartbeatInterval)
	go func() {
		for range ticker.C {
			nm.mu.RLock()
			connected := nm.connected
			gid := nm.userState.GID
			pendingCount := len(nm.pendingCallbacks)
			nm.mu.RUnlock()

			if !connected || gid == 0 {
				ticker.Stop()
				return
			}

			// 检查上次心跳响应时间，超过60秒无响应说明连接有问题
			timeSinceLastResponse := time.Since(lastResponseTime)
			if timeSinceLastResponse > 60*time.Second {
				heartbeatMissCount++
				utils.LogWarn("心跳", fmt.Sprintf("连接可能已断开 (%.0fs 无响应, pending=%d)", 
					timeSinceLastResponse.Seconds(), pendingCount))
				
				if heartbeatMissCount >= 2 {
					utils.Log("心跳", "清理待处理请求...")
					// 清理所有待处理的回调，避免堆积
					nm.mu.Lock()
					for seq, ch := range nm.pendingCallbacks {
						select {
						case ch <- &Response{Err: fmt.Errorf("连接超时，已清理")}:
						default:
						}
						delete(nm.pendingCallbacks, seq)
					}
					nm.mu.Unlock()
					heartbeatMissCount = 0
				}
			}

			req := &userpb.HeartbeatRequest{
				Gid:            gid,
				ClientVersion:  config.Current.ClientVersion,
			}
			resp := &userpb.HeartbeatReply{}
			
			if err := nm.SendProtoMessage("gamepb.userpb.UserService", "Heartbeat", req, resp, 5*time.Second); err != nil {
				utils.LogWarn("心跳", fmt.Sprintf("失败: %v", err))
			} else {
				lastResponseTime = time.Now()
				heartbeatMissCount = 0
				if resp.ServerTime > 0 {
					utils.SyncServerTime(resp.ServerTime)
				}
			}
		}
	}()
}

// handleBasicNotify 处理基本信息变化通知 (升级/金币变化等)
func (nm *NetworkManager) handleBasicNotify(body []byte) {
	var notify userpb.BasicNotify
	if err := proto.Unmarshal(body, &notify); err != nil {
		return
	}

	if notify.Basic == nil {
		return
	}

	oldLevel := nm.userState.Level
	oldGold := nm.userState.Gold

	// 更新等级
	if notify.Basic.Level > 0 {
		nm.userState.UpdateLevel(int(notify.Basic.Level))
	}

	// 更新金币
	if notify.Basic.Gold > 0 {
		nm.userState.UpdateGold(notify.Basic.Gold)
	}

	// 更新经验
	if notify.Basic.Exp > 0 {
		nm.userState.UpdateExp(notify.Basic.Exp)
	}

	// 升级提示
	if nm.userState.Level != oldLevel {
		utils.Log("系统", fmt.Sprintf("升级! Lv%d → Lv%d", oldLevel, nm.userState.Level))
	}

	// 金币变化提示 (大幅增加时)
	if nm.userState.Gold > oldGold+10000 {
		utils.Log("系统", fmt.Sprintf("金币增加: %d → %d (+%d)", oldGold, nm.userState.Gold, nm.userState.Gold-oldGold))
	}
}

// Cleanup 清理资源
func (nm *NetworkManager) Cleanup() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.connected {
		return // 已经清理过了
	}

	nm.connected = false

	if nm.heartbeatTimer != nil {
		nm.heartbeatTimer.Stop()
		nm.heartbeatTimer = nil
	}

	if nm.ws != nil {
		nm.ws.Close()
		nm.ws = nil
	}

	// 清理待处理的回调
	for seq, ch := range nm.pendingCallbacks {
		close(ch)
		delete(nm.pendingCallbacks, seq)
	}

	// 触发断开连接事件
	nm.events.Emit("disconnected", nil)
}

// IsConnected 检查连接状态
func (nm *NetworkManager) IsConnected() bool {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.connected
}
