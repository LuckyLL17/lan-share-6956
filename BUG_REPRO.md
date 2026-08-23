# UDP 定向消息误入本地收件箱复现记录

## Bug 是什么

设备收到发往其他局域网地址的 UDP 定向消息后，消息仍会写入本地收件箱，并增加未读数量。

## 如何触发

向当前设备发送一条来源合法、但目标地址是另一台局域网设备的 UDP 消息。修复前，接收链路没有正确区分广播、发给本机和发给其他设备的消息，定向消息会被持久化到本地消息列表。

## 根因

消息模型将大多数非空目标地址误判为广播，导致服务层的目标判定始终允许消息继续进入持久化流程。Claude 修复将广播判定收敛为明确的广播地址或空目标，并在服务层依据本机地址过滤非目标单播消息，使广播和发给本机的消息仍可进入收件箱。

## 运行指令

```text
go test -v ./internal/service -run '^TestTargetedUDPMessageDoesNotPolluteLocalInbox$' -count=1
```

## 验证结果

修复前目标测试连续五轮失败，发往其他地址的消息会留下本地记录并增加未读数。修复后目标测试连续五轮通过，只有广播或发给本机的消息可以进入本地收件箱。

## 错误信息

修复前，非目标定向消息经过 UDP 接收和服务处理后被写入消息仓库，消息列表与未读数量均出现无关记录。

## 错误堆栈

```text
=== RUN   TestTargetedUDPMessageDoesNotPolluteLocalInbox
    message_target_test.go:70: targeted message persisted: messages=1 unread=1
--- FAIL: TestTargetedUDPMessageDoesNotPolluteLocalInbox (0.01s)
FAIL
FAIL	lan-share/internal/service	0.474s
FAIL
```
