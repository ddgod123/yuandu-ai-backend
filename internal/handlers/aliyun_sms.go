package handlers

import (
	"errors"
	"strings"

	openapiutil "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	dypns "github.com/alibabacloud-go/dypnsapi-20170525/v3/client"
	"github.com/alibabacloud-go/tea/tea"
)

const defaultAliyunSmsTemplateParam = "{\"code\":\"##code##\"}"

func (h *Handler) newAliyunSMSClient() (*dypns.Client, error) {
	accessKeyID := strings.TrimSpace(h.cfg.AliyunAccessKeyId)
	accessKeySecret := strings.TrimSpace(h.cfg.AliyunAccessKeySecret)
	if accessKeyID == "" || accessKeySecret == "" {
		return nil, errors.New("aliyun sms not configured")
	}

	endpoint := strings.TrimSpace(h.cfg.AliyunSmsEndpoint)
	if endpoint == "" {
		endpoint = "dypnsapi.aliyuncs.com"
	}
	region := strings.TrimSpace(h.cfg.AliyunSmsRegionId)
	if region == "" {
		region = "cn-hangzhou"
	}

	cfg := &openapiutil.Config{
		AccessKeyId:     tea.String(accessKeyID),
		AccessKeySecret: tea.String(accessKeySecret),
		Endpoint:        tea.String(endpoint),
		RegionId:        tea.String(region),
	}
	return dypns.NewClient(cfg)
}

func (h *Handler) sendAliyunSMSCode(phone string) (string, int64, bool, error) {
	signName := strings.TrimSpace(h.cfg.AliyunSmsSignName)
	templateCode := strings.TrimSpace(h.cfg.AliyunSmsTemplateCode)
	if signName == "" || templateCode == "" {
		return "", 0, false, errors.New("aliyun sms template not configured")
	}

	templateParam := strings.TrimSpace(h.cfg.AliyunSmsTemplateParam)
	if templateParam == "" {
		templateParam = defaultAliyunSmsTemplateParam
	}

	validTime := h.cfg.AliyunSmsValidTime
	if validTime <= 0 {
		validTime = 300
	}
	interval := h.cfg.AliyunSmsInterval
	if interval <= 0 {
		interval = 60
	}

	client, err := h.newAliyunSMSClient()
	if err != nil {
		return "", 0, false, err
	}

	req := &dypns.SendSmsVerifyCodeRequest{
		PhoneNumber:      tea.String(phone),
		SignName:         tea.String(signName),
		TemplateCode:     tea.String(templateCode),
		TemplateParam:    tea.String(templateParam),
		ValidTime:        tea.Int64(int64(validTime)),
		Interval:         tea.Int64(int64(interval)),
		ReturnVerifyCode: tea.Bool(h.cfg.AliyunSmsReturnCode),
	}
	countryCode := strings.TrimSpace(h.cfg.AliyunSmsCountryCode)
	if countryCode != "" {
		req.CountryCode = tea.String(countryCode)
	}

	resp, err := client.SendSmsVerifyCode(req)
	if err != nil {
		return "", int64(validTime), false, err
	}
	if resp == nil || resp.Body == nil || resp.Body.Success == nil || !*resp.Body.Success {
		message := "短信发送失败"
		if resp != nil && resp.Body != nil && resp.Body.Message != nil {
			message = *resp.Body.Message
		}
		return "", int64(validTime), false, errors.New(message)
	}

	code := ""
	if h.cfg.AliyunSmsReturnCode && resp.Body.Model != nil && resp.Body.Model.VerifyCode != nil {
		code = *resp.Body.Model.VerifyCode
	}
	return code, int64(validTime), h.cfg.AliyunSmsReturnCode, nil
}

func (h *Handler) verifyAliyunSMSCode(phone, code string) (bool, error) {
	client, err := h.newAliyunSMSClient()
	if err != nil {
		return false, err
	}
	req := &dypns.CheckSmsVerifyCodeRequest{
		PhoneNumber: tea.String(phone),
		VerifyCode:  tea.String(code),
	}
	countryCode := strings.TrimSpace(h.cfg.AliyunSmsCountryCode)
	if countryCode != "" {
		req.CountryCode = tea.String(countryCode)
	}
	resp, err := client.CheckSmsVerifyCode(req)
	if err != nil {
		return false, err
	}
	if resp == nil || resp.Body == nil || resp.Body.Success == nil || !*resp.Body.Success {
		message := "验证码校验失败"
		if resp != nil && resp.Body != nil && resp.Body.Message != nil {
			message = *resp.Body.Message
		}
		return false, errors.New(message)
	}
	if resp.Body.Model == nil || resp.Body.Model.VerifyResult == nil {
		return false, errors.New("验证码校验失败")
	}
	return strings.EqualFold(*resp.Body.Model.VerifyResult, "PASS"), nil
}
