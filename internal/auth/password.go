// Package auth 提供密码哈希、客户端 access_token 与管理后台会话三类认证能力。
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

// HashPassword 使用 bcrypt 哈希密码（cost=10，与主流 Web 应用一致）。
func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// CheckPassword 校验明文密码与哈希是否匹配。
// 兼容两种格式：
//   - bcrypt（本服务新建用户，形如 $2a$10$...）
//   - Django pbkdf2_sha256（从 Python 版迁移的用户，形如 pbkdf2_sha256$870000$salt$hash）
func CheckPassword(hash, plain string) bool {
	if strings.HasPrefix(hash, "pbkdf2_sha256$") {
		return checkDjangoPBKDF2(hash, plain)
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// checkDjangoPBKDF2 校验 Django 的 pbkdf2_sha256 哈希。
func checkDjangoPBKDF2(hash, plain string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 || iter > 10_000_000 {
		return false
	}
	salt := parts[2]
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got := pbkdf2.Key([]byte(plain), []byte(salt), iter, len(want), sha256.New)
	return subtle.ConstantTimeCompare(got, want) == 1
}
