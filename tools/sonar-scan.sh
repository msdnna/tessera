#!/usr/bin/env bash
# Анализ рабочего дерева стендом SonarQube (#2937).
#
# Сканер запускается в контейнере, а не бинарём с хоста: sonar-scanner тянет
# JRE и плагины, а на этой машине загрузки с сети нестабильны — образ
# кэшируется один раз.
#
# Переменные:
#   SONAR_TOKEN   — токен пользователя Sonar (обязателен). Можно положить в
#                   ~/.tessera/sonar-token, тогда задавать не нужно.
#   PROJECT_KEY   — ключ проекта. По умолчанию `tessera` на develop и
#                   `tessera-scratch` на любой другой ветке.
#   SONAR_NETWORK — docker-сеть стенда (по умолчанию tessera-sonar-network).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

SONAR_NETWORK="${SONAR_NETWORK:-tessera-sonar-network}"
SCANNER_IMAGE="${SCANNER_IMAGE:-sonarsource/sonar-scanner-cli:11}"
TOKEN_FILE="${TOKEN_FILE:-$HOME/.tessera/sonar-token}"

SONAR_TOKEN="${SONAR_TOKEN:-}"
if [[ -z "$SONAR_TOKEN" && -r "$TOKEN_FILE" ]]; then
	SONAR_TOKEN="$(tr -d '[:space:]' < "$TOKEN_FILE")"
fi
if [[ -z "$SONAR_TOKEN" ]]; then
	cat >&2 <<-MSG
	Нужен токен Sonar:
	  make sonar-up && make sonar-token
	Токен ляжет в $TOKEN_FILE. Вручную — UI на
	http://127.0.0.1:${SONAR_PORT:-9001}, My Account -> Security -> Generate Token
	(тип: Global Analysis Token), затем export SONAR_TOKEN=...
	MSG
	exit 1
fi

BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"
if [[ -z "${PROJECT_KEY:-}" ]]; then
	# Community Build анализирует ровно одну ветку на проект: история develop
	# не должна затираться прогоном по feature-ветке. Поэтому всё, что не
	# develop, льётся в перезаписываемую песочницу отдельным ключом.
	if [[ "$BRANCH" == "develop" ]]; then
		PROJECT_KEY="tessera"
	else
		PROJECT_KEY="tessera-scratch"
	fi
fi

ARGS=(
	"-Dsonar.projectKey=$PROJECT_KEY"
	"-Dsonar.projectVersion=$BRANCH"
	"-Dsonar.scm.revision=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
)
if [[ "$PROJECT_KEY" != "tessera" ]]; then
	ARGS+=("-Dsonar.projectName=Tessera (scratch: $BRANCH)")
fi

# Отчёт о покрытии, которого нет, Sonar считает ошибкой конфигурации и сыплет
# предупреждениями. Передаём только существующие — пустая строка снимает
# значение из sonar-project.properties.
coverage_or_empty() {
	[[ -r "$1" ]] && printf '%s' "$1"
}
ARGS+=(
	"-Dsonar.go.coverage.reportPaths=$(coverage_or_empty backend/coverage.out)"
	"-Dsonar.javascript.lcov.reportPaths=$(coverage_or_empty frontend/coverage/lcov.info)"
	"-Dsonar.coverage.jacoco.xmlReportPaths=$(coverage_or_empty android/app/build/reports/jacoco/jacocoTestReport/jacocoTestReport.xml)"
)

echo "Проект: $PROJECT_KEY (ветка $BRANCH)"

# Рабочий каталог сканера — в ./reports/sonar, а не в корне репозитория:
# .scannerwork иначе замусоривает git status на каждом прогоне.
mkdir -p reports/sonar

exec docker run --rm \
	--network "$SONAR_NETWORK" \
	-e SONAR_HOST_URL="http://sonarqube:9000" \
	-e SONAR_TOKEN="$SONAR_TOKEN" \
	-v "$REPO_ROOT:/usr/src" \
	-v tessera-sonar-scanner-cache:/opt/sonar-scanner/.sonar/cache \
	"$SCANNER_IMAGE" \
	"${ARGS[@]}" \
	-Dsonar.working.directory=/usr/src/reports/sonar
