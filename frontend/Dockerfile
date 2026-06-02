FROM node:22-alpine AS build

ARG HTTP_PROXY=
ARG HTTPS_PROXY=
ARG NO_PROXY=

WORKDIR /app
RUN corepack enable

COPY package.json yarn.lock .yarnrc.yml ./
RUN corepack yarn install --immutable

COPY . .
RUN corepack yarn build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
