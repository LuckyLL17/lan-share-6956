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

// IsBroadcast 是否为广播消息
func (m Message) IsBroadcast() bool {
	return m.ToIP == "" || m.ToIP == "255.255.255.255"
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
