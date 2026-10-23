package auth

import (
	"context"
	"fmt"
	"time"

	gomail "github.com/wneessen/go-mail"
	"github.com/yourGaomu/short_links/back/internal/config"
)

type smtpSender struct {
	config config.Config
}

func newSMTPSender(cfg config.Config) EmailSender {
	if cfg.SMTPHost == "" || cfg.SMTPFrom == "" {
		return nil
	}
	return smtpSender{config: cfg}
}

func (s smtpSender) SendVerificationCode(ctx context.Context, email, purpose, code string) error {
	// 1. 按用途生成统一的邮件主题和文本内容，不将验证码写入应用日志。
	subject := "短链接服务邮箱验证"
	usage := "完成邮箱验证"
	if purpose == "password_reset" {
		subject = "短链接服务密码重置"
		usage = "重置登录密码"
	}
	message := gomail.NewMsg()
	if err := message.From(s.config.SMTPFrom); err != nil {
		return fmt.Errorf("SMTP 发件人配置无效: %w", err)
	}
	if err := message.To(email); err != nil {
		return fmt.Errorf("SMTP 收件人地址无效: %w", err)
	}
	message.Subject(subject)
	message.SetBodyString(gomail.TypeTextPlain, fmt.Sprintf("%s的验证码是：%s\n验证码 10 分钟内有效，请勿向他人透露。", usage, code))

	// 2. 根据部署配置选择 STARTTLS、隐式 TLS 或明文 SMTP。
	options := []gomail.Option{gomail.WithPort(s.config.SMTPPort), gomail.WithTimeout(15 * time.Second)}
	switch s.config.SMTPTLSMode {
	case "starttls":
		options = append(options, gomail.WithTLSPortPolicy(gomail.TLSMandatory))
	case "implicit":
		options = append(options, gomail.WithSSL())
	case "none":
		options = append(options, gomail.WithTLSPortPolicy(gomail.NoTLS))
	}
	if s.config.SMTPUsername != "" {
		options = append(options,
			gomail.WithUsername(s.config.SMTPUsername),
			gomail.WithPassword(s.config.SMTPPassword),
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
		)
	}
	client, err := gomail.NewClient(s.config.SMTPHost, options...)
	if err != nil {
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	return client.DialAndSendWithContext(ctx, message)
}
