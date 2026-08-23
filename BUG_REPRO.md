# 消息发送错误传播诊断记录

## Bug 是什么

向不可达的局域网设备发送消息时，底层发送失败被记录到日志，但 HTTP 接口仍返回发送成功。调用方无法从响应判断失败，因而无法执行可靠重试。

## 如何触发

通过消息发送接口指定不可用的 UDP 目标。修复前，发送路径记录了 UDP 未监听错误，但请求响应仍返回成功状态。

## 根因

错误传播链路从 `internal/network/udp.go` 的 UDP 写入开始，经 `internal/service/discover_service.go` 的消息发送服务，再到 `internal/handler/message_handler.go` 的 HTTP 处理器。UDP 层与服务层将部分发送错误记录后返回 nil，处理器又忽略服务调用返回值并无条件写入成功响应，因此错误无法抵达 HTTP 调用方。UDP 对远端不可达本身不保证即时错误反馈；即使底层出现可观察错误，现有跨层链路也会将其吞掉，造成调用方与实际发送结果不一致。

## 运行指令

```text
go test -v ./internal/handler -run '^TestMessageSendFailureReachesHTTPResponse$' -count=1
```

## 错误信息

测试观察到 UDP 未监听错误，但 HTTP 响应为 200 且消息为 sent。诊断题仅分析错误传播边界，Claude 主轨迹未修改任何代码。

## 错误堆栈

```text
=== RUN   TestMessageSendFailureReachesHTTPResponse
2026/08/23 09:29:11 [discover] send message: udp not listening
message_error_propagation_test.go:57: unreachable UDP target returned success: status=200 body={"code":0,"message":"sent"}
--- FAIL: TestMessageSendFailureReachesHTTPResponse
FAIL
```
