# Stage 1: build frontend
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: build backend
FROM golang:1.25-alpine AS backend
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /server .

# Stage 3: final image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend /server ./server
COPY --from=frontend /app/frontend/dist ./frontend/dist

EXPOSE 8080

ENTRYPOINT ["./server"]
