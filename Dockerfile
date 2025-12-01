# -------- Stage 1: Build the Go binary --------
FROM golang:alpine AS builder

# Instalar herramientas necesarias para compilar dependencias nativas (como SQLite)
RUN apk add --no-cache build-base

# Crear y moverse al directorio de trabajo
WORKDIR /app

# Copiar los archivos de módulos y descargar dependencias
COPY go.mod go.sum ./
RUN go mod download

# Copiar el resto del código fuente
COPY . .

# Compilar el binario principal desde cmd/server/main.go
RUN go build -o /agenda ./cmd/server

# -------- Stage 2: Imagen final ligera --------
    FROM alpine:latest

    # Instalar certificados (para HTTPS) y curl (usado por el healthcheck)
    RUN apk add --no-cache ca-certificates curl
    
    WORKDIR /app
    
    # Copiar el binario compilado
    COPY --from=builder /agenda .
    
    # Copiar la base de datos inicial (opcional) y la carpeta web
    COPY agenda.db ./
    COPY web/ ./web/
    
    # Exponer el puerto del servidor
    EXPOSE 8080
    
    # Comando de arranque
    CMD ["./agenda"]