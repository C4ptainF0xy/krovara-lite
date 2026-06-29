# Build the SvelteKit static site, then stage it into a Caddy image that
# both serves the files and handles TLS + reverse-proxy in prod.
#
# Note: in compose we use one shared Caddy container that mounts the
# built site as a volume. This image exists so CI can build/push a
# stand-alone "web" image if/when we move to per-service deploys.

FROM node:22-alpine AS build
WORKDIR /web
# Pin pnpm 9: corepack's default (pnpm 11) hard-fails on un-approved dependency
# build scripts (ERR_PNPM_IGNORED_BUILDS) and ignores onlyBuiltDependencies
# under --frozen-lockfile. The lockfile is format v9, which pnpm 9 reads
# natively while running build scripts without the v10+ approval gate.
RUN npm i -g pnpm@9.15.9
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
# Turnstile site key is a PUBLIC_ var read via $env/dynamic/public; with the
# static adapter (no runtime server) it's captured at build time, so it must be
# present in the environment during `pnpm build`. Passed as a build arg from
# compose (sourced from .env). Blank → captcha widget simply not rendered.
ARG PUBLIC_TURNSTILE_SITEKEY=""
ENV PUBLIC_TURNSTILE_SITEKEY=$PUBLIC_TURNSTILE_SITEKEY
RUN pnpm build

FROM caddy:2-alpine
COPY --from=build /web/build /srv
