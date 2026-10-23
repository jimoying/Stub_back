package auth

import (
	"fmt"

	"github.com/alexedwards/argon2id"
)

var passwordParams = &argon2id.Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// HashPassword 使用 Argon2id 生成可验证的密码摘要。
func HashPassword(password string) (string, error) {
	// 1. 拒绝空密码，并委托成熟 Argon2id 实现生成随机 salt 与摘要。
	if password == "" {
		return "", fmt.Errorf("密码不能为空")
	}
	hash, err := argon2id.CreateHash(password, passwordParams)
	if err != nil {
		return "", fmt.Errorf("生成密码摘要失败: %w", err)
	}
	return hash, nil
}

// VerifyPassword 比较密码与 Argon2id 编码摘要。
func VerifyPassword(password, encodedHash string) bool {
	// 1. 不将格式错误的数据库数据暴露为服务异常。
	ok, err := argon2id.ComparePasswordAndHash(password, encodedHash)
	return err == nil && ok
}
