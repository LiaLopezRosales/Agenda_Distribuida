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
    RUN apk add --no-cache ca-certificates curl sqlite
    
    WORKDIR /app
    
    # Copiar el binario compilado
    COPY --from=builder /agenda .
    
    # Copiar la base de datos inicial (opcional) y la carpeta web
    COPY agenda.db ./
    # Limpiar datos de la DB en build-time, conservando estructura/tablas.
    # Esto evita arrastrar estado previo (usuarios, raft_log, cluster_nodes, etc.) dentro de la imagen.
    RUN sqlite3 /app/agenda.db "\
      PRAGMA foreign_keys=OFF;\
      BEGIN;\
      DELETE FROM users;\
      DELETE FROM groups;\
      DELETE FROM group_members;\
      DELETE FROM appointments;\
      DELETE FROM participants;\
      DELETE FROM notifications;\
      DELETE FROM events;\
      DELETE FROM cluster_nodes;\
      DELETE FROM audit_logs;\
      DELETE FROM raft_log;\
      DELETE FROM raft_applied;\
      DELETE FROM raft_meta;\
      INSERT OR IGNORE INTO raft_meta(key, value) VALUES\
        ('currentTerm','0'),\
        ('votedFor',''),\
        ('commitIndex','0'),\
        ('lastApplied','0');\
      COMMIT;\
      PRAGMA foreign_keys=ON;\
      VACUUM;\
    "
    COPY web/ ./web/
    
    # Exponer el puerto del servidor
    EXPOSE 8080
    
    # Comando de arranque
    CMD ["./agenda"]