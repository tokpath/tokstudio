# syntax=docker/dockerfile:1
FROM node:22-alpine
WORKDIR /app
COPY package.json package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY . .
RUN --mount=type=cache,target=/app/.next/cache npm run build
EXPOSE 3000
CMD ["npm", "run", "start"]
