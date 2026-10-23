package auth

import (
	"regexp"
	"testing"
)

func TestHashPassword(t *testing.T) {
	// 1. 密码摘要必须可验证，且不能与明文相同。
	password := "correct horse battery staple"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if hash == password || !VerifyPassword(password, hash) {
		t.Fatal("密码摘要无法正确验证")
	}
	if VerifyPassword("wrong password", hash) {
		t.Fatal("错误密码不应通过验证")
	}
}

func TestGenerateNumericCode(t *testing.T) {
	// 1. 邮箱验证码长度固定，且只包含十进制数字。
	code, err := generateNumericCode(6)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(code) {
		t.Fatalf("验证码格式错误: %q", code)
	}
}

func TestNormalizeEmail(t *testing.T) {
	// 1. 邮箱统一小写并拒绝带显示名的非规范输入。
	got, err := normalizeEmail(" User@Example.COM ")
	if err != nil || got != "user@example.com" {
		t.Fatalf("邮箱归一化失败: %q, %v", got, err)
	}
	if _, err := normalizeEmail("User <user@example.com>"); err == nil {
		t.Fatal("带显示名的输入不应作为规范邮箱接受")
	}
}
