# Teamgram Server

[中文](README-zh.md) | **English**

---

Unofficial open-source [MTProto](https://core.telegram.org/mtproto) server implementation in Go. Compatible with Telegram clients; supports self-hosted deployment.

## Features

- **MTProto 2.0**
  - **Abridged**
  - **Intermediate**
  - **Padded intermediate**
  - **Full**
- **API Layer: 223**
- **Core features**
  - **private chat**
  - **basic group**
  - **contacts**
  - **web**

## Architecture

![Architecture](docs/image/architecture-001.png)

- [Architecture Docs](https://deepwiki.com/teamgram/teamgram-server) - `DeepWiki` teamgram/teamgram-server
- [Architecture (specs)](specs/architecture.md) — service topology, data flow, and ports  
- [Service topology and configuration](docs/service-topology.md) — ports, infrastructure dependencies, call graph, and Mermaid diagram ([中文](docs/service-topology-zh.md))

## Prerequisites

| Component | Purpose |
|-----------|---------|
| [MySQL](https://www.mysql.com/) 5.7+ / 8.0 | Primary data store |
| [Redis](https://redis.io/) | Cache, session, deduplication |
| [etcd](https://etcd.io/) | Service discovery & config |
| [Kafka](https://kafka.apache.org/) | Message & event pipeline |
| [MinIO](https://minio.io/) | Object storage |
| [FFmpeg](https://ffmpeg.org/) | Media transcoding (on server) |

Detailed versions and optional monitoring stack: [Dependencies and runtime (specs)](specs/dependencies-and-runtime.md).

---

## Manual installation

For running the server from source (Go build), follow the step-by-step guides:

- **[Manual installation (Linux)](docs/install-manual-linux.md)** — CentOS, Fedora, Ubuntu/Debian
- **[Manual installation (macOS)](docs/install-manual-macos.md)** — Intel and Apple Silicon

Requires Go 1.21+. You must install and configure dependencies (MySQL, Redis, etcd, Kafka, MinIO, FFmpeg), initialize the database and MinIO, then build and run.

---

## Docker installation

For running the full stack with Docker. **No manual data initialization:** the dependency stack initializes the database (via mounted SQL) and MinIO buckets (via `minio-mc`) on first start.

### 1. Clone

```bash
git clone https://github.com/teamgram/teamgram-server.git
cd teamgram-server
```

### 2. Start dependency stack

This starts MySQL, Redis, etcd, Kafka, MinIO (and optional monitoring). The database and MinIO buckets are initialized automatically.

```bash
docker compose -f docker-compose-env.yaml up -d
```

If your environment only has Compose v1, use:

```bash
docker-compose -f docker-compose-env.yaml up -d
```

### 3. Start application

```bash
docker compose up -d
```

If your environment only has Compose v1, use:

```bash
docker-compose up -d
```

---

## Client connection checklist

By default, Teamgram client forks point to Teamgram test servers. Before login testing with your own backend, you must patch client datacenter addresses to your server public IP and port `10443`, then rebuild and reinstall the client.

- Android: `clients/teamgram-android.md`
- iOS: `clients/teamgram-ios.md`
- Desktop: `clients/teamgram-tdesktop.md`

If mobile cannot log in, check backend logs first. If gateway log `conn count` stays `0`, requests are not reaching your server (usually wrong client IP/port, stale app build, or cloud security-group rules).

#### iOS stalls on `Start Messaging` / Chinese is missing in Settings: `langpack.*` is not wired or returns empty data

Symptoms:

- On iOS (especially Simulator), tapping **Start Messaging** on the welcome screen keeps the app stuck there; switching the interface language to English lets it continue to phone login.
- In Settings -> Language, Chinese is missing, or only English is shown.

Cause:

- The iOS client fetches remote language metadata and packs through `langpack.getLanguages`, `langpack.getLanguage`, and `langpack.getLangPack`.
- If the server does not wire `langpack.*` or returns an empty language list / empty string table, non-English startup localization can block the welcome flow; the language list will also miss Chinese.
- In older defaults, `teamgramd/etc*/session.yaml` leaves `"/mtproto.RPCLangpack"` commented out, and `app/bff/bff/client/fake_rpc_result.go` returns an empty result for `langpack.getLanguages`.

Fix:

```bash
# 1) Use the latest code/image that includes the minimal langpack fallback
#    so getLanguages/getLanguage no longer return an empty result

# 2) Rebuild and recreate the app container
docker compose build teamgram
docker compose up -d --force-recreate teamgram

# 3) Check app-container logs for langpack-related RPC errors
docker logs --tail 200 hsgram_server-teamgram-1 | rg "langpack|getLanguage|getLanguages|getLangPack"
```

Notes:

- The minimal fix in this repository only makes **English** and **Simplified Chinese** visible as language entries, which is enough to address “Chinese missing in Settings” and welcome-screen stalls tied to localization selection.
- To serve a **fully translated Chinese UI**, you still need a real data source behind `langpack.getLangPack` / `langpack.getDifference`, not just a language list.

#### Login code is not delivered: missing `777000` system account

Symptoms:

- The server generates `Login code: xxxxx`, but the client never receives the verification message.

Cause:

- The community edition sends login codes as a system message from `777000` (similar to Telegram's official service account).
- If `users.id=777000` is missing, deleted, or not active, the message push can fail.

Fix:

Use a valid `777000` record and normalize its visible name to `HSgram Service` while keeping `username=teamgram` for compatibility:

```bash
docker exec mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -D teamgram -e "
INSERT INTO users (id,user_type,access_hash,secret_key_id,first_name,last_name,username,phone,country_code,verified,support,scam,fake,premium,premium_expire_date,about,state,is_bot,account_days_ttl,photo_id,restricted,restriction_reason,archive_and_mute_new_noncontact_peers,emoji_status_document_id,emoji_status_until,stories_max_id,color,color_background_emoji_id,profile_color,profile_color_background_emoji_id,birthday,personal_channel_id,authorization_ttl_days,saved_music_id,main_tab,deleted,delete_reason)
VALUES (777000,2,922337203685477000,0,'HSgram','Service','teamgram','777000','CN',1,1,0,0,0,0,'',0,0,180,0,0,'',0,0,0,0,0,0,0,0,'',0,180,0,0,0,'')
ON DUPLICATE KEY UPDATE first_name='HSgram',last_name='Service',deleted=0,support=1,state=0,verified=1;
"
```

Verify:

```bash
docker exec mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -D teamgram -e "SELECT id,first_name,last_name,username,deleted,state,support,verified FROM users WHERE id=777000;"
```

---

## Logging, monitoring & tracing

| Area | Brief | Detailed docs |
|------|--------|----------------|
| **Log** | [Filebeat](https://www.elastic.co/beats/filebeat) → Kafka (`teamgram-log`) → [go-stash](https://github.com/kevwan/go-stash) → Elasticsearch → Kibana. Config: `teamgramd/deploy/filebeat/conf/filebeat.yml`, `teamgramd/deploy/go-stash/etc/`. | [Log collection](docs/log-collection.md) ([中文](docs/log-collection-zh.md)) |
| **Monitor** | [Prometheus](https://prometheus.io/) scrapes metrics; [Grafana](https://grafana.com/) for dashboards. Enable via `Prometheus` block in `teamgramd/etc2/*.yaml`. Config: `teamgramd/deploy/prometheus/server/prometheus.yml`. | [Service monitoring](docs/service-monitoring.md) ([中文](docs/service-monitoring-zh.md)) |
| **Tracing** | go-zero [Jaeger](https://www.jaegertracing.io/) / Zipkin support. Set `Telemetry` in `teamgramd/etc2/*.yaml`. Jaeger is included in `docker-compose-env.yaml`. | [Link tracking](docs/link-tracking.md) ([中文](docs/link-tracking-zh.md)) |

---

## Compatible clients

**Default sign-in verification code:** `12345` (change for production.)

| Platform | Repository | Patch Link |
|----------|------------|------------|
| Android | [https://github.com/teamgram/teamgram-android](https://github.com/teamgram/teamgram-android) | [teamgram-android](clients/teamgram-android.md) |
| iOS | [https://github.com/teamgram/teamgram-ios](https://github.com/teamgram/teamgram-ios) | [teamgram-ios](clients/teamgram-ios.md) |
| Desktop (TDesktop) | [https://github.com/teamgram/teamgram-tdesktop](https://github.com/teamgram/teamgram-tdesktop) | [teamgram-tdesktop](clients/teamgram-tdesktop.md) |

---

## Documentation

- [Project specs](specs/README.md) — Architecture, protocol, dependencies, contributing, security, roadmap
- [Service topology and configuration](docs/service-topology.md) — Ports, infrastructure, call graph ([中文](docs/service-topology-zh.md))
- [CONTRIBUTING](CONTRIBUTING.md) · [SECURITY](SECURITY.md) · [CHANGELOG](CHANGELOG.md)


---

## Community & feedback

- **Issues:** bugs and feature requests
- **Telegram:** [Teamgram group](https://t.me/+TjD5LZJ5XLRlCYLF)

---

## Enterprise edition

The following are available in the enterprise edition (contact the [author](https://t.me/benqi)):

- sticker/theme/chat_theme/wallpaper/reactions/secretchat/2fa/sms/push(apns/web/fcm)/web/scheduled/autodelete/... 
- channels/megagroups
- audio/video/group/conferenceCall
- bots
- miniapp

See [specs/roadmap.md](specs/roadmap.md) for community vs. enterprise scope.

---

## License

[Apache License 2.0](LICENSE).

---

## Give a Star! ⭐

If this project helps you, consider giving it a star.
