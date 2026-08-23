# 并发设备发现缓存复现记录

## Bug 是什么

多台设备同时被发现并读取设备列表时，内存缓存发生数据竞争。列表接口可能得到不完整结果，且在竞态检测下会报告运行时访问冲突。

## 如何触发

并发执行设备发现包处理和设备列表读取，使缓存更新与读取同时发生。修复前，设备发现服务中的缓存读写没有统一同步边界，竞态检测能够稳定发现冲突。

## 根因

设备发现服务在处理发现包时更新缓存，同时列表读取路径遍历同一缓存，二者没有受同一并发控制保护。`internal/service/discover_service.go` 负责发现包处理和列表读取，`internal/repository/device_repo.go` 提供设备持久化交互；并发缓存访问导致 map 读写冲突。Claude 修复为缓存访问增加一致的同步边界，使发现更新、列表读取和设备持久化协调完成。

## 运行指令

```text
go test -v -race ./internal/service -run '^TestConcurrentDeviceDiscoveryCache$' -count=20
```

## 错误信息

修复前，竞态检测报告设备发现缓存的并发读写冲突，目标测试失败；修复后在完整五轮稳定性检查中均通过。

## 错误堆栈

```text
=== RUN   TestConcurrentDeviceDiscoveryCache
==================
WARNING: DATA RACE
Write at 0x00c00017e6f0 by goroutine 23:
  lan-share/internal/service.(*DiscoverService).handleDiscoveryPacket()
      internal/service/discover_service.go:143 +0x340

Previous read at 0x00c00017e6f0 by goroutine 42:
  lan-share/internal/service.(*DiscoverService).ListDevices()
      internal/service/discover_service.go:96 +0x1a4

--- FAIL: TestConcurrentDeviceDiscoveryCache
FAIL
```
