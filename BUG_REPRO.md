# 服务退出生命周期诊断记录

## Bug 是什么

服务收到终止信号并完成 HTTP 关闭后，退出阶段仍可能异常中止。现象是发现服务已经停止、主流程已输出退出日志，但进程随后因关闭资源重复执行而崩溃。

## 如何触发

启动服务后发送终止信号，使 HTTP 服务、发现服务与 UDP 资源进入关闭路径。修复前，关闭阶段的多个路径会触发同一发现服务停止动作。

## 根因

`main.go` 的退出流程与 `internal/service/discover_service.go` 的停止逻辑共同驱动发现服务生命周期，`internal/network/udp.go` 持有对应的 UDP 资源。停止方法没有保证关闭操作只执行一次，首次停止已关闭内部 channel 后，后续退出路径再次关闭该 channel，触发运行时 panic。该问题破坏了终止信号到 HTTP 关闭、发现服务停止和 UDP 资源回收之间应当幂等收敛的生命周期契约。

## 运行指令

```text
go test -v . -run '^TestGracefulShutdownLifecycle$' -count=1
```

## 错误信息

测试中服务接收终止信号并开始退出，发现服务记录已停止后进程仍以异常状态退出。诊断题仅分析根因，Claude 主轨迹未修改任何代码。

## 错误堆栈

```text
=== RUN   TestGracefulShutdownLifecycle
service exited abnormally: exit status 2
panic: close of closed channel

goroutine 1 [running]:
lan-share/internal/service.(*DiscoverService).Stop()
    internal/service/discover_service.go:61
main.main()
    main.go:129
--- FAIL: TestGracefulShutdownLifecycle
FAIL
```
