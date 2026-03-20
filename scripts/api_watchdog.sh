#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_DIR="${ROOT_DIR}/tmp"
WATCHDOG_PID_FILE="${STATE_DIR}/api-watchdog.pid"
API_PID_FILE="${STATE_DIR}/api-server.pid"
WATCHDOG_LOG="${STATE_DIR}/api-watchdog.log"
API_LOG="${STATE_DIR}/api-server.log"
API_BIN="${STATE_DIR}/api-watchdog-bin"

API_PORT="${API_PORT:-5050}"
API_HEALTH_URL="${API_HEALTH_URL:-http://127.0.0.1:${API_PORT}/healthz}"
API_HEALTH_CHECK_INTERVAL="${API_HEALTH_CHECK_INTERVAL:-5}"
API_HEALTH_FAIL_THRESHOLD="${API_HEALTH_FAIL_THRESHOLD:-3}"
API_RESTART_BACKOFF_SEC="${API_RESTART_BACKOFF_SEC:-2}"
API_STARTUP_GRACE_SEC="${API_STARTUP_GRACE_SEC:-15}"

mkdir -p "${STATE_DIR}"
cd "${ROOT_DIR}"

is_pid_alive() {
	local pid="${1:-}"
	if [[ -z "${pid}" ]]; then
		return 1
	fi
	kill -0 "${pid}" >/dev/null 2>&1
}

read_pid_file() {
	local path="${1:-}"
	if [[ -f "${path}" ]]; then
		cat "${path}" 2>/dev/null || true
	fi
}

start_api_if_needed() {
	local api_pid
	api_pid="$(read_pid_file "${API_PID_FILE}")"
	if is_pid_alive "${api_pid}"; then
		return 0
	fi
	if ! go build -o "${API_BIN}" ./cmd/api >>"${WATCHDOG_LOG}" 2>&1; then
		echo "[watchdog] build failed; retry after ${API_RESTART_BACKOFF_SEC}s"
		sleep "${API_RESTART_BACKOFF_SEC}"
		return 0
	fi
	"${API_BIN}" >>"${API_LOG}" 2>&1 &
	api_pid=$!
	echo "${api_pid}" >"${API_PID_FILE}"
	echo "[watchdog] started api pid=${api_pid}"
	API_LAST_START_EPOCH="$(date +%s)"
}

stop_api_if_running() {
	local api_pid
	api_pid="$(read_pid_file "${API_PID_FILE}")"
	if is_pid_alive "${api_pid}"; then
		kill "${api_pid}" >/dev/null 2>&1 || true
		sleep 1
		if is_pid_alive "${api_pid}"; then
			kill -9 "${api_pid}" >/dev/null 2>&1 || true
		fi
	fi
	rm -f "${API_PID_FILE}"
}

run_watchdog() {
	echo "[watchdog] running health_url=${API_HEALTH_URL} interval=${API_HEALTH_CHECK_INTERVAL}s threshold=${API_HEALTH_FAIL_THRESHOLD} startup_grace=${API_STARTUP_GRACE_SEC}s"
	local consecutive_fail=0
	API_LAST_START_EPOCH=0
	while true; do
		start_api_if_needed
		local code
		code="$(get_health_code)"
		local now elapsed
		now="$(date +%s)"
		elapsed=$((now - API_LAST_START_EPOCH))
		if [[ "${code}" != "200" ]] && (( API_LAST_START_EPOCH > 0 )) && (( elapsed < API_STARTUP_GRACE_SEC )); then
			echo "[watchdog] startup grace active elapsed=${elapsed}s code=${code}"
			sleep "${API_HEALTH_CHECK_INTERVAL}"
			continue
		fi
		if [[ "${code}" == "200" ]]; then
			consecutive_fail=0
		else
			consecutive_fail=$((consecutive_fail + 1))
			echo "[watchdog] health check failed code=${code} consecutive_fail=${consecutive_fail}"
			if (( consecutive_fail >= API_HEALTH_FAIL_THRESHOLD )); then
				echo "[watchdog] forcing api restart (health check threshold reached)"
				stop_api_if_running
				consecutive_fail=0
				sleep "${API_RESTART_BACKOFF_SEC}"
				continue
			fi
		fi
		sleep "${API_HEALTH_CHECK_INTERVAL}"
	done
}

start_watchdog() {
	local wd_pid
	wd_pid="$(read_pid_file "${WATCHDOG_PID_FILE}")"
	if is_pid_alive "${wd_pid}"; then
		echo "watchdog already running pid=${wd_pid}"
		return 0
	fi
	nohup "${BASH_SOURCE[0]}" run >>"${WATCHDOG_LOG}" 2>&1 &
	wd_pid=$!
	echo "${wd_pid}" >"${WATCHDOG_PID_FILE}"
	echo "watchdog started pid=${wd_pid}"
}

stop_watchdog() {
	local wd_pid
	wd_pid="$(read_pid_file "${WATCHDOG_PID_FILE}")"
	if is_pid_alive "${wd_pid}"; then
		kill "${wd_pid}" >/dev/null 2>&1 || true
		sleep 1
		if is_pid_alive "${wd_pid}"; then
			kill -9 "${wd_pid}" >/dev/null 2>&1 || true
		fi
	fi
	rm -f "${WATCHDOG_PID_FILE}"
	stop_api_if_running
	echo "watchdog stopped"
}

status_watchdog() {
	local wd_pid api_pid
	wd_pid="$(read_pid_file "${WATCHDOG_PID_FILE}")"
	api_pid="$(read_pid_file "${API_PID_FILE}")"
	local wd_alive="no"
	local api_alive="no"
	if [[ -n "${wd_pid}" ]] && is_pid_alive "${wd_pid}"; then
		wd_alive="yes"
	fi
	if [[ -n "${api_pid}" ]] && is_pid_alive "${api_pid}"; then
		api_alive="yes"
	fi
	echo "watchdog_pid=${wd_pid:-none} alive=${wd_alive}"
	echo "api_pid=${api_pid:-none} alive=${api_alive}"
	local code
	code="$(get_health_code)"
	echo "health_code=${code} url=${API_HEALTH_URL}"
}

check_health() {
	local code
	code="$(get_health_code)"
	if [[ "${code}" == "200" ]]; then
		echo "OK healthz ${API_HEALTH_URL}"
		return 0
	fi
	echo "ALERT healthz failed code=${code} url=${API_HEALTH_URL}"
	return 1
}

get_health_code() {
	local code
	code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "${API_HEALTH_URL}" 2>/dev/null || true)"
	if [[ "${code}" =~ ^[0-9]{3}$ ]]; then
		echo "${code}"
		return 0
	fi
	echo "000"
}

case "${1:-}" in
start)
	start_watchdog
	;;
stop)
	stop_watchdog
	;;
run)
	run_watchdog
	;;
status)
	status_watchdog
	;;
check)
	check_health
	;;
logs)
	tail -n 100 "${WATCHDOG_LOG}" "${API_LOG}"
	;;
*)
	echo "Usage: $0 {start|stop|run|status|check|logs}"
	exit 1
	;;
esac
