#!/usr/bin/env bash
# 在 VM-4-8-ubuntu（或任意跑 teamgramd 二进制的主机）上执行，需能查看 root 进程。
# 用法: cd ~/HSgram_server && sudo bash scripts/vm_inspect_teamgram_procs.sh | tee vm_teamgram_proc.txt

set +e

echo "========== $(date -Iseconds) =========="
echo "hostname: $(hostname) user: $(whoami)"
echo

for name in msg sync bff session gnetway authsession; do
  # 用命令行匹配，避免误匹配其它进程里的 "msg" 子串
  pid=$(pgrep -nf "[.]/${name} -f=" 2>/dev/null | head -1 || true)
  if [[ -z "$pid" ]]; then
    echo "--- $name: no pid (pgrep -f ./${name}) ---"
    continue
  fi
  echo "--- $name pid=$pid ---"
  tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null; echo
  # cwd 是 magic symlink，readlink -f 在部分环境会得到空串，用 readlink/ls 更稳
  if [[ -L "/proc/$pid/cwd" ]] || [[ -e "/proc/$pid/cwd" ]]; then
    echo -n "cwd (readlink): "
    readlink "/proc/$pid/cwd" 2>/dev/null || echo "(failed)"
    echo "cwd (ls -l):   $(ls -l "/proc/$pid/cwd" 2>/dev/null | sed 's/.* -> //')"
  else
    echo "cwd: (no /proc/$pid/cwd — 进程可能已退出或无权查看)"
  fi
  ppid=$(awk '/^PPid:/ {print $2}' "/proc/$pid/status" 2>/dev/null)
  if [[ -n "$ppid" ]] && [[ -r "/proc/$ppid/cmdline" ]]; then
    echo -n "ppid=$ppid cmdline: "
    tr '\0' ' ' < "/proc/$ppid/cmdline" 2>/dev/null; echo
  fi
  echo "fd 0/1/2 ->"
  for fd in 0 1 2; do
    [[ -e "/proc/$pid/fd/$fd" ]] && ls -l "/proc/$pid/fd/$fd" 2>/dev/null
  done
  echo "fd 名含 log/out/err 的项:"
  ls -la "/proc/$pid/fd" 2>/dev/null | grep -E 'log|txt|out|err' || echo "(无，多为终端/管道/journal)"
  echo
done

echo "========== teamgramd 常见日志目录（若存在）=========="
for d in /root/teamgramd/logs /home/ubuntu/HSgram_server/teamgramd/logs /opt/teamgramd/logs; do
  [[ -d "$d" ]] && echo "$d:" && ls -la "$d" 2>/dev/null | tail -20
done

echo
echo "========== systemd（若有 unit）=========="
systemctl list-units --type=service --all 2>/dev/null | grep -Ei 'teamgram|msg|bff|sync' || echo "(none or no systemctl)"

echo
echo "========== 运行环境推断 / 如何看日志 =========="
msg_pid=$(pgrep -nf "[.]/msg -f=" 2>/dev/null | head -1 || true)
if [[ -n "$msg_pid" ]]; then
  cwd=$(readlink "/proc/$msg_pid/cwd" 2>/dev/null)
  ppid=$(awk '/^PPid:/ {print $2}' "/proc/$msg_pid/status" 2>/dev/null)
  pcmd=$(tr '\0' ' ' < "/proc/$ppid/cmdline" 2>/dev/null)
  if [[ "$cwd" == /app/bin ]] || echo "$pcmd" | grep -q 'entrypoint'; then
    echo "判断: 进程在 Docker 容器内（cwd=$cwd）。stdout/stderr -> pipe，由 Docker 日志驱动收集；宿主 ~/HSgram_server/teamgramd/logs 为空属正常。"
    echo "ppid 命令行: $pcmd"
    echo "msg 进程 cgroup（节选）:"
    head -8 "/proc/$msg_pid/cgroup" 2>/dev/null || true
    cid=""
    [[ -r "/proc/$msg_pid/cgroup" ]] && cid=$(grep -oE '/docker/[0-9a-f]{12,64}' "/proc/$msg_pid/cgroup" 2>/dev/null | head -1 | sed 's#.*/##')
    [[ -z "$cid" ]] && [[ -r "/proc/$msg_pid/cgroup" ]] && cid=$(grep -oE 'docker-[0-9a-f]{12,64}' "/proc/$msg_pid/cgroup" 2>/dev/null | head -1 | sed 's/docker-//')
    DOCKER=(docker)
    docker info >/dev/null 2>&1 || { sudo -n docker info >/dev/null 2>&1 && DOCKER=(sudo docker); }
    if command -v docker >/dev/null 2>&1 && "${DOCKER[@]}" info >/dev/null 2>&1; then
      if [[ -n "$cid" ]]; then
        echo "从 cgroup 解析的容器 ID 前缀: ${cid:0:12}…"
        "${DOCKER[@]}" inspect -f '{{.Name}} image={{.Config.Image}}' "$cid" 2>/dev/null \
          || "${DOCKER[@]}" ps -a --no-trunc | grep "${cid:0:12}" || true
      else
        echo "未能从 cgroup 解析容器 ID，请: ${DOCKER[*]} ps -a 自行对照"
      fi
      echo "看 Teamgram 日志: ${DOCKER[*]} logs --tail 500 -f <上面对应的容器名或 ID>"
    else
      echo "docker 当前不可用或无权限，请用: sudo docker ps -a && sudo docker logs --tail 500 -f <容器>"
    fi
    echo "若 compose 里接了 filebeat→ES，也可在 Kibana 按服务名搜。"
  fi
fi
