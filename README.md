# Health — multi-user calorie tracker

Vue 3 + shadcn-vue frontend, Go (`net/http`) + SQLite + sqlc backend.

## Run

Backend (`:8080`):

```bash
cd backend
go run .
```

Frontend dev server (`:5173`, proxies `/api` → backend):

```bash
cd frontend
npm install
npm run dev
```

Open <http://localhost:5173>.

## Seed sample data

```bash
cd backend
go run ./cmd/seed
```

## Build for production

```bash
cd frontend && npm run build       # outputs to dist/
cd ../backend && go build -o health
./health                           # serves API + frontend at :8080
```
