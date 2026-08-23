# 共享目录符号链接越界复现记录

## Bug 是什么

共享根目录内的符号链接可以把远程读取和上传请求带到共享根目录之外，导致未授权文件被读取或写入。

## 如何触发

在共享目录下创建一个指向外部目录的符号链接，再通过远程文件浏览接口读取链接下的文件，或通过上传接口把目标路径设置为该链接下的新文件。修复前，读取请求会返回外部文件内容，上传请求会在外部目录创建文件。

## 根因

路径解析只做了词法上的相对路径检查，没有解析磁盘上的符号链接目标。文件浏览、目录列举和上传分别使用不同的路径入口，均可能在符号链接解析后越出共享根目录。Claude 修复在模型层增加真实路径边界校验，在服务层统一校验目录、文件和上传父目录，并让文件处理器使用安全的上传路径解析结果。

## 运行指令

```text
go test -v ./internal/handler -run '^TestShareSymlinkCannotEscapeRoot$' -count=1
```

## 验证结果

修复前目标测试连续 5 轮失败，表现为外部文件被读取。修复后目标测试连续 5 轮通过，读取和上传都被拒绝，外部目录不会产生上传文件。

## 错误信息

修复前读取入口返回成功响应并暴露外部文件内容，上传入口也接受了越界目标。修复后越界路径返回无效路径错误。

## 错误堆栈

```text
=== RUN   TestShareSymlinkCannotEscapeRoot
    share_symlink_boundary_test.go:60: symlink outside shared root was served: {"code":0,"data":{"content":"secret","size":6,"truncated":false,"type":"text"},"message":"ok"}
--- FAIL: TestShareSymlinkCannotEscapeRoot
FAIL
```
