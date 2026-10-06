FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
COPY frontend/package.json frontend/package.json
COPY backoffice/package.json backoffice/package.json
RUN npm ci --workspace frontend --include-workspace-root
COPY frontend/ frontend/
ARG VITE_BACKOFFICE_URL=http://localhost:5174
ENV VITE_BACKOFFICE_URL=$VITE_BACKOFFICE_URL
RUN npm run build --workspace frontend
FROM nginx:1.28-alpine
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/frontend/dist /usr/share/nginx/html
EXPOSE 80
