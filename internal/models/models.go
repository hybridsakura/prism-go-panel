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

// ServerProfile 服务端配置档案（单行，ID 恒为 1）：
// 记录 hbbs/hbbr 的部署信息，用于在管理后台生成客户端配置
// （重命名安装包、--config 导入串、手动填写说明）。
// 注意：这是「档案」，修改它不会改变 hbbs/hbbr 的实际运行参数——
// 那些由 hbbs/hbbr 的启动参数决定；此处记录的值应与实际部署保持一致。
type ServerProfile struct {
	ID          uint      `gorm:"primaryKey"`
	IDServer    string    `gorm:"size:200"` // ID/信令服务器（hbbs），host[:port]，默认端口 21116
	RelayServer string    `gorm:"size:200"` // 中继服务器（hbbr），host[:port]，默认同 ID 服务器、端口 21117
	APIServer   string    `gorm:"size:200"` // 本 API 服务对外地址，host[:port]，默认端口 21114
	Key         string    `gorm:"size:200"` // 服务端公钥（id_ed25519.pub 内容）
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

// IDConflict 远程 ID 冲突记录：同一个 RID 被不同的物理机/系统使用
// （典型来源：硬盘拷贝机克隆系统，把 RustDesk 配置连同 ID 一起复制）。
// 后果是两台机器争抢同一个 ID：hbbs 只保存其中一个公钥，
// 另一台连接时签名校验失败，客户端弹出「无法验证端到端加密」。
// 这里记录了观察到的不同主机名/UUID，用于阻止登录并提示用户更换 ID。
type IDConflict struct {
	ID              uint      `gorm:"primaryKey"`
	RID             string    `gorm:"column:rid;size:60;uniqueIndex"`
	Hostnames       string    `gorm:"size:500"`  // JSON 数组：观察到的不同主机名
	UUIDs           string    `gorm:"size:800"`  // JSON 数组：观察到的不同 UUID
	PrimaryUUID     string    `gorm:"size:100"`  // 最早注册机器的 UUID（原机，登录放行）
	PrimaryHostname string    `gorm:"size:100"`  // 最早注册机器的主机名
	Observations    int       `gorm:"default:1"` // 冲突观察次数
	Blocked         int       `gorm:"default:0"` // 已拒绝登录（提示更换 ID）次数
	FirstSeen       time.Time `gorm:"autoCreateTime"`
	LastSeen        time.Time `gorm:"autoUpdateTime;index"`
}

// EnrollToken 一次性注册码。只保存哈希，明文仅在生成时返回一次。
type EnrollToken struct {
	ID        uint       `gorm:"primaryKey"`
	TokenHash string     `gorm:"size:128;uniqueIndex;not null"`
	GroupName string     `gorm:"size:100"`
	MaxUses   int        `gorm:"default:1"`
	Used      int        `gorm:"default:0"`
	ExpiresAt *time.Time `gorm:"index"`
	Note      string     `gorm:"size:200"`
	CreatedAt time.Time  `gorm:"autoCreateTime"`
}

// Agent 自研终端程序的实例记录。
// 令牌同样只存哈希；Fingerprint 用于同一台机器重复注册的幂等判定。
type Agent struct {
	ID          uint       `gorm:"primaryKey"`
	TokenHash   string     `gorm:"size:128;uniqueIndex;not null"`
	Fingerprint string     `gorm:"size:128;index"`
	RID         string     `gorm:"column:rid;size:60;index"`
	Hostname    string     `gorm:"size:100"`
	OS          string     `gorm:"size:100"`
	Arch        string     `gorm:"size:50"`
	Version     string     `gorm:"size:50"`
	IP          string     `gorm:"size:64"`
	GroupName   string     `gorm:"size:100;index"`
	Status      string     `gorm:"size:20;default:active;index"`
	LastSeenAt  *time.Time `gorm:"index"`
	EnrolledAt  time.Time  `gorm:"autoCreateTime"`
}
