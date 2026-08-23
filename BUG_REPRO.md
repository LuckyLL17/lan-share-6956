# 设备发现启动失败清理复现记录

## Bug 是什么

设备发现服务尚未成功建立 UDP 监听时进入清理流程，关闭操作会等待不存在的后台完成信号，导致进程退出卡住。

## 如何触发

构造尚未启动监听的 UDP 发现实例并调用关闭，再构造尚未成功启动的发现服务并调用停止。修复前，UDP 关闭会一直等待未启动的读取循环，发现服务停止也可能等待未启动的广播循环。

## 根因

UDP 关闭路径把实例创建状态误当成读取循环已经启动，并在所有情况下等待 done 通道；发现服务停止路径也没有区分后台广播循环是否真正启动。启动失败、部分启动和正常运行共用同一等待逻辑，导致未启动分支永久等待。Claude 修复分别记录 UDP 读取循环和发现服务广播循环的实际运行状态，只在对应循环启动后等待完成信号，同时保留幂等停止和资源关闭顺序。

## 运行指令

```text
go test -v ./internal/service -run '^TestStartupFailureCleanupDoesNotHang$' -count=1
```

## 验证结果

修复前目标测试连续 5 轮失败，UDP 关闭在限定时间内未返回。修复后目标测试连续 5 轮通过，未启动的 UDP 实例和发现服务都能在有限时间内完成清理。

## 错误信息

修复前关闭路径等待未启动的后台循环完成信号，测试在限定时间内报告清理卡住。修复后停止操作只等待实际启动的后台循环。

## 错误堆栈

```text
=== RUN   TestStartupFailureCleanupDoesNotHang
    startup_cleanup_test.go:17: UDP close hung during startup cleanup
--- FAIL: TestStartupFailureCleanupDoesNotHang
FAIL
```
