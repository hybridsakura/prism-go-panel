package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

// TestCheckPasswordDjangoVector 用 Python/Django 生成的标准哈希做交叉验证，
// 确保实现与 Django 完全一致（而非仅自洽）。
func TestCheckPasswordDjangoVector(t *testing.T) {
	// python3 -c "import hashlib,base64;print(base64.b64encode(hashlib.pbkdf2_hmac('sha256',b'testpass123',b'abcdefghijkl',260000,32)).decode())"
	const hash = "pbkdf2_sha256$260000$abcdefghijkl$LgLiQfzq+l2d38i/9YT6FCJHbF2dWmmBqjFEioyF9ms="
	if !CheckPassword(hash, "testpass123") {
		t.Fatal("Django 标准矢量校验失败")
	}
	if CheckPassword(hash, "testpass124") {
		t.Fatal("错误密码不应通过")
	}
}

// TestCheckPasswordDjangoPBKDF2 校验 Django 迁移过来的密码哈希。
func TestCheckPasswordDjangoPBKDF2(t *testing.T) {
	plain := "Aqxlws@2025"
	salt := "N4qeQw7lX2dyr9abc"
	iter := 870000
	raw := pbkdf2.Key([]byte(plain), []byte(salt), iter, 32, sha256.New)
	hash := "pbkdf2_sha256$" + strconv.Itoa(iter) + "$" + salt + "$" + base64.StdEncoding.EncodeToString(raw)

	if !CheckPassword(hash, plain) {
		t.Fatal("正确的 Django 哈希应通过校验")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Fatal("错误密码不应通过")
	}
	if CheckPassword("pbkdf2_sha256$bad$salt$hash", plain) {
		t.Fatal("格式错误的哈希应返回 false")
	}
}

// TestCheckPasswordBcrypt 保持对 bcrypt 的兼容。
func TestCheckPasswordBcrypt(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !CheckPassword(hash, "secret123") {
		t.Fatal("bcrypt 正确密码应通过")
	}
	if CheckPassword(hash, "secret124") {
		t.Fatal("bcrypt 错误密码不应通过")
	}
}
