# 局域网文件共享与设备发现（lan-share）

一个面向家庭或小团队的局域网文件共享工具，类似简易版的"局域网网盘"。同一局域网内的设备可以自动发现彼此，无需注册账号、无需云服务即可互传文件，支持文件夹共享、文件预览、断点续传与设备消息。所有数据留在本地，速度快且完全隐私。

---

## 一、功能特性

### 1. 设备发现
- 局域网内通过 UDP 广播自动发现其他在线设备
- 设备列表展示：设备名、IP、端口、操作系统、状态、在线时长
- 主动刷新按钮，立即触发一次发现广播
- 超时未心跳的设备自动标记为离线

### 2. 文件共享
- 选择本地文件夹对外共享
- 设置共享别名，便于其他设备识别
- 权限控制：只读（readonly）/ 可写（readwrite）
- 共享状态开关（启用 / 停用），停用后其他设备不可见
- 防止目录穿越攻击（`..` 段被过滤）

### 3. 文件传输
- 从其他设备下载文件到本机
- **断点续传**：基于 HTTP `Range` 头与 `X-Start-Offset` 头
- 传输进度实时显示（已传输字节 / 总字节 / 百分比）
- 传输队列管理：排队 / 运行 / 暂停 / 取消
- 传输历史记录，可一键清理已终态记录

### 4. 文件预览
- 图片预览（jpg/png/gif/webp/svg/bmp 等）
- 视频在线播放（mp4/webm/mov/mkv 等）
- 音频在线播放（mp3/wav/flac/ogg 等）
- PDF 在线预览
- 文本文件预览（go/py/js/ts/md/json/yaml 等 30+ 种扩展名），自动截断大文件

### 5. 设备消息
- 向局域网内设备发送文本消息（广播或单播）
- 消息列表展示
- 未读消息数量角标提醒，每 5 秒轮询
- 一键全部已读

### 6. 设置
- 设备名称修改
- 自动接收文件开关
- 端口配置（命令行 `-port` 覆盖）
- 系统信息只读展示

---

## 二、技术栈

| 层级 | 技术选型 | 说明 |
|------|----------|------|
| 后端框架 | Gin | HTTP 路由与中间件 |
| 文件服务 | 原生 HTTP | 通过 `c.File` / `c.FileAttachment` 提供 |
| 数据库 | SQLite | 纯 Go 驱动 `modernc.org/sqlite`，免 CGO |
| 前端 | HTML + 原生 CSS + 原生 JS | 单文件 SPA，无构建步骤 |
| 网络 | UDP 广播 + HTTP 文件传输 | 设备发现走 UDP，文件走 HTTP |
| 配置 | Viper | 支持环境变量与 `config.yaml` |

---

## 三、项目结构

```
lan-share/
├── main.go                          # 入口：依赖装配、启动 HTTP+UDP、优雅退出
├── go.mod / go.sum
├── config/
│   └── config.go                    # 配置加载（默认值 + env + yaml）
├── api/
│   └── router.go                    # 路由注册与中间件装配
├── internal/
│   ├── db/db.go                     # SQLite 连接与表迁移
│   ├── model/                       # 数据模型层（4 个文件）
│   │   ├── device.go                # 设备模型 + ID 生成
│   │   ├── share.go                 # 共享模型 + 权限校验
│   │   ├── transfer.go              # 传输模型 + 状态机判定
│   │   └── message.go               # 消息模型 + 校验
│   ├── repository/                  # 数据访问层（4 个文件，每实体一个）
│   │   ├── device_repo.go
│   │   ├── share_repo.go
│   │   ├── transfer_repo.go
│   │   └── message_repo.go
│   ├── service/                     # 业务逻辑层（4 个文件）
│   │   ├── discover_service.go      # UDP 广播/监听/过期清理
│   │   ├── share_service.go         # 共享 CRUD + 目录浏览
│   │   ├── transfer_service.go      # 传输调度 + 断点续传
│   │   └── preview_service.go       # 文件类型识别 + 文本预览
│   ├── handler/                     # HTTP 接口层（6 个文件）
│   │   ├── response.go              # 通用响应/错误工具
│   │   ├── device_handler.go
│   │   ├── share_handler.go
│   │   ├── transfer_handler.go
│   │   ├── file_handler.go          # 浏览/下载/预览/上传
│   │   ├── message_handler.go
│   │   └── settings_handler.go
│   ├── middleware/
│   │   └── cors.go                  # CORS + Recovery + Logger
│   └── network/                     # 底层通信层
│       ├── udp.go                   # UDP 发现与消息广播/接收
│       └── http.go                  # HTTP 文件客户端（含 Range 续传）
└── web/
    └── index.html                   # 单文件前端（暗色主题 SPA）
```

**代码规模**：26 个 Go 文件，有效代码约 3350 行（去空行注释），总行数 4167 行。

---

## 四、分层架构与单一职责

| 层 | 职责 | 不允许做的事 |
|----|------|--------------|
| `model` | 定义结构、枚举、校验 | 不感知数据库与 HTTP |
| `repository` | SQL 持久化（CRUD） | 不包含业务规则 |
| `service` | 业务规则编排 | 不感知 HTTP 编解码 |
| `handler` | HTTP 参数绑定、响应序列化 | 不直接操作数据库 |
| `network` | UDP / HTTP 底层通信 | 不感知业务模型 |
| `middleware` | 横切关注点（CORS/日志/恢复） | 不包含业务逻辑 |

---

## 五、核心 API

### 设备
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/devices` | 获取在线设备列表 |
| POST | `/api/v1/devices/discover` | 主动触发一次发现广播 |
| GET | `/api/v1/devices/:id` | 查询单个设备 |

### 共享
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/shares` | 获取共享列表（支持 `?only_enabled=true`） |
| POST | `/api/v1/shares` | 创建共享 |
| GET | `/api/v1/shares/:id` | 查询共享 |
| PUT | `/api/v1/shares/:id` | 更新共享 |
| PATCH | `/api/v1/shares/:id/toggle` | 切换启用状态 |
| DELETE | `/api/v1/shares/:id` | 删除共享 |
| GET | `/api/v1/shares/:id/items?path=xxx` | 浏览共享目录内容 |

### 传输
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/transfers?limit=100&status=running` | 传输记录列表 |
| POST | `/api/v1/transfers` | 创建并立即启动传输 |
| GET | `/api/v1/transfers/:id` | 查询传输详情 |
| GET | `/api/v1/transfers/:id/progress` | 传输进度 |
| POST | `/api/v1/transfers/:id/start` | 启动/断点续传 |
| POST | `/api/v1/transfers/:id/pause` | 暂停 |
| DELETE | `/api/v1/transfers/:id` | 取消 |
| DELETE | `/api/v1/transfers/history` | 清理已终态记录 |

### 消息
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/messages?limit=100` | 消息列表 |
| POST | `/api/v1/messages` | 发送消息（广播或单播） |
| POST | `/api/v1/messages/:id/read` | 标记已读 |
| POST | `/api/v1/messages/read-all` | 全部已读 |
| GET | `/api/v1/messages/unread` | 未读数量 |
| DELETE | `/api/v1/messages/:id` | 删除消息 |

### 文件
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/files/:alias/*path` | 浏览/下载/预览文件（`?download=true` 强制下载） |
| POST | `/api/v1/uploads/:alias` | 上传文件到对端共享（支持 `X-Start-Offset` 续传） |
| GET | `/api/v1/remote/shares` | 对外暴露本机共享列表 |
| GET | `/api/v1/remote/shares/:alias/items` | 对外暴露共享内目录浏览 |
| GET | `/api/v1/ping` | 心跳探测 |
| GET | `/api/v1/health` | 健康检查 |

### 设置
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/settings` | 获取全部配置 |
| PATCH | `/api/v1/settings` | 修改设备名/自动接收 |
| GET | `/api/v1/me` | 本机设备简表 |

---

## 六、数据表

| 表名 | 说明 | 主要字段 |
|------|------|----------|
| `devices` | 设备记录 | id, name, ip, port, status, first_seen, last_seen, os |
| `shares` | 共享文件夹 | id, alias, path, permission, enabled, created_at, updated_at |
| `transfers` | 传输记录 | id, direction, peer_ip, peer_port, file_name, file_size, bytes_transferred, status, err_msg |
| `messages` | 设备消息 | id, from_ip, from_name, to_ip, content, read, created_at |

数据库文件默认位于 `~/.lan-share/lanshare.db`，启用 WAL 模式。

---

## 七、配置说明

配置优先级：**命令行参数 > 环境变量 > config.yaml > 默认值**。

### 默认值

| 配置项 | 默认值 | 环境变量 |
|--------|--------|----------|
| 设备名 | 主机名 | `LANSHARE_DEVICE_NAME` |
| 自动接收 | true | `LANSHARE_DEVICE_AUTO_RECEIVE` |
| HTTP 监听 | `0.0.0.0:8765` | `LANSHARE_SERVER_HOST` / `LANSHARE_SERVER_PORT` |
| 单文件上限 | 4096 MB | `LANSHARE_SERVER_MAX_UPLOAD_MB` |
| UDP 发现端口 | 18765 | `LANSHARE_NETWORK_UDP_DISCOVER_PORT` |
| 发现间隔 | 5 秒 | `LANSHARE_NETWORK_DISCOVER_INTERVAL` |
| 设备离线判定 | 15 秒 | `LANSHARE_NETWORK_DEVICE_TTL` |
| 数据目录 | `~/.lan-share` | `LANSHARE_STORAGE_DATA_DIR` |
| 共享目录 | `~/.lan-share/shares` | `LANSHARE_STORAGE_SHARE_DIR` |
| 下载目录 | `~/.lan-share/downloads` | `LANSHARE_STORAGE_DOWNLOAD_DIR` |

### config.yaml 示例

放在程序同目录或数据目录下：

```yaml
device:
  name: my-laptop
  auto_receive: true
server:
  host: 0.0.0.0
  port: 8765
  max_upload_mb: 4096
network:
  udp_discover_port: 18765
  discover_interval: 5
  device_ttl: 15
storage:
  data_dir: ~/.lan-share
  share_dir: ~/.lan-share/shares
  download_dir: ~/.lan-share/downloads
```

---

## 八、启动命令

### 编译

```bash
cd lan-share
go build -o lan-share ./main.go
```

### 运行

```bash
# 默认配置
./lan-share

# 指定端口
./lan-share -port 9000

# 指定配置文件路径（可选）
./lan-share -config /path/to/config.yaml

# 查看版本
./lan-share -v
```

启动后访问浏览器：`http://localhost:8765/`（或你指定的端口）。

### 多设备测试

在同一局域网内多台机器分别启动即可自动发现对方。也可在本机用不同端口模拟：

```bash
./lan-share -port 8765   # 实例 A
./lan-share -port 8766   # 实例 B（UDP 端口相同故可互相发现）
```

---

## 九、核心实现说明

### 断点续传
- 下载：客户端发送 `Range: bytes=N-`，服务端响应 `206 Partial Content`；本地以 `O_APPEND` 模式追加写入
- 上传：客户端发送 `X-Start-Offset: N` 头，服务端以 `O_APPEND` 模式接收
- 不支持续传的对端会返回 `200 OK`，客户端检测后重置本地文件从头写入

### 设备发现
- UDP 端口默认 18765，广播包为 JSON：`{"t":"discover","name":"...","ip":"...","port":8765,"os":"..."}`
- 每个发现周期（默认 5 秒）广播一次自身存在
- 超过 TTL（默认 15 秒）未收到心跳的设备标记为 `offline`

### 安全
- 共享路径防穿越：过滤 `..` 段，且最终路径必须以共享根目录为前缀
- 上传需共享权限为 `readwrite`，否则返回 403
- 共享列表对外不暴露真实文件系统路径

---

## 十、验证记录

- `go build ./...` 与 `go vet ./...` 均无错误
- 端到端测试：创建共享 → 触发下载 → 文件落盘，19/19 字节完整传输
- 全部 API 接口返回正常响应格式

---

## 十一、许可

仅供学习与内部使用。
