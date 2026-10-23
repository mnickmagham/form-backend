FROM golang:1.25-alpine

RUN apk add --no-cache git && \
    GOTOOLCHAIN=local go install github.com/cosmtrek/air@v1.52.1

WORKDIR /app

COPY go.mod go.sum ./
COPY internal internal
COPY main.go .
RUN go mod tidy

COPY . .

ENV SERVER_ADDR=:8080 \
    DB_USER=root \
    DB_PASSWORD=rootpassword \
    DB_HOST=db \
    DB_PORT=3306 \
    DB_NAME=students_db

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]
