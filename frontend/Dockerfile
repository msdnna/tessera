FROM node:22-alpine AS build

ARG HTTP_PROXY=
ARG HTTPS_PROXY=
ARG NO_PROXY=

# Build context is the REPO ROOT, not ./frontend — the help centre (#2786)
# globs its Markdown articles and screenshots out of repo-root docs/help (shared
# with the Android client). The Vite globs reach them via ../../../docs/help, so
# docs/help must sit one level above the frontend workdir at build time; a
# ./frontend context can't COPY it and the articles render "не найдена в сборке".
# See docker-compose*.yml (context: .) and .github/workflows/release-web.yml.
WORKDIR /app/frontend
RUN corepack enable

# Build metadata for the sidebar version tooltip (#2747). The context has no .git,
# so these come from docker-compose / CI; vite.config.js reads them from the env.
ARG GIT_COMMIT=
ARG BUILD_DATE=
ENV GIT_COMMIT=${GIT_COMMIT}
ENV BUILD_DATE=${BUILD_DATE}

COPY frontend/package.json frontend/yarn.lock frontend/.yarnrc.yml ./
RUN corepack yarn install --immutable

# Help-centre content, resolved by the globs at ../../../docs/help → /app/docs/help.
COPY docs/help /app/docs/help
COPY frontend/ ./
RUN corepack yarn build

FROM nginx:1.27-alpine
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/frontend/dist /usr/share/nginx/html
EXPOSE 80
