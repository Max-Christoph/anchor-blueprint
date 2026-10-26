# syntax=docker/dockerfile:1

# --- Build-Stage -----------------------------------------------------------
FROM node:24-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci

COPY . .

# Vite inlined Env-Werte zur Build-Zeit.
ARG VITE_API_BASE_URL=""
ARG VITE_DEMO_USER_EMAIL="demo.user@example.com"
ARG VITE_BEARER_TOKEN=""
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL} \
    VITE_DEMO_USER_EMAIL=${VITE_DEMO_USER_EMAIL} \
    VITE_BEARER_TOKEN=${VITE_BEARER_TOKEN}

RUN npm run build

# --- Runtime-Stage ---------------------------------------------------------
FROM nginx:alpine AS runtime

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
