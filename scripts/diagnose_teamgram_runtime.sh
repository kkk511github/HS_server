#!/usr/bin/env bash
# 在「实际跑 Docker / Teamgram」的机器上执行，把输出保存后给开发分析。
# 用法: bash scripts/diagnose_teamgram_runtime.sh | tee teamgram_runtime_diag.txt
# Docker 无权限时: sudo bash scripts/diagnose_teamgram_runtime.sh | tee teamgram_runtime_diag.txt

set -uo pipefail

echo "========== $(date -Iseconds) =========="
echo "hostname: $(hostname)"
echo "user: $(whoami)"
echo

# 选用可访问 Docker 的命令前缀（当前用户无 docker 组时需 sudo）
DOCKER=(docker)
if command -v docker >/dev/null 2>&1; then
  if ! docker info >/dev/null 2>&1; then
    if sudo -n docker info >/dev/null 2>&1; then
      DOCKER=(sudo docker)
      echo "========== docker: 使用 sudo docker（当前用户无 sock 权限）=========="
    else
      echo "========== docker: 无法连接 API（常见: permission denied on /var/run/docker.sock）=========="
      echo "解决: sudo usermod -aG docker $USER 后重新登录，或执行: sudo bash $0"
      echo
    fi
  fi
else
  echo "docker: not in PATH"
fi
echo

echo "========== docker ps =========="
if command -v docker >/dev/null 2>&1; then
  "${DOCKER[@]}" ps -a 2>&1 || true
else
  echo "(skipped)"
fi
echo

echo "========== teamgram container processes (if exists) =========="
names=""
if command -v docker >/dev/null 2>&1 && "${DOCKER[@]}" info >/dev/null 2>&1; then
  names=$("${DOCKER[@]}" ps --format '{{.Names}}' 2>/dev/null | grep -E 'teamgram|hsgram' || true)
fi
if [[ -n "$names" ]]; then
  while read -r n; do
    [[ -z "$n" ]] && continue
    echo "--- $n ---"
    "${DOCKER[@]}" exec "$n" ps aux 2>&1 | head -40 || true
  done <<< "$names"
else
  echo "no container name matching teamgram|hsgram（或 docker 不可用）"
fi
echo

echo "========== 本机进程（不依赖 docker，Teamgram 可能直接跑二进制）=========="
ps aux 2>/dev/null | grep -E '[/]teamgramd/bin/|[/]bin/msg |[/]bin/bff |[/]bin/sync |[/]bin/gnetway |[.]/(msg|bff|sync|gnetway|session) ' || true
ps aux 2>/dev/null | grep -Ei 'teamgram|hsgram|gnetway' | grep -v grep | head -30 || true
echo

echo "========== listening ports (top) =========="
if command -v ss >/dev/null 2>&1; then
  ss -lntp 2>/dev/null | head -50 || true
elif command -v netstat >/dev/null 2>&1; then
  netstat -lntp 2>/dev/null | head -50 || true
fi
echo

echo "========== recent teamgram docker logs (last 200 lines) =========="
if command -v docker >/dev/null 2>&1 && "${DOCKER[@]}" info >/dev/null 2>&1; then
  while read -r n; do
    [[ -z "$n" ]] && continue
    echo "--- docker logs $n (tail) ---"
    "${DOCKER[@]}" logs --tail 200 "$n" 2>&1 || true
    echo
  done <<< "$names"
else
  echo "(skipped: docker 不可用)"
fi

echo "========== hints =========="
echo "1) msg 进程内会启动 Inbox Kafka 消费 (app/messenger/msg/internal/server/server.go)"
echo "2) 私聊对方收不到: 查 Kafka、sync、收件人 messages 分表；日志关键字: InboxSendUserMessageToInboxV2, SyncPushUpdates, skip recipient inbox"
echo "3) 本机已监听 9092/3306/6379 等，更像中间件在本机；Teamgram 可能在其它机器/容器，请用有权限的账号再跑本脚本"
