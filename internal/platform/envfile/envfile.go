package envfile

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// LoadForDevelopment 加载本地环境文件；已存在的进程环境变量优先级更高。
func LoadForDevelopment() error {
	// 1. 依次尝试本地覆盖文件和通用开发文件，不要求文件必须存在。
	for _, path := range []string{".env.local", ".env"} {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := godotenv.Load(path); err != nil {
			return err
		}
	}
	return nil
}
