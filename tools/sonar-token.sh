#!/usr/bin/env bash
# Первичная настройка стенда Sonar без похода в UI (#2937):
# сменить дефолтный пароль admin и выпустить токен для анализа.
#
# Идемпотентно: если токен уже лежит в ~/.tessera/sonar-token и принимается
# стендом — скрипт ничего не делает.
#
# Пароль admin не зашит в репозиторий и не имеет «дефолта по умолчанию»:
# при первом запуске генерится случайный и кладётся рядом с токеном.
set -euo pipefail

PORT="${SONAR_PORT:-9001}"
BASE="http://127.0.0.1:${PORT}"
SECRET_DIR="${SECRET_DIR:-$HOME/.tessera}"
TOKEN_FILE="${TOKEN_FILE:-$SECRET_DIR/sonar-token}"
PASS_FILE="${PASS_FILE:-$SECRET_DIR/sonar-admin-password}"
TOKEN_NAME="${TOKEN_NAME:-tessera-analysis}"

mkdir -p "$SECRET_DIR"
chmod 700 "$SECRET_DIR"

status="$(curl -sS --max-time 5 "$BASE/api/system/status" | sed -n 's/.*"status":"\([A-Z_]*\)".*/\1/p')"
if [[ "$status" != "UP" ]]; then
	echo "Стенд не готов (status=${status:-нет ответа}). Сначала: make sonar-up" >&2
	exit 1
fi

# Уже настроено? Токен проверяем боевым запросом, а не наличием файла.
if [[ -r "$TOKEN_FILE" ]]; then
	tok="$(tr -d '[:space:]' < "$TOKEN_FILE")"
	if curl -sS --max-time 5 -u "$tok:" "$BASE/api/authentication/validate" | grep -q '"valid":true'; then
		echo "Токен уже выпущен и действителен: $TOKEN_FILE"
		exit 0
	fi
	echo "Токен из $TOKEN_FILE стендом не принят — выпускаю новый" >&2
fi

if [[ -r "$PASS_FILE" ]]; then
	ADMIN_PASS="$(tr -d '\n' < "$PASS_FILE")"
else
	# /dev/urandom, а не openssl: на этой машине лишней зависимости в скрипте
	# лучше не заводить, а base64 есть в coreutils.
	ADMIN_PASS="$(head -c 64 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | cut -c1-24)"
fi

# Сменить пароль можно только с текущего; на свежем стенде это admin/admin,
# на уже настроенном — тот, что в PASS_FILE. Пробуем оба, иначе непонятно,
# в каком из двух состояний мы находимся.
auth=""
for candidate in "$ADMIN_PASS" admin; do
	if curl -sS --max-time 5 -u "admin:$candidate" "$BASE/api/authentication/validate" | grep -q '"valid":true'; then
		auth="$candidate"
		break
	fi
done
if [[ -z "$auth" ]]; then
	echo "Пароль admin не подошёл ни из $PASS_FILE, ни дефолтный. Сбросить стенд: make sonar-down && rm -rf \${SONAR_HOME:-\$HOME/sonarqube}" >&2
	exit 1
fi

if [[ "$auth" == "admin" ]]; then
	# Пока пароль дефолтный, Sonar помечает пользователя как "must change
	# password" и отклоняет часть API — меняем в первую очередь.
	curl -sS --fail --max-time 10 -u "admin:admin" -X POST \
		--data-urlencode "login=admin" \
		--data-urlencode "previousPassword=admin" \
		--data-urlencode "password=$ADMIN_PASS" \
		"$BASE/api/users/change_password" > /dev/null
	printf '%s\n' "$ADMIN_PASS" > "$PASS_FILE"
	chmod 600 "$PASS_FILE"
	echo "Пароль admin сменён, сохранён в $PASS_FILE"
fi

# Токен с тем же именем выпустить нельзя — сносим прежний, если он был.
curl -sS --max-time 10 -u "admin:$ADMIN_PASS" -X POST \
	--data-urlencode "name=$TOKEN_NAME" \
	"$BASE/api/user_tokens/revoke" > /dev/null || true

token="$(curl -sS --fail --max-time 10 -u "admin:$ADMIN_PASS" -X POST \
	--data-urlencode "name=$TOKEN_NAME" \
	--data-urlencode "type=GLOBAL_ANALYSIS_TOKEN" \
	"$BASE/api/user_tokens/generate" \
	| sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"

if [[ -z "$token" ]]; then
	echo "Sonar не вернул токен" >&2
	exit 1
fi

printf '%s\n' "$token" > "$TOKEN_FILE"
chmod 600 "$TOKEN_FILE"
echo "Токен выпущен: $TOKEN_FILE"
