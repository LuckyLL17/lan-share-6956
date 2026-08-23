package model

import "time"

// Message 设备间发送的一条简单文本消息。
// 通过 UDP 广播 + 持久化记录的方式实现简易聊天。
type Message struct {
	// ID 主键
	ID int64 `json:"id"`
	// FromIP 发送方 IP
	FromIP string `json:"from_ip"`
	// FromName 发送方设备名
	FromName string `json:"from_name"`
	// ToIP 接收方 IP，空字符串表示广播给所有设备
	ToIP string `json:"to_ip"`
	// Content 文本内容
	Content string `json:"content"`
	// Read 是否已读
	Read bool `json:"read"`
	// CreatedAt 接收时间
	CreatedAt time.Time `json:"created_at"`
}

// broadcastAddr 是规范化后的广播哨兵地址。
// network.normalizeMessageTarget 会把空 ToIP / "0.0.0.0" 统一映射为此值，
// 因此这里以它作为“广播给所有设备”的唯一规范表示。
const broadcastAddr = "255.255.255.255"

// IsBroadcast 是否为广播消息（发给局域网内所有设备）。
// ToIP 为空或为广播哨兵地址 255.255.255.255 时为真；
// 任何具体单播地址都不是广播。
func (m Message) IsBroadcast() bool {
	return m.ToIP == "" || m.ToIP == broadcastAddr
}

// Targets reports whether a message should be handled by the local device.
// 广播消息一律接收；单播消息仅当 ToIP 等于本机地址时接收，
// 从而避免发往其他局域网地址的消息进入本地收件箱。
func (m Message) Targets(localIP string) bool {
	if m.IsBroadcast() {
		return true
	}
	return m.ToIP == localIP
}

// Validate 消息基本校验
func (m Message) Validate() error {
	if m.Content == "" {
		return ErrMessageEmpty
	}
	if len(m.Content) > 4096 {
		return ErrMessageTooLong
	}
	if m.FromIP == "" {
		return ErrMessageFromEmpty
	}
	return nil
}

// MessageError 消息相关错误
type MessageError struct {
	Code string
	Msg  string
}

func (e *MessageError) Error() string { return e.Msg }

var (
	ErrMessageEmpty     = &MessageError{Code: "EMPTY", Msg: "message content is empty"}
	ErrMessageTooLong   = &MessageError{Code: "TOO_LONG", Msg: "message content exceeds 4096 chars"}
	ErrMessageFromEmpty = &MessageError{Code: "FROM_EMPTY", Msg: "from ip is empty"}
)
