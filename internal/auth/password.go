// Package auth 提供密码哈希、客户端 access_token 与管理后台会话三类认证能力。
package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword 使用 bcrypt 哈希密码（cost=10，与主流 Web 应用一致）。
func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// CheckPassword 校验明文密码与哈希是否匹配。
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
