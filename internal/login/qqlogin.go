package login

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gofarm/internal/utils"
)

const (
	chromeUA   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	qua        = "V1_HT5_QDT_0.70.2209190_x64_0_DEV_D"
	farmAppID  = "1112386029"
)

// QRCodeResponse 二维码响应
type QRCodeResponse struct {
	Code int `json:"code"`
	Data struct {
		Code string `json:"code"`
	} `json:"data"`
}

// ScanStatusResponse 扫码状态响应
type ScanStatusResponse struct {
	Code int `json:"code"`
	Data struct {
		Ok     int    `json:"ok"`
		Ticket string `json:"ticket"`
	} `json:"data"`
}

// AuthCodeResponse 授权码响应
type AuthCodeResponse struct {
	Code string `json:"code"`
}

func getHeaders() map[string]string {
	return map[string]string{
		"qua":          qua,
		"host":         "q.qq.com",
		"accept":       "application/json",
		"content-type": "application/json",
		"user-agent":   chromeUA,
	}
}

// requestLoginCode 请求登录二维码
func requestLoginCode() (string, string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", "https://q.qq.com/ide/devtoolAuth/GetLoginCode", nil)
	if err != nil {
		return "", "", err
	}

	for k, v := range getHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var result QRCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	if result.Code != 0 || result.Data.Code == "" {
		return "", "", fmt.Errorf("获取QQ扫码登录码失败")
	}

	loginCode := result.Data.Code
	url := fmt.Sprintf("https://h5.qzone.qq.com/qqq/code/%s?_proxy=1&from=ide", loginCode)

	return loginCode, url, nil
}

// queryScanStatus 查询扫码状态
func queryScanStatus(loginCode string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://q.qq.com/ide/devtoolAuth/syncScanSateGetTicket?code=%s", loginCode)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	for k, v := range getHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "Error", nil
	}

	var result ScanStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "Error", nil
	}

	if result.Code == 0 {
		if result.Data.Ok != 1 {
			return "Wait", nil
		}
		return result.Data.Ticket, nil
	}
	
	if result.Code == -10003 {
		return "Used", nil
	}
	
	return "Error", nil
}

// getAuthCode 获取授权码
func getAuthCode(ticket string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	
	payload := map[string]string{
		"appid":  farmAppID,
		"ticket": ticket,
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://q.qq.com/ide/login", nil)
	if err != nil {
		return "", err
	}

	for k, v := range getHeaders() {
		req.Header.Set(k, v)
	}
	
	// 使用bytes.Reader作为body
	req.Body = io.NopCloser(bytes.NewReader(jsonData))
	req.ContentLength = int64(len(jsonData))

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("获取农场登录code失败")
	}

	var result AuthCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("获取农场登录code失败")
	}

	if result.Code == "" {
		return "", fmt.Errorf("获取农场登录code失败")
	}

	return result.Code, nil
}

// GetQQFarmCodeByScan 通过扫码获取QQ农场登录码
func GetQQFarmCodeByScan(options ...map[string]interface{}) (string, error) {
	pollIntervalMs := 2000
	timeoutMs := 180000

	if len(options) > 0 {
		if v, ok := options[0]["pollIntervalMs"].(int); ok && v > 0 {
			pollIntervalMs = v
		}
		if v, ok := options[0]["timeoutMs"].(int); ok && v > 0 {
			timeoutMs = v
		}
	}

	loginCode, qrURL, err := requestLoginCode()
	if err != nil {
		return "", err
	}

	printQR(qrURL)

	start := time.Now()
	for time.Since(start).Milliseconds() < int64(timeoutMs) {
		status, err := queryScanStatus(loginCode)
		if err != nil {
			return "", err
		}

		if status != "Wait" && status != "Error" && status != "Used" {
			// 获取到ticket
			authCode, err := getAuthCode(status)
			if err != nil {
				return "", err
			}
			return authCode, nil
		}

		if status == "Used" {
			return "", fmt.Errorf("二维码已失效，请重试")
		}

		if status == "Error" {
			return "", fmt.Errorf("扫码状态查询失败，请重试")
		}

		time.Sleep(time.Duration(pollIntervalMs) * time.Millisecond)
	}

	return "", fmt.Errorf("扫码超时，请重试")
}

// printQR 打印二维码
func printQR(url string) {
	fmt.Println()
	fmt.Println("[扫码登录] 请用 QQ 扫描下方二维码确认登录:")
	
	// 使用在线API生成二维码
	qrAPI := fmt.Sprintf("https://api.qrserver.com/v1/create-qr-code/?size=300x300&data=%s", url)
	fmt.Printf("[扫码登录] 二维码链接: %s\n", qrAPI)
	fmt.Printf("[扫码登录] 或直接打开链接: %s\n", url)
	fmt.Println()
	
	// 尝试使用ASCII艺术打印简单二维码
	utils.Log("扫码", "等待扫码...")
}
