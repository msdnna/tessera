#!/usr/bin/env bash
# Ждёт, пока стенд SonarQube не отрапортует status=UP.
#
# Отдельный скрипт, а не строчка в Makefile, по одной причине: контейнер
# поднимается за секунды, а веб-приложение стартует минуты (миграция БД +
# Elasticsearch). Без ожидания `make sonar-up` возвращает ноль на ещё мёртвом
# стенде, и следующий шаг падает непонятным connection refused.
set -euo pipefail

PORT="${1:-9000}"
TIMEOUT="${SONAR_WAIT_TIMEOUT:-300}"
URL="http://127.0.0.1:${PORT}/api/system/status"

printf 'Ждём SonarQube на %s (до %ss)' "$URL" "$TIMEOUT"
deadline=$(( SECONDS + TIMEOUT ))
status=""
while (( SECONDS < deadline )); do
	# --fail не ставим: пока Sonar стартует, он отдаёт 503 с телом,
	# в котором лежит осмысленный статус (DB_MIGRATION_NEEDED и т.п.).
	body="$(curl -sS --max-time 5 "$URL" 2>/dev/null || true)"
	status="$(printf '%s' "$body" | sed -n 's/.*"status":"\([A-Z_]*\)".*/\1/p')"
	case "$status" in
		UP)
			printf '\nSonarQube готов: %s\n' "$URL"
			exit 0
			;;
		DB_MIGRATION_NEEDED|DB_MIGRATION_RUNNING)
			# Штатно при первом запуске и после бампа образа.
			;;
	esac
	printf '.'
	sleep 5
done

printf '\nSonarQube не поднялся за %ss (последний статус: %s)\n' "$TIMEOUT" "${status:-нет ответа}" >&2
echo "Логи: make sonar-logs" >&2
exit 1
