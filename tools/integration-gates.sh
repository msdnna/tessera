#!/usr/bin/env bash
# Полные гейты ветки слияния (integration/* или develop): lint+test backend/frontend,
# web e2e (chromium + mobile), скан backend. Логи и сводка — в $OUT (по умолчанию
# /tmp/tessera-gates). Запускать в фоне: долго (десятки минут). Android deprecated — не гоняем.
set -u
cd "$(dirname "$0")/.."
OUT=${OUT:-/tmp/tessera-gates}; mkdir -p "$OUT"; : > "$OUT/summary"
run(){ n=$1; shift; "$@" > "$OUT/$n.log" 2>&1; echo "$n=$?" >> "$OUT/summary"; }
run lint-backend make lint-backend
run test-backend make test-backend
run lint-frontend make lint-frontend
run test-frontend make test-frontend
make e2e-backend-down >/dev/null 2>&1
run e2e-up make e2e-backend-up
run e2e-chromium make test-e2e-frontend
run e2e-mobile make test-e2e-frontend-mobile
make e2e-backend-down >/dev/null 2>&1
[ "${SKIP_SCAN:-0}" = 1 ] || run scan-backend make scan-backend
echo DONE >> "$OUT/summary"
cat "$OUT/summary"
