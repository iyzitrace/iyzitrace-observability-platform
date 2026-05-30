# Build Stage
FROM node:20-alpine AS builder

WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY tsconfig.json ./
COPY src ./src
RUN npm run build

# Production Stage
FROM node:20-alpine

WORKDIR /app

# Install production deps only
COPY package*.json ./
RUN npm ci --omit=dev \
    && npm cache clean --force \
    && rm -rf /usr/local/lib/node_modules/npm /usr/local/bin/npm /usr/local/bin/npx \
    && rm -f package.json package-lock.json

# Install openssl and zip for certificate generation and bundling
RUN apk add --no-cache openssl zip

# Copy built artifacts
COPY --from=builder /app/dist ./dist

# Copy bundled OpenAPI spec used by the Swagger route
COPY platform-api.openapi.yaml ./platform-api.openapi.yaml

# Copy UI assets
COPY ui ./ui

# Create data directory for SQLite
RUN mkdir -p data && chown -R node:node data

EXPOSE 8080

CMD ["node", "dist/index.js"]
