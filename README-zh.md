# Teamgram Server

**中文** | [English](README.md)

---

基于 Go 实现的开源 [MTProto](https://core.telegram.org/mtproto) 服务端，非官方实现。兼容 Telegram 客户端，支持私有化部署。

## 功能特性

- **MTProto 2.0**
  - **Abridged**
  - **Intermediate**
  - **Padded intermediate**
  - **Full**
- **API Layer: 222**
- **Core features**
  - **private chat**
  - **basic group**
  - **contacts**
  - **web**

## 架构

![架构图](docs/image/architecture-001.png)

- [Architecture Docs](https://deepwiki.com/teamgram/teamgram-server) - `DeepWiki` teamgram/teamgram-server
- [架构说明（specs）](specs/architecture.md) — 服务拓扑、数据流与端口  
- [服务拓扑与配置](docs/service-topology-zh.md) — 各服务端口、基础设施依赖、调用关系与 Mermaid 拓扑图（[English](docs/service-topology.md) 为主文档）

## 前置依赖

| 组件 | 用途 |
|------|------|
| [MySQL](https://www.mysql.com/) 5.7+ / 8.0 | 主数据存储 |
| [Redis](https://redis.io/) | 缓存、会话、去重 |
| [etcd](https://etcd.io/) | 服务发现与配置 |
| [Kafka](https://kafka.apache.org/) | 消息与事件管道 |
| [MinIO](https://minio.io/) | 对象存储 |
| [FFmpeg](https://ffmpeg.org/) | 媒体转码（需在服务端安装） |

版本建议与可选监控栈详见 [依赖与运行环境（specs）](specs/dependencies-and-runtime.md)。

**无 Docker 时的安装文档：**

- [手动安装（Linux）](docs/install-manual-linux-zh.md)
- [手动安装（macOS）](docs/install-manual-macos-zh.md)

**Docker 部署：** [install-docker.md](docs/install-docker.md)（docker-compose 完整栈）。

**一键环境（Docker）：** 使用 [docker-compose-env.yaml](docker-compose-env.yaml)，详见 [README-env-cn.md](README-env-cn.md) / [README-env-en.md](README-env-en.md)。

---

## 手动安装

从源码构建并运行服务时，请按以下文档逐步操作：

- **[手动安装（Linux）](docs/install-manual-linux-zh.md)** — CentOS、Fedora、Ubuntu/Debian
- **[手动安装（macOS）](docs/install-manual-macos-zh.md)** — Intel 与 Apple Silicon

需要 Go 1.21+。需自行安装并配置依赖（MySQL、Redis、etcd、Kafka、MinIO、FFmpeg），初始化数据库与 MinIO，再编译并运行。

---

## Docker 安装

使用 Docker 一键运行完整栈。**无需手动初始化数据**：依赖栈首次启动时会自动初始化数据库（挂载 SQL）和 MinIO 桶（通过 `minio-mc`）。

### 1. 克隆仓库

```bash
git clone https://github.com/teamgram/teamgram-server.git
cd teamgram-server
```

### 2. 启动依赖栈

将启动 MySQL、Redis、etcd、Kafka、MinIO 及可选监控组件。数据库与 MinIO 桶会自动完成初始化。

```bash
docker compose -f docker-compose-env.yaml up -d
```

如果你的环境只有 Compose v1，请使用：

```bash
docker-compose -f docker-compose-env.yaml up -d
```

### 3. 启动应用

```bash
docker compose up -d
```

如果你的环境只有 Compose v1，请使用：

```bash
docker-compose up -d
```

### 常见问题（踩坑 / Troubleshooting）

#### Kafka 启动失败：`AccessDeniedException`（挂载卷权限）

现象：`kafka` 容器启动后退出，日志类似：

- `java.nio.file.AccessDeniedException: /var/lib/kafka/data/...`

原因：`docker-compose-env.yaml` 中 Kafka 使用挂载卷 `./data/kafka/...`，但容器内运行用户（常见为 UID 1000）对宿主机目录无写权限。

修复：

```bash
sudo chown -R 1000:1000 ./data/kafka
sudo chmod -R u+rwX ./data/kafka
docker compose -f docker-compose-env.yaml restart kafka
```

#### Jaeger / go-stash 反复重启：Elasticsearch 不可用（数据目录权限导致 ES 起不来）

现象：`jaeger`、`go-stash` 持续 `Restarting`，日志提示无法连接 `http://elasticsearch:9200`。

原因：`docker-compose-env.yaml` 中 Elasticsearch 使用挂载卷 `./data/es/...`，若目录归属/权限不正确，ES 会报 “failed to obtain node locks … maybe these locations are not writable”，导致 ES 不断重启；进而 Jaeger / go-stash 健康检查失败跟着重启。

修复（推荐在首次启动前就处理好权限）：

```bash
sudo chown -R 1000:0 ./data/es
sudo chmod -R g+rwX ./data/es

# 若之前已经跑崩过，可先停掉 ES 并删除残留锁文件再启动
docker compose -f docker-compose-env.yaml stop elasticsearch
sudo rm -f ./data/es/data/node.lock
docker compose -f docker-compose-env.yaml start elasticsearch

# ES 恢复后再重启 jaeger / go-stash
docker compose -f docker-compose-env.yaml restart jaeger go-stash
```

#### 安卓能连上但登录失败：`produced zero addresses`（bff/msg/sync 未注册到 etcd）

现象：客户端能建立连接，但登录或拉配置失败；服务端日志出现：

- `rpc error: code = Unavailable desc = last resolver error: produced zero addresses`
- 常见于 `bff.bff`、`messenger.msg` 等服务发现路径

原因：应用容器首次启动时，如果 Kafka/etcd 等依赖尚未完全就绪，`msg/sync/bff` 可能在启动阶段退出；该镜像默认不会自动拉起这些子进程，导致后续服务发现为空。

修复：

```bash
# 先确认依赖都已经健康（尤其 kafka / etcd / mysql / redis）
docker compose -f docker-compose-env.yaml ps

# 再重启应用容器，让 runall-docker.sh 重新拉起所有子服务
docker compose restart teamgram

# 验证关键注册（至少应包含 bff.bff / messenger.msg / interface.session）
docker exec etcd etcdctl --endpoints=http://127.0.0.1:2379 get --prefix bff.bff
docker exec etcd etcdctl --endpoints=http://127.0.0.1:2379 get --prefix messenger.msg
docker exec etcd etcdctl --endpoints=http://127.0.0.1:2379 get --prefix interface.session
```

#### 验证码不弹/收不到：`777000` 系统账号缺失（登录码消息投递失败）

现象：登录时客户端不再弹出验证码；服务端日志可见已生成 `Login code: xxxxx`，但发送失败。

原因：社区版默认把登录验证码作为一条“系统消息”由 `777000`（类似 Telegram 的官方服务号）发送给用户；若数据库里缺少 `users.id=777000`（或该用户被标记 `deleted=1` / `state!=0`），会导致投递失败，客户端收不到验证码消息。

修复：在 MySQL `teamgram.users` 表中补齐并启用该系统账号（示例字段按默认表结构最小化填充）：

```bash
docker exec mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -D teamgram -e "
INSERT INTO users (id,user_type,access_hash,secret_key_id,first_name,last_name,username,phone,country_code,verified,support,scam,fake,premium,premium_expire_date,about,state,is_bot,account_days_ttl,photo_id,restricted,restriction_reason,archive_and_mute_new_noncontact_peers,emoji_status_document_id,emoji_status_until,stories_max_id,color,color_background_emoji_id,profile_color,profile_color_background_emoji_id,birthday,personal_channel_id,authorization_ttl_days,saved_music_id,main_tab,deleted,delete_reason)
VALUES (777000,2,922337203685477000,0,'Teamgram','Service','teamgram','777000','CN',1,1,0,0,0,0,'',0,0,180,0,0,'',0,0,0,0,0,0,0,0,'',0,180,0,0,0,'')
ON DUPLICATE KEY UPDATE deleted=0,support=1,state=0,verified=1;
"
```

验证：

```bash
docker exec mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -D teamgram -e "SELECT id,deleted,state,support,verified FROM users WHERE id=777000;"
```

#### 多账号（A/B）消息不弹窗：服务端已推送更新，弹窗取决于客户端/推送能力

说明：

- 服务端会在 `account.registerDevice` 中记录设备 token，并可能携带 `other_uids`（同设备多账号）。
- 服务端侧能产生 `notification:true` 的 updates 并推送到 session，但 **是否弹系统通知** 主要由 Android 客户端决定（通知权限/通知渠道/是否对非当前账号显示通知），以及是否具备 FCM 等推送链路（社区版对 push 能力有限）。

排查建议（高优先级到低优先级）：

- **客户端侧**：检查系统通知权限、通知渠道是否关闭、省电策略/后台限制、以及客户端自身的多账号通知策略（很多分叉只对当前账号弹窗）。
- **服务端侧**：确认 `account.registerDevice` 里 token 有上报且未报错；确认 `session.pushUpdatesData` 中 `notification:true` 正常出现。

---

## 客户端连接检查清单

Teamgram 各客户端分叉默认连接 Teamgram 测试服务器。要连接你自己的后端，必须先把客户端内置的 DC 地址改为你的公网 IP 和端口 `10443`，然后重新编译并安装客户端。

- Android: `clients/teamgram-android.md`
- iOS: `clients/teamgram-ios.md`
- Desktop: `clients/teamgram-tdesktop.md`

若手机无法登录，优先看后端日志。若网关日志里 `conn count` 持续为 `0`，说明请求没有到达服务器（常见原因是客户端 IP/端口未改对、安装了旧包、或云安全组未放行）。

---

## 日志、监控与链路追踪

| 模块 | 简述 | 详细文档 |
|------|------|----------|
| **日志** | Filebeat → Kafka（`teamgram-log`）→ go-stash → Elasticsearch → Kibana。配置：`teamgramd/deploy/filebeat/conf/filebeat.yml`、`teamgramd/deploy/go-stash/etc/`。 | [日志收集](docs/log-collection-zh.md)（[English](docs/log-collection.md)） |
| **监控** | Prometheus 拉取指标，Grafana 展示。在 `teamgramd/etc2/*.yaml` 中配置 `Prometheus`。配置：`teamgramd/deploy/prometheus/server/prometheus.yml`。 | [服务监控](docs/service-monitoring-zh.md)（[English](docs/service-monitoring.md)） |
| **链路追踪** | go-zero 支持 Jaeger / Zipkin，在 `teamgramd/etc2/*.yaml` 中配置 `Telemetry`。`docker-compose-env.yaml` 已包含 Jaeger。 | [链路追踪](docs/link-tracking-zh.md)（[English](docs/link-tracking.md)） |

---

## 兼容客户端

**默认登录验证码：** `12345`（生产环境请修改。）

| Platform | Repository | Patch Link |
|----------|------------|------------|
| Android | [https://github.com/teamgram/teamgram-android](https://github.com/teamgram/teamgram-android) | [teamgram-android](clients/teamgram-android.md) |
| iOS | [https://github.com/teamgram/teamgram-ios](https://github.com/teamgram/teamgram-ios) | [teamgram-ios](clients/teamgram-ios.md) |
| Desktop (TDesktop) | [https://github.com/teamgram/teamgram-tdesktop](https://github.com/teamgram/teamgram-tdesktop) | [teamgram-tdesktop](clients/teamgram-tdesktop.md) |

---

## 文档

- [项目规范与设计文档（specs）](specs/README.md) — 架构、协议、依赖、贡献、安全、路线图
- [服务拓扑与配置](docs/service-topology-zh.md) — 端口、基础设施、调用关系（[English](docs/service-topology.md)）
- [CONTRIBUTING](CONTRIBUTING.md) · [SECURITY](SECURITY.md) · [CHANGELOG](CHANGELOG.md)

---

## 社区与反馈

- **Issues：** 缺陷与功能建议
- **Telegram：** [Teamgram 群组](https://t.me/+TjD5LZJ5XLRlCYLF)

---

## 企业版

以下能力在企业版中提供，请联系[作者](https://t.me/benqi)：

- sticker/theme/chat_theme/wallpaper/reactions/secretchat/2fa/sms/push(apns/web/fcm)/web/scheduled/autodelete/... 
- channels/megagroups
- audio/video/group/conferenceCall
- bots
- miniapp

社区版与企业版边界见 [specs/roadmap.md](specs/roadmap.md)。

---

## 许可证

[Apache License 2.0](LICENSE)。

---

## Star ⭐

若本项目对你有帮助，欢迎 Star。
