FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
COPY frontend/package.json frontend/package.json
COPY backoffice/package.json backoffice/package.json
RUN npm ci --workspace backoffice --include-workspace-root
COPY backoffice/ backoffice/
ARG VITE_GALLERY_URL=http://localhost:5173
ENV VITE_GALLERY_URL=$VITE_GALLERY_URL
RUN npm run build --workspace backoffice
FROM nginx:1.28-alpine
COPY backoffice/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/backoffice/dist /usr/share/nginx/html
EXPOSE 80
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s CMD wget -q -O /dev/null http://127.0.0.1/api/health || exit 1
