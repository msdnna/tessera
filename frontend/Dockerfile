FROM node:22-alpine AS build

ARG HTTP_PROXY=
ARG HTTPS_PROXY=
ARG NO_PROXY=

WORKDIR /app
RUN corepack enable

# Build metadata for the sidebar version tooltip (#2747). The context is
# ./frontend (no .git), so these come from docker-compose; vite.config.js reads
# them from the environment.
ARG GIT_COMMIT=
ARG BUILD_DATE=
ENV GIT_COMMIT=${GIT_COMMIT}
ENV BUILD_DATE=${BUILD_DATE}

COPY package.json yarn.lock .yarnrc.yml ./
RUN corepack yarn install --immutable

COPY . .
RUN corepack yarn build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
