.PHONY: all web build test check dev-agent dev-web clean

all: build

web/node_modules: web/package.json
	cd web && npm install
	touch web/node_modules

web: web/node_modules
	cd web && npm run build

# Linux binary with the frontend embedded, ready to copy to the server.
build: web
	cd agent && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ../bin/dashd ./cmd/dashd

test:
	cd agent && go vet ./... && go test ./...

check: web/node_modules
	cd web && npm run check

# Local development: run these in two terminals, then open http://localhost:5173
# First time: cd dev && go run ../agent/cmd/dashd init -mock -o config.yaml
dev-agent:
	cd dev && go run ../agent/cmd/dashd serve -config config.yaml

dev-web: web/node_modules
	cd web && npm run dev

clean:
	rm -rf bin agent/internal/webui/dist/assets agent/internal/webui/dist/index.html
