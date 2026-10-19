FROM golang:1.25-alpine

WORKDIR /app

COPY . .

RUN mkdir -p /app/bin && (go mod download || true) && if [ -d ./cmd/server ]; then go mod tidy && go build -o /app/bin/server ./cmd/server; elif ls ./*.go >/dev/null 2>&1; then go mod tidy && go build -o /app/bin/server .; else echo 'WARNING: no Go source files found in /app (only go.mod is present); nothing to build'; fi

EXPOSE 8080

CMD ["sh", "-c", "/app/bin/server"]
