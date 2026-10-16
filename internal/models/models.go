// Package models 定义全部数据模型，字段与 RustDesk 客户端 API 契约一一对应。
package models

import "time"

// User 平台用户。第一个注册的用户自动成为管理员（与 Python 参考版一致）。
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:50;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"column:password_hash;size:100;not null" json:"-"`
	RID          string    `gorm:"column:rid;size:20" json:"rid"`
	UUID         string    `gorm:"column:uuid;size:64" json:"uuid"`
	AutoLogin    bool      `gorm:"column:auto_login;default:true" json:"auto_login"`
	RType        string    `gorm:"column:rtype;size:20" json:"rtype"`
	DeviceInfo   string    `gorm:"column:device_info;type:text" json:"device_info"`
	IsActive     bool      `gorm:"column:is_active;default:true" json:"is_active"`
	IsAdmin      bool      `gorm:"column:is_admin;default:false" json:"is_admin"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// Token 客户端 access_token。ExpireAt 之前的 token 有效，
// 心跳会不断续期（与参考版 create_time 超前量的等价实现）。
type Token struct {
	ID          uint      `gorm:"primaryKey"`
	Username    string    `gorm:"size:50;index"`
	UID         uint      `gorm:"index"`
	RID         string    `gorm:"column:rid;size:20;index"`
	UUID        string    `gorm:"column:uuid;size:64"`
	AccessToken string    `gorm:"size:64;uniqueIndex;not null"`
	ExpireAt    time.Time `gorm:"index"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}

// Tag 用户地址簿标签（按 uid 隔离）。
type Tag struct {
	ID    uint   `gorm:"primaryKey"`
	UID   uint   `gorm:"index"`
	Name  string `gorm:"size:60"`
	Color string `gorm:"size:60"`
}

// Peer 用户地址簿中的设备条目。
type Peer struct {
	ID       uint   `gorm:"primaryKey"`
	UID      uint   `gorm:"index:idx_peer_uid_rid"`
	RID      string `gorm:"column:rid;size:60;index:idx_peer_uid_rid"`
	Username string `gorm:"size:100"`
	Hostname string `gorm:"size:100"`
	Alias    string `gorm:"size:100"`
	Platform string `gorm:"size:50"`
	Tags     string `gorm:"size:200"`
	RHash    string `gorm:"column:rhash;size:100"`
}

// Device 客户端通过 sysinfo 上报的设备资产信息，(RID, UUID) 唯一。
type Device struct {
	ID         uint      `gorm:"primaryKey"`
	RID        string    `gorm:"column:rid;size:60;uniqueIndex:idx_device_rid_uuid"`
	CPU        string    `gorm:"size:200"`
	Hostname   string    `gorm:"size:200"`
	Memory     string    `gorm:"size:200"`
	OS         string    `gorm:"size:200"`
	UUID       string    `gorm:"column:uuid;size:100;uniqueIndex:idx_device_rid_uuid"`
	Username   string    `gorm:"size:100"`
	Version    string    `gorm:"size:50"`
	IPAddress  string    `gorm:"column:ip_address;size:64"`
	CreateTime time.Time `gorm:"autoCreateTime"`
	UpdateTime time.Time `gorm:"autoUpdateTime;index"`
}

// ConnLog 远程连接日志（客户端 audit 接口上报）。
type ConnLog struct {
	ID        uint       `gorm:"primaryKey"`
	Action    string     `gorm:"size:20;index"`
	ConnID    string     `gorm:"column:conn_id;size:20;index"`
	FromIP    string     `gorm:"column:from_ip;size:64"`
	FromID    string     `gorm:"column:from_id;size:20"`
	RID       string     `gorm:"column:rid;size:20;index"`
	ConnStart time.Time  `gorm:"column:conn_start;index"`
	ConnEnd   *time.Time `gorm:"column:conn_end;index"`
	SessionID string     `gorm:"column:session_id;size:64"`
	UUID      string     `gorm:"column:uuid;size:64"`
}

// FileLog 文件传输日志。
type FileLog struct {
	ID        uint      `gorm:"primaryKey"`
	File      string    `gorm:"size:500"`
	RemoteID  string    `gorm:"size:20"`
	UserID    string    `gorm:"size:20"`
	UserIP    string    `gorm:"size:64"`
	Filesize  string    `gorm:"size:50"`
	Direction int       `gorm:"index"`
	LoggedAt  time.Time `gorm:"index"`
}

// ShareLink 地址簿分享链接，生成 15 分钟后过期。
type ShareLink struct {
	ID         uint      `gorm:"primaryKey"`
	UID        uint      `gorm:"index"`
	SHash      string    `gorm:"column:shash;size:64;uniqueIndex"`
	Peers      string    `gorm:"size:500"`
	IsUsed     bool      `gorm:"default:false;index"`
	IsExpired  bool      `gorm:"default:false;index"`
	CreateTime time.Time `gorm:"autoCreateTime"`
}
