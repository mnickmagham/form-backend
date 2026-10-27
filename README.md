# form-backend

Student management API built in Go. Register, get, update, delete, and search students backed by MySQL, with structured logging, per-IP rate limiting, panic recovery, and Swagger docs.

## Configure with `.env`

Copy `server/.env.example` to `server/.env` and adjust the values for your environment:

```bash
cd server
cp .env.example .env
# edit .env with your own credentials / ports
```

The Go server reads `server/.env` directly. For Docker Compose there's a separate root `.env.example` (copy to `.env` at the repo root) that overrides compose-only values like `WEB_PORT`, `DB_PASSWORD`, `DB_NAME`, and `DOMAIN`. `server/.env` is git-ignored — keep your local values out of version control.

## Run with Docker Compose

From the repo root (root `docker-compose.yml` is the single compose file):

```bash
docker compose up -d --build
```

This starts MySQL (`db`), the Go API (`api`), and the nginx frontend (`web`). The database seeds from `./students_db.sql` on first start.

- UI + API proxy: http://localhost:8080  (override with `WEB_PORT`)
- Swagger UI: http://localhost:8080/swagger/
- Stop: `docker compose down` (add `-v` to delete the database volume)

## Run locally (2 commands)

You need MySQL running on localhost:3306, then the Go server reads from `server/.env`.

1. Start the database (MySQL), then load the seed:

```bash
# 1. Start MySQL (example via docker, or use your local MySQL)
docker run --name form-mysql -e MYSQL_ROOT_PASSWORD=rootpassword -e MYSQL_DATABASE=students_db -p 3306:3306 -d mysql:8
# wait for it to be ready, then:
docker exec -i form-mysql mysql -uroot -prootpassword students_db < ../students_db.sql
```

2. Run the Go server using the `.env` file:

```bash
cd server
export $(grep -v '^#' .env | xargs) && go run main.go
```

- API Base URL: http://localhost:8080
- Swagger UI: http://localhost:8080/swagger/

## Showcase — what we did

A small CRUD API for students with these endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/students` | Register a new student |
| `GET` | `/students/{id}` | Get a student by ID |
| `PUT` | `/students/{id}` | Update a student |
| `DELETE` | `/students/{id}` | Delete a student |
| `GET` | `/students` | Search/filter + paginate students (global `q` or field filters) |

Plus Swagger docs at `/swagger/` and a static test UI at `/`.

What's included:

- **CRUD** — register, get, update, delete students with validation and conflict handling
- **Search** — global search across fields plus per-field filters, pagination, and sorting
- **Database** — MySQL schema with 1,000 seeded students (`students_db.sql`)
- **Middleware** — structured JSON logger with correlation IDs, per-IP rate limiting (429 + headers), panic recovery returning 500 JSON
- **Config** — everything via `.env` (server addr/port, DB creds, log level, rate limit)
- **Tests** — unit tests for store, handler, logger, rate limiter, and recovery
- **Docker Compose** — one-command DB + app with healthchecks and seeded data
