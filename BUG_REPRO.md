# 传输暂停状态复现记录

## Bug 是什么

大文件传输被用户主动暂停后，进度接口最初返回暂停，但后台任务因 context 取消结束时，记录会被改写为失败。已传输字节数仍然存在，但失败终态破坏了继续传输的条件。

## 如何触发

创建一个正在执行或已经产生部分进度的传输任务，调用暂停接口后读取进度记录。修复前，暂停状态在持久化边界被转换为失败，后台取消错误又会进入失败处理路径。

## 根因

服务层没有把用户主动暂停与真实执行错误区分开，取消传输 context 后只依据错误结果处理收尾；仓库层又把 paused 状态强制转换为 failed；模型层的暂停状态判断反向识别 failed。三层契约不一致，导致暂停无法保留为可恢复状态。Claude 修复在服务层记录主动暂停并避免覆盖暂停结果，在仓库层保留 paused 的非终态语义，在模型层正确识别 paused，同时继续让真实 I/O 错误进入 failed 并保留错误信息。

## 运行指令

```text
go test -v ./internal/service -run '^TestTransferPauseKeepsPausedState$' -count=1
```

## 验证结果

修复前目标测试稳定 5/5 失败，核心现象为暂停记录被改写为 failed。修复后目标测试稳定 5/5 通过，并验证真实本地文件错误仍然进入 failed 且错误信息不为空。

## 错误信息

修复前，暂停接口返回成功后，状态查询得到 failed；修复后，状态保持 paused，原有已传输字节数保持不变，任务仍满足恢复条件。

## 错误堆栈

```text
=== RUN   TestTransferPauseKeepsPausedState
    transfer_pause_test.go:70: paused transfer changed to failed
--- FAIL: TestTransferPauseKeepsPausedState
FAIL
```
